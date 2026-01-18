package handlers

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"sync"
)

// ConnectionState tracks RTMP connection state (window ack size, chunk size, etc.)
// This is part of Protocol Layer (Level 1)
type ConnectionState struct {
	mu sync.RWMutex

	// Chunk size (from SetChunkSize message)
	chunkSize uint32

	// Window acknowledgment size (from WindowAckSize message)
	windowAckSize uint32

	// Peer bandwidth (from SetPeerBandwidth message)
	peerBandwidth uint32
	peerBandwidthLimitType uint8 // 0=hard, 1=soft, 2=dynamic

	// Acknowledgment tracking
	bytesReceived uint32
	lastAckBytes  uint32
	lastAckTime   int64

	// Chunk stream state (reused from ChunkParser)
	lastTimestamp      map[uint32]uint32
	lastMessageHeader  map[uint32]*MessageHeader
	partialMessages    map[uint32]*MessageBuilder
	
	// New fields for ProcessChunk approach (alternative to ReconstructMessage)
	// Note: Both approaches share lastMessageHeader and lastTimestamp for compatibility
	// You can use either approach, but not both on the same connection simultaneously
	messageBuilders map[uint32][]byte // Payload builders per chunk stream ID (ProcessChunk only)
	receivedBytes   map[uint32]uint32  // Received bytes per chunk stream ID (ProcessChunk only)
	previousHeaders map[uint32]*MessageHeader // Previous headers per chunk stream ID (ProcessChunk only)
	currentChunkSize uint32 // Current chunk size for reading (ProcessChunk only)
}

// NewConnectionState creates a new connection state
func NewConnectionState() *ConnectionState {
	return &ConnectionState{
		chunkSize:         DefaultChunkSize,
		windowAckSize:     2500000, // Default 2.5MB
		peerBandwidth:     2500000, // Default 2.5MB
		lastTimestamp:     make(map[uint32]uint32),
		lastMessageHeader: make(map[uint32]*MessageHeader),
		partialMessages:   make(map[uint32]*MessageBuilder),
		messageBuilders:   make(map[uint32][]byte),
		receivedBytes:     make(map[uint32]uint32),
		previousHeaders:   make(map[uint32]*MessageHeader),
		currentChunkSize:  DefaultChunkSize,
	}
}

// SetChunkSize sets the chunk size
func (s *ConnectionState) SetChunkSize(size uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if size > 0 && size <= MaxChunkSize {
		s.chunkSize = size
		s.currentChunkSize = size // Also update current chunk size for ProcessChunk
	}
}

// GetChunkSize returns the current chunk size
func (s *ConnectionState) GetChunkSize() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chunkSize
}

// SetWindowAckSize sets the window acknowledgment size
func (s *ConnectionState) SetWindowAckSize(size uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windowAckSize = size
}

// GetWindowAckSize returns the window acknowledgment size
func (s *ConnectionState) GetWindowAckSize() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.windowAckSize
}

// SetPeerBandwidth sets the peer bandwidth
func (s *ConnectionState) SetPeerBandwidth(bandwidth uint32, limitType uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerBandwidth = bandwidth
	s.peerBandwidthLimitType = limitType
}

// GetPeerBandwidth returns the peer bandwidth
func (s *ConnectionState) GetPeerBandwidth() (uint32, uint8) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.peerBandwidth, s.peerBandwidthLimitType
}

// UpdateBytesReceived updates the bytes received count and checks if acknowledgment is needed
func (s *ConnectionState) UpdateBytesReceived(bytes uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bytesReceived += bytes
	
	// Check if we need to send acknowledgment
	if s.windowAckSize > 0 {
		bytesSinceAck := s.bytesReceived - s.lastAckBytes
		if bytesSinceAck >= s.windowAckSize {
			s.lastAckBytes = s.bytesReceived
			return true // Need to send ack
		}
	}
	return false
}

