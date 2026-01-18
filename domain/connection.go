package domain

import (
	"sync"
	"time"
)

// MediaWriter is a callback interface for writing media data to a connection
type MediaWriter interface {
	WriteMediaChunk(msgType uint8, timestamp uint32, payload []byte) error
}

// ConnectionState represents the state of an RTMP connection
type ConnectionState string

const (
	ConnectionStateHandshaking   ConnectionState = "handshaking"
	ConnectionStateConnected     ConnectionState = "connected"
	ConnectionStatePublishing    ConnectionState = "publishing"
	ConnectionStatePlaying       ConnectionState = "playing"
	ConnectionStateDisconnected  ConnectionState = "disconnected"
)

// RTMPConnection represents an RTMP client connection
// This is a domain entity with business rules
type RTMPConnection struct {
	ID          string
	RemoteAddr  string
	AppName     string
	StreamName  string
	State       ConnectionState
	ConnectedAt time.Time
	LastSeen    time.Time
	
	// MediaWriter for forwarding media data to this connection (used for viewers)
	mediaWriter MediaWriter
	
	// Viewer state: tracks if viewer has received initial keyframe
	// This is important for proper GOP (Group of Pictures) handling
	hasReceivedKeyframe bool
	
	mu sync.RWMutex
}

// NewConnection creates a new RTMP connection
func NewConnection(id, remoteAddr string) *RTMPConnection {
	now := time.Now()
	return &RTMPConnection{
		ID:          id,
		RemoteAddr:  remoteAddr,
		State:       ConnectionStateHandshaking,
		ConnectedAt: now,
		LastSeen:    now,
	}
}

// SetAppName sets the application name for the connection
func (c *RTMPConnection) SetAppName(appName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AppName = appName
}

// SetStreamName sets the stream name for the connection
func (c *RTMPConnection) SetStreamName(streamName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.StreamName = streamName
}

// SetState sets the connection state
func (c *RTMPConnection) SetState(state ConnectionState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.State = state
	c.LastSeen = time.Now()
}

// GetState returns the current connection state
func (c *RTMPConnection) GetState() ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.State
}

// UpdateLastSeen updates the last seen timestamp
func (c *RTMPConnection) UpdateLastSeen() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastSeen = time.Now()
}

// CanPublish returns true if the connection can publish
func (c *RTMPConnection) CanPublish() bool {
	state := c.GetState()
	return state == ConnectionStateConnected || state == ConnectionStateHandshaking
}

// CanPlay returns true if the connection can play
func (c *RTMPConnection) CanPlay() bool {
	state := c.GetState()
	return state == ConnectionStateConnected || state == ConnectionStateHandshaking
}

// IsConnected returns true if the connection is in a connected state
func (c *RTMPConnection) IsConnected() bool {
	state := c.GetState()
	return state != ConnectionStateDisconnected && state != ConnectionStateHandshaking
}

// Validate validates the connection entity
func (c *RTMPConnection) Validate() error {
	if c.ID == "" {
		return &DomainError{
			Code:    "INVALID_CONNECTION_ID",
			Message: "Connection ID cannot be empty",
		}
	}
	if c.RemoteAddr == "" {
		return &DomainError{
			Code:    "INVALID_REMOTE_ADDR",
			Message: "Remote address cannot be empty",
		}
	}
	return nil
}

// SetMediaWriter sets the media writer for this connection
func (c *RTMPConnection) SetMediaWriter(writer MediaWriter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mediaWriter = writer
}

// GetMediaWriter returns the media writer for this connection
func (c *RTMPConnection) GetMediaWriter() MediaWriter {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mediaWriter
}

// WriteMedia writes media data to this connection (if viewer)
func (c *RTMPConnection) WriteMedia(msgType uint8, timestamp uint32, payload []byte) error {
	c.mu.RLock()
	writer := c.mediaWriter
	c.mu.RUnlock()
	
	if writer == nil {
		return nil // No writer set, skip silently
	}
	
	return writer.WriteMediaChunk(msgType, timestamp, payload)
}

// WriteData writes AMF data to this connection (metadata, etc.)
func (c *RTMPConnection) WriteData(msgType uint8, timestamp uint32, payload []byte) error {
	return c.WriteMedia(msgType, timestamp, payload)
}

// HasReceivedKeyframe returns true if viewer has received initial keyframe
func (c *RTMPConnection) HasReceivedKeyframe() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hasReceivedKeyframe
}

// SetHasReceivedKeyframe sets whether viewer has received keyframe
func (c *RTMPConnection) SetHasReceivedKeyframe(received bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hasReceivedKeyframe = received
}
