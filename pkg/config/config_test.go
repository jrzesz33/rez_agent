package config

import (
	"os"
	"testing"

	"github.com/jrzesz33/rez_agent/internal/models"
)

func TestLoad(t *testing.T) {
	// Save original env vars
	originalStage := os.Getenv("STAGE")
	originalRegion := os.Getenv("AWS_REGION")
	originalTable := os.Getenv("DYNAMODB_TABLE_NAME")
	originalSNS := os.Getenv("SNS_TOPIC_ARN")
	originalSQS := os.Getenv("SQS_QUEUE_URL")
	originalNtfy := os.Getenv("NTFY_URL")

	// Restore after test
	defer func() {
		os.Setenv("STAGE", originalStage)
		os.Setenv("AWS_REGION", originalRegion)
		os.Setenv("DYNAMODB_TABLE_NAME", originalTable)
		os.Setenv("SNS_TOPIC_ARN", originalSNS)
		os.Setenv("SQS_QUEUE_URL", originalSQS)
		os.Setenv("NTFY_URL", originalNtfy)
	}()

	tests := []struct {
		name      string
		envVars   map[string]string
		wantErr   bool
		checkFunc func(*testing.T, *Config)
	}{
		{
			name: "valid configuration with all env vars",
			envVars: map[string]string{
				"STAGE":               "dev",
				"AWS_REGION":          "us-west-2",
				"DYNAMODB_TABLE_NAME": "test-table",
				"SNS_TOPIC_ARN":       "arn:aws:sns:us-west-2:123456789012:test-topic",
				"SQS_QUEUE_URL":       "https://sqs.us-west-2.amazonaws.com/123456789012/test-queue",
				"NTFY_URL":            "https://ntfy.sh/test",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Stage != models.StageDev {
					t.Errorf("Stage = %v, want %v", cfg.Stage, models.StageDev)
				}
				if cfg.AWSRegion != "us-west-2" {
					t.Errorf("AWSRegion = %v, want %v", cfg.AWSRegion, "us-west-2")
				}
				if cfg.DynamoDBTableName != "test-table" {
					t.Errorf("DynamoDBTableName = %v, want %v", cfg.DynamoDBTableName, "test-table")
				}
			},
		},
		{
			name: "defaults when optional vars not set",
			envVars: map[string]string{
				"SNS_TOPIC_ARN": "arn:aws:sns:us-west-2:123456789012:test-topic",
				"SQS_QUEUE_URL": "https://sqs.us-west-2.amazonaws.com/123456789012/test-queue",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Stage != models.StageDev {
					t.Errorf("Stage = %v, want default %v", cfg.Stage, models.StageDev)
				}
				if cfg.AWSRegion != "us-east-1" {
					t.Errorf("AWSRegion = %v, want default %v", cfg.AWSRegion, "us-east-1")
				}
				if cfg.DynamoDBTableName != "rez-agent-messages" {
					t.Errorf("DynamoDBTableName = %v, want default %v", cfg.DynamoDBTableName, "rez-agent-messages")
				}
				if cfg.NtfyURL != "https://ntfy.sh/rzesz-alerts" {
					t.Errorf("NtfyURL = %v, want default %v", cfg.NtfyURL, "https://ntfy.sh/rzesz-alerts")
				}
			},
		},
		{
			name: "invalid stage value",
			envVars: map[string]string{
				"STAGE":         "invalid",
				"SNS_TOPIC_ARN": "arn:aws:sns:us-west-2:123456789012:test-topic",
				"SQS_QUEUE_URL": "https://sqs.us-west-2.amazonaws.com/123456789012/test-queue",
			},
			wantErr: true,
		},
		{
			name: "missing SNS_TOPIC_ARN",
			envVars: map[string]string{
				"SQS_QUEUE_URL": "https://sqs.us-west-2.amazonaws.com/123456789012/test-queue",
			},
			wantErr: true,
		},
		{
			name: "missing SQS_QUEUE_URL",
			envVars: map[string]string{
				"SNS_TOPIC_ARN": "arn:aws:sns:us-west-2:123456789012:test-topic",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars
			os.Unsetenv("STAGE")
			os.Unsetenv("AWS_REGION")
			os.Unsetenv("DYNAMODB_TABLE_NAME")
			os.Unsetenv("SNS_TOPIC_ARN")
			os.Unsetenv("SQS_QUEUE_URL")
			os.Unsetenv("NTFY_URL")

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			cfg, err := Load()
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, cfg)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Stage:             models.StageDev,
				AWSRegion:         "us-east-1",
				DynamoDBTableName: "test-table",
				SNSTopicArn:       "arn:aws:sns:us-east-1:123456789012:test",
				SQSQueueURL:       "https://sqs.us-east-1.amazonaws.com/123456789012/test",
				NtfyURL:           "https://ntfy.sh/test",
			},
			wantErr: false,
		},
		{
			name: "invalid stage",
			config: &Config{
				Stage:             models.Stage("invalid"),
				AWSRegion:         "us-east-1",
				DynamoDBTableName: "test-table",
				SNSTopicArn:       "arn:aws:sns:us-east-1:123456789012:test",
				SQSQueueURL:       "https://sqs.us-east-1.amazonaws.com/123456789012/test",
				NtfyURL:           "https://ntfy.sh/test",
			},
			wantErr: true,
		},
		{
			name: "missing aws region",
			config: &Config{
				Stage:             models.StageDev,
				DynamoDBTableName: "test-table",
				SNSTopicArn:       "arn:aws:sns:us-east-1:123456789012:test",
				SQSQueueURL:       "https://sqs.us-east-1.amazonaws.com/123456789012/test",
				NtfyURL:           "https://ntfy.sh/test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_EnvironmentChecks(t *testing.T) {
	tests := []struct {
		name           string
		stage          models.Stage
		isDevelopment  bool
		isStaging      bool
		isProduction   bool
	}{
		{"dev environment", models.StageDev, true, false, false},
		{"staging environment", models.StageStage, false, true, false},
		{"production environment", models.StageProd, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Stage: tt.stage}

			if got := cfg.IsDevelopment(); got != tt.isDevelopment {
				t.Errorf("IsDevelopment() = %v, want %v", got, tt.isDevelopment)
			}
			if got := cfg.IsStaging(); got != tt.isStaging {
				t.Errorf("IsStaging() = %v, want %v", got, tt.isStaging)
			}
			if got := cfg.IsProduction(); got != tt.isProduction {
				t.Errorf("IsProduction() = %v, want %v", got, tt.isProduction)
			}
		})
	}
}