// ReconstructMessage reconstructs a complete message from a chunk
// This is the traditional approach: use ChunkParser.ReadChunk() first, then call this method
// Returns the complete message if ready, nil if still building
// 
// Usage:
//   chunk, err := chunkParser.ReadChunk(reader, state.GetRemainingBytes)
//   message, complete := state.ReconstructMessage(chunk)
func (s *ConnectionState) ReconstructMessage(chunk *Chunk) (*Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	csID := chunk.BasicHeader.ChunkStreamID

	var builder *MessageBuilder
	var exists bool

	log.Printf("[ConnectionState] ReconstructMessage: format=%d, csid=%d, hasHeader=%v, payload=%d bytes", 
		chunk.BasicHeader.Format, csID, chunk.MessageHeader != nil, len(chunk.Payload))

	// If this is the first chunk of a message
	// Format 0: always new message (full header)
	// Format 1, 2: new message if MessageHeader != nil (has new header data)
	// Format 3: always continuation (reuses all header, never new message)
	isFirstChunk := chunk.BasicHeader.Format != 3 && chunk.MessageHeader != nil
	
	if isFirstChunk {
		// Format 0, 1, or 2 with new message header - this is a new message
		// Update last message header
		s.lastMessageHeader[csID] = chunk.MessageHeader

		// Calculate timestamp
		timestamp := chunk.MessageHeader.Timestamp
		if chunk.ExtendedTimestamp != nil {
			timestamp = *chunk.ExtendedTimestamp
		} else if timestamp == MaxTimestamp {
			// Should have extended timestamp but doesn't - this is an error
			return nil, false
		}
		s.lastTimestamp[csID] = timestamp

		// Create new message builder
		builder = &MessageBuilder{
			ChunkStreamID:  csID,
			Timestamp:      timestamp,
			MessageLength:  chunk.MessageHeader.MessageLength,
			MessageType:    chunk.MessageHeader.MessageType,
			MessageStreamID: chunk.MessageHeader.MessageStreamID,
			Payload:        make([]byte, 0, chunk.MessageHeader.MessageLength),
			ReceivedLength: 0,
		}
		s.partialMessages[csID] = builder
	} else {
		// Use last message header (format 1, 2, or 3)
		lastHeader, headerExists := s.lastMessageHeader[csID]
		if !headerExists {
			// No previous header - cannot reconstruct
			return nil, false
		}

		// Get or create builder
		builder, exists = s.partialMessages[csID]
		if !exists {
			// No existing builder - create one using last header
			timestamp := s.lastTimestamp[csID]
			builder = &MessageBuilder{
				ChunkStreamID:  csID,
				Timestamp:      timestamp,
				MessageLength:  lastHeader.MessageLength,
				MessageType:    lastHeader.MessageType,
				MessageStreamID: lastHeader.MessageStreamID,
				Payload:        make([]byte, 0, lastHeader.MessageLength),
				ReceivedLength: 0,
			}
			s.partialMessages[csID] = builder
		}

		// Update timestamp for format 1 or 2
		if chunk.MessageHeader != nil && chunk.MessageHeader.TimestampDelta > 0 {
			timestamp := s.lastTimestamp[csID] + chunk.MessageHeader.TimestampDelta
			if chunk.ExtendedTimestamp != nil {
				timestamp = *chunk.ExtendedTimestamp
			}
			s.lastTimestamp[csID] = timestamp
			builder.Timestamp = timestamp
		}
	}
	
	// Append payload
	builder.Payload = append(builder.Payload, chunk.Payload...)
	builder.ReceivedLength += uint32(len(chunk.Payload))
	
	log.Printf("[ConnectionState] Builder state: received=%d/%d bytes, type=0x%02x, csid=%d", 
		builder.ReceivedLength, builder.MessageLength, builder.MessageType, builder.ChunkStreamID)

	// Check if message is complete
	if builder.ReceivedLength >= builder.MessageLength {
		// Message is complete
		message := &Message{
			ChunkStreamID:  builder.ChunkStreamID,
			Timestamp:      builder.Timestamp,
			MessageLength:  builder.MessageLength,
			MessageType:    builder.MessageType,
			MessageStreamID: builder.MessageStreamID,
			Payload:        builder.Payload[:builder.MessageLength],
		}

		// Remove builder (message complete)
		delete(s.partialMessages, csID)

		log.Printf("[ConnectionState] Message complete: type=0x%02x, length=%d, csid=%d", 
			message.MessageType, message.MessageLength, message.ChunkStreamID)
		return message, true
	}

	// Message not complete yet
	log.Printf("[ConnectionState] Message incomplete: need %d more bytes", 
		builder.MessageLength - builder.ReceivedLength)
	return nil, false
}

