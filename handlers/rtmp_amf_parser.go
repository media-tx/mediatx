package handlers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// AMFParser parses AMF0/AMF3 encoded data
type AMFParser struct {
	// For now, simplified AMF0 parser
	// Full implementation would support AMF0 and AMF3
}

// NewAMFParser creates a new AMF parser
func NewAMFParser() *AMFParser {
	return &AMFParser{}
}

// DecodeCommand decodes an AMF command from message payload
func (p *AMFParser) DecodeCommand(payload []byte) (*Command, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("payload too short")
	}

	// Simplified: For now, just return a basic command structure
	// Full implementation would parse AMF0/AMF3 format
	// AMF0 format:
	//   - String: 2 bytes length (big-endian) + UTF-8 data
	//   - Number: 8 bytes (double, big-endian)
	//   - Object: key-value pairs + 0x000009 terminator
	//   - Array: strict array or ECMA array
	//   - Null, Boolean, Date, XML, etc.

	// Parse command name (first AMF string)
	// AMF commands start with type marker (0x02 for String)
	// decodeValue handles the type marker automatically
	cmdNameValue, offset, err := p.decodeValue(payload, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to decode command name: %w", err)
	}
	
	commandName, ok := cmdNameValue.(string)
	if !ok {
		return nil, fmt.Errorf("command name must be a string, got %T", cmdNameValue)
	}

	// Parse transaction ID (AMF number)
	// decodeValue handles the type marker automatically
	txnValue, offset, err := p.decodeValue(payload, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to decode transaction ID: %w", err)
	}
	
	transactionID, ok := txnValue.(float64)
	if !ok {
		return nil, fmt.Errorf("transaction ID must be a number, got %T", txnValue)
	}

	// Parse command object (AMF object or null)
	// Check if next value is null
	var commandObject map[string]interface{}
	if offset < len(payload) && payload[offset] == AMF0Null {
		_, offset, _ = p.decodeValue(payload, offset)
		commandObject = make(map[string]interface{})
	} else {
		// decodeValue handles the type marker automatically
		objValue, newOffset, err := p.decodeValue(payload, offset)
		if err != nil {
			// Command object might be missing or null, that's OK
			commandObject = make(map[string]interface{})
		} else {
			offset = newOffset
			commandObject, ok = objValue.(map[string]interface{})
			if !ok {
				// If not an object, treat as empty
				commandObject = make(map[string]interface{})
			}
		}
	}

	// Parse command arguments (remaining AMF values)
	args := make([]interface{}, 0)
	for offset < len(payload) {
		arg, newOffset, err := p.decodeValue(payload, offset)
		if err != nil {
			break
		}
		args = append(args, arg)
		offset = newOffset
	}

	return &Command{
		Name:          commandName,
		TransactionID: transactionID,
		CommandObject: commandObject,
		Args:          args,
	}, nil
}

