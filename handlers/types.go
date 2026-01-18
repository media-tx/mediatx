package handlers

// RTMP Chunk Stream Types

// Chunk represents an RTMP chunk
type Chunk struct {
	BasicHeader     *BasicHeader
	MessageHeader   *MessageHeader
	ExtendedTimestamp *uint32
	Payload         []byte
}

// BasicHeader represents RTMP chunk basic header (1-3 bytes)
type BasicHeader struct {
	Format          uint8 // 2 bits: chunk format (0-3)
	ChunkStreamID   uint32 // 6-22 bits: chunk stream ID
	HeaderLength    int    // Total bytes: 1, 2, or 3
}

// MessageHeader represents RTMP chunk message header (0, 3, 7, or 11 bytes)
type MessageHeader struct {
	Timestamp       uint32 // 3 bytes (0xFFFFFF if extended)
	TimestampDelta  uint32 // 3 bytes (for type 1, 2)
	MessageLength   uint32 // 3 bytes
	MessageType     uint8  // 1 byte
	MessageStreamID uint32 // 4 bytes (little-endian)
}

// RTMP Message Types

// Message represents a complete RTMP message (reconstructed from chunks)
type Message struct {
	ChunkStreamID   uint32
	Timestamp       uint32
	MessageLength   uint32
	MessageType     uint8
	MessageStreamID uint32
	Payload         []byte
}

// MessageType constants
const (
	MessageTypeSetChunkSize     uint8 = 0x01
	MessageTypeAbortMessage     uint8 = 0x02
	MessageTypeAcknowledgement  uint8 = 0x03
	MessageTypeUserControl      uint8 = 0x04
	MessageTypeWindowAckSize    uint8 = 0x05
	MessageTypeSetPeerBandwidth uint8 = 0x06
	MessageTypeAudioData        uint8 = 0x08
	MessageTypeVideoData        uint8 = 0x09
	MessageTypeAMF3Data         uint8 = 0x0F
	MessageTypeAMF3SharedObject uint8 = 0x10
	MessageTypeAMF3Command      uint8 = 0x11
	MessageTypeAMF0Data         uint8 = 0x12
	MessageTypeAMF0SharedObject uint8 = 0x13
	MessageTypeAMF0Command      uint8 = 0x14
	MessageTypeAggregate        uint8 = 0x16
	MessageTypeAMF0Metadata     uint8 = 0x17
)

// RTMP Command Types

// Command represents an RTMP command (AMF encoded)
type Command struct {
	Name          string
	TransactionID float64
	CommandObject interface{} // map[string]interface{} or nil
	Args          []interface{}
}

// CommandResponse represents an RTMP command response
type CommandResponse struct {
	Name          string
	TransactionID float64
	CommandObject map[string]interface{}
	Properties    map[string]interface{}
	Information   map[string]interface{}
	RawBytes      []byte // Pre-encoded AMF bytes (used for special cases like getStreamLength)
}

// Common RTMP commands
const (
	CommandConnect     = "connect"
	CommandDisconnect  = "disconnect"
	CommandCreateStream = "createStream"
	CommandDeleteStream = "deleteStream"
	CommandPublish     = "publish"
	CommandPlay        = "play"
	CommandPlay2       = "play2"
	CommandSeek        = "seek"
	CommandPause       = "pause"
	CommandReleaseStream = "releaseStream"
	CommandFCPublish   = "FCPublish"
	CommandFCUnpublish = "FCUnpublish"
	CommandGetStreamLength = "getStreamLength"
	
	// Response commands
	CommandResult      = "_result"
	CommandError       = "_error"
	CommandOnStatus    = "onStatus"
	CommandOnBWDone    = "onBWDone"
	CommandOnFCSubscribe = "onFCSubscribe"
)

// RTMP Handshake Types

// HandshakeState represents the state of RTMP handshake
type HandshakeState uint8

const (
	HandshakeStateC0 HandshakeState = iota
	HandshakeStateC1
	HandshakeStateC2
	HandshakeStateS0
	HandshakeStateS1
	HandshakeStateS2
	HandshakeStateComplete
)
