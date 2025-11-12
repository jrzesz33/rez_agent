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
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/logging"
	"github.com/jrzesz33/rez_agent/internal/messaging"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/repository"
	internalscheduler "github.com/jrzesz33/rez_agent/internal/scheduler"
	"github.com/jrzesz33/rez_agent/internal/secrets"
	appconfig "github.com/jrzesz33/rez_agent/pkg/config"
)

// SchedulerHandler handles EventBridge scheduled events and schedule creation
type SchedulerHandler struct {
	config               *appconfig.Config
	messageRepository    repository.MessageRepository
	scheduleRepository   repository.ScheduleRepository
	publisher            messaging.SNSPublisher
	sqsProcessor         *messaging.SQSBatchProcessor
	eventBridgeScheduler internalscheduler.EventBridgeScheduler
	agentEventHandler    internalscheduler.AgentEventHandler
	logger               *slog.Logger
}

// NewSchedulerHandler creates a new scheduler handler instance
func NewSchedulerHandler(
	cfg *appconfig.Config,
	messageRepo repository.MessageRepository,
	scheduleRepo repository.ScheduleRepository,
	pub messaging.SNSPublisher,
	ebScheduler internalscheduler.EventBridgeScheduler,
	agentHandler internalscheduler.AgentEventHandler,
	sqsProcessor *messaging.SQSBatchProcessor,
	logger *slog.Logger,
) *SchedulerHandler {
	return &SchedulerHandler{
		config:               cfg,
		messageRepository:    messageRepo,
		scheduleRepository:   scheduleRepo,
		publisher:            pub,
		eventBridgeScheduler: ebScheduler,
		agentEventHandler:    agentHandler,
		sqsProcessor:         sqsProcessor,
		logger:               logger,
	}
}

// HandleEvent processes both EventBridge scheduled events and SNS schedule creation events
func (h *SchedulerHandler) HandleEvent(ctx context.Context, event interface{}) error {
	h.logger.InfoContext(ctx, "scheduler Lambda invoked",
		slog.String("stage", h.config.Stage.String()),
	)

	// Try to parse as SNS event
	if snsEvent, ok := event.(events.SQSEvent); ok {
		h.logger.InfoContext(ctx, "detected SNS event for schedule management",
			slog.Int("record_count", len(snsEvent.Records)),
		)
		// Use SQS batch processor to handle messages with proper retry logic
		resp, err := h.sqsProcessor.ProcessBatch(ctx, snsEvent, h.handleSNSEvent)
		h.logger.InfoContext(ctx, "sqs battch processing completed",
			slog.Any("failures", resp.BatchItemFailures),
		)
		return err
	}

	// Try to parse as map (could be from EventBridge Scheduler with custom input)
	if eventMap, ok := event.(map[string]interface{}); ok {
		if triggeredBy, exists := eventMap["triggered_by"]; exists && triggeredBy == "eventbridge_scheduler" {
			h.logger.InfoContext(ctx, "detected EventBridge Scheduler trigger with custom payload")
			return h.handleDynamicScheduleExecution(ctx, eventMap)
		}

		// Check if this is an agent event
		if eventType, exists := eventMap["event_type"]; exists && eventType == "agent_scheduled" {
			h.logger.InfoContext(ctx, "detected agent scheduled event")
			return h.handleAgentEvent(ctx, eventMap)
		}
	}

	return nil
}

