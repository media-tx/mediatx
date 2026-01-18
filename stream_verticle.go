package main

import (
	"fmt"
	"log"
	"time"

	"github.com/fluxorio/fluxor/pkg/config"
	"github.com/fluxorio/fluxor/pkg/core"
	"github.com/fluxorio/fluxor/pkg/tcp"
	"github.com/fluxorio/fluxor/pkg/udp"
	"github.com/fluxorio/fluxor/apps/rtmp-server/domain"
	"github.com/fluxorio/fluxor/apps/rtmp-server/handlers"
)

// RTMPServerVerticle demonstrates RTMP/SRT server using 2-level architecture
// Level 1: Protocol Layer (handlers)
// Level 2: Domain Layer (domain)
type RTMPServerVerticle struct {
	*core.BaseVerticle

	tcpServer *tcp.TCPServer
	udpServer *udp.UDPServer

	// Domain Layer (Level 2)
	connectionManager domain.ConnectionManager
	streamManager     domain.StreamManager

	// Protocol Layer Handlers (Level 1)
	rtmpHandler *handlers.RTMPConnectionHandler
	udpHandler  *handlers.RTMPUDPHandler
}

// NewRTMPServerVerticle creates a new RTMP/SRT server verticle
func NewRTMPServerVerticle() *RTMPServerVerticle {
	return &RTMPServerVerticle{
		BaseVerticle: core.NewBaseVerticle("rtmp-server"),
	}
}

// Start initializes the TCP and UDP servers using 2-level architecture
func (v *RTMPServerVerticle) Start(ctx core.FluxorContext) error {
	if err := v.BaseVerticle.Start(ctx); err != nil {
		return err
	}

	// Load configuration
	var rtmpConfig RTMPConfig
	configPath := "config.json"
	if err := config.LoadJSON(configPath, &rtmpConfig); err != nil {
		return fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}

	log.Printf("Loaded RTMP/SRT server configuration from %s", configPath)
	log.Printf("TCP Server: %s", rtmpConfig.TCP.Addr)
	log.Printf("UDP Server: %s", rtmpConfig.UDP.Addr)
	log.Printf("SRT Server: %s", rtmpConfig.SRT.Addr)

	// Initialize Domain Layer (Level 2)
	connMgr := domain.NewInMemoryConnectionManager()
	streamMgr := domain.NewInMemoryStreamManager()
	v.connectionManager = connMgr
	v.streamManager = streamMgr

	// Start cleanup goroutines for Domain Layer
	connMgr.StartCleanup(ctx.Context(), 30*time.Second, 5*time.Minute)
	streamMgr.StartCleanup(ctx.Context(), 30*time.Second, 5*time.Minute)

	log.Println("Domain Layer initialized:")
	log.Println("  - ConnectionManager: InMemory implementation")
	log.Println("  - StreamManager: InMemory implementation")

	// Get EventBus from context
	eventBus := ctx.EventBus()

	// Initialize Protocol Layer Handlers (Level 1)
	v.rtmpHandler = handlers.NewRTMPConnectionHandler(
		v.connectionManager,
		v.streamManager,
		eventBus,
	)
	v.udpHandler = handlers.NewRTMPUDPHandler(eventBus)

	log.Println("Protocol Layer Handlers initialized:")
	log.Println("  - RTMPConnectionHandler: RTMP protocol handler")
	log.Println("  - RTMPUDPHandler: UDP packet handler")

	// Parse timeouts
	tcpReadTimeout, _ := time.ParseDuration(rtmpConfig.TCP.ReadTimeout)
	if tcpReadTimeout <= 0 {
		tcpReadTimeout = 30 * time.Second
	}
	tcpWriteTimeout, _ := time.ParseDuration(rtmpConfig.TCP.WriteTimeout)
	if tcpWriteTimeout <= 0 {
		tcpWriteTimeout = 30 * time.Second
	}

	udpReadTimeout, _ := time.ParseDuration(rtmpConfig.UDP.ReadTimeout)
	if udpReadTimeout <= 0 {
		udpReadTimeout = 5 * time.Second
	}

	// Create TCP server for RTMP protocol (port 1935)
	tcpConfig := &tcp.TCPServerConfig{
		Addr:           rtmpConfig.TCP.Addr,
		MaxQueue:       rtmpConfig.TCP.MaxQueue,
		Workers:        rtmpConfig.TCP.Workers,
		MaxConns:       rtmpConfig.TCP.MaxConns,
		ReadTimeout:    tcpReadTimeout,
		WriteTimeout:   tcpWriteTimeout,
		AcceptGoroutines: 1,
	}

	v.tcpServer = tcp.NewTCPServer(ctx.GoCMD(), tcpConfig)
	v.tcpServer.SetHandler(v.rtmpHandler.HandleConnection)

	// Create UDP server for RTMP low-latency features (port 1936)
	udpConfig := &udp.UDPServerConfig{
		Addr:          rtmpConfig.UDP.Addr,
		MaxQueue:      rtmpConfig.UDP.MaxQueue,
		Workers:       rtmpConfig.UDP.Workers,
		MaxPPS:        rtmpConfig.UDP.MaxPPS,
		BufferSize:    rtmpConfig.UDP.BufferSize,
		ReadTimeout:   udpReadTimeout,
		ReadGoroutines: 1,
	}

	v.udpServer = udp.NewUDPServer(ctx.GoCMD(), udpConfig)
	v.udpServer.SetHandler(v.udpHandler.HandlePacket)

	// Start TCP server in a goroutine (non-blocking)
	v.ExecuteOn(func() {
		log.Printf("Starting RTMP TCP server on %s...", tcpConfig.Addr)
		if err := v.tcpServer.Start(); err != nil {
			log.Printf("❌ RTMP TCP server failed: %v", err)
		}
	})

	// Start UDP server in a goroutine (non-blocking)
	v.ExecuteOn(func() {
		log.Printf("Starting RTMP UDP server on %s...", udpConfig.Addr)
		if err := v.udpServer.Start(); err != nil {
			log.Printf("❌ RTMP UDP server failed: %v", err)
		}
	})

	// Start metrics reporting
	v.startMetricsReporter()

	log.Println("MediaTX started successfully")
	log.Println("  - RTMP: rtmp://localhost" + tcpConfig.Addr)
	log.Println("  - UDP:  Handling low-latency features on", udpConfig.Addr)
	log.Println("  - SRT Server: Configuration ready (implementation pending)")

	return nil
}

