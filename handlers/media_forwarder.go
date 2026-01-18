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
	"log"
	"sync"

	"github.com/fluxorio/fluxor/apps/rtmp-server/domain"
)

// MediaForwarder handles forwarding media data from publishers to viewers
// It manages sequence headers, metadata caching, and keyframe detection
type MediaForwarder struct {
	streamManager domain.StreamManager
	mu            sync.RWMutex
	
	// Stats
	videoFramesForwarded uint64
	audioFramesForwarded uint64
	keyframesForwarded   uint64
}

// NewMediaForwarder creates a new media forwarder
func NewMediaForwarder(streamManager domain.StreamManager) *MediaForwarder {
	return &MediaForwarder{
		streamManager: streamManager,
	}
}

// VideoFrameInfo contains parsed video frame information
type VideoFrameInfo struct {
	IsKeyframe       bool
	IsSequenceHeader bool
	CodecID          uint8 // 7=AVC/H.264, 12=HEVC/H.265
	FrameType        uint8 // 1=keyframe, 2=inter frame
	PacketType       uint8 // 0=sequence header, 1=NALU, 2=end of sequence
}

// AudioFrameInfo contains parsed audio frame information
type AudioFrameInfo struct {
	IsSequenceHeader bool
	SoundFormat      uint8 // 10=AAC
	PacketType       uint8 // 0=sequence header, 1=raw
}

// ParseVideoFrame parses video frame header to extract frame info
func (f *MediaForwarder) ParseVideoFrame(payload []byte) *VideoFrameInfo {
	if len(payload) < 2 {
		return nil
	}
	
	frameType := (payload[0] & 0xF0) >> 4 // Upper 4 bits
	codecID := payload[0] & 0x0F          // Lower 4 bits
	packetType := payload[1]
	
	return &VideoFrameInfo{
		IsKeyframe:       frameType == 1,
		IsSequenceHeader: frameType == 1 && (codecID == 7 || codecID == 12) && packetType == 0,
		CodecID:          codecID,
		FrameType:        frameType,
		PacketType:       packetType,
	}
}

// ParseAudioFrame parses audio frame header to extract frame info
func (f *MediaForwarder) ParseAudioFrame(payload []byte) *AudioFrameInfo {
	if len(payload) < 2 {
		return nil
	}
	
	soundFormat := (payload[0] & 0xF0) >> 4 // Upper 4 bits
	packetType := payload[1]
	
	return &AudioFrameInfo{
		IsSequenceHeader: soundFormat == 10 && packetType == 0, // AAC sequence header
		SoundFormat:      soundFormat,
		PacketType:       packetType,
	}
}

// ForwardVideoData forwards video data to all viewers
func (f *MediaForwarder) ForwardVideoData(stream *domain.RTMPStream, timestamp uint32, payload []byte) {
	if stream == nil || len(payload) < 2 {
		return
	}
	
	// Parse video frame
	frameInfo := f.ParseVideoFrame(payload)
	if frameInfo == nil {
		return
	}
	
	// Cache sequence header for new viewers
	if frameInfo.IsSequenceHeader {
		log.Printf("[MediaForwarder] Caching video sequence header (%d bytes, codec=%d)", len(payload), frameInfo.CodecID)
		stream.SetVideoSequenceHeader(payload)
		return // Don't forward sequence header as regular frame
	}
	
	// Get viewers
	viewers := stream.GetViewers()
	if len(viewers) == 0 {
		return
	}
	
	// Forward to viewers
	var failedViewers []string
	successCount := 0
	
	for _, viewer := range viewers {
		// Check if viewer needs keyframe first (GOP cache)
		if !viewer.HasReceivedKeyframe() && !frameInfo.IsKeyframe {
			// Skip non-keyframes until viewer receives a keyframe
			continue
		}
		
		if frameInfo.IsKeyframe {
			viewer.SetHasReceivedKeyframe(true)
		}
		
		if err := viewer.WriteMedia(MessageTypeVideoData, timestamp, payload); err != nil {
			failedViewers = append(failedViewers, viewer.ID)
		} else {
			successCount++
		}
	}
	
	// Log periodically (reduced frequency)
	if timestamp%1000 < 34 {
		log.Printf("[MediaForwarder] Video: %d bytes, keyframe=%v, viewers=%d, success=%d, failed=%d",
			len(payload), frameInfo.IsKeyframe, len(viewers), successCount, len(failedViewers))
	}
	
	// Remove failed viewers
	f.removeFailedViewers(stream, failedViewers)
	
	// Update stats
	f.mu.Lock()
	f.videoFramesForwarded++
	if frameInfo.IsKeyframe {
		f.keyframesForwarded++
	}
	f.mu.Unlock()
	
	stream.UpdateLastMedia()
}

