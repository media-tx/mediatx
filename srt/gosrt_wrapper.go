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

package srt

// This file will contain the actual gosrt wrapper implementation
// when github.com/datarhei/gosrt dependency is added

/*
Example implementation after adding gosrt:

import (
	"github.com/datarhei/gosrt/srt"
)

// Wrap gosrt.Conn
type gosrtConn struct {
	conn *srt.Conn
	streamId string
}

func newSRTConnFromGosrt(conn *srt.Conn) *gosrtConn {
	return &gosrtConn{
		conn: conn,
		streamId: conn.StreamId(),
	}
}

func (c *gosrtConn) Read(p []byte) (int, error) {
	return c.conn.Read(p)
}

func (c *gosrtConn) Write(p []byte) (int, error) {
	return c.conn.Write(p)
}

func (c *gosrtConn) Close() error {
	return c.conn.Close()
}

func (c *gosrtConn) RemoteAddr() string {
	return c.conn.RemoteAddr().String()
}

func (c *gosrtConn) StreamId() string {
	return c.streamId
}

func (c *gosrtConn) SetStreamId(streamId string) error {
	c.streamId = streamId
	// gosrt may not have SetStreamId, so we track it ourselves
	return nil
}

func (c *gosrtConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *gosrtConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// Wrap gosrt.Listener
type gosrtListener struct {
	listener *srt.Listener
	addr     string
}

func newSRTListenerFromGosrt(listener *srt.Listener, addr string) *gosrtListener {
	return &gosrtListener{
		listener: listener,
		addr:     addr,
	}
}

func (l *gosrtListener) Accept() (Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}
	return newSRTConnFromGosrt(conn), nil
}

func (l *gosrtListener) Close() error {
	return l.listener.Close()
}

func (l *gosrtListener) Addr() string {
	return l.addr
}

// Update SRTServer.Listen to use gosrt
func (s *SRTServer) Listen(network, addr string, config *Config) (Listener, error) {
	gosrtConfig := srt.Config{
		StreamId:   config.StreamId,
		Latency:    config.Latency,
		Passphrase: config.Passphrase,
		PbKeyLen:   config.PbKeyLen,
	}
	
	listener, err := srt.Listen(network, addr, gosrtConfig)
	if err != nil {
		return nil, err
	}
	
	return newSRTListenerFromGosrt(listener, addr), nil
}

// Update SRTServer.Dial to use gosrt
func (s *SRTServer) Dial(network, addr string, config *Config) (Conn, error) {
	gosrtConfig := srt.Config{
		StreamId:   config.StreamId,
		Latency:    config.Latency,
		Passphrase: config.Passphrase,
		PbKeyLen:   config.PbKeyLen,
	}
	
	conn, err := srt.Dial(network, addr, gosrtConfig)
	if err != nil {
		return nil, err
	}
	
	return newSRTConnFromGosrt(conn), nil
}
*/
