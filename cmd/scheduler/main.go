package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/jrzesz33/rez_agent/internal/logging"
	"github.com/jrzesz33/rez_agent/internal/messaging"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/repository"
	internalscheduler "github.com/jrzesz33/rez_agent/internal/scheduler"
	appconfig "github.com/jrzesz33/rez_agent/pkg/config"
)

// SchedulerHandler handles EventBridge scheduled events and schedule creation
type SchedulerHandler struct {
	config              *appconfig.Config
	messageRepository   repository.MessageRepository
	scheduleRepository  repository.ScheduleRepository
	publisher           messaging.SNSPublisher
	eventBridgeScheduler internalscheduler.EventBridgeScheduler
	logger              *slog.Logger
}

// NewSchedulerHandler creates a new scheduler handler instance
func NewSchedulerHandler(
	cfg *appconfig.Config,
	messageRepo repository.MessageRepository,
	scheduleRepo repository.ScheduleRepository,
	pub messaging.SNSPublisher,
	ebScheduler internalscheduler.EventBridgeScheduler,
	logger *slog.Logger,
) *SchedulerHandler {
	return &SchedulerHandler{
		config:              cfg,
		messageRepository:   messageRepo,
		scheduleRepository:  scheduleRepo,
		publisher:           pub,
		eventBridgeScheduler: ebScheduler,
		logger:              logger,
	}
}

// HandleEvent processes both EventBridge scheduled events and SNS schedule creation events
func (h *SchedulerHandler) HandleEvent(ctx context.Context, event interface{}) error {
	h.logger.InfoContext(ctx, "scheduler Lambda invoked",
		slog.String("stage", h.config.Stage.String()),
	)

	// Try to parse as SNS event (for schedule creation)
	if snsEvent, ok := event.(events.SNSEvent); ok && len(snsEvent.Records) > 0 {
		h.logger.InfoContext(ctx, "detected SNS event, handling schedule creation")
		return h.handleSNSEvent(ctx, snsEvent)
	}

	// Try to parse as map (could be from EventBridge Scheduler with custom input)
	if eventMap, ok := event.(map[string]interface{}); ok {
		if triggeredBy, exists := eventMap["triggered_by"]; exists && triggeredBy == "eventbridge_scheduler" {
			h.logger.InfoContext(ctx, "detected EventBridge Scheduler trigger with custom payload")
			return h.handleDynamicScheduleExecution(ctx, eventMap)
		}
	}

	// Default: treat as standard EventBridge trigger
	h.logger.InfoContext(ctx, "handling standard EventBridge scheduled event")
	return h.handleScheduledEvent(ctx)
}

// handleSNSEvent processes SNS schedule creation/management requests
func (h *SchedulerHandler) handleSNSEvent(ctx context.Context, event events.SNSEvent) error {
	for _, record := range event.Records {
		h.logger.InfoContext(ctx, "processing SNS record",
			slog.String("message_id", record.SNS.MessageID),
		)

		var req models.ScheduleCreationRequest
		if err := json.Unmarshal([]byte(record.SNS.Message), &req); err != nil {
			h.logger.ErrorContext(ctx, "failed to unmarshal SNS message",
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("failed to unmarshal schedule request: %w", err)
		}

		switch req.Action {
		case "create":
			return h.createSchedule(ctx, &req.Schedule)
		case "delete":
			return h.deleteSchedule(ctx, req.Schedule.Name)
		case "pause":
			return h.pauseSchedule(ctx, req.Schedule.Name)
		case "resume":
			return h.resumeSchedule(ctx, req.Schedule.Name)
		default:
			return fmt.Errorf("unknown action: %s", req.Action)
		}
	}

	return nil
}

// handleScheduledEvent processes standard EventBridge scheduled events (legacy behavior)
func (h *SchedulerHandler) handleScheduledEvent(ctx context.Context) error {
	h.logger.InfoContext(ctx, "processing standard scheduled event")

	// Create a new "hello world" message
	message := models.NewMessage(
		"scheduler",
		h.config.Stage,
		models.MessageTypeScheduled,
		"Hello World! This is a scheduled message from rez_agent.",
	)

	h.logger.DebugContext(ctx, "created message",
		slog.String("message_id", message.ID),
		slog.String("type", message.MessageType.String()),
	)

	// Save message to DynamoDB
	err := h.messageRepository.SaveMessage(ctx, message)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to save message",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to save message: %w", err)
	}

	h.logger.DebugContext(ctx, "saved message to DynamoDB",
		slog.String("message_id", message.ID),
	)

	// Mark message as queued
	message.MarkQueued()

	// Update message status in DynamoDB
	err = h.messageRepository.UpdateStatus(ctx, message.ID, message.Status, "")
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to update message status",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to update message status: %w", err)
	}

	// Publish message to SNS
	err = h.publisher.PublishMessage(ctx, message)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to publish message",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to publish message: %w", err)
	}

	h.logger.InfoContext(ctx, "successfully processed scheduled event",
		slog.String("message_id", message.ID),
		slog.String("status", message.Status.String()),
	)

	return nil
}

