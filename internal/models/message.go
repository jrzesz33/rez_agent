package models

import (
	"time"
)

// Stage represents the deployment environment
type Stage string

const (
	// StageDev represents the development environment
	StageDev Stage = "dev"
	// StageStage represents the staging environment
	StageStage Stage = "stage"
	// StageProd represents the production environment
	StageProd Stage = "prod"
)

// IsValid checks if the stage value is valid
func (s Stage) IsValid() bool {
	switch s {
	case StageDev, StageStage, StageProd:
		return true
	default:
		return false
	}
}

// String returns the string representation of the stage
func (s Stage) String() string {
	return string(s)
}

// Status represents the current state of a message
type Status string

const (
	// StatusCreated indicates the message has been created but not yet queued
	StatusCreated Status = "created"
	// StatusQueued indicates the message has been queued for processing
	StatusQueued Status = "queued"
	// StatusProcessing indicates the message is currently being processed
	StatusProcessing Status = "processing"
	// StatusCompleted indicates the message has been successfully processed
	StatusCompleted Status = "completed"
	// StatusFailed indicates the message processing has failed
	StatusFailed Status = "failed"
)

// IsValid checks if the status value is valid
func (s Status) IsValid() bool {
	switch s {
	case StatusCreated, StatusQueued, StatusProcessing, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

// String returns the string representation of the status
func (s Status) String() string {
	return string(s)
}

// MessageType represents the type of message
type MessageType string

const (
	// MessageTypeHelloWorld is a simple hello world message type
	MessageTypeHelloWorld MessageType = "hello_world"
	// MessageTypeManual is a manually created message
	MessageTypeNotification MessageType = "notify"
	// MessageTypeScheduled is a scheduled message
	MessageTypeScheduled MessageType = "scheduled"
	// MessageTypeWebAction is a web action request (HTTP REST API call)
	MessageTypeWebAction MessageType = "web_action"
)

// IsValid checks if the message type value is valid
func (mt MessageType) IsValid() bool {
	switch mt {
	case MessageTypeHelloWorld, MessageTypeNotification, MessageTypeScheduled, MessageTypeWebAction:
		return true
	default:
		return false
	}
}

// String returns the string representation of the message type
func (mt MessageType) String() string {
	return string(mt)
}

// Message represents a message in the system with metadata and payload
type Message struct {
	// ID is the unique identifier for the message
	ID string `json:"id" dynamodbav:"id"`

	// CreatedDate is when the message was created
	CreatedDate time.Time `json:"created_date" dynamodbav:"created_date"`

	// CreatedBy is the system or user that created the message
	CreatedBy string `json:"created_by" dynamodbav:"created_by"`

	// Stage is the target environment (dev, stage, prod)
	Stage Stage `json:"stage" dynamodbav:"stage"`

	// MessageType is the type of message
	MessageType MessageType `json:"message_type" dynamodbav:"message_type"`

	// Status is the current state of the message
	Status Status `json:"status" dynamodbav:"status"`

	// Payload is the message content
	Payload string `json:"payload" dynamodbav:"payload"`

	// UpdatedDate is when the message was last updated
	UpdatedDate time.Time `json:"updated_date,omitempty" dynamodbav:"updated_date,omitempty"`

	// ErrorMessage contains error details if Status is Failed
	ErrorMessage string `json:"error_message,omitempty" dynamodbav:"error_message,omitempty"`

	// RetryCount tracks the number of retry attempts
	RetryCount int `json:"retry_count" dynamodbav:"retry_count"`
}

// NewMessage creates a new message with default values
func NewMessage(createdBy string, stage Stage, messageType MessageType, payload string) *Message {
	now := time.Now().UTC()
	return &Message{
		ID:          generateMessageID(now),
		CreatedDate: now,
		CreatedBy:   createdBy,
		Stage:       stage,
		MessageType: messageType,
		Status:      StatusCreated,
		Payload:     payload,
		UpdatedDate: now,
		RetryCount:  0,
	}
}

// generateMessageID generates a unique message ID based on timestamp and random component
func generateMessageID(t time.Time) string {
	// Format: msg_<unix_timestamp>_<nanoseconds>
	return "msg_" + t.Format("20060102150405") + "_" + string(rune(t.Nanosecond()%1000000))
}

// MarkQueued updates the message status to queued
func (m *Message) MarkQueued() {
	m.Status = StatusQueued
	m.UpdatedDate = time.Now().UTC()
}

// MarkProcessing updates the message status to processing
func (m *Message) MarkProcessing() {
	m.Status = StatusProcessing
	m.UpdatedDate = time.Now().UTC()
}

// MarkCompleted updates the message status to completed
func (m *Message) MarkCompleted() {
	m.Status = StatusCompleted
	m.UpdatedDate = time.Now().UTC()
}

// MarkFailed updates the message status to failed with an error message
func (m *Message) MarkFailed(errorMessage string) {
	m.Status = StatusFailed
	m.ErrorMessage = errorMessage
	m.UpdatedDate = time.Now().UTC()
}

// IncrementRetry increments the retry count
func (m *Message) IncrementRetry() {
	m.RetryCount++
	m.UpdatedDate = time.Now().UTC()
}
