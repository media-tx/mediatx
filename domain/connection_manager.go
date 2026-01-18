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
	"sync"
	"time"
)

// ConnectionManager manages RTMP connections
// This interface belongs to the domain layer (dependency inversion)
type ConnectionManager interface {
	// CreateConnection creates a new connection
	CreateConnection(id, remoteAddr string) (*RTMPConnection, error)
	
	// GetConnection retrieves a connection by ID
	GetConnection(id string) (*RTMPConnection, error)
	
	// RemoveConnection removes a connection
	RemoveConnection(id string)
	
	// GetAllConnections returns all active connections
	GetAllConnections() []*RTMPConnection
}

// InMemoryConnectionManager is an in-memory implementation of ConnectionManager
type InMemoryConnectionManager struct {
	connections map[string]*RTMPConnection
	mu          sync.RWMutex
}

// NewInMemoryConnectionManager creates a new in-memory connection manager
func NewInMemoryConnectionManager() *InMemoryConnectionManager {
	return &InMemoryConnectionManager{
		connections: make(map[string]*RTMPConnection),
	}
}

// CreateConnection creates a new connection
func (m *InMemoryConnectionManager) CreateConnection(id, remoteAddr string) (*RTMPConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	conn := NewConnection(id, remoteAddr)
	if err := conn.Validate(); err != nil {
		return nil, err
	}
	
	m.connections[id] = conn
	return conn, nil
}

// GetConnection retrieves a connection by ID
func (m *InMemoryConnectionManager) GetConnection(id string) (*RTMPConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	conn, exists := m.connections[id]
	if !exists {
		return nil, ErrConnectionNotFound
	}
	
	return conn, nil
}

// RemoveConnection removes a connection
func (m *InMemoryConnectionManager) RemoveConnection(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if conn, exists := m.connections[id]; exists {
		conn.SetState(ConnectionStateDisconnected)
		delete(m.connections, id)
	}
}

// GetAllConnections returns all active connections
func (m *InMemoryConnectionManager) GetAllConnections() []*RTMPConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	connections := make([]*RTMPConnection, 0, len(m.connections))
	for _, conn := range m.connections {
		connections = append(connections, conn)
	}
	return connections
}

// CleanupStaleConnections removes connections that haven't been seen in a while
func (m *InMemoryConnectionManager) CleanupStaleConnections(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	for id, conn := range m.connections {
		if now.Sub(conn.LastSeen) > timeout {
			conn.SetState(ConnectionStateDisconnected)
			delete(m.connections, id)
		}
	}
}

// StartCleanup starts a background goroutine to clean up stale connections
func (m *InMemoryConnectionManager) StartCleanup(ctx context.Context, interval, timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.CleanupStaleConnections(timeout)
			}
		}
	}()
}
