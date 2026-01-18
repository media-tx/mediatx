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
	"fmt"
	"log"
	"strings"

	"github.com/fluxorio/fluxor/pkg/core"
	"github.com/fluxorio/fluxor/apps/rtmp-server/domain"
)

// RTMPCommandHandler dispatches RTMP commands to domain layer
// This is part of Protocol Layer (Level 1)
type RTMPCommandHandler struct {
	connectionManager domain.ConnectionManager
	streamManager     domain.StreamManager
	eventBus          core.EventBus
	amfParser         *AMFParser
	mediaForwarder    *MediaForwarder
}

// NewRTMPCommandHandler creates a new RTMP command handler
func NewRTMPCommandHandler(
	connectionManager domain.ConnectionManager,
	streamManager domain.StreamManager,
	eventBus core.EventBus,
	amfParser *AMFParser,
) *RTMPCommandHandler {
	return &RTMPCommandHandler{
		connectionManager: connectionManager,
		streamManager:     streamManager,
		eventBus:          eventBus,
		amfParser:         amfParser,
		mediaForwarder:    NewMediaForwarder(streamManager),
	}
}

// Handle handles a complete RTMP message and returns response chunks
// This is the main entry point for processing messages in the new structure
func (h *RTMPCommandHandler) Handle(
	message *Message,
	state *ConnectionState,
	rtmpConn *domain.RTMPConnection,
) ([]*Chunk, error) {
	// Handle protocol control messages first
	switch message.MessageType {
	case MessageTypeSetChunkSize:
		// Handle SetChunkSize message
		if len(message.Payload) >= 4 {
			chunkSize := uint32(message.Payload[0])<<24 |
				uint32(message.Payload[1])<<16 |
				uint32(message.Payload[2])<<8 |
				uint32(message.Payload[3])
			state.SetChunkSize(chunkSize)
			log.Printf("[RTMP] SetChunkSize: %d", chunkSize)
		}
		return nil, nil // No response needed

	case MessageTypeWindowAckSize:
		// Handle WindowAckSize message
		if len(message.Payload) >= 4 {
			windowAckSize := uint32(message.Payload[0])<<24 |
				uint32(message.Payload[1])<<16 |
				uint32(message.Payload[2])<<8 |
				uint32(message.Payload[3])
			state.SetWindowAckSize(windowAckSize)
			log.Printf("[RTMP] WindowAckSize: %d", windowAckSize)
		}
		return nil, nil // No response needed

	case MessageTypeSetPeerBandwidth:
		// Handle SetPeerBandwidth message
		if len(message.Payload) >= 5 {
			bandwidth := uint32(message.Payload[0])<<24 |
				uint32(message.Payload[1])<<16 |
				uint32(message.Payload[2])<<8 |
				uint32(message.Payload[3])
			limitType := message.Payload[4]
			state.SetPeerBandwidth(bandwidth, limitType)
			log.Printf("[RTMP] SetPeerBandwidth: %d (type=%d)", bandwidth, limitType)
		}
		return nil, nil // No response needed

	case MessageTypeAMF0Command, MessageTypeAMF3Command:
		// Handle AMF command - decode and process
		return h.handleAMFCommand(message, state, rtmpConn)

	case MessageTypeVideoData:
		// Forward video data using MediaForwarder
		if rtmpConn.GetState() == domain.ConnectionStatePublishing {
			if stream := h.getPublisherStream(rtmpConn); stream != nil {
				h.mediaForwarder.ForwardVideoData(stream, message.Timestamp, message.Payload)
			}
		}
		return nil, nil

	case MessageTypeAudioData:
		// Forward audio data using MediaForwarder
		if rtmpConn.GetState() == domain.ConnectionStatePublishing {
			if stream := h.getPublisherStream(rtmpConn); stream != nil {
				h.mediaForwarder.ForwardAudioData(stream, message.Timestamp, message.Payload)
			}
		}
		return nil, nil
	
	case MessageTypeAMF0Data, MessageTypeAMF3Data:
		// Forward metadata using MediaForwarder
		if rtmpConn.GetState() == domain.ConnectionStatePublishing {
			if stream := h.getPublisherStream(rtmpConn); stream != nil {
				h.mediaForwarder.ForwardMetadata(stream, message.Timestamp, message.Payload)
			}
		}
		return nil, nil

	default:
		log.Printf("[RTMP] Unknown message type: 0x%02x", message.MessageType)
		return nil, nil
	}
}

