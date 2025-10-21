package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/yourusername/rez_agent/internal/models"
)

// SNSPublisher defines the interface for publishing messages to SNS
type SNSPublisher interface {
	PublishMessage(ctx context.Context, message *models.Message) error
}

// SNSClient implements SNSPublisher using AWS SNS
type SNSClient struct {
	client   *sns.Client
	topicArn string
	logger   *slog.Logger
}

// NewSNSClient creates a new SNS client instance
func NewSNSClient(client *sns.Client, topicArn string, logger *slog.Logger) *SNSClient {
	if logger == nil {
		logger = slog.Default()
	}

	return &SNSClient{
		client:   client,
		topicArn: topicArn,
		logger:   logger,
	}
}

// PublishMessage publishes a message to the SNS topic
func (s *SNSClient) PublishMessage(ctx context.Context, message *models.Message) error {
	// Serialize message to JSON
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	// Publish to SNS
	input := &sns.PublishInput{
		TopicArn: aws.String(s.topicArn),
		Message:  aws.String(string(messageBytes)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"stage": {
				DataType:    aws.String("String"),
				StringValue: aws.String(message.Stage.String()),
			},
			"message_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String(message.MessageType.String()),
			},
			"status": {
				DataType:    aws.String("String"),
				StringValue: aws.String(message.Status.String()),
			},
		},
	}

	result, err := s.client.Publish(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to publish message to SNS: %w", err)
	}

	s.logger.InfoContext(ctx, "message published to SNS",
		slog.String("message_id", message.ID),
		slog.String("sns_message_id", aws.ToString(result.MessageId)),
		slog.String("topic_arn", s.topicArn),
	)

	return nil
}