// handleDynamicScheduleExecution handles executions from dynamically created schedules
func (h *SchedulerHandler) handleDynamicScheduleExecution(ctx context.Context, eventData map[string]interface{}) error {
	scheduleID, _ := eventData["schedule_id"].(string)
	targetType, _ := eventData["target_type"].(string)
	payload, _ := eventData["payload"].(map[string]interface{})

	h.logger.InfoContext(ctx, "handling dynamic schedule execution",
		slog.String("schedule_id", scheduleID),
		slog.String("target_type", targetType),
	)

	// Load schedule from DynamoDB to get latest info
	schedule, err := h.scheduleRepository.GetSchedule(ctx, scheduleID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to load schedule",
			slog.String("schedule_id", scheduleID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to load schedule: %w", err)
	}

	// Create message based on target type
	var message *models.Message
	payloadJSON, _ := json.Marshal(payload)

	switch models.TargetType(targetType) {
	case models.TargetTypeWebAction:
		message = models.NewMessage(
			fmt.Sprintf("schedule:%s", scheduleID),
			h.config.Stage,
			models.MessageTypeWebAction,
			string(payloadJSON),
		)
	case models.TargetTypeNotification:
		message = models.NewMessage(
			fmt.Sprintf("schedule:%s", scheduleID),
			h.config.Stage,
			models.MessageTypeNotification,
			string(payloadJSON),
		)
	default:
		return fmt.Errorf("unsupported target type: %s", targetType)
	}

	// Save and publish message
	if err := h.messageRepository.SaveMessage(ctx, message); err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	message.MarkQueued()
	if err := h.messageRepository.UpdateStatus(ctx, message.ID, message.Status, ""); err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}

	if err := h.publisher.PublishMessage(ctx, message); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	// Record execution in schedule
	schedule.RecordExecution()
	if err := h.scheduleRepository.UpdateSchedule(ctx, schedule); err != nil {
		h.logger.WarnContext(ctx, "failed to record schedule execution",
			slog.String("schedule_id", scheduleID),
			slog.String("error", err.Error()),
		)
	}

	h.logger.InfoContext(ctx, "successfully executed dynamic schedule",
		slog.String("schedule_id", scheduleID),
		slog.String("message_id", message.ID),
	)

	return nil
}

// createSchedule creates a new EventBridge Schedule
func (h *SchedulerHandler) createSchedule(ctx context.Context, def *models.ScheduleDefinition) error {
	h.logger.InfoContext(ctx, "creating new schedule",
		slog.String("name", def.Name),
		slog.String("expression", def.ScheduleExpression),
	)

	// Validate schedule definition
	if err := def.Validate(); err != nil {
		h.logger.ErrorContext(ctx, "invalid schedule definition",
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("invalid schedule: %w", err)
	}

	// Determine target topic ARN based on target type
	var targetTopicArn string
	switch models.TargetType(def.TargetType) {
	case models.TargetTypeWebAction:
		targetTopicArn = h.config.WebActionsSNSTopicArn
	case models.TargetTypeNotification:
		targetTopicArn = h.config.NotificationsSNSTopicArn
	default:
		targetTopicArn = h.config.NotificationsSNSTopicArn
	}

	// Create schedule model
	schedule, err := models.NewSchedule(
		def.Name,
		def.ScheduleExpression,
		def.Timezone,
		models.TargetType(def.TargetType),
		targetTopicArn,
		def.Payload,
		"scheduler-lambda", // created_by
		h.config.Stage,
	)
	if err != nil {
		return fmt.Errorf("failed to create schedule model: %w", err)
	}

	if def.Description != "" {
		schedule.Description = def.Description
	}

	// Get current Lambda ARN (self-invoke for schedule execution)
	lambdaArn := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	if lambdaArn == "" {
		// Fallback: construct ARN
		lambdaArn = fmt.Sprintf("arn:aws:lambda:%s::function:rez-agent-scheduler-%s",
			h.config.AWSRegion, h.config.Stage)
	}

	// Create EventBridge Schedule
	ebArn, err := h.eventBridgeScheduler.CreateSchedule(ctx, schedule, lambdaArn)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to create EventBridge schedule",
			slog.String("name", schedule.Name),
			slog.String("error", err.Error()),
		)
		schedule.MarkError(err.Error())
		// Still save to DynamoDB for tracking
		h.scheduleRepository.SaveSchedule(ctx, schedule)
		return fmt.Errorf("failed to create EventBridge schedule: %w", err)
	}

	// Update schedule with EventBridge ARN
	schedule.UpdateEventBridgeArn(ebArn)

	// Save schedule to DynamoDB
	if err := h.scheduleRepository.SaveSchedule(ctx, schedule); err != nil {
		h.logger.ErrorContext(ctx, "failed to save schedule to DynamoDB",
			slog.String("schedule_id", schedule.ID),
			slog.String("error", err.Error()),
		)
		// Try to delete the EventBridge schedule since DB save failed
		h.eventBridgeScheduler.DeleteSchedule(ctx, schedule.EventBridgeName)
		return fmt.Errorf("failed to save schedule: %w", err)
	}

	h.logger.InfoContext(ctx, "successfully created schedule",
		slog.String("schedule_id", schedule.ID),
		slog.String("eventbridge_arn", ebArn),
	)

	return nil
}

