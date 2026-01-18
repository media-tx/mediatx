// Copyright (c) 2026 MediaTX
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package handlers

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/fluxorio/fluxor/pkg/core"
	"github.com/fluxorio/fluxor/pkg/tcp"
	"github.com/fluxorio/fluxor/apps/rtmp-server/domain"
)

// RTMPConnectionHandler handles RTMP TCP connections
// This is part of Protocol Layer (Level 1)
// Uses: ConnectionState.ProcessChunk → AMFParser → CommandHandler (simplified)
type RTMPConnectionHandler struct {
	connectionManager domain.ConnectionManager
	streamManager     domain.StreamManager
	eventBus          core.EventBus

	// Parsers and handlers (ChunkParser and MessageParser removed - using ProcessChunk instead)
	amfParser      *AMFParser
	commandHandler *RTMPCommandHandler
	
	// Track viewer writers for cleanup
	viewerWriters map[string]*ViewerMediaWriter
	writersMu     sync.Mutex
}

// NewRTMPConnectionHandler creates a new RTMP connection handler
func NewRTMPConnectionHandler(
	connectionManager domain.ConnectionManager,
	streamManager domain.StreamManager,
	eventBus core.EventBus,
) *RTMPConnectionHandler {
	// Initialize parsers (using ProcessChunk instead of ChunkParser + MessageParser)
	amfParser := NewAMFParser()
	commandHandler := NewRTMPCommandHandler(connectionManager, streamManager, eventBus, amfParser)

	return &RTMPConnectionHandler{
		connectionManager: connectionManager,
		streamManager:     streamManager,
		eventBus:          eventBus,
		amfParser:         amfParser,
		commandHandler:    commandHandler,
		viewerWriters:     make(map[string]*ViewerMediaWriter),
	}
}