// handleAgentEvent processes agent scheduled events
func (h *SchedulerHandler) handleAgentEvent(ctx context.Context, eventMap map[string]interface{}) error {
	// Parse event into ScheduledAgentEvent
	scheduleID, _ := eventMap["schedule_id"].(string)
	userPrompt, _ := eventMap["user_prompt"].(string)
	courseName, _ := eventMap["course_name"].(string)
	numPlayers, _ := eventMap["num_players"].(float64) // JSON numbers are float64

	event := &internalscheduler.ScheduledAgentEvent{
		ScheduleID: scheduleID,
		UserPrompt: userPrompt,
		CourseName: courseName,
		NumPlayers: int(numPlayers),
	}

	h.logger.InfoContext(ctx, "processing agent event",
		slog.String("schedule_id", event.ScheduleID),
		slog.String("user_prompt", event.UserPrompt),
	)

	// Execute agent event handler
	if err := h.agentEventHandler.ExecuteScheduledEvent(ctx, event); err != nil {
		h.logger.ErrorContext(ctx, "agent event execution failed",
			slog.String("schedule_id", event.ScheduleID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("agent event execution failed: %w", err)
	}

	h.logger.InfoContext(ctx, "agent event executed successfully",
		slog.String("schedule_id", event.ScheduleID),
	)

	return nil
}

// handleSNSEvent processes SNS schedule creation/management requests
func (h *SchedulerHandler) handleSNSEvent(ctx context.Context, message *models.Message) error {

	/*var req models.ScheduleDefinition
	if err := json.Unmarshal([]byte(message.Payload), &req); err != nil {
		h.logger.ErrorContext(ctx, "failed to unmarshal SNS message",
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to unmarshal schedule request: %w", err)
	}*/

	action := message.Arguments["action"].(string)
	scheduleID := ""
	if message.Arguments["schedule_id"] != nil {
		scheduleID, _ = message.Arguments["schedule_id"].(string)
	}
	switch action {
	case "create":
		return h.createSchedule(ctx, message)
	case "delete":
		return h.deleteSchedule(ctx, scheduleID)
	case "pause":
		return h.pauseSchedule(ctx, scheduleID)
	case "resume":
		return h.resumeSchedule(ctx, scheduleID)
	case "execute":
		return h.executeSchedule(ctx, scheduleID)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

}

// handleDynamicScheduleExecution handles executions from dynamically created schedules
func (h *SchedulerHandler) handleDynamicScheduleExecution(ctx context.Context, eventData map[string]interface{}) error {
	scheduleID, _ := eventData["schedule_id"].(string)
	targetType, _ := eventData["target_type"].(string)
	payload, _ := eventData["payload"].(string)

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
	// Declare a map to hold the unmarshaled JSON
	var result map[string]interface{}

	// Unmarshal the JSON byte slice into the map
	err = json.Unmarshal([]byte(payload), &result)
	if err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return fmt.Errorf("unsupported payload: %s", targetType)
	}

	switch models.TargetType(targetType) {
	case models.TargetTypeWebAction:
		message = models.NewMessage(
			"schedule-executor", nil,
			"1.0", h.config.Stage,
			models.MessageTypeWebAction,
			result)
	case models.TargetTypeNotification:
		message = models.NewMessage(
			"schedule-executor", nil,
			"1.0", h.config.Stage,
			models.MessageTypeNotification,
			result)
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
func (h *SchedulerHandler) createSchedule(ctx context.Context, def *models.Message) error {

	_name := def.Arguments["name"].(string)
	_expression := def.Arguments["schedule_expression"].(string)
	_timezone := def.Arguments["timezone"].(string)
	_targetType := def.Arguments["target_type"].(string)

	h.logger.InfoContext(ctx, "creating new schedule",
		slog.String("name", _name),
		slog.String("expression", _expression),
	)

	// Determine target topic ARN based on target type
	var targetTopicArn string
	switch models.TargetType(_targetType) {
	case models.TargetTypeWebAction:
		targetTopicArn = h.config.WebActionsSNSTopicArn
	case models.TargetTypeNotification:
		targetTopicArn = h.config.NotificationsSNSTopicArn
	default:
		targetTopicArn = h.config.NotificationsSNSTopicArn
	}

	// Create schedule model
	schedule, err := models.NewSchedule(
		_name,
		_expression,
		_timezone,
		models.TargetType(_targetType),
		targetTopicArn,
		def.Payload,
		"scheduler-lambda", // created_by
		h.config.Stage,
	)
	if err != nil {
		return fmt.Errorf("failed to create schedule model: %w", err)
	}

	if def.Arguments["Description"] != nil {
		schedule.Description = def.Arguments["Description"].(string)
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

	// Check if run_now is requested
	runNow := false
	if def.Arguments["run_now"] != nil {
		runNow, _ = def.Arguments["run_now"].(bool)
	}

	// If run_now is true, publish an execute message to the Schedule Creation Topic
	if runNow {
		h.logger.InfoContext(ctx, "run_now requested, publishing execute message",
			slog.String("schedule_id", schedule.ID),
		)

		executeMsg := models.NewMessage(
			"scheduler-lambda",           // createdBy
			map[string]interface{}{       // arguments
				"action":      "execute",
				"schedule_id": schedule.ID,
			},
			"1.0",                        // version
			h.config.Stage,               // stage
			models.MessageTypeScheduleCreation, // messageType
			nil,                          // payload (not needed for execute)
		)

		// Publish to Schedule Creation Topic (will go through queue)
		if err := h.publisher.PublishMessage(ctx, executeMsg); err != nil {
			h.logger.WarnContext(ctx, "failed to publish execute message",
				slog.String("schedule_id", schedule.ID),
				slog.String("error", err.Error()),
			)
			// Don't fail the creation if execute publish fails
		}
	}

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

// executeSchedule manually executes a schedule (run now functionality)
func (h *SchedulerHandler) executeSchedule(ctx context.Context, scheduleID string) error {
	h.logger.InfoContext(ctx, "executing schedule immediately",
		slog.String("schedule_id", scheduleID),
	)

	// Get schedule from DynamoDB
	schedule, err := h.scheduleRepository.GetSchedule(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to get schedule: %w", err)
	}

	// Verify schedule is active
	if schedule.Status != models.ScheduleStatusActive {
		return fmt.Errorf("cannot execute inactive schedule: status=%s", schedule.Status)
	}

	// Prepare the execution event (same format as EventBridge would send)
	payloadMap, err := schedule.GetPayloadMap()
	if err != nil {
		return fmt.Errorf("failed to get payload map: %w", err)
	}

	executionEvent := map[string]interface{}{
		"schedule_id":   schedule.ID,
		"schedule_name": schedule.Name,
		"target_type":   schedule.TargetType,
		"payload":       payloadMap,
		"triggered_by":  "manual_execution",
	}

	// Route to appropriate handler based on target type
	h.logger.InfoContext(ctx, "executing schedule with target type",
		slog.String("target_type", string(schedule.TargetType)),
	)

	return h.handleDynamicScheduleExecution(ctx, executionEvent)
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
	bedrockClient := bedrockruntime.NewFromConfig(awsCfg)

	// Create repositories
	messageRepo := repository.NewDynamoDBRepository(dynamoClient, cfg.DynamoDBTableName)
	scheduleRepo := repository.NewDynamoDBScheduleRepository(dynamoClient, cfg.SchedulesTableName)

	// Create publisher
	publisher := messaging.NewTopicRoutingSNSClient(snsClient, cfg.WebActionsSNSTopicArn, cfg.NotificationsSNSTopicArn, cfg.AgentResponseTopicArn, cfg.ScheduleCreationTopicArn, logger)

	// Initialize SQS processor
	sqsProcessor := messaging.NewSQSBatchProcessor(logger)

	// Create EventBridge Scheduler service
	ebScheduler := internalscheduler.NewAWSEventBridgeScheduler(schedulerClient, cfg.EventBridgeExecutionRoleArn)

	// Create HTTP client and secrets manager for agent event handler
	httpClient := httpclient.NewClient(logger)
	secretsManager := secrets.NewManager(awsCfg, logger)

	// Create agent event handler
	agentHandler := internalscheduler.NewAWSAgentEventHandler(
		bedrockClient,
		httpClient,
		secretsManager,
		logger,
	)

	// Create handler
	handler := NewSchedulerHandler(cfg, messageRepo, scheduleRepo, publisher, ebScheduler, agentHandler, sqsProcessor, logger)

	// Start Lambda handler
	lambda.Start(handler.HandleEvent)
}
