package domain

import (
	"sync"
	"time"
)

// StreamState represents the state of an RTMP stream
type StreamState string

const (
	StreamStateIdle       StreamState = "idle"
	StreamStatePublishing StreamState = "publishing"
	StreamStatePlaying    StreamState = "playing"
	StreamStateClosed     StreamState = "closed"
)

// RTMPStream represents a published RTMP stream
// This is a domain entity with business rules
type RTMPStream struct {
	ID          string
	AppName     string
	StreamName  string
	Publisher   *RTMPConnection
	Viewers     map[string]*RTMPConnection
	State       StreamState
	CreatedAt   time.Time
	PublishedAt time.Time
	LastMediaAt time.Time
	
	// Cached codec/metadata for new viewers
	Metadata           []byte // onMetaData from @setDataFrame
	VideoSequenceHeader []byte // AVC/HEVC sequence header (SPS/PPS)
	AudioSequenceHeader []byte // AAC sequence header
	
	mu sync.RWMutex
}

// NewStream creates a new RTMP stream
func NewStream(id, appName, streamName string) *RTMPStream {
	now := time.Now()
	return &RTMPStream{
		ID:          id,
		AppName:     appName,
		StreamName:  streamName,
		Viewers:     make(map[string]*RTMPConnection),
		State:       StreamStateIdle,
		CreatedAt:   now,
	}
}

// CanPublish returns true if the stream can be published
func (s *RTMPStream) CanPublish() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == StreamStateIdle
}

// CanPlay returns true if the stream can be played
func (s *RTMPStream) CanPlay() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == StreamStatePublishing
}

// SetPublisher sets the publisher for the stream
func (s *RTMPStream) SetPublisher(publisher *RTMPConnection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.State != StreamStateIdle {
		return ErrStreamAlreadyPublishing
	}
	
	s.Publisher = publisher
	s.State = StreamStatePublishing
	s.PublishedAt = time.Now()
	return nil
}

// AddViewer adds a viewer to the stream
func (s *RTMPStream) AddViewer(viewer *RTMPConnection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.State != StreamStatePublishing {
		return ErrStreamNotPublishing
	}
	
	s.Viewers[viewer.ID] = viewer
	return nil
}

// RemoveViewer removes a viewer from the stream
func (s *RTMPStream) RemoveViewer(viewerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Viewers, viewerID)
}

// GetViewers returns a copy of all viewers
func (s *RTMPStream) GetViewers() []*RTMPConnection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	viewers := make([]*RTMPConnection, 0, len(s.Viewers))
	for _, viewer := range s.Viewers {
		viewers = append(viewers, viewer)
	}
	return viewers
}

// UpdateLastMedia updates the last media timestamp
func (s *RTMPStream) UpdateLastMedia() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastMediaAt = time.Now()
}

// SetMetadata sets the cached metadata
func (s *RTMPStream) SetMetadata(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Metadata = make([]byte, len(data))
	copy(s.Metadata, data)
}

// GetMetadata returns the cached metadata
func (s *RTMPStream) GetMetadata() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil
	}
	result := make([]byte, len(s.Metadata))
	copy(result, s.Metadata)
	return result
}

// SetVideoSequenceHeader sets the cached video sequence header
func (s *RTMPStream) SetVideoSequenceHeader(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.VideoSequenceHeader = make([]byte, len(data))
	copy(s.VideoSequenceHeader, data)
}

// GetVideoSequenceHeader returns the cached video sequence header
func (s *RTMPStream) GetVideoSequenceHeader() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.VideoSequenceHeader == nil {
		return nil
	}
	result := make([]byte, len(s.VideoSequenceHeader))
	copy(result, s.VideoSequenceHeader)
	return result
}

// SetAudioSequenceHeader sets the cached audio sequence header
func (s *RTMPStream) SetAudioSequenceHeader(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AudioSequenceHeader = make([]byte, len(data))
	copy(s.AudioSequenceHeader, data)
}

// GetAudioSequenceHeader returns the cached audio sequence header
func (s *RTMPStream) GetAudioSequenceHeader() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.AudioSequenceHeader == nil {
		return nil
	}
	result := make([]byte, len(s.AudioSequenceHeader))
	copy(result, s.AudioSequenceHeader)
	return result
}

// Close closes the stream
func (s *RTMPStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = StreamStateClosed
	s.Publisher = nil
	s.Viewers = make(map[string]*RTMPConnection)
}

// Validate validates the stream entity
func (s *RTMPStream) Validate() error {
	if s.ID == "" {
		return &DomainError{
			Code:    "INVALID_STREAM_ID",
			Message: "Stream ID cannot be empty",
		}
	}
	if s.AppName == "" {
		return ErrInvalidAppName
	}
	if s.StreamName == "" {
		return ErrInvalidStreamName
	}
	return nil
}
