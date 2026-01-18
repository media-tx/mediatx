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
	"encoding/binary"
	"fmt"
	"io"
	"log"
)

const (
	// RTMP chunk constants
	MaxChunkSize         = 65536
	DefaultChunkSize     = 128
	MaxTimestamp         = 0xFFFFFF
	MaxMessageHeaderSize = 11
)

// ChunkParser parses RTMP chunk streams
type ChunkParser struct {
	chunkSize      uint32 // Current chunk size (negotiated)
	lastTimestamp  map[uint32]uint32 // Last timestamp per chunk stream ID
	lastMessageHeader map[uint32]*MessageHeader // Last message header per chunk stream ID
	partialMessages map[uint32]*MessageBuilder // Partial messages per chunk stream ID
}

// MessageBuilder builds a complete message from chunks
type MessageBuilder struct {
	ChunkStreamID   uint32
	Timestamp       uint32
	MessageLength   uint32
	MessageType     uint8
	MessageStreamID uint32
	Payload         []byte
	ReceivedLength  uint32
}

// NewChunkParser creates a new chunk parser
func NewChunkParser() *ChunkParser {
	return &ChunkParser{
		chunkSize:         DefaultChunkSize,
		lastTimestamp:     make(map[uint32]uint32),
		lastMessageHeader: make(map[uint32]*MessageHeader),
		partialMessages:   make(map[uint32]*MessageBuilder),
	}
}

// SetChunkSize sets the chunk size (from SetChunkSize message)
func (p *ChunkParser) SetChunkSize(size uint32) {
	if size > 0 && size <= MaxChunkSize {
		p.chunkSize = size
	}
}

// GetChunkSize returns the current chunk size
func (p *ChunkParser) GetChunkSize() uint32 {
	return p.chunkSize
}

// ReadChunk reads a chunk from the connection
// getRemainingBytes is an optional callback to get remaining bytes for format=3 chunks
// It should return (remaining, total, found) - found is false if no partial message exists
func (p *ChunkParser) ReadChunk(reader io.Reader, getRemainingBytes func(uint32) (uint32, uint32, bool)) (*Chunk, error) {
	// Read basic header (1-3 bytes)
	basicHeader, err := p.readBasicHeader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read basic header: %w", err)
	}
	log.Printf("[ChunkParser] Read basic header: format=%d, csid=%d", basicHeader.Format, basicHeader.ChunkStreamID)

	// Read message header (0, 3, 7, or 11 bytes) based on format
	messageHeader, extendedTimestamp, err := p.readMessageHeader(reader, basicHeader.Format, basicHeader.ChunkStreamID)
	if err != nil {
		return nil, fmt.Errorf("failed to read message header: %w", err)
	}

	// Determine payload size
	var payloadSize uint32
	
	if basicHeader.Format == 3 {
		// Format 3: continuation chunk - calculate remaining bytes
		if messageHeader == nil {
			return nil, fmt.Errorf("format 3 chunk requires previous message header for chunk stream %d", basicHeader.ChunkStreamID)
		}
		
		// Get remaining bytes from partial message if callback is provided
		if getRemainingBytes != nil {
			remaining, total, found := getRemainingBytes(basicHeader.ChunkStreamID)
			if found {
				// Calculate payload size: min(remaining, chunkSize)
				if remaining < p.chunkSize {
					payloadSize = remaining
				} else {
					payloadSize = p.chunkSize
				}
				log.Printf("[ChunkParser] Format=3 continuation: remaining=%d/%d bytes, payloadSize=%d", 
					remaining, total, payloadSize)
			} else {
				// No partial message yet - use chunkSize (shouldn't happen normally)
				payloadSize = p.chunkSize
				log.Printf("[ChunkParser] Format=3 but no partial message found, using chunkSize=%d", payloadSize)
			}
		} else {
			// No callback - fallback to chunkSize (legacy behavior)
			payloadSize = p.chunkSize
			log.Printf("[ChunkParser] Format=3 continuation: no callback, using chunkSize=%d", payloadSize)
		}
	} else if messageHeader != nil && messageHeader.MessageLength > 0 {
		// Format 0, 1, or 2: first chunk of a message
		// Read min(MessageLength, chunkSize)
		if messageHeader.MessageLength < p.chunkSize {
			payloadSize = messageHeader.MessageLength
		} else {
			payloadSize = p.chunkSize
		}
		log.Printf("[ChunkParser] MessageHeader: type=0x%02x, length=%d, calculated payloadSize=%d, chunkSize=%d", 
			messageHeader.MessageType, messageHeader.MessageLength, payloadSize, p.chunkSize)
	} else {
		// Shouldn't happen
		payloadSize = p.chunkSize
		log.Printf("[ChunkParser] Warning: no message header, using chunkSize=%d", payloadSize)
	}

	// Read payload
	payload := make([]byte, payloadSize)
	n, err := io.ReadFull(reader, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to read chunk payload (expected %d bytes, got %d): %w", payloadSize, n, err)
	}
	log.Printf("[ChunkParser] Read payload: expected=%d, actual=%d, format=%d, csid=%d", payloadSize, n, basicHeader.Format, basicHeader.ChunkStreamID)

	chunk := &Chunk{
		BasicHeader:     basicHeader,
		MessageHeader:   messageHeader,
		ExtendedTimestamp: extendedTimestamp,
		Payload:         payload[:n],
	}

	// Update last timestamp and message header
	if messageHeader != nil {
		p.lastMessageHeader[basicHeader.ChunkStreamID] = messageHeader
		if extendedTimestamp != nil {
			p.lastTimestamp[basicHeader.ChunkStreamID] = *extendedTimestamp
		} else if messageHeader.Timestamp > 0 {
			p.lastTimestamp[basicHeader.ChunkStreamID] = messageHeader.Timestamp
		}
	}

	return chunk, nil
}

