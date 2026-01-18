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
