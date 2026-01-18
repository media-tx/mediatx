package srt

import (
	"fmt"
	"time"

	grt "github.com/datarhei/gosrt"
)

// Config represents SRT connection configuration
type Config struct {
	StreamId        string
	Latency         time.Duration
	Passphrase      string
	PbKeyLen        int
	MaxBW           int64
	MSS             int
	ConnectTimeout  time.Duration
	SendBufSize     int
	RecvBufSize     int
	TLPKTDROP       bool
	MessageAPI      bool
}

// DefaultConfig returns default SRT configuration
func DefaultConfig() *Config {
	return &Config{
		StreamId:        "",
		Latency:         120 * time.Millisecond,
		Passphrase:      "",
		PbKeyLen:        16,
		MaxBW:           -1, // Unlimited
		MSS:             1500,
		ConnectTimeout:  30 * time.Second,
		SendBufSize:     8 * 1024 * 1024, // 8MB
		RecvBufSize:     8 * 1024 * 1024, // 8MB
		TLPKTDROP:       false,
		MessageAPI:      false,
	}
}

// Server represents an SRT server
type Server interface {
	// Listen starts listening on the specified address
	Listen(network, addr string, config *Config) (Listener, error)
	
	// Dial connects to an SRT server
	Dial(network, addr string, config *Config) (Conn, error)
}

// Listener represents an SRT listener (server side)
type Listener interface {
	// Accept waits for and returns the next connection
	Accept() (Conn, error)
	
	// Close closes the listener
	Close() error
	
	// Addr returns the listener's network address
	Addr() string
}

// Conn represents an SRT connection
type Conn interface {
	// Read reads data from the connection
	Read([]byte) (int, error)
	
	// Write writes data to the connection
	Write([]byte) (int, error)
	
	// Close closes the connection
	Close() error
	
	// RemoteAddr returns the remote network address
	RemoteAddr() string
	
	// StreamId returns the stream ID
	StreamId() string
	
	// SetStreamId sets the stream ID
	SetStreamId(streamId string) error
	
	// SetReadDeadline sets the read deadline
	SetReadDeadline(t time.Time) error
	
	// SetWriteDeadline sets the write deadline
	SetWriteDeadline(t time.Time) error
	
	// SetReadTimeout sets the read timeout
	SetReadTimeout(duration time.Duration) error
	
	// SetWriteTimeout sets the write timeout
	SetWriteTimeout(duration time.Duration) error
}

// SRTServer is a wrapper around gosrt SRT server
// This abstracts the gosrt implementation for easier testing and future changes
type SRTServer struct {
	// For now, this is a placeholder
	// When gosrt is added, wrap srt.Listen and srt.Dial here
}

// NewSRTServer creates a new SRT server wrapper
func NewSRTServer() *SRTServer {
	return &SRTServer{}
}

// Listen starts listening on the specified address
func (s *SRTServer) Listen(network, addr string, config *Config) (Listener, error) {
	// Convert our Config to gosrt.Config
	gosrtConfig := grt.Config{
		StreamId:          config.StreamId,
		Latency:           config.Latency,
		Passphrase:        config.Passphrase,
		PBKeylen:          config.PbKeyLen,
		MaxBW:             config.MaxBW,
		MSS:               uint32(config.MSS),
		ConnectionTimeout: config.ConnectTimeout,
		SendBufferSize:    uint32(config.SendBufSize),
		ReceiverBufferSize: uint32(config.RecvBufSize),
		TooLatePacketDrop: config.TLPKTDROP,
		MessageAPI:        config.MessageAPI,
		// Set defaults required by gosrt
		Congestion:     "live",
		TransmissionType: "live",
		TSBPDMode:      true,
		NAKReport:      true,
		EnforcedEncryption: false,
	}

	// Validate config
	if err := gosrtConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid SRT config: %w", err)
	}

	// Use gosrt.Listen - Note: gosrt.Listen doesn't take AcceptFunc, 
	// AcceptFunc is passed to Accept() instead
	listener, err := grt.Listen(network, addr, gosrtConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// Wrap AcceptFunc to return PUBLISH or SUBSCRIBE based on StreamId
	acceptFunc := func(req grt.ConnRequest) grt.ConnType {
		streamID := req.StreamId()
		
		// Check if it's a viewer (has "viewer:" prefix)
		if len(streamID) > 7 && streamID[:7] == "viewer:" {
			return grt.SUBSCRIBE
		}
		// Otherwise, treat as publisher
		return grt.PUBLISH
	}

	return newSRTListener(listener, addr, acceptFunc), nil
}

// Dial connects to an SRT server
func (s *SRTServer) Dial(network, addr string, config *Config) (Conn, error) {
	// Convert our Config to gosrt.Config
	gosrtConfig := grt.Config{
		StreamId:          config.StreamId,
		Latency:           config.Latency,
		Passphrase:        config.Passphrase,
		PBKeylen:          config.PbKeyLen,
		MaxBW:             config.MaxBW,
		MSS:               uint32(config.MSS),
		ConnectionTimeout: config.ConnectTimeout,
		SendBufferSize:    uint32(config.SendBufSize),
		ReceiverBufferSize: uint32(config.RecvBufSize),
		TooLatePacketDrop: config.TLPKTDROP,
		MessageAPI:        config.MessageAPI,
		// Set defaults required by gosrt
		Congestion:       "live",
		TransmissionType: "live",
		TSBPDMode:        true,
		NAKReport:        true,
		EnforcedEncryption: false,
	}

	// Validate config
	if err := gosrtConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid SRT config: %w", err)
	}

	// Use gosrt.Dial
	conn, err := grt.Dial(network, addr, gosrtConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	return newSRTConn(conn), nil
}