// getPublisherStream returns the stream for a publishing connection
func (h *RTMPCommandHandler) getPublisherStream(rtmpConn *domain.RTMPConnection) *domain.RTMPStream {
	if rtmpConn.AppName == "" || rtmpConn.StreamName == "" {
		return nil
	}
	stream, err := h.streamManager.GetStreamByAppAndName(rtmpConn.AppName, rtmpConn.StreamName)
	if err != nil {
		return nil
	}
	return stream
}

// GetMediaForwarder returns the media forwarder (for sending initial data to viewers)
func (h *RTMPCommandHandler) GetMediaForwarder() *MediaForwarder {
	return h.mediaForwarder
}

// handleAMFCommand handles AMF command messages and returns response chunks
func (h *RTMPCommandHandler) handleAMFCommand(
	message *Message,
	state *ConnectionState,
	rtmpConn *domain.RTMPConnection,
) ([]*Chunk, error) {
	log.Printf("[RTMP] Handling AMF command: type=0x%02x, payload=%d bytes", message.MessageType, len(message.Payload))
	
	// Decode AMF command
	command, err := h.amfParser.DecodeCommand(message.Payload)
	if err != nil {
		payloadPreview := message.Payload
		if len(payloadPreview) > 100 {
			payloadPreview = payloadPreview[:100]
		}
		log.Printf("[RTMP] Failed to decode AMF command: %v, payload first 100 bytes: %x", err, payloadPreview)
		return nil, fmt.Errorf("failed to decode AMF command: %w", err)
	}

	// Handle command using existing HandleCommand method
	response, err := h.HandleCommand(command, rtmpConn)
	if err != nil {
		return nil, fmt.Errorf("command handler error: %w", err)
	}

	// Special handling for connect command: send _result with 2 objects (like gortmplib)
	// Format: _result (string) + transactionID (number) + Properties object + Information object
	// Per gortmplib server_conn.go line 312-328: single _result message with 2 Arguments
	if command.Name == CommandConnect && response != nil {
		return h.handleConnectResponse(command, response, message, state, rtmpConn)
	}

	// Special handling for play command: send UserControlStreamBegin + onStatus
	if command.Name == CommandPlay && response != nil {
		return h.handlePlayResponse(command, response, message, state, rtmpConn)
	}

	// If no response, return empty chunks
	if response == nil {
		log.Printf("[RTMP] Command '%s' returned no response", command.Name)
		return nil, nil
	}

	log.Printf("[RTMP] Encoding response for command '%s': %+v", command.Name, response)

	// Use pre-encoded bytes if available (for special cases like getStreamLength)
	var encoded []byte
	if len(response.RawBytes) > 0 {
		encoded = response.RawBytes
		log.Printf("[RTMP] Using pre-encoded response: %d bytes, hex: %x", len(encoded), encoded)
	} else {
		// Encode response to AMF
		var err error
		encoded, err = h.amfParser.EncodeResponse(response)
		if err != nil {
			log.Printf("[RTMP] Failed to encode response: %v", err)
			return nil, fmt.Errorf("failed to encode response: %w", err)
		}
	}
	
	encodedPreview := encoded
	if len(encodedPreview) > 64 {
		encodedPreview = encodedPreview[:64]
	}
	log.Printf("[RTMP] Response encoded: %d bytes, first 64 bytes: %x", len(encoded), encodedPreview)

	// Determine chunk stream ID and message stream ID for response
	// Based on gortmplib: onStatus responses use ChunkStreamID=5 and MessageStreamID=0x1000000
	var chunkStreamID uint32
	var messageStreamID uint32
	
	if response.Name == CommandOnStatus {
		// onStatus responses use chunk stream ID 5 (like gortmplib line 475)
		chunkStreamID = 5
		// MessageStreamID = 0x1000000 (little-endian format for stream ID 1, like gortmplib line 478)
		messageStreamID = 0x1000000
		log.Printf("[RTMP] onStatus response: using ChunkStreamID=5, MessageStreamID=0x%08x", messageStreamID)
	} else {
		// For _result responses (connect, createStream), use same chunk stream ID and message stream ID
		chunkStreamID = message.ChunkStreamID
		messageStreamID = message.MessageStreamID
	}

		// Log raw hex dump for onStatus responses (debug)
		if response.Name == CommandOnStatus {
			hexDump := make([]string, len(encoded))
			for i, b := range encoded {
				hexDump[i] = fmt.Sprintf("%02x", b)
			}
			hexStr := strings.Join(hexDump, "")
			first64 := hexStr
			if len(hexStr) > 128 {
				first64 = hexStr[:128]
			}
			log.Printf("[RTMP] onStatus response RAW (txn=%.1f): %d bytes, hex=%s...", 
				response.TransactionID, len(encoded), first64)
		}

		// Create response message
		responseMessage := &Message{
			ChunkStreamID:  chunkStreamID,
			Timestamp:      0,                     // Response timestamp
			MessageLength:  uint32(len(encoded)),
			MessageType:    message.MessageType,   // Use same message type (AMF0/AMF3)
			MessageStreamID: messageStreamID,
			Payload:        encoded,
		}

	// Convert message to chunks (simplified - single chunk for now)
	// In real implementation, this should split into multiple chunks if needed
	chunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        0, // Format 0 for first chunk
			ChunkStreamID: chunkStreamID,
			HeaderLength:  1, // Simplified
		},
		MessageHeader: &MessageHeader{
			Timestamp:       responseMessage.Timestamp,
			MessageLength:   responseMessage.MessageLength,
			MessageType:     responseMessage.MessageType,
			MessageStreamID: messageStreamID,
		},
		Payload: responseMessage.Payload,
	}

	return []*Chunk{chunk}, nil
}

