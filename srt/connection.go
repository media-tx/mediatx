package srt

import (
	"time"

	grt "github.com/datarhei/gosrt"
)

// SRTConn wraps gosrt.Conn to implement the Conn interface
type SRTConn struct {
	conn grt.Conn
}

// Read reads data from the connection
func (c *SRTConn) Read(p []byte) (int, error) {
	return c.conn.Read(p)
}

// Write writes data to the connection
func (c *SRTConn) Write(p []byte) (int, error) {
	return c.conn.Write(p)
}

// Close closes the connection
func (c *SRTConn) Close() error {
	return c.conn.Close()
}

// RemoteAddr returns the remote network address
func (c *SRTConn) RemoteAddr() string {
	return c.conn.RemoteAddr().String()
}

// StreamId returns the stream ID
func (c *SRTConn) StreamId() string {
	return c.conn.StreamId()
}

// SetStreamId sets the stream ID
// Note: In gosrt, StreamId is set during Dial/Listen, not after connection
// This method is kept for interface compatibility but may not work as expected
func (c *SRTConn) SetStreamId(streamId string) error {
	// StreamId cannot be changed after connection in gosrt
	// This is a no-op for compatibility
	return nil
}

// SetReadDeadline sets the read deadline
func (c *SRTConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline
func (c *SRTConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// SetReadTimeout sets the read timeout
func (c *SRTConn) SetReadTimeout(duration time.Duration) error {
	return c.SetReadDeadline(time.Now().Add(duration))
}

// SetWriteTimeout sets the write timeout
func (c *SRTConn) SetWriteTimeout(duration time.Duration) error {
	return c.SetWriteDeadline(time.Now().Add(duration))
}

// SRTListener wraps gosrt.Listener to implement the Listener interface
type SRTListener struct {
	listener  grt.Listener
	addr      string
	acceptFunc grt.AcceptFunc
}

// newSRTListener creates a new SRT listener wrapper
func newSRTListener(listener grt.Listener, addr string, acceptFunc grt.AcceptFunc) *SRTListener {
	return &SRTListener{
		listener:   listener,
		addr:       addr,
		acceptFunc: acceptFunc,
	}
}

// Accept waits for and returns the next connection
func (l *SRTListener) Accept() (Conn, error) {
	// gosrt.Listener.Accept returns (conn, mode, error)
	// We ignore the mode here as it's handled in the AcceptFunc
	conn, _, err := l.listener.Accept(l.acceptFunc)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		// Connection was rejected, continue accepting
		return l.Accept()
	}
	return newSRTConn(conn), nil
}

// Close closes the listener
func (l *SRTListener) Close() error {
	l.listener.Close()
	return nil
}

// Addr returns the listener's network address
func (l *SRTListener) Addr() string {
	return l.addr
}

// newSRTConn creates a new SRT connection wrapper
func newSRTConn(conn grt.Conn) *SRTConn {
	return &SRTConn{
		conn: conn,
	}
}
