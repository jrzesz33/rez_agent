package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/logging"
	"github.com/jrzesz33/rez_agent/internal/messaging"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/repository"
	"github.com/jrzesz33/rez_agent/internal/secrets"
	"github.com/jrzesz33/rez_agent/internal/webaction"
	"github.com/jrzesz33/rez_agent/pkg/config"
)

type Handler struct {
	messageRepo           repository.MessageRepository
	resultRepo            repository.WebActionResultRepository
	snsPublisher          messaging.SNSPublisher
	handlerRegistry       *webaction.HandlerRegistry
	sqsProcessor          *messaging.SQSBatchProcessor
	agentResponseTopicArn string
	logger                *slog.Logger
}

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logging.GetLogLevel(),
	}))

	logger.Info("Web Action Function Starting...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", slog.String("error", err.Error()))
		panic(err)
	}

	// Initialize AWS SDK config
	awsCfg, err := awsConfig.LoadDefaultConfig(context.Background(),
		awsConfig.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("failed to load AWS SDK config", slog.String("error", err.Error()))
		panic(err)
	}
	logger.Info("Web Action Function Initialized Configuration")

	// Initialize AWS clients
	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	snsClient := sns.NewFromConfig(awsCfg)

	logger.Info("Initialized AWS Clients")

	// Initialize repositories
	messageRepo := repository.NewDynamoDBRepository(dynamoClient, cfg.DynamoDBTableName)
	resultRepo := repository.NewDynamoDBWebActionRepository(dynamoClient, cfg.WebActionResultsTableName)

	logger.Info("Initialized Repositories")

	// Initialize SNS publisher
	snsPublisher := messaging.NewTopicRoutingSNSClient(snsClient, cfg.WebActionsSNSTopicArn, cfg.NotificationsSNSTopicArn, cfg.AgentResponseTopicArn, cfg.ScheduleCreationTopicArn, logger)

	// Initialize SQS processor
	sqsProcessor := messaging.NewSQSBatchProcessor(logger)

	logger.Info("Initialized SNS & SQS")

	// Initialize HTTP client and secrets manager
	httpClient := httpclient.NewClient(logger)
	secretsManager := secrets.NewManager(awsCfg, logger)
	oauthClient := httpclient.NewOAuthClient(httpClient, secretsManager, logger)

	logger.Info("Initialized HTTP Clients and Secrets Manager")

	// Initialize action handler registry
	handlerRegistry := webaction.NewHandlerRegistry(logger)

	// Register handlers
	weatherHandler := webaction.NewWeatherHandler(httpClient, logger)
	if err := handlerRegistry.Register(weatherHandler); err != nil {
		logger.Error("failed to register weather handler", slog.String("error", err.Error()))
		panic(err)
	}

	golfHandler := webaction.NewGolfHandler(httpClient, oauthClient, secretsManager, logger)
	if err := handlerRegistry.Register(golfHandler); err != nil {
		logger.Error("failed to register golf handler", slog.String("error", err.Error()))
		panic(err)
	}

	logger.Info("web action processor initialized",
		slog.Int("registered_handlers", len(handlerRegistry.ListHandlers())),
	)

	// Create handler
	handler := &Handler{
		messageRepo:           messageRepo,
		resultRepo:            resultRepo,
		snsPublisher:          snsPublisher,
		handlerRegistry:       handlerRegistry,
		sqsProcessor:          sqsProcessor,
		agentResponseTopicArn: cfg.AgentResponseTopicArn,
		logger:                logger,
	}

	// Start Lambda
	lambda.Start(handler.HandleSQSEvent)
}

// HandleSQSEvent processes SQS messages containing web action requests
func (h *Handler) HandleSQSEvent(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	h.logger.Info("received SQS event",
		slog.Int("record_count", len(event.Records)),
	)

	// Use SQS batch processor to handle messages with proper retry logic
	return h.sqsProcessor.ProcessBatch(ctx, event, h.processMessage)
}