// handleConnectResponse handles connect command response by sending _result with both Properties and Information (like gortmplib)
// Format: _result (string) + transactionID (number) + null + Properties object + Information object
// Per gortmplib: single _result message with 2 objects (Properties + Information)
// This matches FFmpeg expectations better than separate messages
func (h *RTMPCommandHandler) handleConnectResponse(
	command *Command,
	resultResponse *CommandResponse,
	message *Message,
	state *ConnectionState,
	rtmpConn *domain.RTMPConnection,
) ([]*Chunk, error) {
	// Encode _result with 2 objects: Properties + Information (like gortmplib line 316-327)
	// Format: _result (string) + transactionID (number) + null + Properties object + Information object
	log.Printf("[RTMP] Encoding _result response for connect (2 objects): %+v", resultResponse)
	encoded, err := h.amfParser.EncodeConnectResult(resultResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to encode _result: %w", err)
	}
	
	encodedPreview := encoded
	if len(encodedPreview) > 64 {
		encodedPreview = encodedPreview[:64]
	}
	log.Printf("[RTMP] _result encoded: %d bytes, first 64 bytes: %x", len(encoded), encodedPreview)

	// Create _result chunk (single message with both objects)
	resultChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        0,
			ChunkStreamID: message.ChunkStreamID, // Use same CSID as connect command (usually 3)
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   uint32(len(encoded)),
			MessageType:     message.MessageType, // Same message type (AMF0/AMF3)
			MessageStreamID: message.MessageStreamID,
		},
		Payload: encoded,
	}

	log.Printf("[RTMP] ✅ Connect response: _result (CSID=%d) with 2 objects (Properties + Information) = 1 chunk", message.ChunkStreamID)
	return []*Chunk{resultChunk}, nil
}

