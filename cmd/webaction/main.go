package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/logging"
	"github.com/jrzesz33/rez_agent/internal/messaging"
	"github.com/jrzesz33/rez_agent/internal/repository"
	"github.com/jrzesz33/rez_agent/internal/secrets"
	"github.com/jrzesz33/rez_agent/internal/webaction"
	"github.com/jrzesz33/rez_agent/pkg/config"
)

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
	handler := webaction.NewHandler(messageRepo,
		resultRepo,
		snsPublisher,
		handlerRegistry,
		cfg.AgentResponseTopicArn,
		logger,
		sqsProcessor)

	// Start Lambda
	lambda.Start(handler.HandleSQSEvent)
}
