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

package domain

import "fmt"

// DomainError represents a domain-specific error
type DomainError struct {
	Code    string
	Message string
}

func (e *DomainError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Domain errors
var (
	ErrConnectionNotFound    = &DomainError{Code: "CONNECTION_NOT_FOUND", Message: "Connection not found"}
	ErrConnectionClosed      = &DomainError{Code: "CONNECTION_CLOSED", Message: "Connection is closed"}
	ErrInvalidAppName        = &DomainError{Code: "INVALID_APP_NAME", Message: "Invalid application name"}
	ErrInvalidStreamName     = &DomainError{Code: "INVALID_STREAM_NAME", Message: "Invalid stream name"}
	ErrStreamNotFound        = &DomainError{Code: "STREAM_NOT_FOUND", Message: "Stream not found"}
	ErrStreamAlreadyExists   = &DomainError{Code: "STREAM_ALREADY_EXISTS", Message: "Stream already exists"}
	ErrStreamNotPublishing   = &DomainError{Code: "STREAM_NOT_PUBLISHING", Message: "Stream is not publishing"}
	ErrStreamAlreadyPublishing = &DomainError{Code: "STREAM_ALREADY_PUBLISHING", Message: "Stream is already being published"}
	ErrPublisherNotFound     = &DomainError{Code: "PUBLISHER_NOT_FOUND", Message: "Publisher not found"}
	ErrViewerNotFound        = &DomainError{Code: "VIEWER_NOT_FOUND", Message: "Viewer not found"}
)