// handlePlayResponse handles play command response by sending all required messages (like gortmplib)
// Per gortmplib server_conn.go line 370-418: send UserControlStreamIsRecorded, UserControlStreamBegin,
// onStatus (Reset), onStatus (Start) in sequence
func (h *RTMPCommandHandler) handlePlayResponse(
	command *Command,
	response *CommandResponse,
	message *Message,
	state *ConnectionState,
	rtmpConn *domain.RTMPConnection,
) ([]*Chunk, error) {
	var chunks []*Chunk
	streamID := uint32(1) // Stream ID = 1

	// 1. Send UserControlStreamIsRecorded (per gortmplib line 370-375)
	// UserControl message: type=0x04, event type=4 (StreamIsRecorded), stream ID=1
	streamIsRecordedPayload := make([]byte, 6)
	streamIsRecordedPayload[0] = 0x00 // Event type high byte (StreamIsRecorded = 4)
	streamIsRecordedPayload[1] = 0x04 // Event type low byte
	streamIsRecordedPayload[2] = byte(streamID >> 24) // Stream ID (big-endian)
	streamIsRecordedPayload[3] = byte(streamID >> 16)
	streamIsRecordedPayload[4] = byte(streamID >> 8)
	streamIsRecordedPayload[5] = byte(streamID)

	streamIsRecordedChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        0,
			ChunkStreamID: 2, // UserControl messages use CSID=2 (control stream)
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   6,
			MessageType:     MessageTypeUserControl,
			MessageStreamID: 0, // UserControl uses MSID=0
		},
		Payload: streamIsRecordedPayload,
	}
	chunks = append(chunks, streamIsRecordedChunk)
	log.Printf("[RTMP] UserControlStreamIsRecorded queued (CSID=2, StreamID=1)")

	// 2. Send UserControlStreamBegin (per gortmplib line 377-382)
	streamBeginPayload := make([]byte, 6)
	streamBeginPayload[0] = 0x00 // Event type high byte (StreamBegin = 0)
	streamBeginPayload[1] = 0x00 // Event type low byte
	streamBeginPayload[2] = byte(streamID >> 24) // Stream ID (big-endian)
	streamBeginPayload[3] = byte(streamID >> 16)
	streamBeginPayload[4] = byte(streamID >> 8)
	streamBeginPayload[5] = byte(streamID)

	streamBeginChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        0,
			ChunkStreamID: 2,
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   6,
			MessageType:     MessageTypeUserControl,
			MessageStreamID: 0,
		},
		Payload: streamBeginPayload,
	}
	chunks = append(chunks, streamBeginChunk)
	log.Printf("[RTMP] UserControlStreamBegin queued (CSID=2, StreamID=1)")

	// 3. Send onStatus with NetStream.Play.Reset (per gortmplib line 384-400)
	resetResponse := &CommandResponse{
		Name:          CommandOnStatus,
		TransactionID: command.TransactionID, // Use command's transaction ID (usually 0.0)
		CommandObject: nil,
		Properties:    map[string]interface{}{},
		Information: map[string]interface{}{
			"level":       "status",
			"code":        "NetStream.Play.Reset",
			"description": "play reset",
		},
	}
	encodedReset, err := h.amfParser.EncodeResponse(resetResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to encode onStatus Reset: %w", err)
	}
	onStatusResetChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        0,
			ChunkStreamID: 5, // onStatus uses CSID=5
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   uint32(len(encodedReset)),
			MessageType:     message.MessageType,
			MessageStreamID: 0x1000000, // Stream ID 1
		},
		Payload: encodedReset,
	}
	chunks = append(chunks, onStatusResetChunk)
	log.Printf("[RTMP] onStatus (NetStream.Play.Reset) queued (CSID=5)")

	// 4. Send onStatus with NetStream.Play.Start (per gortmplib line 402-418)
	startResponse := &CommandResponse{
		Name:          CommandOnStatus,
		TransactionID: command.TransactionID, // Use command's transaction ID (usually 0.0)
		CommandObject: nil,
		Properties:    map[string]interface{}{},
		Information: map[string]interface{}{
			"level":       "status",
			"code":        "NetStream.Play.Start",
			"description": "play start",
		},
	}
	encodedStart, err := h.amfParser.EncodeResponse(startResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to encode onStatus Start: %w", err)
	}
	onStatusStartChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        0,
			ChunkStreamID: 5,
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   uint32(len(encodedStart)),
			MessageType:     message.MessageType,
			MessageStreamID: 0x1000000,
		},
		Payload: encodedStart,
	}
	chunks = append(chunks, onStatusStartChunk)
	log.Printf("[RTMP] onStatus (NetStream.Play.Start) queued (CSID=5)")

	// 5. Send onStatus with NetStream.Data.Start (per gortmplib line 420-434)
	dataStartResponse := &CommandResponse{
		Name:          CommandOnStatus,
		TransactionID: command.TransactionID, // Use command's transaction ID (usually 0.0)
		CommandObject: nil,
		Properties:    map[string]interface{}{},
		Information: map[string]interface{}{
			"level":       "status",
			"code":        "NetStream.Data.Start",
			"description": "data start",
		},
	}
	encodedDataStart, err := h.amfParser.EncodeResponse(dataStartResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to encode onStatus Data.Start: %w", err)
	}
	onStatusDataStartChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        0,
			ChunkStreamID: 5,
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   uint32(len(encodedDataStart)),
			MessageType:     message.MessageType,
			MessageStreamID: 0x1000000,
		},
		Payload: encodedDataStart,
	}
	chunks = append(chunks, onStatusDataStartChunk)
	log.Printf("[RTMP] onStatus (NetStream.Data.Start) queued (CSID=5)")

	// 6. Send onStatus with NetStream.Play.PublishNotify (per gortmplib line 438-454)
	publishNotifyResponse := &CommandResponse{
		Name:          CommandOnStatus,
		TransactionID: command.TransactionID, // Use command's transaction ID (usually 0.0)
		CommandObject: nil,
		Properties:    map[string]interface{}{},
		Information: map[string]interface{}{
			"level":       "status",
			"code":        "NetStream.Play.PublishNotify",
			"description": "publish notify",
		},
	}
	encodedPublishNotify, err := h.amfParser.EncodeResponse(publishNotifyResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to encode onStatus PublishNotify: %w", err)
	}
	onStatusPublishNotifyChunk := &Chunk{
		BasicHeader: &BasicHeader{
			Format:        0,
			ChunkStreamID: 5,
			HeaderLength:  1,
		},
		MessageHeader: &MessageHeader{
			Timestamp:       0,
			MessageLength:   uint32(len(encodedPublishNotify)),
			MessageType:     message.MessageType,
			MessageStreamID: 0x1000000,
		},
		Payload: encodedPublishNotify,
	}
	chunks = append(chunks, onStatusPublishNotifyChunk)
	log.Printf("[RTMP] onStatus (NetStream.Play.PublishNotify) queued (CSID=5)")

	log.Printf("[RTMP] ✅ Play response: UserControlStreamIsRecorded + UserControlStreamBegin + onStatus(Reset) + onStatus(Start) + onStatus(Data.Start) + onStatus(PublishNotify) = 6 chunks")
	
	// Get stream to send cached metadata and sequence headers
	appName := rtmpConn.AppName
	if appName == "" {
		appName = "live"
	}
	streamName := ""
	if len(command.Args) > 0 {
		if sn, ok := command.Args[0].(string); ok {
			streamName = sn
		}
	}
	
	if streamName != "" {
		stream, err := h.streamManager.GetStreamByAppAndName(appName, streamName)
		if err == nil && stream != nil {
			// Send cached metadata (onMetaData) if available
			if metadata := stream.GetMetadata(); metadata != nil {
				log.Printf("[RTMP] Sending cached metadata (%d bytes) to new viewer %s", len(metadata), rtmpConn.ID)
				metadataChunk := &Chunk{
					BasicHeader: &BasicHeader{
						Format:        0,
						ChunkStreamID: 5, // Same as onStatus
						HeaderLength:  1,
					},
					MessageHeader: &MessageHeader{
						Timestamp:       0,
						MessageLength:   uint32(len(metadata)),
						MessageType:     MessageTypeAMF0Data,
						MessageStreamID: 0x1000000,
					},
					Payload: metadata,
				}
				chunks = append(chunks, metadataChunk)
			}
			
			// Send cached audio sequence header if available
			if audioSeqHeader := stream.GetAudioSequenceHeader(); audioSeqHeader != nil {
				log.Printf("[RTMP] Sending cached audio sequence header (%d bytes) to new viewer %s", len(audioSeqHeader), rtmpConn.ID)
				audioSeqChunk := &Chunk{
					BasicHeader: &BasicHeader{
						Format:        0,
						ChunkStreamID: 4, // Audio uses CSID 4
						HeaderLength:  1,
					},
					MessageHeader: &MessageHeader{
						Timestamp:       0,
						MessageLength:   uint32(len(audioSeqHeader)),
						MessageType:     MessageTypeAudioData,
						MessageStreamID: 0x1000000,
					},
					Payload: audioSeqHeader,
				}
				chunks = append(chunks, audioSeqChunk)
			}
			
			// Send cached video sequence header if available
			if videoSeqHeader := stream.GetVideoSequenceHeader(); videoSeqHeader != nil {
				log.Printf("[RTMP] Sending cached video sequence header (%d bytes) to new viewer %s", len(videoSeqHeader), rtmpConn.ID)
				videoSeqChunk := &Chunk{
					BasicHeader: &BasicHeader{
						Format:        0,
						ChunkStreamID: 6, // Video uses CSID 6
						HeaderLength:  1,
					},
					MessageHeader: &MessageHeader{
						Timestamp:       0,
						MessageLength:   uint32(len(videoSeqHeader)),
						MessageType:     MessageTypeVideoData,
						MessageStreamID: 0x1000000,
					},
					Payload: videoSeqHeader,
				}
				chunks = append(chunks, videoSeqChunk)
			}
		}
	}
	
	return chunks, nil
}

