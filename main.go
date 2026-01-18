package main

import (
	"fmt"
	"log"

	"github.com/fluxorio/fluxor/pkg/entrypoint"
)

const (
	// MediaTX version and branding
	Version   = "1.0.0"
	Name      = "MediaTX"
	Banner    = `
╔╦╗┌─┐┌┬┐┬┌─┐╔╦╗═╗ ╦
║║║├┤  │││├─┤ ║ ╔╩╦╝
╩ ╩└─┘─┴┘┴┴ ┴ ╩ ╩ ╚═
`
)

func printBanner() {
	fmt.Println(Banner)
	fmt.Printf("%s v%s - High Performance Media Server\n", Name, Version)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("• RTMP Streaming  • Low Latency")
	fmt.Println("• SRT Support     • GOP Cache")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

func main() {
	printBanner()
	log.Printf("Starting %s v%s...", Name, Version)

	// Create Fluxor application
	app, err := entrypoint.NewMainVerticle("")
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Deploy MediaTX server verticle
	_, err = app.DeployVerticle(NewRTMPServerVerticle())
	if err != nil {
		log.Fatalf("Failed to deploy %s verticle: %v", Name, err)
	}

	// Start application (blocks until SIGINT/SIGTERM)
	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}

	// Stop application
	if err := app.Stop(); err != nil {
		log.Printf("Error stopping application: %v", err)
	}

	log.Printf("%s stopped gracefully", Name)
}