// readBasicHeader reads chunk basic header (1-3 bytes)
// Format:
//   Format 0 (0b00xxxxxx): Chunk stream ID 2-63 (1 byte)
//   Format 1 (0b01xxxxxx): Chunk stream ID 64-319 (2 bytes)
//   Format 2 (0b10xxxxxx): Chunk stream ID 64-65599 (3 bytes)
//   Format 3 (0b11xxxxxx): 11 bytes (reuse previous header)
func (p *ChunkParser) readBasicHeader(reader io.Reader) (*BasicHeader, error) {
	firstByte := make([]byte, 1)
	if _, err := reader.Read(firstByte); err != nil {
		return nil, err
	}

	format := (firstByte[0] >> 6) & 0x03
	chunkStreamID := uint32(firstByte[0] & 0x3F)
	headerLength := 1

	// Read additional bytes for chunk stream ID if needed
	if chunkStreamID == 0 {
		// Format 1: 2 bytes total
		secondByte := make([]byte, 1)
		if _, err := reader.Read(secondByte); err != nil {
			return nil, err
		}
		chunkStreamID = uint32(secondByte[0]) + 64
		headerLength = 2
	} else if chunkStreamID == 1 {
		// Format 2: 3 bytes total
		bytes := make([]byte, 2)
		if _, err := reader.Read(bytes); err != nil {
			return nil, err
		}
		chunkStreamID = uint32(bytes[0])*256 + uint32(bytes[1]) + 64
		headerLength = 3
	}

	return &BasicHeader{
		Format:        format,
		ChunkStreamID: chunkStreamID,
		HeaderLength:  headerLength,
	}, nil
}

// readMessageHeader reads chunk message header based on format
func (p *ChunkParser) readMessageHeader(reader io.Reader, format uint8, chunkStreamID uint32) (*MessageHeader, *uint32, error) {
	var header *MessageHeader
	var extendedTimestamp *uint32

	switch format {
	case 0:
		// Type 0: 11 bytes (full header)
		header, extendedTimestamp, _ = p.readType0Header(reader)
	case 1:
		// Type 1: 7 bytes (reuse stream ID)
		lastHeader := p.lastMessageHeader[chunkStreamID]
		if lastHeader == nil {
			return nil, nil, fmt.Errorf("type 1 header requires previous header for chunk stream %d", chunkStreamID)
		}
		header, extendedTimestamp, _ = p.readType1Header(reader, lastHeader)
	case 2:
		// Type 2: 3 bytes (only timestamp delta)
		lastHeader := p.lastMessageHeader[chunkStreamID]
		if lastHeader == nil {
			return nil, nil, fmt.Errorf("type 2 header requires previous header for chunk stream %d", chunkStreamID)
		}
		header, extendedTimestamp, _ = p.readType2Header(reader, lastHeader)
	case 3:
		// Type 3: 0 bytes (reuse all previous header)
		lastHeader := p.lastMessageHeader[chunkStreamID]
		if lastHeader == nil {
			return nil, nil, fmt.Errorf("type 3 header requires previous header for chunk stream %d", chunkStreamID)
		}
		header = lastHeader
		// Check if extended timestamp needed
		if lastHeader.Timestamp >= MaxTimestamp {
			extendedTimestamp = p.readExtendedTimestamp(reader)
		}
	}

	return header, extendedTimestamp, nil
}