// HandleCommand handles an RTMP command
// Returns response or nil if no response needed
func (h *RTMPCommandHandler) HandleCommand(
	command *Command,
	rtmpConn *domain.RTMPConnection,
) (*CommandResponse, error) {
	log.Printf("[RTMP] Command: %s (txn=%v) from %s", command.Name, command.TransactionID, rtmpConn.ID)

	switch command.Name {
	case CommandConnect:
		return h.handleConnect(command, rtmpConn)

	case CommandCreateStream:
		return h.handleCreateStream(command, rtmpConn)

	case CommandPublish:
		return h.handlePublish(command, rtmpConn)

	case CommandPlay:
		return h.handlePlay(command, rtmpConn)

	case CommandDeleteStream:
		return h.handleDeleteStream(command, rtmpConn)

	case CommandReleaseStream:
		return h.handleReleaseStream(command, rtmpConn)

	case CommandFCPublish:
		return h.handleFCPublish(command, rtmpConn)

	case CommandFCUnpublish:
		return h.handleFCUnpublish(command, rtmpConn)

	case CommandGetStreamLength:
		// Handle getStreamLength command (client queries stream duration)
		// Return _result with duration or -1 if unknown (like gortmplib/native clients)
		log.Printf("[RTMP] getStreamLength command from %s (returning -1 for unknown duration)", rtmpConn.ID)
		// Return _result with duration = -1.0 (unknown/live stream)
		// Format: _result (string) + transactionID (number) + null + duration (number)
		// Duration must be a direct number, not wrapped in an object
		// Use special encoder for getStreamLength response
		encoded, err := h.amfParser.EncodeGetStreamLengthResult(CommandResult, command.TransactionID, -1.0)
		if err != nil {
			log.Printf("[RTMP] Failed to encode getStreamLength response: %v", err)
			return nil, fmt.Errorf("failed to encode getStreamLength response: %w", err)
		}
		log.Printf("[RTMP] getStreamLength response encoded: %d bytes, hex: %x", len(encoded), encoded)
		// Return a special response that will be handled differently
		// We'll send the encoded bytes directly in the connection handler
		return &CommandResponse{
			Name:          CommandResult,
			TransactionID: command.TransactionID,
			RawBytes:       encoded, // Store pre-encoded bytes
		}, nil

	default:
		// Unknown command - log warning but don't close connection
		// Some clients send optional commands that we don't need to support
		log.Printf("[RTMP] ⚠️  Unknown/unsupported command: %s (txn=%v) from %s - ignoring (connection stays open)", 
			command.Name, command.TransactionID, rtmpConn.ID)
		// Return nil - no response, but connection stays open
		// Some commands are optional and clients should handle "no response" gracefully
		return nil, nil
	}
}