// EncodeResponse encodes a command response to AMF
func (p *AMFParser) EncodeResponse(response *CommandResponse) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Encode command name (string with type marker)
	buf.WriteByte(AMF0String)
	if err := p.encodeString(buf, response.Name); err != nil {
		return nil, err
	}

	// Encode transaction ID (number with type marker)
	buf.WriteByte(AMF0Number)
	if err := p.encodeNumber(buf, response.TransactionID); err != nil {
		return nil, err
	}

	// Encode command object (with type marker 0x03 for AMF0 Object)
	if response.CommandObject == nil || len(response.CommandObject) == 0 {
		// Null or empty object - encode as null (0x05) for command object
		buf.WriteByte(AMF0Null)
	} else {
		buf.WriteByte(AMF0Object)
		if err := p.encodeObject(buf, response.CommandObject); err != nil {
			return nil, err
		}
	}

	// Encode properties/information if present (with type marker 0x03)
	// Only encode if map is not nil AND not empty
	if response.Properties != nil && len(response.Properties) > 0 {
		buf.WriteByte(AMF0Object)
		if err := p.encodeObject(buf, response.Properties); err != nil {
			return nil, err
		}
	}

	if response.Information != nil && len(response.Information) > 0 {
		buf.WriteByte(AMF0Object)
		if err := p.encodeObject(buf, response.Information); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// EncodeCreateStreamResult encodes a createStream _result response
// Format: _result (string) + transactionID (number) + null + streamID (number directly)
// Per RTMP spec: createStream returns the stream ID as a direct number, not in an object
func (p *AMFParser) EncodeCreateStreamResult(name string, transactionID float64, streamID float64) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Encode command name (_result)
	buf.WriteByte(AMF0String)
	if err := p.encodeString(buf, name); err != nil {
		return nil, err
	}

	// Encode transaction ID
	buf.WriteByte(AMF0Number)
	if err := p.encodeNumber(buf, transactionID); err != nil {
		return nil, err
	}

	// Encode null (command object)
	buf.WriteByte(AMF0Null)

	// Encode stream ID directly as a number (not in an object!)
	buf.WriteByte(AMF0Number)
	if err := p.encodeNumber(buf, streamID); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// EncodeConnectResult encodes a connect command _result response with null + Properties + Information (for max compatibility)
// Format: _result (string) + transactionID (number) + null + Properties object + Information object
// Per user's example: includes null between transactionID and Properties for max compatibility
func (p *AMFParser) EncodeConnectResult(response *CommandResponse) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Encode command name (string with type marker)
	buf.WriteByte(AMF0String)
	if err := p.encodeString(buf, response.Name); err != nil {
		return nil, err
	}

	// Encode transaction ID (number with type marker)
	buf.WriteByte(AMF0Number)
	if err := p.encodeNumber(buf, response.TransactionID); err != nil {
		return nil, err
	}

	// Encode null (for max compatibility, per user's example)
	buf.WriteByte(AMF0Null)

	// Encode Properties object (first argument)
	if response.Properties != nil && len(response.Properties) > 0 {
		buf.WriteByte(AMF0Object)
		if err := p.encodeObject(buf, response.Properties); err != nil {
			return nil, err
		}
	} else {
		// If no properties, encode empty object
		buf.WriteByte(AMF0Object)
		buf.WriteByte(0x00)
		buf.WriteByte(0x00)
		buf.WriteByte(AMF0ObjectEnd)
	}

	// Encode Information object (second argument)
	if response.Information != nil && len(response.Information) > 0 {
		buf.WriteByte(AMF0Object)
		if err := p.encodeObject(buf, response.Information); err != nil {
			return nil, err
		}
	} else {
		// If no information, encode empty object
		buf.WriteByte(AMF0Object)
		buf.WriteByte(0x00)
		buf.WriteByte(0x00)
		buf.WriteByte(AMF0ObjectEnd)
	}

	return buf.Bytes(), nil
}

// EncodeGetStreamLengthResult encodes a getStreamLength command _result response
// Format: _result (string) + transactionID (number) + null + duration (number)
// Duration is a direct number value, not wrapped in an object
func (p *AMFParser) EncodeGetStreamLengthResult(commandName string, transactionID float64, duration float64) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Encode command name (string with type marker)
	buf.WriteByte(AMF0String)
	if err := p.encodeString(buf, commandName); err != nil {
		return nil, err
	}

	// Encode transaction ID (number with type marker)
	buf.WriteByte(AMF0Number)
	if err := p.encodeNumber(buf, transactionID); err != nil {
		return nil, err
	}

	// Encode null (CommandObject)
	buf.WriteByte(AMF0Null)

	// Encode duration as direct number (not in object)
	buf.WriteByte(AMF0Number)
	if err := p.encodeNumber(buf, duration); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// AMF0 Type Markers