// processMessage processes a single message
func (h *Handler) processMessage(ctx context.Context, message *models.Message) error {
	startTime := time.Now()

	h.logger.Debug("processing web action message",
		slog.String("message_id", message.ID),
	)

	// Validate message type
	if message.MessageType != models.MessageTypeWebAction {
		return fmt.Errorf("invalid message type: expected %s, got %s", models.MessageTypeWebAction, message.MessageType)
	}

	// Update message status to processing
	message.MarkProcessing()
	if err := h.messageRepo.SaveMessage(ctx, message); err != nil {
		h.logger.Warn("failed to update message status to processing", slog.String("error", err.Error()))
	}

	// Parse web action payload
	payload, err := models.ParseWebActionPayload(message.Payload)
	if err != nil {
		h.markMessageFailed(ctx, message, fmt.Sprintf("Invalid payload: %v", err))
		return fmt.Errorf("failed to parse web action payload: %w", err)
	}

	// SECURITY: Log redacted payload
	redactedPayload := payload.RedactSensitiveData()
	h.logger.Debug("parsed web action payload",
		slog.String("action", redactedPayload.Action.String()),
		slog.String("url", redactedPayload.URL),
	)

	// Create web action result record
	result := models.NewWebActionResult(message.ID, payload.Action, payload.URL, message.Stage)
	if err := h.resultRepo.SaveResult(ctx, result); err != nil {
		h.logger.Warn("failed to save initial result", slog.String("error", err.Error()))
	}

	// Execute web action
	notificationMessage, err := h.executeWebAction(ctx, message.Arguments, payload)
	executionTime := time.Since(startTime)

	if err != nil {
		// Mark result as failed
		result.MarkFailure(err.Error(), executionTime.Milliseconds())
		if saveErr := h.resultRepo.SaveResult(ctx, result); saveErr != nil {
			h.logger.Warn("failed to save failed result", slog.String("error", saveErr.Error()))
		}

		// Mark message as failed
		h.markMessageFailed(ctx, message, err.Error())

		return fmt.Errorf("web action execution failed: %w", err)
	}

	// Mark result as successful
	result.MarkSuccess(200, strings.Join(notificationMessage, "\n"), executionTime.Milliseconds())
	if err := h.resultRepo.SaveResult(ctx, result); err != nil {
		h.logger.Warn("failed to save successful result", slog.String("error", err.Error()))
	}

	// Mark message as completed
	message.MarkCompleted()
	if err := h.messageRepo.SaveMessage(ctx, message); err != nil {
		h.logger.Warn("failed to update message status to completed", slog.String("error", err.Error()))
	}
	// Publish notification messages
	for i := range notificationMessage {
		// Publish notification message
		_msgPayload := make(map[string]interface{})
		_msgPayload["message"] = notificationMessage[i]
		if err := h.publishNotification(ctx, message, _msgPayload); err != nil {
			h.logger.Error("failed to publish notification",
				slog.String("error", err.Error()),
			)
			// Don't return error - the web action succeeded even if notification failed
		}
	}

	h.logger.Debug("web action completed successfully",
		slog.String("message_id", message.ID),
		slog.String("action", payload.Action.String()),
		slog.Duration("execution_time", executionTime),
	)

	return nil
}

// executeWebAction executes a web action using the appropriate handler
func (h *Handler) executeWebAction(ctx context.Context, args map[string]interface{}, payload *models.WebActionPayload) ([]string, error) {
	// Get handler for action type
	handler, err := h.handlerRegistry.GetHandler(payload.Action)
	if err != nil {
		return nil, fmt.Errorf("no handler available: %w", err)
	}

	// Execute action
	return handler.Execute(ctx, args, payload)
}

// publishNotification publishes a notification message to SNS
// If the original message was created by the AI agent, route response to agent topic
func (h *Handler) publishNotification(ctx context.Context, originalMessage *models.Message, notificationContent map[string]interface{}) error {
	// Check if message was created by AI agent
	isAgentMessage := originalMessage.CreatedBy == "ai-agent"

	// Create notification message
	var notificationMsg *models.Message
	if isAgentMessage {
		// For agent-created messages, send response back to agent topic
		notificationMsg = models.NewMessage(
			"web-action-processor", nil, "1.0",
			originalMessage.Stage,
			models.MessageTypeAgentResponse,
			notificationContent,
		)

		h.logger.Debug("routing response to agent",
			slog.String("original_message_id", originalMessage.ID),
			slog.String("created_by", originalMessage.CreatedBy),
		)
	} else {
		// For non-agent messages, use normal notification routing
		notificationMsg = models.NewMessage(
			"web-action-processor", nil, "1.0",
			originalMessage.Stage,
			models.MessageTypeNotification,
			notificationContent,
		)
	}

	// Save notification message
	if err := h.messageRepo.SaveMessage(ctx, notificationMsg); err != nil {
		return fmt.Errorf("failed to save notification message: %w", err)
	}

	// Publish to normal notification topic
	if err := h.snsPublisher.PublishMessage(ctx, notificationMsg); err != nil {
		return fmt.Errorf("failed to publish notification to SNS: %w", err)
	}

	h.logger.Debug("response published",
		slog.String("notification_message_id", notificationMsg.ID),
		slog.String("original_message_id", originalMessage.ID),
	)

	return nil
}

// markMessageFailed marks a message as failed
func (h *Handler) markMessageFailed(ctx context.Context, message *models.Message, errorMessage string) {
	message.MarkFailed(errorMessage)
	if err := h.messageRepo.SaveMessage(ctx, message); err != nil {
		h.logger.Error("failed to mark message as failed",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
	}
}
