package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/jrzesz33/rez_agent/internal/models"
)

// SNSPublisher defines the interface for publishing messages to SNS
type SNSPublisher interface {
	PublishMessage(ctx context.Context, message *models.Message) error
}

/* SNSClient implements SNSPublisher using AWS SNS with a single topic
type SNSClient struct {
	client   *sns.Client
	topicArn string
	logger   *slog.Logger
}

// NewSNSClient creates a new SNS client instance for a single topic
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
*/

// TopicRoutingSNSClient implements SNSPublisher with message-type-based topic routing
type TopicRoutingSNSClient struct {
	client                *sns.Client
	webActionsTopicArn    string
	notificationsTopicArn string
	logger                *slog.Logger
}

// NewTopicRoutingSNSClient creates a new topic-routing SNS client
func NewTopicRoutingSNSClient(client *sns.Client, webActionsTopicArn, notificationsTopicArn string, logger *slog.Logger) *TopicRoutingSNSClient {
	if logger == nil {
		logger = slog.Default()
	}

	return &TopicRoutingSNSClient{
		client:                client,
		webActionsTopicArn:    webActionsTopicArn,
		notificationsTopicArn: notificationsTopicArn,
		logger:                logger,
	}
}

// getTopicForMessageType returns the appropriate topic ARN based on message type
func (s *TopicRoutingSNSClient) getTopicForMessageType(messageType models.MessageType) string {
	switch messageType {
	case models.MessageTypeWebAction:
		return s.webActionsTopicArn
	default:
		// All other message types (scheduled, manual, hello_world) go to notifications topic
		return s.notificationsTopicArn
	}
}

// PublishMessage publishes a message to the appropriate topic based on message type
func (s *TopicRoutingSNSClient) PublishMessage(ctx context.Context, message *models.Message) error {
	// Determine which topic to use based on message type
	topicArn := s.getTopicForMessageType(message.MessageType)

	// Serialize message to JSON
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	// Publish to SNS
	input := &sns.PublishInput{
		TopicArn: aws.String(topicArn),
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
		return fmt.Errorf("failed to publish message to SNS topic %s: %w", topicArn, err)
	}

	s.logger.InfoContext(ctx, "message published to topic-routed SNS",
		slog.String("message_id", message.ID),
		slog.String("message_type", message.MessageType.String()),
		slog.String("sns_message_id", aws.ToString(result.MessageId)),
		slog.String("topic_arn", topicArn),
	)

	return nil
}
