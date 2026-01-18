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

package domain

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StreamManager manages RTMP streams
// This interface belongs to the domain layer (dependency inversion)
type StreamManager interface {
	// CreateStream creates a new stream
	CreateStream(appName, streamName string) (*RTMPStream, error)
	
	// GetStream retrieves a stream by ID
	GetStream(streamID string) (*RTMPStream, error)
	
	// GetStreamByAppAndName retrieves a stream by app name and stream name
	GetStreamByAppAndName(appName, streamName string) (*RTMPStream, error)
	
	// PublishStream starts publishing a stream
	PublishStream(streamID string, publisher *RTMPConnection) error
	
	// PlayStream starts playing a stream
	PlayStream(streamID string, viewer *RTMPConnection) error
	
	// StopPublishing stops publishing a stream
	StopPublishing(streamID string) error
	
	// StopPlaying stops a viewer from playing a stream
	StopPlaying(streamID string, viewerID string) error
	
	// BroadcastMedia broadcasts media data to all viewers of a stream
	BroadcastMedia(streamID string, mediaData []byte) error
	
	// GetAllStreams returns all active streams
	GetAllStreams() []*RTMPStream
}

// InMemoryStreamManager is an in-memory implementation of StreamManager
type InMemoryStreamManager struct {
	streams map[string]*RTMPStream
	mu      sync.RWMutex
}

// NewInMemoryStreamManager creates a new in-memory stream manager
func NewInMemoryStreamManager() *InMemoryStreamManager {
	return &InMemoryStreamManager{
		streams: make(map[string]*RTMPStream),
	}
}

// generateStreamID generates a unique stream ID
func generateStreamID(appName, streamName string) string {
	return fmt.Sprintf("%s/%s", appName, streamName)
}

// CreateStream creates a new stream
func (m *InMemoryStreamManager) CreateStream(appName, streamName string) (*RTMPStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	streamID := generateStreamID(appName, streamName)
	
	// Check if stream already exists
	if _, exists := m.streams[streamID]; exists {
		return nil, ErrStreamAlreadyExists
	}
	
	stream := NewStream(streamID, appName, streamName)
	if err := stream.Validate(); err != nil {
		return nil, err
	}
	
	m.streams[streamID] = stream
	return stream, nil
}

// GetStream retrieves a stream by ID
func (m *InMemoryStreamManager) GetStream(streamID string) (*RTMPStream, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	stream, exists := m.streams[streamID]
	if !exists {
		return nil, ErrStreamNotFound
	}
	
	return stream, nil
}

// GetStreamByAppAndName retrieves a stream by app name and stream name
func (m *InMemoryStreamManager) GetStreamByAppAndName(appName, streamName string) (*RTMPStream, error) {
	streamID := generateStreamID(appName, streamName)
	return m.GetStream(streamID)
}

// PublishStream starts publishing a stream
func (m *InMemoryStreamManager) PublishStream(streamID string, publisher *RTMPConnection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	stream, exists := m.streams[streamID]
	if !exists {
		return ErrStreamNotFound
	}
	
	return stream.SetPublisher(publisher)
}

// PlayStream starts playing a stream
func (m *InMemoryStreamManager) PlayStream(streamID string, viewer *RTMPConnection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	stream, exists := m.streams[streamID]
	if !exists {
		return ErrStreamNotFound
	}
	
	return stream.AddViewer(viewer)
}

// StopPublishing stops publishing a stream
func (m *InMemoryStreamManager) StopPublishing(streamID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	stream, exists := m.streams[streamID]
	if !exists {
		return ErrStreamNotFound
	}
	
	stream.Close()
	delete(m.streams, streamID)
	return nil
}

// StopPlaying stops a viewer from playing a stream
func (m *InMemoryStreamManager) StopPlaying(streamID string, viewerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	stream, exists := m.streams[streamID]
	if !exists {
		return ErrStreamNotFound
	}
	
	stream.RemoveViewer(viewerID)
	return nil
}

// BroadcastMedia broadcasts media data to all viewers of a stream
func (m *InMemoryStreamManager) BroadcastMedia(streamID string, mediaData []byte) error {
	m.mu.RLock()
	stream, exists := m.streams[streamID]
	m.mu.RUnlock()
	
	if !exists {
		return ErrStreamNotFound
	}
	
	if stream.State != StreamStatePublishing {
		return ErrStreamNotPublishing
	}
	
	stream.UpdateLastMedia()
	// Media broadcasting logic will be implemented in protocol layer
	// This just updates the stream state
	
	return nil
}

// BroadcastMediaToViewers broadcasts media data to all viewers of a stream with message type and timestamp
func (m *InMemoryStreamManager) BroadcastMediaToViewers(streamID string, msgType uint8, timestamp uint32, payload []byte) error {
	m.mu.RLock()
	stream, exists := m.streams[streamID]
	m.mu.RUnlock()
	
	if !exists {
		return ErrStreamNotFound
	}
	
	if stream.State != StreamStatePublishing {
		return ErrStreamNotPublishing
	}
	
	stream.UpdateLastMedia()
	
	// Get all viewers and send media to each
	viewers := stream.GetViewers()
	for _, viewer := range viewers {
		if err := viewer.WriteMedia(msgType, timestamp, payload); err != nil {
			// Log error but continue broadcasting to other viewers
			// Don't fail the entire broadcast for one failed viewer
			continue
		}
	}
	
	return nil
}

// GetAllStreams returns all active streams
func (m *InMemoryStreamManager) GetAllStreams() []*RTMPStream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	streams := make([]*RTMPStream, 0, len(m.streams))
	for _, stream := range m.streams {
		streams = append(streams, stream)
	}
	return streams
}

// CleanupStaleStreams removes streams that haven't received media in a while
func (m *InMemoryStreamManager) CleanupStaleStreams(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	for streamID, stream := range m.streams {
		if stream.State == StreamStatePublishing && !stream.LastMediaAt.IsZero() {
			if now.Sub(stream.LastMediaAt) > timeout {
				stream.Close()
				delete(m.streams, streamID)
			}
		}
	}
}

// StartCleanup starts a background goroutine to clean up stale streams
func (m *InMemoryStreamManager) StartCleanup(ctx context.Context, interval, timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.CleanupStaleStreams(timeout)
			}
		}
	}()
}