// HandleConnection handles a new RTMP TCP connection
// Refactored to match cleaner structure: handshake → state → parse chunks → reconstruct messages → handle commands → write responses
func (h *RTMPConnectionHandler) HandleConnection(ctx *tcp.ConnContext) error {
	connID := ctx.RemoteAddr.String()
	log.Printf("[RTMP TCP] New connection from %s", connID)

	// Create connection using Domain Layer
	rtmpConn, err := h.connectionManager.CreateConnection(connID, connID)
	if err != nil {
		log.Printf("[RTMP TCP] Failed to create connection for %s: %v", connID, err)
		return err
	}

	// Handshake C0/C1/C2
	if err := h.doRTMPHandshake(ctx.Conn, rtmpConn); err != nil {
		log.Printf("[RTMP TCP] Handshake failed for %s: %v", connID, err)
		ctx.Conn.Close()
		h.connectionManager.RemoveConnection(connID)
		return err
	}

	rtmpConn.SetState(domain.ConnectionStateConnected)
	log.Printf("[RTMP TCP] Handshake completed for %s", connID)

	// Enable TCP_NODELAY for lower latency (disable Nagle's algorithm)
	// Also set smaller TCP write buffer for reduced buffering
	if tcpConn, ok := ctx.Conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
		tcpConn.SetWriteBuffer(16 * 1024) // 16KB write buffer for low latency
		log.Printf("[RTMP TCP] TCP_NODELAY enabled for %s (low latency mode)", connID)
	}

	// Publish connection created event
	if h.eventBus != nil {
		_ = h.eventBus.Publish("rtmp.connection.created", map[string]interface{}{
			"connection_id": connID,
			"remote_addr":   connID,
		})
	}

	// Create connection state (window ack size, chunk size, etc.)
	state := NewConnectionState()

	// Create buffered reader and writer
	// Use smaller write buffer for lower latency (512 bytes instead of default 4096)
	reader := bufio.NewReader(ctx.Conn)
	writer := bufio.NewWriterSize(ctx.Conn, 512) // Low latency buffer

	// Chunk writer for writing response chunks
	chunkWriter := NewChunkWriter(writer, state.GetChunkSize())

	// Note: Don't send initialization messages here
	// According to nginx-rtmp-module, initialization messages should be sent
	// AFTER receiving connect command but BEFORE sending connect response

	// Handle RTMP messages using ProcessChunk (simplified approach)
	for {
		// Set read deadline - longer timeout for media streaming (5 minutes)
		// This allows client to send media data without timeout, but still detects dead connections
		ctx.Conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

		// Process chunk directly - reads chunk, reconstructs message, and calls callback when complete
		log.Printf("[RTMP TCP] Calling ProcessChunk for %s", connID)
		err := state.ProcessChunk(reader, func(message *Message) error {
			log.Printf("[RTMP TCP] ProcessChunk callback called for message type=0x%02x from %s", message.MessageType, connID)
			// Update bytes received (for acknowledgment)
			state.UpdateBytesReceived(message.MessageLength)

			// Update last seen
			rtmpConn.UpdateLastSeen()

		// Log received message (debug)
		log.Printf("[RTMP TCP] Received message: type=0x%02x (%s), len=%d, csid=%d, msid=%d", 
			message.MessageType, getMessageTypeName(message.MessageType), 
			message.MessageLength, message.ChunkStreamID, message.MessageStreamID)

		// Enhanced debug for AMF commands (similar to user's snippet)
		if message.MessageType == MessageTypeAMF0Command || message.MessageType == MessageTypeAMF3Command {
			// Quick & dirty check: first few bytes of payload
			if len(message.Payload) > 10 {
				cmdBytes := message.Payload[:10]
				log.Printf("[DEBUG] Received command payload start: % x", cmdBytes)
			}
			
			// Try to decode command name directly from payload
			if len(message.Payload) > 0 && message.Payload[0] == 0x02 { // AMF0 String marker
				if len(message.Payload) >= 3 {
					cmdNameLen := int(message.Payload[1])<<8 | int(message.Payload[2])
					if cmdNameLen > 0 && len(message.Payload) >= 3+cmdNameLen {
						cmdName := string(message.Payload[3:3+cmdNameLen])
						log.Printf("[AMF] Command name: %s (from direct payload parsing)", cmdName)
						
						if cmdName == "connect" {
							log.Println("[RTMP] ✅ GOT CONNECT COMMAND! Now sending success reply")
						}
					}
				}
			}
		}

		// Update chunk writer if client sends SetChunkSize
			if message.MessageType == MessageTypeSetChunkSize {
				if len(message.Payload) >= 4 {
					chunkSize := uint32(message.Payload[0])<<24 |
						uint32(message.Payload[1])<<16 |
						uint32(message.Payload[2])<<8 |
						uint32(message.Payload[3])
					chunkWriter.SetChunkSize(chunkSize)
					state.SetChunkSize(chunkSize) // Also update state's chunk size
					log.Printf("[RTMP] Updated chunk writer chunk size to: %d", chunkSize)
				}
				return nil // No response for SetChunkSize
			}

			// Dispatch message to command handler (returns response chunks)
			responseChunks, err := h.commandHandler.Handle(message, state, rtmpConn)
			if err != nil {
				log.Printf("[RTMP TCP] Command handler error for %s: %v", connID, err)
				return err // Return error to close connection
			}

			// Check if this was a connect command
			isConnectCommand := false
			if message.MessageType == MessageTypeAMF0Command || message.MessageType == MessageTypeAMF3Command {
				if len(message.Payload) >= 10 && message.Payload[0] == 0x02 {
					cmdNameLen := int(message.Payload[1])<<8 | int(message.Payload[2])
					if cmdNameLen == 7 && len(message.Payload) >= 10 {
						cmdName := string(message.Payload[3:10])
						if cmdName == "connect" && len(responseChunks) > 0 {
							isConnectCommand = true
							log.Printf("[RTMP] ✅ Detected connect command (like gortmplib)")
						}
					}
				}
			}

			// Check if this is a play command
			isPlayCommand := false
			if message.MessageType == MessageTypeAMF0Command || message.MessageType == MessageTypeAMF3Command {
				if len(message.Payload) >= 7 && message.Payload[0] == 0x02 {
					cmdNameLen := int(message.Payload[1])<<8 | int(message.Payload[2])
					if cmdNameLen == 4 && len(message.Payload) >= 7 {
						cmdName := string(message.Payload[3:7])
						if cmdName == "play" && len(responseChunks) > 0 {
							isPlayCommand = true
							log.Printf("[RTMP] ✅ Detected play command")
						}
					}
				}
			}

			// Send response chunks for non-connect commands
			if len(responseChunks) > 0 && !isConnectCommand {
				log.Printf("[RTMP TCP] Sending %d response chunk(s) for %s", len(responseChunks), connID)
				for i, respChunk := range responseChunks {
					if err := chunkWriter.WriteChunk(respChunk); err != nil {
						log.Printf("[RTMP TCP] Write chunk error for %s: %v", connID, err)
						return err
					}
					log.Printf("[RTMP TCP] Response chunk %d/%d written: type=0x%02x, size=%d bytes", 
						i+1, len(responseChunks), respChunk.MessageHeader.MessageType, len(respChunk.Payload))
				}
				if err := writer.Flush(); err != nil {
					log.Printf("[RTMP TCP] Flush error for %s: %v", connID, err)
					return err
				}
				log.Printf("[RTMP TCP] Response flushed successfully for %s", connID)
				
				// After play command response is sent, set up MediaWriter for viewer
				if isPlayCommand && rtmpConn.GetState() == domain.ConnectionStatePlaying {
					log.Printf("[RTMP] Setting up MediaWriter for viewer %s", connID)
					viewerWriter := NewViewerMediaWriter(chunkWriter, writer)
					rtmpConn.SetMediaWriter(viewerWriter)
					h.writersMu.Lock()
					h.viewerWriters[connID] = viewerWriter
					h.writersMu.Unlock()
					log.Printf("[RTMP] ✅ MediaWriter set up for viewer %s - will receive media data", connID)
				}
			}
			
			// For connect command: send initialization messages THEN response (like gortmplib)
			if isConnectCommand {
				log.Printf("[RTMP] Sending initialization messages BEFORE connect response (like gortmplib)")
				
				// Send initialization messages FIRST (like gortmplib lines 290-310)
				if err := h.sendInitializationMessages(chunkWriter, state); err != nil {
					log.Printf("[RTMP TCP] Failed to send initialization messages for %s: %v", connID, err)
					return err
				}
				log.Printf("[RTMP] ✅ Initialization messages queued (WinAckSize, SetPeerBandwidth, SetChunkSize)")
				
				// Then send connect response (like gortmplib line 312)
				if len(responseChunks) > 0 {
					log.Printf("[RTMP TCP] Sending connect response chunk(s) for %s", connID)
					for i, respChunk := range responseChunks {
						// Debug: log payload hex for connect response
						payloadPreview := respChunk.Payload
						if len(payloadPreview) > 128 {
							payloadPreview = payloadPreview[:128]
						}
						log.Printf("[RTMP TCP] Connect response chunk %d/%d: type=0x%02x, size=%d bytes, CSID=%d, MSID=%d, first 128 bytes hex: %x", 
							i+1, len(responseChunks), respChunk.MessageHeader.MessageType, len(respChunk.Payload),
							respChunk.BasicHeader.ChunkStreamID, respChunk.MessageHeader.MessageStreamID, payloadPreview)
						
						if err := chunkWriter.WriteChunk(respChunk); err != nil {
							log.Printf("[RTMP TCP] Write chunk error for %s: %v", connID, err)
							return err
						}
						log.Printf("[RTMP TCP] Response chunk %d/%d written: type=0x%02x, size=%d bytes", 
							i+1, len(responseChunks), respChunk.MessageHeader.MessageType, len(respChunk.Payload))
					}
				} else {
					log.Printf("[RTMP TCP] WARNING: Connect command but no response chunks!")
				}
				
				// Flush all queued messages at once
				if err := writer.Flush(); err != nil {
					log.Printf("[RTMP TCP] Flush error for %s: %v", connID, err)
					return err
				}
				log.Printf("[RTMP TCP] ✅ Initialization messages + Connect response flushed successfully for %s", connID)
			}

			return nil // Success
		})

		// ProcessChunk returns error if there's a problem reading or processing
		if err != nil {
			// Check if it's a network timeout (vs EOF/connection closed)
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				log.Printf("[RTMP TCP] Read timeout for %s (no data received for 5 minutes), closing connection", connID)
				break
			}
			
			if err == io.EOF {
				log.Printf("[RTMP TCP] Connection closed by client (EOF) for %s", connID)
				// Flush any pending write data before closing
				if flushErr := writer.Flush(); flushErr != nil {
					log.Printf("[RTMP TCP] Warning: Failed to flush writer before close: %v", flushErr)
				}
				break
			}
			
			log.Printf("[RTMP TCP] ProcessChunk error for %s: %v", connID, err)
			// Flush any pending write data before closing on error
			if flushErr := writer.Flush(); flushErr != nil {
				log.Printf("[RTMP TCP] Warning: Failed to flush writer on error: %v", flushErr)
			}
			break
		}
	}

	// Ensure all pending data is flushed before removing connection
	if flushErr := writer.Flush(); flushErr != nil {
		log.Printf("[RTMP TCP] Warning: Failed to flush writer on connection close: %v", flushErr)
	}
	
	// Cleanup stream if this connection was publishing
	if rtmpConn.GetState() == domain.ConnectionStatePublishing {
		// Get stream info from connection
		streamName := rtmpConn.StreamName // Direct access - connection is being closed
		appName := rtmpConn.AppName
		if streamName != "" {
			if appName == "" {
				appName = "live" // Default app name
			}
			stream, err := h.streamManager.GetStreamByAppAndName(appName, streamName)
			if err == nil && stream != nil && stream.Publisher != nil && stream.Publisher.ID == connID {
				log.Printf("[RTMP TCP] Cleaning up published stream: %s/%s (was published by %s)", appName, streamName, connID)
				// Stop publishing (will reset stream state to Idle and delete stream)
				if err := h.streamManager.StopPublishing(stream.ID); err != nil {
					log.Printf("[RTMP TCP] Warning: Failed to stop publishing stream %s: %v", stream.ID, err)
				} else {
					log.Printf("[RTMP TCP] ✅ Stream %s/%s cleanup completed", appName, streamName)
				}
			}
		}
	}
	
	// Cleanup if this connection was a viewer (playing)
	if rtmpConn.GetState() == domain.ConnectionStatePlaying {
		streamName := rtmpConn.StreamName
		appName := rtmpConn.AppName
		if streamName != "" {
			if appName == "" {
				appName = "live"
			}
			stream, err := h.streamManager.GetStreamByAppAndName(appName, streamName)
			if err == nil && stream != nil {
				stream.RemoveViewer(connID)
				log.Printf("[RTMP TCP] Removed viewer %s from stream %s/%s", connID, appName, streamName)
			}
		}
		// Also cleanup from viewerWriters map
		h.writersMu.Lock()
		delete(h.viewerWriters, connID)
		h.writersMu.Unlock()
	}
	
	h.connectionManager.RemoveConnection(connID)
	log.Printf("[RTMP TCP] Connection closed for %s", connID)

	return nil
}

