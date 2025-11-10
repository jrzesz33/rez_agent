package models

import (
	"testing"
	"time"
)

func TestStage_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		stage Stage
		want  bool
	}{
		{"dev is valid", StageDev, true},
		{"stage is valid", StageStage, true},
		{"prod is valid", StageProd, true},
		{"invalid stage", Stage("invalid"), false},
		{"empty stage", Stage(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stage.IsValid(); got != tt.want {
				t.Errorf("Stage.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStage_String(t *testing.T) {
	tests := []struct {
		name  string
		stage Stage
		want  string
	}{
		{"dev to string", StageDev, "dev"},
		{"stage to string", StageStage, "stage"},
		{"prod to string", StageProd, "prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stage.String(); got != tt.want {
				t.Errorf("Stage.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"created is valid", StatusCreated, true},
		{"queued is valid", StatusQueued, true},
		{"processing is valid", StatusProcessing, true},
		{"completed is valid", StatusCompleted, true},
		{"failed is valid", StatusFailed, true},
		{"invalid status", Status("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("Status.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessageType_IsValid(t *testing.T) {
	tests := []struct {
		name        string
		messageType MessageType
		want        bool
	}{
		{"hello_world is valid", MessageTypeHelloWorld, true},
		{"notify is valid", MessageTypeNotification, true},
		{"scheduled is valid", MessageTypeScheduled, true},
		{"invalid type", MessageType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.messageType.IsValid(); got != tt.want {
				t.Errorf("MessageType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewMessage(t *testing.T) {
	createdBy := "test-system"
	stage := StageDev
	messageType := MessageTypeHelloWorld
	payload := make(map[string]interface{})
	payload["key"] = "value"

	msg := NewMessage(createdBy, nil, "1.0", stage, messageType, payload)

	if msg.CreatedBy != createdBy {
		t.Errorf("NewMessage() CreatedBy = %v, want %v", msg.CreatedBy, createdBy)
	}
	if msg.Stage != stage {
		t.Errorf("NewMessage() Stage = %v, want %v", msg.Stage, stage)
	}
	if msg.MessageType != messageType {
		t.Errorf("NewMessage() MessageType = %v, want %v", msg.MessageType, messageType)
	}
	if msg.Payload != nil {
		t.Errorf("NewMessage() Payload = %v, want %v", msg.Payload, payload)
	}
	if msg.Status != StatusCreated {
		t.Errorf("NewMessage() Status = %v, want %v", msg.Status, StatusCreated)
	}
	if msg.RetryCount != 0 {
		t.Errorf("NewMessage() RetryCount = %v, want %v", msg.RetryCount, 0)
	}
	if msg.ID == "" {
		t.Error("NewMessage() ID is empty")
	}
	if msg.CreatedDate.IsZero() {
		t.Error("NewMessage() CreatedDate is zero")
	}
	if msg.UpdatedDate.IsZero() {
		t.Error("NewMessage() UpdatedDate is zero")
	}
}

func TestMessage_MarkQueued(t *testing.T) {
	_payload := make(map[string]interface{})
	_payload["key"] = "value"
	msg := NewMessage("test", nil, "1.0", StageDev, MessageTypeHelloWorld, _payload)
	originalUpdated := msg.UpdatedDate

	// Sleep to ensure time difference
	time.Sleep(10 * time.Millisecond)

	msg.MarkQueued()

	if msg.Status != StatusQueued {
		t.Errorf("MarkQueued() Status = %v, want %v", msg.Status, StatusQueued)
	}
	if !msg.UpdatedDate.After(originalUpdated) {
		t.Error("MarkQueued() did not update UpdatedDate")
	}
}

func TestMessage_MarkProcessing(t *testing.T) {
	_payload := make(map[string]interface{})
	_payload["key"] = "value"
	msg := NewMessage("test", nil, "1.0", StageDev, MessageTypeHelloWorld, _payload)
	originalUpdated := msg.UpdatedDate

	time.Sleep(10 * time.Millisecond)

	msg.MarkProcessing()

	if msg.Status != StatusProcessing {
		t.Errorf("MarkProcessing() Status = %v, want %v", msg.Status, StatusProcessing)
	}
	if !msg.UpdatedDate.After(originalUpdated) {
		t.Error("MarkProcessing() did not update UpdatedDate")
	}
}

func TestMessage_MarkCompleted(t *testing.T) {
	_payload := make(map[string]interface{})
	_payload["key"] = "value"
	msg := NewMessage("test", nil, "1.0", StageDev, MessageTypeHelloWorld, _payload)
	originalUpdated := msg.UpdatedDate

	time.Sleep(10 * time.Millisecond)

	msg.MarkCompleted()

	if msg.Status != StatusCompleted {
		t.Errorf("MarkCompleted() Status = %v, want %v", msg.Status, StatusCompleted)
	}
	if !msg.UpdatedDate.After(originalUpdated) {
		t.Error("MarkCompleted() did not update UpdatedDate")
	}
}

func TestMessage_MarkFailed(t *testing.T) {
	_payload := make(map[string]interface{})
	_payload["key"] = "value"
	msg := NewMessage("test", nil, "1.0", StageDev, MessageTypeHelloWorld, _payload)
	errorMessage := "test error"
	originalUpdated := msg.UpdatedDate

	time.Sleep(10 * time.Millisecond)

	msg.MarkFailed(errorMessage)

	if msg.Status != StatusFailed {
		t.Errorf("MarkFailed() Status = %v, want %v", msg.Status, StatusFailed)
	}
	if msg.ErrorMessage != errorMessage {
		t.Errorf("MarkFailed() ErrorMessage = %v, want %v", msg.ErrorMessage, errorMessage)
	}
	if !msg.UpdatedDate.After(originalUpdated) {
		t.Error("MarkFailed() did not update UpdatedDate")
	}
}

func TestMessage_IncrementRetry(t *testing.T) {
	_payload := make(map[string]interface{})
	_payload["key"] = "value"
	msg := NewMessage("test", nil, "1.0", StageDev, MessageTypeHelloWorld, _payload)
	originalRetryCount := msg.RetryCount
	originalUpdated := msg.UpdatedDate

	time.Sleep(10 * time.Millisecond)

	msg.IncrementRetry()

	if msg.RetryCount != originalRetryCount+1 {
		t.Errorf("IncrementRetry() RetryCount = %v, want %v", msg.RetryCount, originalRetryCount+1)
	}
	if !msg.UpdatedDate.After(originalUpdated) {
		t.Error("IncrementRetry() did not update UpdatedDate")
	}
}
