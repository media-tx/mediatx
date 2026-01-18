package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/fluxorio/fluxor/pkg/core"
	"github.com/fluxorio/fluxor/apps/rtmp-server/domain"
)

// SRTConnection represents an SRT connection interface
// This abstracts the gosrt.Conn interface for easier testing and future changes
type SRTConnection interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	RemoteAddr() string
	StreamId() string
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

// SRTConnectionHandler handles SRT connections
// This is part of Protocol Layer (Level 1)
// 
// To use with gosrt, wrap gosrt.Conn to implement SRTConnection interface:
//   type gosrtConn struct { *srt.Conn }
//   func (c *gosrtConn) RemoteAddr() string { return c.Conn.RemoteAddr().String() }
//   func (c *gosrtConn) StreamId() string { return c.Conn.StreamId() }
type SRTConnectionHandler struct {
	connectionManager domain.ConnectionManager
	streamManager     domain.StreamManager
	eventBus          core.EventBus
	latency           time.Duration
	passphrase        string
}

// NewSRTConnectionHandler creates a new SRT connection handler
func NewSRTConnectionHandler(
	connectionManager domain.ConnectionManager,
	streamManager domain.StreamManager,
	eventBus core.EventBus,
	latency time.Duration,
	passphrase string,
) *SRTConnectionHandler {
	return &SRTConnectionHandler{
		connectionManager: connectionManager,
		streamManager:     streamManager,
		eventBus:          eventBus,
		latency:           latency,
		passphrase:        passphrase,
	}
}

// HandleConnection handles a new SRT connection
// 
// StreamID format: "app/streamname" or "viewer:app/streamname"
// - Without prefix: Publisher
// - With "viewer:" prefix: Viewer/Subscriber
func (h *SRTConnectionHandler) HandleConnection(conn SRTConnection) error {
	streamID := conn.StreamId()
	connID := conn.RemoteAddr()
	
	log.Printf("[SRT] New connection from %s, streamID=%s", connID, streamID)

	// Parse StreamID
	appName, streamName, isViewer, err := h.parseStreamID(streamID)
	if err != nil {
		return fmt.Errorf("invalid streamID: %w", err)
	}

	// Create connection using Domain Layer
	rtmpConn, err := h.connectionManager.CreateConnection(connID, connID)
	if err != nil {
		return fmt.Errorf("failed to create connection: %w", err)
	}

	rtmpConn.SetAppName(appName)
	rtmpConn.SetStreamName(streamName)

	// Publish connection created event
	if h.eventBus != nil {
		_ = h.eventBus.Publish("srt.connection.created", map[string]interface{}{
			"connection_id": connID,
			"remote_addr":   connID,
			"stream_id":     streamID,
			"app_name":      appName,
			"stream_name":   streamName,
			"is_viewer":     isViewer,
		})
	}

	if isViewer {
		// Handle viewer/subscriber
		return h.handleViewer(conn, rtmpConn, appName, streamName)
	} else {
		// Handle publisher
		return h.handlePublisher(conn, rtmpConn, appName, streamName)
	}
}

// parseStreamID parses StreamID and returns app name, stream name, and viewer flag
// Format: "app/streamname" or "viewer:app/streamname"
func (h *SRTConnectionHandler) parseStreamID(streamID string) (appName, streamName string, isViewer bool, err error) {
	if streamID == "" {
		return "", "", false, fmt.Errorf("streamID cannot be empty")
	}

	// Check if viewer prefix
	if strings.HasPrefix(streamID, "viewer:") {
		isViewer = true
		streamID = strings.TrimPrefix(streamID, "viewer:")
	}

	// Split by "/"
	parts := strings.Split(streamID, "/")
	if len(parts) != 2 {
		return "", "", false, fmt.Errorf("invalid streamID format: expected 'app/streamname', got '%s'", streamID)
	}

	appName = parts[0]
	streamName = parts[1]

	if appName == "" || streamName == "" {
		return "", "", false, fmt.Errorf("app name and stream name cannot be empty")
	}

	return appName, streamName, isViewer, nil
}

