package config

import (
	"fmt"
	"os"

	"github.com/jrzesz33/rez_agent/internal/models"
)

// Config holds all configuration for the application
type Config struct {
	// Stage is the deployment environment (dev, stage, prod)
	Stage models.Stage

	// AWS Configuration
	AWSRegion string

	// DynamoDB Configuration
	DynamoDBTableName          string
	WebActionResultsTableName  string

	// SNS Configuration
	SNSTopicArn           string
	WebActionSNSTopicArn  string

	// SQS Configuration
	SQSQueueURL           string
	WebActionSQSQueueURL  string

	// Ntfy Configuration
	NtfyURL string

	// Secrets Manager Configuration
	GolfSecretName string

	// Lambda Configuration
	LambdaTimeout int
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	stage := os.Getenv("STAGE")
	if stage == "" {
		stage = "dev"
	}

	stageEnum := models.Stage(stage)
	if !stageEnum.IsValid() {
		return nil, fmt.Errorf("invalid STAGE value: %s (must be dev, stage, or prod)", stage)
	}

	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-east-1"
	}

	dynamoDBTableName := os.Getenv("DYNAMODB_TABLE_NAME")
	if dynamoDBTableName == "" {
		dynamoDBTableName = "rez-agent-messages"
	}

	webActionResultsTableName := os.Getenv("WEB_ACTION_RESULTS_TABLE_NAME")
	if webActionResultsTableName == "" {
		webActionResultsTableName = fmt.Sprintf("rez-agent-web-action-results-%s", stage)
	}

	snsTopicArn := os.Getenv("SNS_TOPIC_ARN")
	if snsTopicArn == "" {
		return nil, fmt.Errorf("SNS_TOPIC_ARN environment variable is required")
	}

	webActionSNSTopicArn := os.Getenv("WEB_ACTION_SNS_TOPIC_ARN")
	if webActionSNSTopicArn == "" {
		return nil, fmt.Errorf("WEB_ACTION_SNS_TOPIC_ARN environment variable is required")
	}

	sqsQueueURL := os.Getenv("SQS_QUEUE_URL")
	if sqsQueueURL == "" {
		return nil, fmt.Errorf("SQS_QUEUE_URL environment variable is required")
	}

	webActionSQSQueueURL := os.Getenv("WEB_ACTION_SQS_QUEUE_URL")
	if webActionSQSQueueURL == "" {
		return nil, fmt.Errorf("WEB_ACTION_SQS_QUEUE_URL environment variable is required")
	}

	ntfyURL := os.Getenv("NTFY_URL")
	if ntfyURL == "" {
		ntfyURL = "https://ntfy.sh/rzesz-alerts"
	}

	golfSecretName := os.Getenv("GOLF_SECRET_NAME")
	if golfSecretName == "" {
		golfSecretName = fmt.Sprintf("rez-agent/golf/credentials-%s", stage)
	}

	return &Config{
		Stage:                     stageEnum,
		AWSRegion:                 awsRegion,
		DynamoDBTableName:         dynamoDBTableName,
		WebActionResultsTableName: webActionResultsTableName,
		SNSTopicArn:               snsTopicArn,
		WebActionSNSTopicArn:      webActionSNSTopicArn,
		SQSQueueURL:               sqsQueueURL,
		WebActionSQSQueueURL:      webActionSQSQueueURL,
		NtfyURL:                   ntfyURL,
		GolfSecretName:            golfSecretName,
		LambdaTimeout:             30,
	}, nil
}

// MustLoad loads configuration and panics if there's an error
// This is useful for Lambda handlers where configuration errors should prevent startup
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load configuration: %v", err))
	}
	return cfg
}

// Validate checks that all required configuration is present
func (c *Config) Validate() error {
	if !c.Stage.IsValid() {
		return fmt.Errorf("invalid stage: %s", c.Stage)
	}

	if c.AWSRegion == "" {
		return fmt.Errorf("AWS region is required")
	}

	if c.DynamoDBTableName == "" {
		return fmt.Errorf("DynamoDB table name is required")
	}

	if c.SNSTopicArn == "" {
		return fmt.Errorf("SNS topic ARN is required")
	}

	if c.SQSQueueURL == "" {
		return fmt.Errorf("SQS queue URL is required")
	}

	if c.NtfyURL == "" {
		return fmt.Errorf("Ntfy URL is required")
	}

	return nil
}

// IsDevelopment returns true if the stage is development
func (c *Config) IsDevelopment() bool {
	return c.Stage == models.StageDev
}

// IsStaging returns true if the stage is staging
func (c *Config) IsStaging() bool {
	return c.Stage == models.StageStage
}

// IsProduction returns true if the stage is production
func (c *Config) IsProduction() bool {
	return c.Stage == models.StageProd
}
