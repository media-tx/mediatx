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
