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
)

// MessageParser reconstructs complete RTMP messages from chunks
type MessageParser struct {
	chunkParser *ChunkParser
}

// NewMessageParser creates a new message parser
func NewMessageParser(chunkParser *ChunkParser) *MessageParser {
	return &MessageParser{
		chunkParser: chunkParser,
	}
}

// ReconstructMessage reconstructs a complete message from chunks
// Returns the complete message when all chunks are received
func (p *MessageParser) ReconstructMessage(chunk *Chunk) (*Message, bool, error) {
	chunkStreamID := chunk.BasicHeader.ChunkStreamID

	// Get or create message builder
	builder, exists := p.chunkParser.partialMessages[chunkStreamID]
	if !exists || chunk.MessageHeader != nil {
		// New message or type 0 header (full header)
		if chunk.MessageHeader == nil {
			return nil, false, fmt.Errorf("chunk type 3 requires previous message header")
		}

		builder = &MessageBuilder{
			ChunkStreamID:   chunkStreamID,
			Timestamp:       chunk.MessageHeader.Timestamp,
			MessageLength:   chunk.MessageHeader.MessageLength,
			MessageType:     chunk.MessageHeader.MessageType,
			MessageStreamID: chunk.MessageHeader.MessageStreamID,
			Payload:         make([]byte, 0, chunk.MessageHeader.MessageLength),
			ReceivedLength:  0,
		}

		// Handle extended timestamp
		if chunk.ExtendedTimestamp != nil {
			builder.Timestamp = *chunk.ExtendedTimestamp
		}

		// Update partial messages map
		p.chunkParser.partialMessages[chunkStreamID] = builder
	}

	// Append chunk payload
	builder.Payload = append(builder.Payload, chunk.Payload...)
	builder.ReceivedLength += uint32(len(chunk.Payload))

	// Check if message is complete
	if builder.ReceivedLength >= builder.MessageLength {
		// Message complete
		message := &Message{
			ChunkStreamID:   builder.ChunkStreamID,
			Timestamp:       builder.Timestamp,
			MessageLength:   builder.MessageLength,
			MessageType:     builder.MessageType,
			MessageStreamID: builder.MessageStreamID,
			Payload:         builder.Payload[:builder.MessageLength],
		}

		// Clean up partial message
		delete(p.chunkParser.partialMessages, chunkStreamID)

		return message, true, nil
	}

	// Message not complete yet
	return nil, false, nil
}

// HasPartialMessage checks if there's a partial message for chunk stream ID
func (p *MessageParser) HasPartialMessage(chunkStreamID uint32) bool {
	_, exists := p.chunkParser.partialMessages[chunkStreamID]
	return exists
}

// ClearPartialMessage clears partial message for chunk stream ID
func (p *MessageParser) ClearPartialMessage(chunkStreamID uint32) {
	delete(p.chunkParser.partialMessages, chunkStreamID)
}