// handleConnect handles connect command
func (h *RTMPCommandHandler) handleConnect(command *Command, rtmpConn *domain.RTMPConnection) (*CommandResponse, error) {
	// Extract app name from command object
	appName := ""
	if commandObject, ok := command.CommandObject.(map[string]interface{}); ok {
		if app, ok := commandObject["app"].(string); ok {
			appName = app
			rtmpConn.SetAppName(appName)
		}
		// tcUrl format: rtmp://server/app
		// Extract app name from tcUrl if not in command object
		if appName == "" {
			if tcURL, ok := commandObject["tcUrl"].(string); ok && tcURL != "" {
				// Parse tcURL to extract app name
				// Simplified: assume format rtmp://host/app
				// In real implementation, parse URL properly
				_ = tcURL // TODO: Parse URL to extract app name
			}
		}
	}

	// Connection successful
	rtmpConn.SetState(domain.ConnectionStateConnected)

	// Publish connection success event
	if h.eventBus != nil {
		_ = h.eventBus.Publish("rtmp.connection.connected", map[string]interface{}{
			"connection_id": rtmpConn.ID,
			"app_name":      appName,
		})
	}

	// Response: _result with null + Properties + Information (for max compatibility)
	// Format: _result (string) + transactionID (number) + null + Properties object + Information object
	// Per user's example: includes null between transactionID and Properties for max compatibility
	response := &CommandResponse{
		Name:          CommandResult,
		TransactionID: command.TransactionID,
		CommandObject: nil, // Not used, but null will be encoded in response
		Properties: map[string]interface{}{
			"fmsVer":       "FMS/3,5,7,7009", // Per user's example (changed from LNX 9,0,124,2)
			"capabilities": 127.0,            // Per user's example (changed from 31.0)
		},
		Information: map[string]interface{}{ // Second object in _result
			"level":         "status",
			"code":          "NetConnection.Connect.Success",
			"description":   "Connection succeeded.",
			"objectEncoding": 0.0, // 0=AMF0, 3=AMF3
		},
	}

	return response, nil
}

