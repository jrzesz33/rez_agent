package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/logging"
	"github.com/jrzesz33/rez_agent/internal/messaging"
	"github.com/jrzesz33/rez_agent/internal/repository"
	internalscheduler "github.com/jrzesz33/rez_agent/internal/scheduler"
	"github.com/jrzesz33/rez_agent/internal/secrets"
	appconfig "github.com/jrzesz33/rez_agent/pkg/config"
)

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
	s3Client := s3.NewFromConfig(awsCfg)

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

	// Create agent logger for S3 logging
	agentLogsBucket := os.Getenv("AGENT_LOGS_BUCKET")
	var agentLogger *internalscheduler.AgentLogger
	if agentLogsBucket != "" {
		agentLogger = internalscheduler.NewAgentLogger(s3Client, agentLogsBucket, cfg.Stage.String(), logger)
		logger.Info("agent logger initialized",
			slog.String("bucket", agentLogsBucket),
			slog.String("stage", cfg.Stage.String()),
		)
	} else {
		logger.Warn("AGENT_LOGS_BUCKET not configured, agent logging disabled")
	}

	// Create agent event handler
	agentHandler := internalscheduler.NewAWSAgentEventHandler(
		bedrockClient,
		httpClient,
		secretsManager,
		agentLogger,
		logger,
	)

	// Create handler
	handler := internalscheduler.NewSchedulerHandler(cfg, messageRepo, scheduleRepo, publisher, ebScheduler, sqsProcessor, logger, agentHandler)

	// Start Lambda handler
	lambda.Start(handler.HandleEvent)
}