// getMessageTypeName returns a human-readable name for message type
func getMessageTypeName(msgType uint8) string {
	switch msgType {
	case MessageTypeSetChunkSize:
		return "SetChunkSize"
	case MessageTypeAbortMessage:
		return "AbortMessage"
	case MessageTypeAcknowledgement:
		return "Acknowledgement"
	case MessageTypeUserControl:
		return "UserControl"
	case MessageTypeWindowAckSize:
		return "WindowAckSize"
	case MessageTypeSetPeerBandwidth:
		return "SetPeerBandwidth"
	case MessageTypeAudioData:
		return "AudioData"
	case MessageTypeVideoData:
		return "VideoData"
	case MessageTypeAMF3Data:
		return "AMF3Data"
	case MessageTypeAMF3SharedObject:
		return "AMF3SharedObject"
	case MessageTypeAMF3Command:
		return "AMF3Command"
	case MessageTypeAMF0Data:
		return "AMF0Data"
	case MessageTypeAMF0SharedObject:
		return "AMF0SharedObject"
	case MessageTypeAMF0Command:
		return "AMF0Command"
	case MessageTypeAggregate:
		return "Aggregate"
	case MessageTypeAMF0Metadata:
		return "AMF0Metadata"
	default:
		return "Unknown"
	}
}

