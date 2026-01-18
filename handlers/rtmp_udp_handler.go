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

	"github.com/fluxorio/fluxor/pkg/core"
	"github.com/fluxorio/fluxor/pkg/udp"
)

// RTMPUDPHandler handles RTMP UDP packets for low-latency features
// This is part of Protocol Layer (Level 1)
type RTMPUDPHandler struct {
	eventBus core.EventBus
}

// NewRTMPUDPHandler creates a new RTMP UDP handler
func NewRTMPUDPHandler(eventBus core.EventBus) *RTMPUDPHandler {
	return &RTMPUDPHandler{
		eventBus: eventBus,
	}
}

// HandlePacket handles RTMP UDP packets
func (h *RTMPUDPHandler) HandlePacket(ctx *udp.PacketContext) error {
	// UDP is used for RTMP low-latency streaming features
	// This could include:
	// - Fast media packet delivery
	// - Back-channel feedback
	// - Statistics/telemetry

	if len(ctx.Data) < 1 {
		return nil
	}

	log.Printf("[RTMP UDP] Packet from %s: length=%d", ctx.RemoteAddr.String(), len(ctx.Data))

	// Simplified: echo packet back for testing
	// In real implementation, parse and process RTMP UDP protocol
	_, err := ctx.WriteTo(ctx.Data)
	return err
}
