package handlers

import (
	"encoding/binary"
	"io"
	"sync"
)

// ChunkWriter writes RTMP chunks to a connection
type ChunkWriter struct {
	writer     io.Writer
	chunkSize  uint32
	lastChunkStreamID map[uint32]uint32 // Last chunk stream ID used per message stream
}

// SetChunkSize updates the chunk size for writing
func (w *ChunkWriter) SetChunkSize(size uint32) {
	if size > 0 && size <= MaxChunkSize {
		w.chunkSize = size
	}
}

// NewChunkWriter creates a new chunk writer
func NewChunkWriter(writer io.Writer, chunkSize uint32) *ChunkWriter {
	return &ChunkWriter{
		writer:    writer,
		chunkSize: chunkSize,
		lastChunkStreamID: make(map[uint32]uint32),
	}
}

// WriteChunk writes a chunk to the connection
func (w *ChunkWriter) WriteChunk(chunk *Chunk) error {
	// Write basic header (1-3 bytes)
	if err := w.writeBasicHeader(chunk.BasicHeader); err != nil {
		return err
	}

	// Write message header based on format
	if err := w.writeMessageHeader(chunk.BasicHeader.Format, chunk.MessageHeader, chunk.ExtendedTimestamp); err != nil {
		return err
	}

	// Write payload
	if _, err := w.writer.Write(chunk.Payload); err != nil {
		return err
	}

	return nil
}

// WriteMessage writes a complete message as one or more chunks
func (w *ChunkWriter) WriteMessage(message *Message, chunkStreamID uint32) error {
	chunkSize := w.chunkSize
	if chunkSize == 0 {
		chunkSize = DefaultChunkSize
	}

	messageLength := uint32(len(message.Payload))
	offset := uint32(0)
	format := uint8(0) // Start with format 0 for first chunk

	for offset < messageLength {
		// Calculate payload size for this chunk
		remaining := messageLength - offset
		payloadSize := chunkSize
		if remaining < chunkSize {
			payloadSize = remaining
		}

		// Create chunk
		chunk := &Chunk{
			BasicHeader: &BasicHeader{
				Format:        format,
				ChunkStreamID: chunkStreamID,
				HeaderLength:  w.getBasicHeaderLength(chunkStreamID),
			},
			MessageHeader: &MessageHeader{
				Timestamp:       message.Timestamp,
				MessageLength:   messageLength,
				MessageType:     message.MessageType,
				MessageStreamID: message.MessageStreamID,
			},
			Payload: message.Payload[offset : offset+payloadSize],
		}

		// Handle extended timestamp if needed
		if message.Timestamp >= MaxTimestamp {
			chunk.ExtendedTimestamp = &message.Timestamp
			chunk.MessageHeader.Timestamp = MaxTimestamp
		}

		// Write chunk
		if err := w.WriteChunk(chunk); err != nil {
			return err
		}

		// Subsequent chunks use format 3 (reuse all header)
		format = 3
		offset += payloadSize
	}

	return nil
}

// writeBasicHeader writes basic header (1-3 bytes)
func (w *ChunkWriter) writeBasicHeader(header *BasicHeader) error {
	var data []byte

	if header.ChunkStreamID < 64 {
		// 1 byte: format (2 bits) + chunk stream ID (6 bits)
		data = []byte{
			(header.Format << 6) | uint8(header.ChunkStreamID),
		}
	} else if header.ChunkStreamID < 320 {
		// 2 bytes: format (2 bits) + 0 (6 bits) + chunk stream ID - 64 (8 bits)
		data = []byte{
			header.Format << 6,
			uint8(header.ChunkStreamID - 64),
		}
	} else {
		// 3 bytes: format (2 bits) + 1 (6 bits) + (chunk stream ID - 64) (16 bits, big-endian)
		id := header.ChunkStreamID - 64
		data = []byte{
			(header.Format << 6) | 1,
			uint8(id >> 8),
			uint8(id),
		}
	}

	_, err := w.writer.Write(data)
	return err
}