// doRTMPHandshake performs RTMP handshake (simplified version)
// Real RTMP handshake: C0+C1 -> S0+S1+S2 -> C2 -> ready
func (h *RTMPConnectionHandler) doRTMPHandshake(conn net.Conn, rtmpConn *domain.RTMPConnection) error {
	// Read C0 (1 byte: version)
	c0 := make([]byte, 1)
	if _, err := conn.Read(c0); err != nil {
		return fmt.Errorf("failed to read C0: %w", err)
	}

	if c0[0] != 0x03 {
		return fmt.Errorf("unsupported RTMP version: %d", c0[0])
	}

	// Read C1 (1536 bytes: timestamp + version + random)
	c1 := make([]byte, 1536)
	if _, err := conn.Read(c1); err != nil {
		return fmt.Errorf("failed to read C1: %w", err)
	}

	// Send S0 (1 byte: version 0x03)
	s0 := []byte{0x03}
	if _, err := conn.Write(s0); err != nil {
		return fmt.Errorf("failed to send S0: %w", err)
	}

	// Send S1 (1536 bytes: timestamp + version + random)
	s1 := make([]byte, 1536)
	s1[0] = 0x00
	s1[1] = 0x00
	s1[2] = 0x00
	s1[3] = 0x00 // timestamp
	s1[4] = 0x00
	s1[5] = 0x00
	s1[6] = 0x00
	s1[7] = 0x00 // version
	// Fill rest with random data (simplified - use c1 data)
	copy(s1[8:], c1[8:])

	if _, err := conn.Write(s1); err != nil {
		return fmt.Errorf("failed to send S1: %w", err)
	}

	// Send S2 (1536 bytes: timestamp + timestamp2 + random echo)
	s2 := make([]byte, 1536)
	s2[0] = 0x00
	s2[1] = 0x00
	s2[2] = 0x00
	s2[3] = 0x00 // timestamp
	s2[4] = 0x00
	s2[5] = 0x00
	s2[6] = 0x00
	s2[7] = 0x00 // timestamp2
	// Echo C1 data
	copy(s2[8:], c1[8:])

	if _, err := conn.Write(s2); err != nil {
		return fmt.Errorf("failed to send S2: %w", err)
	}

	// Read C2 (1536 bytes: timestamp + timestamp2 + random echo)
	c2 := make([]byte, 1536)
	if _, err := conn.Read(c2); err != nil {
		return fmt.Errorf("failed to read C2: %w", err)
	}

	return nil
}