const (
	AMF0Number       = 0x00
	AMF0Boolean      = 0x01
	AMF0String       = 0x02
	AMF0Object       = 0x03
	AMF0MovieClip    = 0x04
	AMF0Null         = 0x05
	AMF0Undefined    = 0x06
	AMF0Reference    = 0x07
	AMF0ECMAArray    = 0x08
	AMF0ObjectEnd    = 0x09
	AMF0StrictArray  = 0x0A
	AMF0Date         = 0x0B
	AMF0LongString   = 0x0C
	AMF0Unsupported  = 0x0D
	AMF0RecordSet    = 0x0E
	AMF0XMLDocument  = 0x0F
	AMF0TypedObject  = 0x10
	AMF0AVMPlus      = 0x11
)

// decodeString decodes AMF0 string
func (p *AMFParser) decodeString(data []byte, offset int) (string, int, error) {
	if offset+2 > len(data) {
		return "", offset, fmt.Errorf("insufficient data for string length")
	}

	length := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2

	if offset+length > len(data) {
		return "", offset, fmt.Errorf("insufficient data for string")
	}

	value := string(data[offset : offset+length])
	offset += length

	return value, offset, nil
}

// decodeNumber decodes AMF0 number (double)
func (p *AMFParser) decodeNumber(data []byte, offset int) (float64, int, error) {
	if offset+8 > len(data) {
		return 0, offset, fmt.Errorf("insufficient data for number")
	}

	bits := binary.BigEndian.Uint64(data[offset : offset+8])
	value := math.Float64frombits(bits)
	offset += 8

	return value, offset, nil
}

// decodeObject decodes AMF0 object
func (p *AMFParser) decodeObject(data []byte, offset int) (map[string]interface{}, int, error) {
	obj := make(map[string]interface{})

	for offset < len(data) {
		// Check for object end marker: empty string key (0x00 0x00) + marker (0x09) = 3 bytes
		// Per gortmplib data.go:162-164: check for empty string key length (0x00 0x00) then marker (0x09)
		if offset+3 <= len(data) && data[offset] == 0x00 && data[offset+1] == 0x00 && data[offset+2] == AMF0ObjectEnd {
			offset += 3
			break
		}

		// Read key (string)
		key, newOffset, err := p.decodeString(data, offset)
		if err != nil {
			break
		}
		offset = newOffset

		// Read value
		value, newOffset, err := p.decodeValue(data, offset)
		if err != nil {
			break
		}
		offset = newOffset

		obj[key] = value
	}

	return obj, offset, nil
}

// decodeValue decodes an AMF0 value (dispatches based on type marker)
func (p *AMFParser) decodeValue(data []byte, offset int) (interface{}, int, error) {
	if offset >= len(data) {
		return nil, offset, fmt.Errorf("insufficient data for type marker")
	}

	marker := data[offset]
	offset++

	switch marker {
	case AMF0Number:
		return p.decodeNumber(data, offset)
	case AMF0Boolean:
		if offset >= len(data) {
			return nil, offset, fmt.Errorf("insufficient data for boolean")
		}
		value := data[offset] != 0
		offset++
		return value, offset, nil
	case AMF0String:
		return p.decodeString(data, offset)
	case AMF0Object:
		return p.decodeObject(data, offset)
	case AMF0Null:
		return nil, offset, nil
	case AMF0Undefined:
		return nil, offset, nil
	default:
		return nil, offset, fmt.Errorf("unsupported AMF0 type marker: 0x%02x", marker)
	}
}

// encodeString encodes AMF0 string
func (p *AMFParser) encodeString(buf *bytes.Buffer, value string) error {
	length := uint16(len(value))
	if err := binary.Write(buf, binary.BigEndian, length); err != nil {
		return err
	}
	_, err := buf.WriteString(value)
	return err
}

// encodeNumber encodes AMF0 number
func (p *AMFParser) encodeNumber(buf *bytes.Buffer, value float64) error {
	return binary.Write(buf, binary.BigEndian, math.Float64bits(value))
}