// GetRemainingBytes returns the remaining bytes needed for a partial message
// Returns (remaining, total, found) - found is false if no partial message exists
func (s *ConnectionState) GetRemainingBytes(chunkStreamID uint32) (uint32, uint32, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if builder, exists := s.partialMessages[chunkStreamID]; exists {
		remaining := builder.MessageLength - builder.ReceivedLength
		return remaining, builder.MessageLength, true
	}
	return 0, 0, false
}

// Reset resets the connection state (for new connections)
func (s *ConnectionState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bytesReceived = 0
	s.lastAckBytes = 0
	s.lastTimestamp = make(map[uint32]uint32)
	s.lastMessageHeader = make(map[uint32]*MessageHeader)
	s.partialMessages = make(map[uint32]*MessageBuilder)
	s.messageBuilders = make(map[uint32][]byte)
	s.receivedBytes = make(map[uint32]uint32)
	s.previousHeaders = make(map[uint32]*MessageHeader)
	s.currentChunkSize = DefaultChunkSize
}

// ProcessChunk processes a chunk directly from reader and handles message reconstruction
// This is an alternative approach that combines chunk reading and message reconstruction in one step
// handleFullMessage is a callback that will be called when a complete message is ready
// 
// Usage:
//   err := state.ProcessChunk(reader, func(message *Message) error {
//       // Handle complete message here
//       return processMessage(message)
//   })
// 
// Note: Both ProcessChunk and ReconstructMessage share the same state (lastMessageHeader, etc.),
// so you can use either approach, but not both on the same connection at the same time.
func (s *ConnectionState) ProcessChunk(r io.Reader, handleFullMessage func(*Message) error) error {
	// Đọc basic header trước (không cần lock vì chỉ đọc từ reader)
	fmt, csid, err := s.readBasicHeader(r)
	if err != nil {
		log.Printf("[ProcessChunk] Failed to read basic header: %v", err)
		return err
	}
	log.Printf("[ProcessChunk] Read basic header: format=%d, csid=%d", fmt, csid)

	// Lock để đọc state và update
	s.mu.Lock()
	var msgHeader *MessageHeader
	if fmt != 3 {
		// Full header: cần đọc previous state để tính timestamp cho format 1,2
		lastHeader := s.lastMessageHeader[csid]
		lastTimestamp := s.lastTimestamp[csid]
		s.mu.Unlock()
		
		// Đọc message header (không lock trong I/O)
		msgHeader, err = s.readMessageHeaderUnlocked(r, fmt, csid, lastHeader, lastTimestamp)
		if err != nil {
			log.Printf("[ProcessChunk] Failed to read message header (fmt=%d, csid=%d): %v", fmt, csid, err)
			return err
		}
		log.Printf("[ProcessChunk] Read message header: type=0x%02x, length=%d, csid=%d", msgHeader.MessageType, msgHeader.MessageLength, csid)
		
		// Extended timestamp nếu cần
		if msgHeader.Timestamp >= MaxTimestamp {
			extTs := s.readExtendedTimestamp(r)
			msgHeader.Timestamp = extTs
		}
		
		// Lock lại để update state
		s.mu.Lock()
		s.lastTimestamp[csid] = msgHeader.Timestamp
		s.lastMessageHeader[csid] = msgHeader // Also update for ReconstructMessage compatibility
		s.previousHeaders[csid] = msgHeader
		s.receivedBytes[csid] = 0
		s.messageBuilders[csid] = make([]byte, 0, msgHeader.MessageLength)
		s.mu.Unlock()
	} else {
		// Continuation: lấy header cũ
		msgHeader = s.previousHeaders[csid]
		if msgHeader == nil {
			s.mu.Unlock()
			return errors.New("format=3 without previous header")
		}
		
		// Extended timestamp nếu cần
		needsExtTs := msgHeader.Timestamp >= MaxTimestamp
		s.mu.Unlock()
		
		if needsExtTs {
			extTs := s.readExtendedTimestamp(r)
			s.mu.Lock()
			msgHeader.Timestamp = extTs
			s.lastTimestamp[csid] = extTs
			s.mu.Unlock()
		}
	}

	// Tính remaining & payload size (cần lock để đọc state)
	s.mu.Lock()
	remaining := msgHeader.MessageLength - s.receivedBytes[csid]
	chunkSize := s.currentChunkSize
	if s.chunkSize > 0 {
		chunkSize = s.chunkSize
	}
	payloadSize := remaining
	if payloadSize > chunkSize {
		payloadSize = chunkSize
	}
	s.mu.Unlock()

	// Đọc payload (không lock trong I/O)
	payload := make([]byte, payloadSize)
	_, err = io.ReadFull(r, payload)
	if err != nil {
		log.Printf("[ProcessChunk] Failed to read payload (expected %d bytes): %v", payloadSize, err)
		return err
	}
	log.Printf("[ProcessChunk] Read payload: %d bytes, remaining=%d/%d, csid=%d", payloadSize, remaining-payloadSize, msgHeader.MessageLength, csid)

	// Lock để update state
	s.mu.Lock()
	s.messageBuilders[csid] = append(s.messageBuilders[csid], payload...)
	s.receivedBytes[csid] += uint32(payloadSize)
	receivedBytes := s.receivedBytes[csid]
	messageLength := msgHeader.MessageLength

	// Check complete
	if receivedBytes >= messageLength {
		fullPayload := s.messageBuilders[csid]
		// Reset builder
		s.messageBuilders[csid] = nil
		s.receivedBytes[csid] = 0
		s.mu.Unlock()
		
		log.Printf("[ProcessChunk] Message complete: type=0x%02x, length=%d, csid=%d", msgHeader.MessageType, messageLength, csid)
		
		// Create complete message
		message := &Message{
			ChunkStreamID:  csid,
			Timestamp:      msgHeader.Timestamp,
			MessageLength:  messageLength,
			MessageType:    msgHeader.MessageType,
			MessageStreamID: msgHeader.MessageStreamID,
			Payload:        fullPayload[:messageLength],
		}
		
		// Dispatch full message (không lock khi gọi callback)
		if handleFullMessage != nil {
			log.Printf("[ProcessChunk] Calling handleFullMessage callback for message type=0x%02x", message.MessageType)
			return handleFullMessage(message)
		}
	} else {
		s.mu.Unlock()
		log.Printf("[ProcessChunk] Message incomplete: received=%d/%d bytes, csid=%d", receivedBytes, messageLength, csid)
	}

	return nil
}

