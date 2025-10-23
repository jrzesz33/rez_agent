package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jrzesz33/rez_agent/internal/models"
)

// SQSMessageWrapper represents the structure of messages received from SQS
type SQSMessageWrapper struct {
	Message *models.Message `json:"Message"`
}

// ParseSQSEvent extracts messages from an SQS event
func ParseSQSEvent(event events.SQSEvent, logger *slog.Logger) ([]*models.Message, error) {
	if logger == nil {
		logger = slog.Default()
	}

	messages := make([]*models.Message, 0, len(event.Records))

	for _, record := range event.Records {
		// The message body from SNS-to-SQS subscription is wrapped
		var snsMessage struct {
			Message string `json:"Message"`
		}

		// First, try to unmarshal as SNS message
		err := json.Unmarshal([]byte(record.Body), &snsMessage)
		if err != nil {
			logger.Warn("failed to unmarshal SNS wrapper, trying direct unmarshal",
				slog.String("error", err.Error()),
				slog.String("message_id", record.MessageId),
			)
			// If that fails, try direct unmarshal
			snsMessage.Message = record.Body
		}

		// Now unmarshal the actual message
		var message models.Message
		err = json.Unmarshal([]byte(snsMessage.Message), &message)
		if err != nil {
			logger.Error("failed to unmarshal message",
				slog.String("error", err.Error()),
				slog.String("message_id", record.MessageId),
			)
			return nil, fmt.Errorf("failed to unmarshal message from SQS record %s: %w", record.MessageId, err)
		}

		logger.Info("parsed message from SQS",
			slog.String("message_id", message.ID),
			slog.String("sqs_message_id", record.MessageId),
			slog.String("stage", message.Stage.String()),
			slog.String("status", message.Status.String()),
		)

		messages = append(messages, &message)
	}

	return messages, nil
}

// SQSBatchProcessor processes SQS messages in batch
type SQSBatchProcessor struct {
	logger *slog.Logger
}

// NewSQSBatchProcessor creates a new SQS batch processor
func NewSQSBatchProcessor(logger *slog.Logger) *SQSBatchProcessor {
	if logger == nil {
		logger = slog.Default()
	}

	return &SQSBatchProcessor{
		logger: logger,
	}
}

// ProcessBatch processes a batch of SQS messages
func (p *SQSBatchProcessor) ProcessBatch(ctx context.Context, event events.SQSEvent, handler func(context.Context, *models.Message) error) (events.SQSEventResponse, error) {
	response := events.SQSEventResponse{
		BatchItemFailures: []events.SQSBatchItemFailure{},
	}

	messages, err := ParseSQSEvent(event, p.logger)
	if err != nil {
		p.logger.ErrorContext(ctx, "failed to parse SQS event", slog.String("error", err.Error()))
		// Mark all messages as failed
		for _, record := range event.Records {
			response.BatchItemFailures = append(response.BatchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
		}
		return response, err
	}

	// Process each message
	for i, message := range messages {
		record := event.Records[i]

		err := handler(ctx, message)
		if err != nil {
			p.logger.ErrorContext(ctx, "failed to process message",
				slog.String("message_id", message.ID),
				slog.String("sqs_message_id", record.MessageId),
				slog.String("error", err.Error()),
			)
			// Add to batch item failures for retry
			response.BatchItemFailures = append(response.BatchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
		} else {
			p.logger.InfoContext(ctx, "successfully processed message",
				slog.String("message_id", message.ID),
				slog.String("sqs_message_id", record.MessageId),
			)
		}
	}

	return response, nil
}