// ForwardAudioData forwards audio data to all viewers
func (f *MediaForwarder) ForwardAudioData(stream *domain.RTMPStream, timestamp uint32, payload []byte) {
	if stream == nil || len(payload) < 2 {
		return
	}
	
	// Parse audio frame
	frameInfo := f.ParseAudioFrame(payload)
	if frameInfo == nil {
		return
	}
	
	// Cache sequence header for new viewers
	if frameInfo.IsSequenceHeader {
		log.Printf("[MediaForwarder] Caching audio sequence header (%d bytes, format=%d)", len(payload), frameInfo.SoundFormat)
		stream.SetAudioSequenceHeader(payload)
		return // Don't forward sequence header as regular frame
	}
	
	// Get viewers
	viewers := stream.GetViewers()
	if len(viewers) == 0 {
		return
	}
	
	// Forward to viewers (only if they've received keyframe)
	var failedViewers []string
	successCount := 0
	
	for _, viewer := range viewers {
		// Only forward audio after viewer has received video keyframe
		if !viewer.HasReceivedKeyframe() {
			continue
		}
		
		if err := viewer.WriteMedia(MessageTypeAudioData, timestamp, payload); err != nil {
			failedViewers = append(failedViewers, viewer.ID)
		} else {
			successCount++
		}
	}
	
	// Log periodically
	if timestamp%1000 < 34 {
		log.Printf("[MediaForwarder] Audio: %d bytes, viewers=%d, success=%d, failed=%d",
			len(payload), len(viewers), successCount, len(failedViewers))
	}
	
	// Remove failed viewers
	f.removeFailedViewers(stream, failedViewers)
	
	// Update stats
	f.mu.Lock()
	f.audioFramesForwarded++
	f.mu.Unlock()
	
	stream.UpdateLastMedia()
}

// ForwardMetadata forwards metadata to all viewers
func (f *MediaForwarder) ForwardMetadata(stream *domain.RTMPStream, timestamp uint32, payload []byte) {
	if stream == nil || len(payload) == 0 {
		return
	}
	
	// Cache metadata for new viewers
	log.Printf("[MediaForwarder] Caching metadata (%d bytes)", len(payload))
	stream.SetMetadata(payload)
	
	// Forward to current viewers
	viewers := stream.GetViewers()
	for _, viewer := range viewers {
		if err := viewer.WriteData(MessageTypeAMF0Data, timestamp, payload); err != nil {
			log.Printf("[MediaForwarder] Failed to forward metadata to viewer %s: %v", viewer.ID, err)
		}
	}
}

// SendInitialDataToViewer sends cached sequence headers and metadata to a new viewer
func (f *MediaForwarder) SendInitialDataToViewer(stream *domain.RTMPStream, viewer *domain.RTMPConnection) error {
	if stream == nil || viewer == nil {
		return nil
	}
	
	// Send metadata first
	if metadata := stream.GetMetadata(); metadata != nil {
		log.Printf("[MediaForwarder] Sending cached metadata (%d bytes) to viewer %s", len(metadata), viewer.ID)
		if err := viewer.WriteData(MessageTypeAMF0Data, 0, metadata); err != nil {
			log.Printf("[MediaForwarder] Failed to send metadata: %v", err)
		}
	}
	
	// Send video sequence header
	if videoSeqHeader := stream.GetVideoSequenceHeader(); videoSeqHeader != nil {
		log.Printf("[MediaForwarder] Sending video sequence header (%d bytes) to viewer %s", len(videoSeqHeader), viewer.ID)
		if err := viewer.WriteMedia(MessageTypeVideoData, 0, videoSeqHeader); err != nil {
			log.Printf("[MediaForwarder] Failed to send video sequence header: %v", err)
		}
	}
	
	// Send audio sequence header
	if audioSeqHeader := stream.GetAudioSequenceHeader(); audioSeqHeader != nil {
		log.Printf("[MediaForwarder] Sending audio sequence header (%d bytes) to viewer %s", len(audioSeqHeader), viewer.ID)
		if err := viewer.WriteMedia(MessageTypeAudioData, 0, audioSeqHeader); err != nil {
			log.Printf("[MediaForwarder] Failed to send audio sequence header: %v", err)
		}
	}
	
	log.Printf("[MediaForwarder] Initial data sent to viewer %s", viewer.ID)
	return nil
}

// removeFailedViewers removes viewers that failed to receive media
func (f *MediaForwarder) removeFailedViewers(stream *domain.RTMPStream, failedViewers []string) {
	for _, viewerID := range failedViewers {
		stream.RemoveViewer(viewerID)
		log.Printf("[MediaForwarder] Removed disconnected viewer %s", viewerID)
	}
}

// GetStats returns forwarding statistics
func (f *MediaForwarder) GetStats() (videoFrames, audioFrames, keyframes uint64) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.videoFramesForwarded, f.audioFramesForwarded, f.keyframesForwarded
}