// readMessageHeaderUnlocked reads message header without locking (caller must handle locking)
// lastHeader and lastTimestamp are passed in to avoid accessing state without lock
func (s *ConnectionState) readMessageHeaderUnlocked(r io.Reader, format uint8, csid uint32, lastHeader *MessageHeader, lastTimestamp uint32) (*MessageHeader, error) {
	var header *MessageHeader

	switch format {
	case 0:
		// Type 0: 11 bytes (full header)
		data := make([]byte, 11)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}

		timestamp := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
		messageLength := uint32(data[3])<<16 | uint32(data[4])<<8 | uint32(data[5])
		messageType := data[6]
		messageStreamID := binary.LittleEndian.Uint32(data[7:11])

		header = &MessageHeader{
			Timestamp:       timestamp,
			MessageLength:   messageLength,
			MessageType:     messageType,
			MessageStreamID: messageStreamID,
		}

	case 1:
		// Type 1: 7 bytes (reuse stream ID)
		if lastHeader == nil {
			return nil, errors.New("type 1 header requires previous header")
		}

		data := make([]byte, 7)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}

		timestampDelta := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
		messageLength := uint32(data[3])<<16 | uint32(data[4])<<8 | uint32(data[5])
		messageType := data[6]

		timestamp := lastTimestamp + timestampDelta
		if timestampDelta >= MaxTimestamp {
			timestampDelta = MaxTimestamp // Indicates extended timestamp
		}

		header = &MessageHeader{
			Timestamp:       timestamp,
			TimestampDelta:  timestampDelta,
			MessageLength:   messageLength,
			MessageType:     messageType,
			MessageStreamID: lastHeader.MessageStreamID,
		}

	case 2:
		// Type 2: 3 bytes (only timestamp delta)
		if lastHeader == nil {
			return nil, errors.New("type 2 header requires previous header")
		}

		data := make([]byte, 3)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}

		timestampDelta := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
		timestamp := lastTimestamp + timestampDelta
		if timestampDelta >= MaxTimestamp {
			timestampDelta = MaxTimestamp // Indicates extended timestamp
		}

		header = &MessageHeader{
			Timestamp:       timestamp,
			TimestampDelta:  timestampDelta,
			MessageLength:   lastHeader.MessageLength,
			MessageType:     lastHeader.MessageType,
			MessageStreamID: lastHeader.MessageStreamID,
		}

		default:
			return nil, errors.New("unsupported format")
	}

	return header, nil
}