// handleCreateStream handles createStream command
func (h *RTMPCommandHandler) handleCreateStream(command *Command, rtmpConn *domain.RTMPConnection) (*CommandResponse, error) {
	// Response: _result with stream ID as direct number (not in object!)
	// Stream ID is typically a sequential number (1, 2, 3, ...)
	streamID := float64(1) // Simplified: use 1 as stream ID

	// Use special encoder for createStream result
	encoded, err := h.amfParser.EncodeCreateStreamResult(CommandResult, command.TransactionID, streamID)
	if err != nil {
		return nil, fmt.Errorf("failed to encode createStream response: %w", err)
	}
	
	log.Printf("[RTMP] createStream response encoded: %d bytes, hex: %x", len(encoded), encoded)

	response := &CommandResponse{
		Name:          CommandResult,
		TransactionID: command.TransactionID,
		RawBytes:      encoded, // Use pre-encoded bytes
	}

	return response, nil
}

// handlePublish handles publish command
func (h *RTMPCommandHandler) handlePublish(command *Command, rtmpConn *domain.RTMPConnection) (*CommandResponse, error) {
	// Extract stream name and publishing type from command args
	if len(command.Args) < 2 {
		return nil, fmt.Errorf("publish command requires stream name and type")
	}

	streamName, ok := command.Args[0].(string)
	if !ok {
		return nil, fmt.Errorf("invalid stream name")
	}

	publishingType, ok := command.Args[1].(string)
	if !ok {
		return nil, fmt.Errorf("invalid publishing type")
	}

	appName := rtmpConn.AppName
	if appName == "" {
		appName = "live" // Default app name
	}

	log.Printf("[RTMP] Publish: %s/%s (type=%s) from %s", appName, streamName, publishingType, rtmpConn.ID)

	// Get or create stream
	stream, err := h.streamManager.GetStreamByAppAndName(appName, streamName)
	if err != nil {
		// Stream doesn't exist, create it
		stream, err = h.streamManager.CreateStream(appName, streamName)
		if err != nil {
			return nil, fmt.Errorf("failed to create stream: %w", err)
		}
	}

	// Start publishing
	err = h.streamManager.PublishStream(stream.ID, rtmpConn)
	if err != nil {
		return nil, fmt.Errorf("failed to publish stream: %w", err)
	}

	rtmpConn.SetStreamName(streamName)
	rtmpConn.SetState(domain.ConnectionStatePublishing)

	// Publish stream published event
	if h.eventBus != nil {
		_ = h.eventBus.Publish("rtmp.stream.published", map[string]interface{}{
			"stream_id":    stream.ID,
			"publisher_id": rtmpConn.ID,
			"app_name":     appName,
			"stream_name":  streamName,
			"type":         publishingType,
		})
	}

	// Response: onStatus with NetStream.Publish.Start
	// Based on gortmplib: use CommandID from publish command (not 0.0)
	response := &CommandResponse{
		Name:          CommandOnStatus,
		TransactionID: command.TransactionID, // Use transaction ID from publish command (like gortmplib line 477)
		CommandObject: nil,
		Properties:    map[string]interface{}{},
		Information: map[string]interface{}{
			"level":       "status",
			"code":        "NetStream.Publish.Start",
			"description": "Stream is now published",
		},
	}

	return response, nil
}