// handlePublisher handles a publisher connection
func (h *SRTConnectionHandler) handlePublisher(
	conn SRTConnection,
	rtmpConn *domain.RTMPConnection,
	appName, streamName string,
) error {
	log.Printf("[SRT] Publisher: %s/%s from %s", appName, streamName, rtmpConn.ID)

	// Create or get stream
	stream, err := h.streamManager.GetStreamByAppAndName(appName, streamName)
	if err != nil {
		// Stream doesn't exist, create it
		stream, err = h.streamManager.CreateStream(appName, streamName)
		if err != nil {
			return fmt.Errorf("failed to create stream: %w", err)
		}
	}

	// Start publishing
	err = h.streamManager.PublishStream(stream.ID, rtmpConn)
	if err != nil {
		return fmt.Errorf("failed to publish stream: %w", err)
	}

	rtmpConn.SetState(domain.ConnectionStatePublishing)

	// Publish stream published event
	if h.eventBus != nil {
		_ = h.eventBus.Publish("srt.stream.published", map[string]interface{}{
			"stream_id":    stream.ID,
			"publisher_id": rtmpConn.ID,
			"app_name":     appName,
			"stream_name":  streamName,
		})
	}

	// Read media data from publisher
	buffer := make([]byte, 1316) // SRT max payload size (MTU - headers)
	
	for {
		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		n, err := conn.Read(buffer)
		if err != nil {
			log.Printf("[SRT] Publisher read error for %s: %v", rtmpConn.ID, err)
			break
		}

		if n > 0 {
			// Update last seen
			rtmpConn.UpdateLastSeen()

			// Broadcast to all viewers
			// In real implementation, this would route media to viewer connections
			mediaData := make([]byte, n)
			copy(mediaData, buffer[:n])
			
			if err := h.streamManager.BroadcastMedia(stream.ID, mediaData); err != nil {
				log.Printf("[SRT] Broadcast error: %v", err)
			}

			// Publish media broadcast event
			if h.eventBus != nil {
				viewers := stream.GetViewers()
				_ = h.eventBus.Publish("srt.media.broadcasted", map[string]interface{}{
					"stream_id":    stream.ID,
					"viewer_count": len(viewers),
					"data_size":    n,
				})
			}
		}
	}

	// Stop publishing
	h.streamManager.StopPublishing(stream.ID)
	h.connectionManager.RemoveConnection(rtmpConn.ID)

	log.Printf("[SRT] Publisher disconnected: %s", rtmpConn.ID)
	return nil
}

// handleViewer handles a viewer/subscriber connection
func (h *SRTConnectionHandler) handleViewer(
	conn SRTConnection,
	rtmpConn *domain.RTMPConnection,
	appName, streamName string,
) error {
	log.Printf("[SRT] Viewer: %s/%s from %s", appName, streamName, rtmpConn.ID)

	// Get stream
	stream, err := h.streamManager.GetStreamByAppAndName(appName, streamName)
	if err != nil {
		return fmt.Errorf("stream not found: %s/%s", appName, streamName)
	}

	// Start playing
	err = h.streamManager.PlayStream(stream.ID, rtmpConn)
	if err != nil {
		return fmt.Errorf("failed to play stream: %w", err)
	}

	rtmpConn.SetState(domain.ConnectionStatePlaying)

	// Publish stream played event
	if h.eventBus != nil {
		_ = h.eventBus.Publish("srt.stream.played", map[string]interface{}{
			"stream_id": stream.ID,
			"viewer_id": rtmpConn.ID,
			"app_name":  appName,
			"stream_name": streamName,
		})
	}

	// In real implementation, this would:
	// 1. Receive media from stream's publisher
	// 2. Forward media to this viewer's connection
	// 3. Handle viewer disconnection
	
	// For now, keep connection alive and wait for disconnect
	buffer := make([]byte, 1)
	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, err := conn.Read(buffer)
		if err != nil {
			log.Printf("[SRT] Viewer read error for %s: %v", rtmpConn.ID, err)
			break
		}
		rtmpConn.UpdateLastSeen()
	}

	// Stop playing
	h.streamManager.StopPlaying(stream.ID, rtmpConn.ID)
	h.connectionManager.RemoveConnection(rtmpConn.ID)

	log.Printf("[SRT] Viewer disconnected: %s", rtmpConn.ID)
	return nil
}