// encodeObject encodes AMF0 object
// Note: For connect response, both Properties and Information objects need specific field order
// Properties: fmsVer, capabilities (as per gortmplib and nginx-rtmp-module)
// Information: level, code, description, objectEncoding (as per gortmplib)
func (p *AMFParser) encodeObject(buf *bytes.Buffer, obj map[string]interface{}) error {
	// Check if this is a Properties object (has "fmsVer" key)
	isPropertiesObject := false
	if _, hasFmsVer := obj["fmsVer"]; hasFmsVer {
		if _, hasCapabilities := obj["capabilities"]; hasCapabilities {
			isPropertiesObject = true
		}
	}

	// Check if this is an Information object (has "level" key with value "status")
	// AND has "code" key (typical of Information object in connect response)
	isInformationObject := false
	if levelVal, hasLevel := obj["level"]; hasLevel {
		if levelStr, ok := levelVal.(string); ok && levelStr == "status" {
			if _, hasCode := obj["code"]; hasCode {
				isInformationObject = true
			}
		}
	}

	if isPropertiesObject {
		// This is a Properties object - encode in specific order (fmsVer first, then capabilities)
		// Per gortmplib and nginx-rtmp-module: fmsVer should come before capabilities
		orderedKeys := []string{"fmsVer", "capabilities"}
		for _, key := range orderedKeys {
			if value, exists := obj[key]; exists {
				if err := p.encodeString(buf, key); err != nil {
					return err
				}
				if err := p.encodeValue(buf, value); err != nil {
					return err
				}
			}
		}
		// Encode any remaining keys (shouldn't happen for Properties object, but just in case)
		for key, value := range obj {
			skip := false
			for _, orderedKey := range orderedKeys {
				if key == orderedKey {
					skip = true
					break
				}
			}
			if !skip {
				if err := p.encodeString(buf, key); err != nil {
					return err
				}
				if err := p.encodeValue(buf, value); err != nil {
					return err
				}
			}
		}
	} else if isInformationObject {
		// This is an Information object - encode in specific order (as per gortmplib)
		orderedKeys := []string{"level", "code", "description", "objectEncoding"}
		for _, key := range orderedKeys {
			if value, exists := obj[key]; exists {
				if err := p.encodeString(buf, key); err != nil {
					return err
				}
				if err := p.encodeValue(buf, value); err != nil {
					return err
				}
			}
		}
		// Encode any remaining keys (shouldn't happen for Information object, but just in case)
		for key, value := range obj {
			skip := false
			for _, orderedKey := range orderedKeys {
				if key == orderedKey {
					skip = true
					break
				}
			}
			if !skip {
				if err := p.encodeString(buf, key); err != nil {
					return err
				}
				if err := p.encodeValue(buf, value); err != nil {
					return err
				}
			}
		}
	} else {
		// For other objects, use random order (Go map iteration)
		for key, value := range obj {
			if err := p.encodeString(buf, key); err != nil {
				return err
			}
			if err := p.encodeValue(buf, value); err != nil {
				return err
			}
		}
	}
	// Object end marker: empty string key (0x00 0x00) + marker (0x09) = 3 bytes total
	// Per gortmplib data.go:390-392 and METERIAL4.md line 44: must be 0x00, 0x00, 0x09
	if err := buf.WriteByte(0x00); err != nil {
		return err
	}
	if err := buf.WriteByte(0x00); err != nil {
		return err
	}
	return buf.WriteByte(AMF0ObjectEnd) // 0x09
}

// encodeValue encodes an AMF0 value
func (p *AMFParser) encodeValue(buf *bytes.Buffer, value interface{}) error {
	switch v := value.(type) {
	case float64:
		buf.WriteByte(AMF0Number)
		return p.encodeNumber(buf, v)
	case bool:
		buf.WriteByte(AMF0Boolean)
		if v {
			return buf.WriteByte(1)
		}
		return buf.WriteByte(0)
	case string:
		buf.WriteByte(AMF0String)
		return p.encodeString(buf, v)
	case map[string]interface{}:
		buf.WriteByte(AMF0Object)
		return p.encodeObject(buf, v)
	case nil:
		return buf.WriteByte(AMF0Null)
	default:
		return fmt.Errorf("unsupported AMF0 type: %T", value)
	}
}