// handlePlay handles play command
func (h *RTMPCommandHandler) handlePlay(command *Command, rtmpConn *domain.RTMPConnection) (*CommandResponse, error) {
	// Extract stream name from command args
	if len(command.Args) < 1 {
		return nil, fmt.Errorf("play command requires stream name")
	}

	streamName, ok := command.Args[0].(string)
	if !ok {
		return nil, fmt.Errorf("invalid stream name")
	}

	appName := rtmpConn.AppName
	if appName == "" {
		appName = "live" // Default app name
	}

	log.Printf("[RTMP] Play: %s/%s from %s", appName, streamName, rtmpConn.ID)

	// Get stream
	stream, err := h.streamManager.GetStreamByAppAndName(appName, streamName)
	if err != nil {
		return nil, fmt.Errorf("stream not found: %s/%s", appName, streamName)
	}

	// Start playing
	err = h.streamManager.PlayStream(stream.ID, rtmpConn)
	if err != nil {
		return nil, fmt.Errorf("failed to play stream: %w", err)
	}

	rtmpConn.SetStreamName(streamName)
	rtmpConn.SetState(domain.ConnectionStatePlaying)

	// Publish stream played event
	if h.eventBus != nil {
		_ = h.eventBus.Publish("rtmp.stream.played", map[string]interface{}{
			"stream_id": stream.ID,
			"viewer_id": rtmpConn.ID,
			"app_name":  appName,
			"stream_name": streamName,
		})
	}

	// Response: onStatus with NetStream.Play.Start
	response := &CommandResponse{
		Name:          CommandOnStatus,
		TransactionID: 0.0,
		CommandObject: nil,
		Properties:    map[string]interface{}{},
		Information: map[string]interface{}{
			"level":       "status",
			"code":        "NetStream.Play.Start",
			"description": "Stream started",
		},
	}

	return response, nil
}

// handleDeleteStream handles deleteStream command
func (h *RTMPCommandHandler) handleDeleteStream(command *Command, rtmpConn *domain.RTMPConnection) (*CommandResponse, error) {
	// Extract stream ID from command args
	if len(command.Args) < 1 {
		return nil, fmt.Errorf("deleteStream command requires stream ID")
	}

	appName := rtmpConn.AppName
	streamName := rtmpConn.StreamName

	if appName != "" && streamName != "" {
		stream, err := h.streamManager.GetStreamByAppAndName(appName, streamName)
		if err == nil {
			h.streamManager.StopPublishing(stream.ID)
		}
	}

	rtmpConn.SetState(domain.ConnectionStateConnected)

	// No response for deleteStream
	return nil, nil
}

// handleReleaseStream handles releaseStream command
func (h *RTMPCommandHandler) handleReleaseStream(command *Command, rtmpConn *domain.RTMPConnection) (*CommandResponse, error) {
	// ReleaseStream is typically called before publish
	// For now, just acknowledge
	log.Printf("[RTMP] ReleaseStream from %s", rtmpConn.ID)
	return nil, nil
}

// handleFCPublish handles FCPublish command
func (h *RTMPCommandHandler) handleFCPublish(command *Command, rtmpConn *domain.RTMPConnection) (*CommandResponse, error) {
	// FCPublish is typically called before publish
	// For now, just acknowledge
	log.Printf("[RTMP] FCPublish from %s", rtmpConn.ID)
	return nil, nil
}

// handleFCUnpublish handles FCUnpublish command
func (h *RTMPCommandHandler) handleFCUnpublish(command *Command, rtmpConn *domain.RTMPConnection) (*CommandResponse, error) {
	// FCUnpublish is typically called before unpublish
	// For now, just acknowledge
	log.Printf("[RTMP] FCUnpublish from %s", rtmpConn.ID)
	return nil, nil
}