// sendInitializationMessages sends required initialization messages after connect command
// Based on gortmplib implementation (server_conn.go Accept() lines 290-331), send in this order:
// 1. Window Acknowledgment Size (message type 0x05)
// 2. Set Peer Bandwidth (message type 0x06)
// 3. Set Chunk Size (message type 0x01) - gortmplib sends this, unlike go-rtmp
// Note: UserCtrl StreamBegin is NOT sent here (only sent on play command in gortmplib)
// All messages are sent on chunk stream ID 2 (control channel)
func (h *RTMPConnectionHandler) sendInitializationMessages(chunkWriter *ChunkWriter, state *ConnectionState) error {
	const controlChunkStreamID = 2 // Control channel chunk stream ID
	
	// 1. Window Acknowledgment Size (2500000 = 2.5MB)
	windowAckSize := uint32(2500000)
	windowAckPayload := make([]byte, 4)
	windowAckPayload[0] = uint8(windowAckSize >> 24)
	windowAckPayload[1] = uint8(windowAckSize >> 16)
	windowAckPayload[2] = uint8(windowAckSize >> 8)
	windowAckPayload[3] = uint8(windowAckSize)
	
	windowAckChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        0, // Format 0 for first chunk
			ChunkStreamID: controlChunkStreamID,
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   4,
			MessageType:     MessageTypeWindowAckSize,
			MessageStreamID: 0,
		},
		Payload: windowAckPayload,
	}
	
	if err := chunkWriter.WriteChunk(windowAckChunk); err != nil {
		return fmt.Errorf("failed to write WindowAckSize: %w", err)
	}
	
	// 2. Set Peer Bandwidth (2500000 = 2.5MB, limit type 2 = dynamic)
	peerBandwidth := uint32(2500000)
	limitType := uint8(2) // Dynamic limit
	peerBandwidthPayload := make([]byte, 5)
	peerBandwidthPayload[0] = uint8(peerBandwidth >> 24)
	peerBandwidthPayload[1] = uint8(peerBandwidth >> 16)
	peerBandwidthPayload[2] = uint8(peerBandwidth >> 8)
	peerBandwidthPayload[3] = uint8(peerBandwidth)
	peerBandwidthPayload[4] = limitType
	
	peerBandwidthChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        1, // Format 1 (reuse stream ID)
			ChunkStreamID: controlChunkStreamID,
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   5,
			MessageType:     MessageTypeSetPeerBandwidth,
			MessageStreamID: 0,
		},
		Payload: peerBandwidthPayload,
	}
	
	if err := chunkWriter.WriteChunk(peerBandwidthChunk); err != nil {
		return fmt.Errorf("failed to write SetPeerBandwidth: %w", err)
	}
	
	// 3. Set Chunk Size (65536 bytes, like gortmplib line 305-310)
	chunkSize := uint32(65536)
	chunkSizePayload := make([]byte, 4)
	chunkSizePayload[0] = uint8(chunkSize >> 24)
	chunkSizePayload[1] = uint8(chunkSize >> 16)
	chunkSizePayload[2] = uint8(chunkSize >> 8)
	chunkSizePayload[3] = uint8(chunkSize)
	
	chunkSizeChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        1, // Format 1 (reuse stream ID)
			ChunkStreamID: controlChunkStreamID,
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   4,
			MessageType:     MessageTypeSetChunkSize,
			MessageStreamID: 0,
		},
		Payload: chunkSizePayload,
	}
	
	if err := chunkWriter.WriteChunk(chunkSizeChunk); err != nil {
		return fmt.Errorf("failed to write SetChunkSize: %w", err)
	}
	
	// Update state with sent values
	state.SetWindowAckSize(windowAckSize)
	state.SetPeerBandwidth(peerBandwidth, limitType)
	state.SetChunkSize(chunkSize)
	// Update chunk writer with new chunk size
	chunkWriter.SetChunkSize(chunkSize)
	
	log.Printf("[RTMP] Initialization messages sent: WindowAckSize=%d, PeerBandwidth=%d, ChunkSize=%d (like gortmplib)", 
		windowAckSize, peerBandwidth, chunkSize)
	
	return nil
}

