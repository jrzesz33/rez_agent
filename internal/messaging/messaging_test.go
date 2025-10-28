package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jrzesz33/rez_agent/internal/models"
)

func TestNewSQSBatchProcessor(t *testing.T) {
	processor := NewSQSBatchProcessor(nil)

	if processor == nil {
		t.Fatal("NewSQSBatchProcessor() returned nil")
	}
	if processor.logger == nil {
		t.Error("logger should not be nil when default is used")
	}
}

func TestParseSQSEvent(t *testing.T) {
	message := models.NewMessage("test-system", models.StageDev, models.MessageTypeHelloWorld, "test payload")
	messageJSON, _ := json.Marshal(message)

	// Create SNS-wrapped message properly
	snsWrapper := map[string]string{"Message": string(messageJSON)}
	snsWrapperJSON, _ := json.Marshal(snsWrapper)

	tests := []struct {
		name    string
		event   events.SQSEvent
		wantLen int
		wantErr bool
	}{
		{
			name: "valid SNS-wrapped message",
			event: events.SQSEvent{
				Records: []events.SQSMessage{
					{
						MessageId: "msg-1",
						Body:      string(snsWrapperJSON),
					},
				},
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "multiple SNS-wrapped messages",
			event: events.SQSEvent{
				Records: []events.SQSMessage{
					{
						MessageId: "msg-3",
						Body:      string(snsWrapperJSON),
					},
					{
						MessageId: "msg-4",
						Body:      string(snsWrapperJSON),
					},
				},
			},
			wantLen: 2,
			wantErr: false,
		},
		{
			name: "invalid JSON",
			event: events.SQSEvent{
				Records: []events.SQSMessage{
					{
						MessageId: "msg-5",
						Body:      "invalid json",
					},
				},
			},
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := ParseSQSEvent(tt.event, slog.Default())
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSQSEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(messages) != tt.wantLen {
				t.Errorf("ParseSQSEvent() returned %d messages, want %d", len(messages), tt.wantLen)
			}
		})
	}
}

func TestSQSBatchProcessor_ProcessBatch(t *testing.T) {
	message := models.NewMessage("test-system", models.StageDev, models.MessageTypeHelloWorld, "test payload")
	messageJSON, _ := json.Marshal(message)

	// Create SNS-wrapped message properly
	snsWrapper := map[string]string{"Message": string(messageJSON)}
	snsWrapperJSON, _ := json.Marshal(snsWrapper)

	tests := []struct {
		name             string
		event            events.SQSEvent
		handler          func(context.Context, *models.Message) error
		wantFailureCount int
	}{
		{
			name: "all messages succeed",
			event: events.SQSEvent{
				Records: []events.SQSMessage{
					{
						MessageId: "msg-1",
						Body:      string(snsWrapperJSON),
					},
				},
			},
			handler: func(ctx context.Context, msg *models.Message) error {
				return nil
			},
			wantFailureCount: 0,
		},
		{
			name: "all messages fail",
			event: events.SQSEvent{
				Records: []events.SQSMessage{
					{
						MessageId: "msg-2",
						Body:      string(snsWrapperJSON),
					},
				},
			},
			handler: func(ctx context.Context, msg *models.Message) error {
				return errors.New("processing failed")
			},
			wantFailureCount: 1,
		},
		{
			name: "mixed success and failure",
			event: events.SQSEvent{
				Records: []events.SQSMessage{
					{
						MessageId: "msg-3",
						Body:      string(snsWrapperJSON),
					},
					{
						MessageId: "msg-4",
						Body:      string(snsWrapperJSON),
					},
				},
			},
			handler: func() func(context.Context, *models.Message) error {
				count := 0
				return func(ctx context.Context, msg *models.Message) error {
					count++
					if count == 1 {
						return nil // First succeeds
					}
					return errors.New("second message failed") // Second fails
				}
			}(),
			wantFailureCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewSQSBatchProcessor(slog.Default())
			ctx := context.Background()

			response, _ := processor.ProcessBatch(ctx, tt.event, tt.handler)

			if len(response.BatchItemFailures) != tt.wantFailureCount {
				t.Errorf("ProcessBatch() returned %d failures, want %d", len(response.BatchItemFailures), tt.wantFailureCount)
			}
		})
	}
}