// readBasicHeader reads chunk basic header (1-3 bytes)
func (s *ConnectionState) readBasicHeader(r io.Reader) (uint8, uint32, error) {
	firstByte := make([]byte, 1)
	if _, err := r.Read(firstByte); err != nil {
		return 0, 0, err
	}

	format := (firstByte[0] >> 6) & 0x03
	chunkStreamID := uint32(firstByte[0] & 0x3F)

	// Read additional bytes for chunk stream ID if needed
	if chunkStreamID == 0 {
		// Format 1: 2 bytes total
		secondByte := make([]byte, 1)
		if _, err := r.Read(secondByte); err != nil {
			return 0, 0, err
		}
		chunkStreamID = uint32(secondByte[0]) + 64
	} else if chunkStreamID == 1 {
		// Format 2: 3 bytes total
		bytes := make([]byte, 2)
		if _, err := r.Read(bytes); err != nil {
			return 0, 0, err
		}
		chunkStreamID = uint32(bytes[0])*256 + uint32(bytes[1]) + 64
	}

	return format, chunkStreamID, nil
}

// readMessageHeader reads chunk message header based on format
func (s *ConnectionState) readMessageHeader(r io.Reader, format uint8, csid uint32) (*MessageHeader, error) {
	var header *MessageHeader

	switch format {
	case 0:
		// Type 0: 11 bytes (full header)
		data := make([]byte, 11)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}

		timestamp := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
		messageLength := uint32(data[3])<<16 | uint32(data[4])<<8 | uint32(data[5])
		messageType := data[6]
		messageStreamID := binary.LittleEndian.Uint32(data[7:11])

		header = &MessageHeader{
			Timestamp:       timestamp,
			MessageLength:   messageLength,
			MessageType:     messageType,
			MessageStreamID: messageStreamID,
		}
		s.lastMessageHeader[csid] = header
		s.lastTimestamp[csid] = timestamp

	case 1:
		// Type 1: 7 bytes (reuse stream ID)
		lastHeader := s.lastMessageHeader[csid]
		if lastHeader == nil {
			return nil, errors.New("type 1 header requires previous header")
		}

		data := make([]byte, 7)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}

		timestampDelta := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
		messageLength := uint32(data[3])<<16 | uint32(data[4])<<8 | uint32(data[5])
		messageType := data[6]

		timestamp := s.lastTimestamp[csid] + timestampDelta
		if timestampDelta >= MaxTimestamp {
			timestampDelta = MaxTimestamp // Indicates extended timestamp
		}

		header = &MessageHeader{
			Timestamp:       timestamp,
			TimestampDelta:  timestampDelta,
			MessageLength:   messageLength,
			MessageType:     messageType,
			MessageStreamID: lastHeader.MessageStreamID,
		}
		s.lastMessageHeader[csid] = header
		s.lastTimestamp[csid] = timestamp

	case 2:
		// Type 2: 3 bytes (only timestamp delta)
		lastHeader := s.lastMessageHeader[csid]
		if lastHeader == nil {
			return nil, errors.New("type 2 header requires previous header")
		}

		data := make([]byte, 3)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}

		timestampDelta := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])

		timestamp := s.lastTimestamp[csid] + timestampDelta
		if timestampDelta >= MaxTimestamp {
			timestampDelta = MaxTimestamp // Indicates extended timestamp
		}

		header = &MessageHeader{
			Timestamp:       timestamp,
			TimestampDelta:  timestampDelta,
			MessageLength:   lastHeader.MessageLength,
			MessageType:     lastHeader.MessageType,
			MessageStreamID: lastHeader.MessageStreamID,
		}
		s.lastMessageHeader[csid] = header
		s.lastTimestamp[csid] = timestamp

	case 3:
		// Type 3: 0 bytes (reuse all previous header)
		lastHeader := s.lastMessageHeader[csid]
		if lastHeader == nil {
			return nil, errors.New("type 3 header requires previous header")
		}
		header = lastHeader
	}

	return header, nil
}

// readExtendedTimestamp reads extended timestamp (4 bytes)
func (s *ConnectionState) readExtendedTimestamp(r io.Reader) uint32 {
	data := make([]byte, 4)
	if _, err := io.ReadFull(r, data); err != nil {
		return 0
	}
	return binary.BigEndian.Uint32(data)
}