// processMessage processes a complete RTMP message
func (h *RTMPConnectionHandler) processMessage(
	conn net.Conn,
	rtmpConn *domain.RTMPConnection,
	message *Message,
) error {
	switch message.MessageType {
	case MessageTypeSetChunkSize:
		// Handle SetChunkSize message
		// Note: chunk size is already handled in ProcessChunk callback
		if len(message.Payload) >= 4 {
			chunkSize := uint32(message.Payload[0])<<24 |
				uint32(message.Payload[1])<<16 |
				uint32(message.Payload[2])<<8 |
				uint32(message.Payload[3])
			log.Printf("[RTMP] SetChunkSize: %d (already handled in ProcessChunk callback)", chunkSize)
		}

	case MessageTypeWindowAckSize:
		// Handle WindowAckSize message
		log.Printf("[RTMP] WindowAckSize received")

	case MessageTypeUserControl:
		// Handle UserControl message
		log.Printf("[RTMP] UserControl message received")

	case MessageTypeAudioData:
		// Handle audio data
		return h.handleMediaData(rtmpConn, message, "audio")

	case MessageTypeVideoData:
		// Handle video data
		return h.handleMediaData(rtmpConn, message, "video")

	case MessageTypeAMF0Command, MessageTypeAMF3Command:
		// Handle AMF command
		return h.handleAMFCommand(conn, rtmpConn, message)

	case MessageTypeAMF0Data, MessageTypeAMF3Data:
		// Handle AMF data message
		log.Printf("[RTMP] AMF data message received")

	case MessageTypeAMF0Metadata, MessageTypeAMF3SharedObject:
		// Handle metadata
		log.Printf("[RTMP] Metadata message received")

	default:
		log.Printf("[RTMP] Unknown message type: 0x%02x", message.MessageType)
	}

	return nil
}

