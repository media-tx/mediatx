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
