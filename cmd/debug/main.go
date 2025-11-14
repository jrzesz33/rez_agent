package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/events"
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

type Debugger struct {
	config               *appconfig.Config
	scheduleRepository   repository.ScheduleRepository
	eventBridgeScheduler internalscheduler.EventBridgeScheduler
	logger               *slog.Logger
	agentEventHandler    internalscheduler.AgentEventHandler
	publisher            messaging.SNSPublisher
	messageRepository    repository.MessageRepository
	ctx                  context.Context
	scheduleHandler      internalscheduler.SchedulerHandler
}

func main() {

	fmt.Println("Starting Debugger")
	debug := NewDebugger()
	//err := debug.SchedulerEvent("web_api_create_schedule")
	err := debug.SchedulerEvent("test")
	//err := debug.SchedulerEvent("web_api_create_sched_agent")
	if err != nil {
		debug.logger.Error("failed to create schedule", slog.String("error", err.Error()))
	} else {
		debug.logger.Info("schedule created successfully")
	}
}

func (d *Debugger) GetEvent(name string) (events.SQSEvent, error) {
	var event events.SQSEvent
	bytes, err := os.ReadFile(fmt.Sprintf("/workspaces/rez_agent/docs/test/messages/%s.json", name))
	if err != nil {
		return event, fmt.Errorf("failed to read event file: %w", err)
	}

	var msg models.Message
	err = json.Unmarshal(bytes, &msg)
	if err != nil {
		return event, fmt.Errorf("failed to unmarshal event JSON: %w", err)
	}

	event.Records = []events.SQSMessage{
		{
			MessageId:     "debug-message-id",
			ReceiptHandle: "debug-receipt-handle",
			Body:          string(bytes),
		},
	}

	return event, nil
}

func NewDebugger() *Debugger {

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

	// Initialize SQS processor
	sqsProcessor := messaging.NewSQSBatchProcessor(logger)

	// Create handler
	handler := internalscheduler.NewSchedulerHandler(
		cfg,
		messageRepo,
		scheduleRepo,
		publisher,
		ebScheduler,
		sqsProcessor,
		logger,
		agentHandler)

	return &Debugger{
		config:               cfg,
		scheduleRepository:   scheduleRepo,
		eventBridgeScheduler: ebScheduler,
		logger:               logger,
		agentEventHandler:    agentHandler,
		publisher:            publisher,
		messageRepository:    messageRepo,
		ctx:                  context.Background(),
		scheduleHandler:      *handler,
	}

}

func (d *Debugger) SchedulerEvent(event string) error {
	def, err := d.GetEvent(event)
	if err != nil {
		return fmt.Errorf("failed to get create schedule event: %w", err)
	}

	err = d.scheduleHandler.HandleEvent(d.ctx, def)
	if err != nil {
		return fmt.Errorf("failed to handle create schedule: %w", err)
	}

	return nil
}