// handleAMFCommand handles AMF command messages
func (h *RTMPConnectionHandler) handleAMFCommand(
	conn net.Conn,
	rtmpConn *domain.RTMPConnection,
	message *Message,
) error {
	// Decode AMF command
	command, err := h.amfParser.DecodeCommand(message.Payload)
	if err != nil {
		return fmt.Errorf("failed to decode AMF command: %w", err)
	}

	// Dispatch command to command handler
	response, err := h.commandHandler.HandleCommand(command, rtmpConn)
	if err != nil {
		log.Printf("[RTMP] Command handler error: %v", err)
		return err
	}

	// Send response if any
	if response != nil {
		encoded, err := h.amfParser.EncodeResponse(response)
		if err != nil {
			return fmt.Errorf("failed to encode response: %w", err)
		}

		// Send response message (simplified - should use chunk writer)
		if _, err := conn.Write(encoded); err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}
	}

	return nil
}

// handleMediaData handles audio/video media data
func (h *RTMPConnectionHandler) handleMediaData(
	rtmpConn *domain.RTMPConnection,
	message *Message,
	mediaType string,
) error {
	appName := rtmpConn.AppName
	streamName := rtmpConn.StreamName

	if appName == "" || streamName == "" {
		// Not publishing yet, ignore media data
		return nil
	}

	// Get stream
	stream, err := h.streamManager.GetStreamByAppAndName(appName, streamName)
	if err != nil {
		return fmt.Errorf("stream not found: %s/%s", appName, streamName)
	}

	// Broadcast media to viewers
	if err := h.streamManager.BroadcastMedia(stream.ID, message.Payload); err != nil {
		return fmt.Errorf("failed to broadcast media: %w", err)
	}

	stream.UpdateLastMedia()

	// Publish media broadcast event
	if h.eventBus != nil {
		viewers := stream.GetViewers()
		_ = h.eventBus.Publish("rtmp.media.broadcasted", map[string]interface{}{
			"stream_id":    stream.ID,
			"viewer_count": len(viewers),
			"media_type":   mediaType,
			"data_size":    len(message.Payload),
		})
	}

	return nil
}

// chunkReader wraps net.Conn to provide buffered reading for chunks
type chunkReader struct {
	conn   net.Conn
	buffer []byte
	offset int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	// If buffer is empty, read from connection
	if r.offset >= len(r.buffer) {
		r.buffer = make([]byte, len(p))
		n, err := r.conn.Read(r.buffer)
		if err != nil {
			return 0, err
		}
		r.buffer = r.buffer[:n]
		r.offset = 0
	}

	// Copy from buffer
	n := copy(p, r.buffer[r.offset:])
	r.offset += n

	// Reset buffer if fully consumed
	if r.offset >= len(r.buffer) {
		r.buffer = r.buffer[:0]
		r.offset = 0
	}

	return n, nil
}