// Stop stops both TCP and UDP servers
func (v *RTMPServerVerticle) Stop(ctx core.FluxorContext) error {
	log.Println("Stopping MediaTX...")

	if v.tcpServer != nil {
		if err := v.tcpServer.Stop(); err != nil {
			log.Printf("Error stopping TCP server: %v", err)
		} else {
			log.Println("TCP server stopped")
		}
	}

	if v.udpServer != nil {
		if err := v.udpServer.Stop(); err != nil {
			log.Printf("Error stopping UDP server: %v", err)
		} else {
			log.Println("UDP server stopped")
		}
	}

	// Display final statistics
	v.displayStatistics()

	return v.BaseVerticle.Stop(ctx)
}

// startMetricsReporter starts periodic metrics reporting
func (v *RTMPServerVerticle) startMetricsReporter() {
	// Store context from GoCMD before starting goroutine
	ctx := v.GoCMD().Context()
	
	v.ExecuteOn(func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				v.reportMetrics()
			case <-ctx.Done():
				return
			}
		}
	})
}

// reportMetrics reports server metrics
func (v *RTMPServerVerticle) reportMetrics() {
	// Get metrics from Domain Layer
	connections := v.connectionManager.GetAllConnections()
	streams := v.streamManager.GetAllStreams()

	var tcpMetrics tcp.ServerMetrics
	var udpMetrics udp.ServerMetrics

	if v.tcpServer != nil {
		tcpMetrics = v.tcpServer.Metrics()
	}
	if v.udpServer != nil {
		udpMetrics = v.udpServer.Metrics()
	}

	log.Printf("[Metrics] Connections: %d (TCP active: %d), Streams: %d, TCP queued: %d, UDP queued: %d",
		len(connections), tcpMetrics.ActiveConnections, len(streams), tcpMetrics.QueuedConnections, udpMetrics.QueuedPackets)
}

// displayStatistics displays final statistics on shutdown
func (v *RTMPServerVerticle) displayStatistics() {
	// Get statistics from Domain Layer
	connections := v.connectionManager.GetAllConnections()
	streams := v.streamManager.GetAllStreams()

	var tcpMetrics tcp.ServerMetrics
	var udpMetrics udp.ServerMetrics

	if v.tcpServer != nil {
		tcpMetrics = v.tcpServer.Metrics()
	}
	if v.udpServer != nil {
		udpMetrics = v.udpServer.Metrics()
	}

	log.Printf("\n[MediaTX Statistics]")
	log.Printf("  Connections handled: %d", tcpMetrics.HandledConnections)
	log.Printf("  Current connections: %d", len(connections))
	log.Printf("  Active streams: %d", len(streams))
	log.Printf("  TCP - Queued: %d, Total accepted: %d, Errors: %d", tcpMetrics.QueuedConnections, tcpMetrics.TotalAccepted, tcpMetrics.ErrorConnections)
	log.Printf("  UDP - Queued: %d, Total received: %d, Errors: %d", udpMetrics.QueuedPackets, udpMetrics.TotalReceived, udpMetrics.ErrorPackets)
}