// deleteSchedule deletes an EventBridge Schedule
func (h *SchedulerHandler) deleteSchedule(ctx context.Context, scheduleID string) error {
	h.logger.InfoContext(ctx, "deleting schedule",
		slog.String("schedule_id", scheduleID),
	)

	// Get schedule from DynamoDB
	schedule, err := h.scheduleRepository.GetSchedule(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to get schedule: %w", err)
	}

	// Delete from EventBridge
	if err := h.eventBridgeScheduler.DeleteSchedule(ctx, schedule.EventBridgeName); err != nil {
		h.logger.WarnContext(ctx, "failed to delete EventBridge schedule",
			slog.String("schedule_name", schedule.EventBridgeName),
			slog.String("error", err.Error()),
		)
	}

	// Mark as deleted in DynamoDB
	if err := h.scheduleRepository.DeleteSchedule(ctx, scheduleID); err != nil {
		return fmt.Errorf("failed to mark schedule as deleted: %w", err)
	}

	h.logger.InfoContext(ctx, "successfully deleted schedule",
		slog.String("schedule_id", scheduleID),
	)

	return nil
}

// pauseSchedule pauses a schedule
func (h *SchedulerHandler) pauseSchedule(ctx context.Context, scheduleID string) error {
	schedule, err := h.scheduleRepository.GetSchedule(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to get schedule: %w", err)
	}

	if err := h.eventBridgeScheduler.PauseSchedule(ctx, schedule.EventBridgeName); err != nil {
		return fmt.Errorf("failed to pause EventBridge schedule: %w", err)
	}

	schedule.MarkPaused()
	if err := h.scheduleRepository.UpdateSchedule(ctx, schedule); err != nil {
		return fmt.Errorf("failed to update schedule status: %w", err)
	}

	return nil
}

// resumeSchedule resumes a paused schedule
func (h *SchedulerHandler) resumeSchedule(ctx context.Context, scheduleID string) error {
	schedule, err := h.scheduleRepository.GetSchedule(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to get schedule: %w", err)
	}

	if err := h.eventBridgeScheduler.ResumeSchedule(ctx, schedule.EventBridgeName); err != nil {
		return fmt.Errorf("failed to resume EventBridge schedule: %w", err)
	}

	schedule.MarkActive()
	if err := h.scheduleRepository.UpdateSchedule(ctx, schedule); err != nil {
		return fmt.Errorf("failed to update schedule status: %w", err)
	}

	return nil
}

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logging.GetLogLevel(),
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg := appconfig.MustLoad()

	logger.Info("scheduler lambda starting",
		slog.String("stage", cfg.Stage.String()),
		slog.String("region", cfg.AWSRegion),
	)

	// Initialize AWS SDK
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("failed to load AWS config", slog.String("error", err.Error()))
		panic(fmt.Sprintf("failed to load AWS config: %v", err))
	}

	// Create AWS clients
	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	snsClient := sns.NewFromConfig(awsCfg)
	schedulerClient := scheduler.NewFromConfig(awsCfg)

	// Create repositories
	messageRepo := repository.NewDynamoDBRepository(dynamoClient, cfg.DynamoDBTableName)
	scheduleRepo := repository.NewDynamoDBScheduleRepository(dynamoClient, cfg.SchedulesTableName)

	// Create publisher
	publisher := messaging.NewTopicRoutingSNSClient(snsClient, cfg.WebActionsSNSTopicArn, cfg.NotificationsSNSTopicArn, cfg.AgentResponseTopicArn, cfg.ScheduleCreationTopicArn, logger)

	// Create EventBridge Scheduler service
	ebScheduler := internalscheduler.NewAWSEventBridgeScheduler(schedulerClient, cfg.EventBridgeExecutionRoleArn)

	// Create handler
	handler := NewSchedulerHandler(cfg, messageRepo, scheduleRepo, publisher, ebScheduler, logger)

	// Start Lambda handler
	lambda.Start(handler.HandleEvent)
}