// readType0Header reads type 0 message header (11 bytes)
func (p *ChunkParser) readType0Header(reader io.Reader) (*MessageHeader, *uint32, error) {
	data := make([]byte, 11)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, nil, err
	}

	timestamp := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
	messageLength := uint32(data[3])<<16 | uint32(data[4])<<8 | uint32(data[5])
	messageType := data[6]
	messageStreamID := binary.LittleEndian.Uint32(data[7:11])

	var extendedTimestamp *uint32
	if timestamp >= MaxTimestamp {
		extendedTimestamp = p.readExtendedTimestamp(reader)
		if extendedTimestamp != nil {
			timestamp = *extendedTimestamp
		}
	}

	header := &MessageHeader{
		Timestamp:       timestamp,
		MessageLength:   messageLength,
		MessageType:     messageType,
		MessageStreamID: messageStreamID,
	}

	return header, extendedTimestamp, nil
}

// readType1Header reads type 1 message header (7 bytes)
func (p *ChunkParser) readType1Header(reader io.Reader, lastHeader *MessageHeader) (*MessageHeader, *uint32, error) {
	data := make([]byte, 7)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, nil, err
	}

	timestampDelta := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
	messageLength := uint32(data[3])<<16 | uint32(data[4])<<8 | uint32(data[5])
	messageType := data[6]

	// Calculate timestamp
	timestamp := lastHeader.Timestamp + timestampDelta
	if timestampDelta >= MaxTimestamp {
		timestampDelta = MaxTimestamp // Indicates extended timestamp
	}

	var extendedTimestamp *uint32
	if timestampDelta >= MaxTimestamp || timestamp >= MaxTimestamp {
		extendedTimestamp = p.readExtendedTimestamp(reader)
		if extendedTimestamp != nil {
			timestamp = *extendedTimestamp
		} else {
			timestamp = lastHeader.Timestamp + timestampDelta
		}
	}

	header := &MessageHeader{
		Timestamp:      timestamp,
		TimestampDelta: timestampDelta,
		MessageLength:  messageLength,
		MessageType:    messageType,
		MessageStreamID: lastHeader.MessageStreamID, // Reuse from last header
	}

	return header, extendedTimestamp, nil
}

// readType2Header reads type 2 message header (3 bytes)
func (p *ChunkParser) readType2Header(reader io.Reader, lastHeader *MessageHeader) (*MessageHeader, *uint32, error) {
	data := make([]byte, 3)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, nil, err
	}

	timestampDelta := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])

	// Calculate timestamp
	timestamp := lastHeader.Timestamp + timestampDelta
	if timestampDelta >= MaxTimestamp {
		timestampDelta = MaxTimestamp // Indicates extended timestamp
	}

	var extendedTimestamp *uint32
	if timestampDelta >= MaxTimestamp || timestamp >= MaxTimestamp {
		extendedTimestamp = p.readExtendedTimestamp(reader)
		if extendedTimestamp != nil {
			timestamp = *extendedTimestamp
		} else {
			timestamp = lastHeader.Timestamp + timestampDelta
		}
	}

	header := &MessageHeader{
		Timestamp:       timestamp,
		TimestampDelta:  timestampDelta,
		MessageLength:   lastHeader.MessageLength,   // Reuse from last header
		MessageType:     lastHeader.MessageType,     // Reuse from last header
		MessageStreamID: lastHeader.MessageStreamID, // Reuse from last header
	}

	return header, extendedTimestamp, nil
}

// readExtendedTimestamp reads extended timestamp (4 bytes) if timestamp >= 0xFFFFFF
func (p *ChunkParser) readExtendedTimestamp(reader io.Reader) *uint32 {
	data := make([]byte, 4)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil
	}
	timestamp := binary.BigEndian.Uint32(data)
	return &timestamp
}

// getPartialMessageLength returns the length of partial message for chunk stream ID
func (p *ChunkParser) getPartialMessageLength(chunkStreamID uint32) uint32 {
	if builder, exists := p.partialMessages[chunkStreamID]; exists {
		return builder.ReceivedLength
	}
	return 0
}