// writeMessageHeader writes message header based on format
func (w *ChunkWriter) writeMessageHeader(format uint8, header *MessageHeader, extendedTimestamp *uint32) error {
	var data []byte

	switch format {
	case 0:
		// Type 0: 11 bytes (full header)
		data = make([]byte, 11)
		data[0] = uint8(header.Timestamp >> 16)
		data[1] = uint8(header.Timestamp >> 8)
		data[2] = uint8(header.Timestamp)
		data[3] = uint8(header.MessageLength >> 16)
		data[4] = uint8(header.MessageLength >> 8)
		data[5] = uint8(header.MessageLength)
		data[6] = header.MessageType
		binary.LittleEndian.PutUint32(data[7:11], header.MessageStreamID)

		// If timestamp >= 0xFFFFFF, use extended timestamp
		if header.Timestamp >= MaxTimestamp {
			data[0] = 0xFF
			data[1] = 0xFF
			data[2] = 0xFF
		}

	case 1:
		// Type 1: 7 bytes (reuse stream ID)
		data = make([]byte, 7)
		data[0] = uint8(header.TimestampDelta >> 16)
		data[1] = uint8(header.TimestampDelta >> 8)
		data[2] = uint8(header.TimestampDelta)
		data[3] = uint8(header.MessageLength >> 16)
		data[4] = uint8(header.MessageLength >> 8)
		data[5] = uint8(header.MessageLength)
		data[6] = header.MessageType

		// If timestamp delta >= 0xFFFFFF, use extended timestamp
		if header.TimestampDelta >= MaxTimestamp {
			data[0] = 0xFF
			data[1] = 0xFF
			data[2] = 0xFF
		}

	case 2:
		// Type 2: 3 bytes (only timestamp delta)
		data = make([]byte, 3)
		data[0] = uint8(header.TimestampDelta >> 16)
		data[1] = uint8(header.TimestampDelta >> 8)
		data[2] = uint8(header.TimestampDelta)

		// If timestamp delta >= 0xFFFFFF, use extended timestamp
		if header.TimestampDelta >= MaxTimestamp {
			data[0] = 0xFF
			data[1] = 0xFF
			data[2] = 0xFF
		}

	case 3:
		// Type 3: 0 bytes (reuse all previous header)
		data = []byte{}
	}

	// Write header
	if len(data) > 0 {
		if _, err := w.writer.Write(data); err != nil {
			return err
		}
	}

	// Write extended timestamp if needed
	if extendedTimestamp != nil {
		extData := make([]byte, 4)
		binary.BigEndian.PutUint32(extData, *extendedTimestamp)
		if _, err := w.writer.Write(extData); err != nil {
			return err
		}
	}

	return nil
}

// getBasicHeaderLength calculates the length of basic header for chunk stream ID
func (w *ChunkWriter) getBasicHeaderLength(chunkStreamID uint32) int {
	if chunkStreamID < 64 {
		return 1
	} else if chunkStreamID < 320 {
		return 2
	}
	return 3
}

// ViewerMediaWriter implements domain.MediaWriter for forwarding media to viewers
// It wraps ChunkWriter and handles thread-safe writing
type ViewerMediaWriter struct {
	chunkWriter *ChunkWriter
	flusher     Flusher
	mu          sync.Mutex
}

// Flusher interface for flushing buffered writers
type Flusher interface {
	Flush() error
}

// NewViewerMediaWriter creates a new viewer media writer
func NewViewerMediaWriter(chunkWriter *ChunkWriter, flusher Flusher) *ViewerMediaWriter {
	return &ViewerMediaWriter{
		chunkWriter: chunkWriter,
		flusher:     flusher,
	}
}

// WriteMediaChunk writes a media chunk to the viewer
// Implements domain.MediaWriter interface
func (v *ViewerMediaWriter) WriteMediaChunk(msgType uint8, timestamp uint32, payload []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	// Determine chunk stream ID based on message type
	// Video uses CSID 6, Audio uses CSID 4 (common convention)
	var chunkStreamID uint32
	if msgType == MessageTypeVideoData {
		chunkStreamID = 6
	} else if msgType == MessageTypeAudioData {
		chunkStreamID = 4
	} else {
		chunkStreamID = 5 // Default
	}
	
	// Create message
	message := &Message{
		ChunkStreamID:   chunkStreamID,
		Timestamp:       timestamp,
		MessageLength:   uint32(len(payload)),
		MessageType:     msgType,
		MessageStreamID: 1, // Stream ID 1 (little-endian encoding handled in WriteMessage)
		Payload:         payload,
	}
	
	// Write message as chunks
	if err := v.chunkWriter.WriteMessage(message, chunkStreamID); err != nil {
		return err
	}
	
	// Flush immediately for low latency
	if v.flusher != nil {
		return v.flusher.Flush()
	}
	
	return nil
}
