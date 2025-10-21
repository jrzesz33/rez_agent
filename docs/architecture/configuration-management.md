# rez_agent Configuration Management

## Overview

This document defines the configuration management strategy for the rez_agent event-driven messaging system.

**Principles**:
1. **Separation of concerns**: Code vs configuration
2. **Environment-specific**: dev, stage, prod configurations
3. **Security**: Secrets encrypted and access-controlled
4. **Immutability**: Configuration changes trigger deployments
5. **Auditability**: Track configuration changes

**Configuration Sources**:
1. **Environment variables**: Lambda function configuration
2. **AWS Systems Manager Parameter Store**: Secrets and shared config
3. **DynamoDB**: Dynamic configuration (feature flags, circuit breaker state)
4. **Infrastructure as Code**: CDK/Terraform for infrastructure config

---

## Environment Variables

### Purpose

Non-sensitive configuration passed to Lambda functions at deployment time.

**Characteristics**:
- **Stored in**: Lambda function configuration (encrypted at rest)
- **Accessed via**: `os.Getenv()` in Go
- **Managed by**: CDK/Terraform deployment
- **Versioned**: Part of Lambda function version

---

### Environment Variables per Lambda

#### Scheduler Lambda

```bash
STAGE=dev
DYNAMODB_TABLE_NAME=rez-agent-messages-dev
SNS_TOPIC_ARN=arn:aws:sns:us-east-1:123456789012:rez-agent-messages-dev
LOG_LEVEL=INFO
AWS_REGION=us-east-1
```

#### Web API Lambda

```bash
STAGE=dev
DYNAMODB_TABLE_NAME=rez-agent-messages-dev
SNS_TOPIC_ARN=arn:aws:sns:us-east-1:123456789012:rez-agent-messages-dev
COGNITO_USER_POOL_ID=us-east-1_ABC123
COGNITO_CLIENT_ID=3n4u5v6w7x8y9z
COGNITO_REGION=us-east-1
LOG_LEVEL=INFO
AWS_REGION=us-east-1
```

#### Message Processor Lambda

```bash
STAGE=dev
DYNAMODB_TABLE_NAME=rez-agent-messages-dev
NOTIFICATION_SERVICE_ARN=arn:aws:lambda:us-east-1:123456789012:function:rez-agent-notification-service-dev
LOG_LEVEL=INFO
AWS_REGION=us-east-1
```

#### Notification Service Lambda

```bash
STAGE=dev
NTFY_URL_PARAM=/rez-agent/dev/ntfy-url
NTFY_API_KEY_PARAM=/rez-agent/dev/ntfy-api-key
CIRCUIT_BREAKER_TABLE=rez-agent-circuit-breaker-dev
MAX_RETRIES=3
LOG_LEVEL=INFO
AWS_REGION=us-east-1
```

#### JWT Authorizer Lambda

```bash
STAGE=dev
COGNITO_USER_POOL_ID=us-east-1_ABC123
COGNITO_REGION=us-east-1
JWKS_CACHE_TTL=86400
LOG_LEVEL=INFO
AWS_REGION=us-east-1
```

---

### Go Configuration Struct

**Package**: `internal/config`

```go
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Stage              string
	AWSRegion          string
	LogLevel           string
	DynamoDBTableName  string
	SNSTopicARN        string
	NotificationServiceARN string
	CognitoUserPoolID  string
	CognitoClientID    string
	CognitoRegion      string
	NtfyURLParam       string
	NtfyAPIKeyParam    string
	CircuitBreakerTable string
	MaxRetries         int
	JWKSCacheTTL       int
}

func Load() (*Config, error) {
	cfg := &Config{
		Stage:     getEnv("STAGE", "dev"),
		AWSRegion: getEnv("AWS_REGION", "us-east-1"),
		LogLevel:  getEnv("LOG_LEVEL", "INFO"),
	}

	// Load Lambda-specific config
	cfg.DynamoDBTableName = os.Getenv("DYNAMODB_TABLE_NAME")
	cfg.SNSTopicARN = os.Getenv("SNS_TOPIC_ARN")
	cfg.NotificationServiceARN = os.Getenv("NOTIFICATION_SERVICE_ARN")
	cfg.CognitoUserPoolID = os.Getenv("COGNITO_USER_POOL_ID")
	cfg.CognitoClientID = os.Getenv("COGNITO_CLIENT_ID")
	cfg.CognitoRegion = getEnv("COGNITO_REGION", cfg.AWSRegion)
	cfg.NtfyURLParam = os.Getenv("NTFY_URL_PARAM")
	cfg.NtfyAPIKeyParam = os.Getenv("NTFY_API_KEY_PARAM")
	cfg.CircuitBreakerTable = os.Getenv("CIRCUIT_BREAKER_TABLE")

	// Parse integers
	cfg.MaxRetries = getEnvInt("MAX_RETRIES", 3)
	cfg.JWKSCacheTTL = getEnvInt("JWKS_CACHE_TTL", 86400)

	// Validate required config
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	// Validate based on Lambda context
	// Example: Web API requires COGNITO_USER_POOL_ID
	// (Implementation depends on Lambda function)
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
```

**Usage in Lambda**:
```go
package main

import (
	"context"
	"log"
	"github.com/rez-agent/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Use config
	log.Printf("Running in stage: %s", cfg.Stage)

	// ... rest of Lambda handler
}
```

---

## AWS Systems Manager Parameter Store

### Purpose

Centralized storage for secrets and shared configuration.

**Benefits**:
- **Encryption**: SecureString parameters encrypted with KMS
- **Access Control**: IAM policies restrict parameter access
- **Versioning**: Track parameter changes (audit trail)
- **CloudFormation/CDK Integration**: Reference parameters in IaC
- **Cost**: Free tier (10k parameters)

---

### Parameter Hierarchy

**Naming Convention**: `/rez-agent/{stage}/{parameter-name}`

**Examples**:
```
/rez-agent/dev/cognito-user-pool-id (String)
/rez-agent/dev/cognito-client-id (String)
/rez-agent/dev/ntfy-url (String)
/rez-agent/dev/ntfy-api-key (SecureString, KMS encrypted)

/rez-agent/stage/cognito-user-pool-id (String)
/rez-agent/stage/ntfy-url (String)
/rez-agent/stage/ntfy-api-key (SecureString)

/rez-agent/prod/cognito-user-pool-id (String)
/rez-agent/prod/ntfy-url (String)
/rez-agent/prod/ntfy-api-key (SecureString)
```

---

### Parameter Types

#### Standard Parameters (String)

**Use Case**: Non-sensitive configuration

**Examples**:
- Cognito User Pool ID
- SNS Topic ARN
- ntfy.sh URL (non-sensitive)

**Encryption**: Not encrypted (stored as plaintext)

**Cost**: Free

---

#### SecureString Parameters

**Use Case**: Sensitive data (API keys, passwords)

**Examples**:
- ntfy.sh API key
- OAuth client secret (if used)

**Encryption**: KMS encryption at rest

**Cost**: Free (uses AWS-managed KMS key `aws/ssm`)

**Custom KMS Key** (optional):
- Use customer-managed KMS key for enhanced audit logging
- Cost: $1/month per key + $0.03 per 10k API requests

---

### Go SDK: Fetching Parameters

**Package**: `internal/ssm`

```go
package ssm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type ParameterStore struct {
	client *ssm.Client
	cache  map[string]cachedParameter
	mu     sync.RWMutex
}

type cachedParameter struct {
	value     string
	expiresAt time.Time
}

func New(client *ssm.Client) *ParameterStore {
	return &ParameterStore{
		client: client,
		cache:  make(map[string]cachedParameter),
	}
}

// GetParameter fetches parameter with caching
func (ps *ParameterStore) GetParameter(ctx context.Context, name string) (string, error) {
	// Check cache
	ps.mu.RLock()
	if cached, ok := ps.cache[name]; ok && time.Now().Before(cached.expiresAt) {
		ps.mu.RUnlock()
		return cached.value, nil
	}
	ps.mu.RUnlock()

	// Fetch from Parameter Store
	result, err := ps.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true), // Decrypt SecureString
	})
	if err != nil {
		return "", fmt.Errorf("failed to get parameter %s: %w", name, err)
	}

	value := *result.Parameter.Value

	// Cache for 5 minutes (reduce API calls)
	ps.mu.Lock()
	ps.cache[name] = cachedParameter{
		value:     value,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	ps.mu.Unlock()

	return value, nil
}

// GetParameters fetches multiple parameters in one call
func (ps *ParameterStore) GetParameters(ctx context.Context, names []string) (map[string]string, error) {
	awsNames := make([]string, len(names))
	for i, name := range names {
		awsNames[i] = name
	}

	result, err := ps.client.GetParameters(ctx, &ssm.GetParametersInput{
		Names:          awsNames,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	for _, param := range result.Parameters {
		params[*param.Name] = *param.Value
	}

	// Cache all parameters
	ps.mu.Lock()
	for name, value := range params {
		ps.cache[name] = cachedParameter{
			value:     value,
			expiresAt: time.Now().Add(5 * time.Minute),
		}
	}
	ps.mu.Unlock()

	return params, nil
}
```

**Usage in Lambda**:
```go
func (ns *NotificationService) Initialize(ctx context.Context) error {
	paramStore := ssm.New(ns.ssmClient)

	// Fetch ntfy.sh URL and API key
	ntfyURL, err := paramStore.GetParameter(ctx, ns.config.NtfyURLParam)
	if err != nil {
		return err
	}

	ntfyAPIKey, err := paramStore.GetParameter(ctx, ns.config.NtfyAPIKeyParam)
	if err != nil {
		return err
	}

	ns.ntfyURL = ntfyURL
	ns.ntfyAPIKey = ntfyAPIKey

	return nil
}
```

**Caching Strategy**:
- Cache parameters in Lambda global variables (reuse across invocations)
- TTL: 5 minutes (balance between freshness and API calls)
- Invalidate cache on parameter update (or accept eventual consistency)

---

### IAM Permissions for Parameter Access

**Scheduler Lambda Role** (no parameter access needed):
```json
{
  "Effect": "Deny",
  "Action": ["ssm:GetParameter"],
  "Resource": "*"
}
```

**Web API Lambda Role** (access Cognito config):
```json
{
  "Effect": "Allow",
  "Action": ["ssm:GetParameter"],
  "Resource": [
    "arn:aws:ssm:us-east-1:123456789012:parameter/rez-agent/dev/cognito-*"
  ]
}
```

**Notification Service Lambda Role** (access ntfy.sh config):
```json
{
  "Effect": "Allow",
  "Action": ["ssm:GetParameter"],
  "Resource": [
    "arn:aws:ssm:us-east-1:123456789012:parameter/rez-agent/dev/ntfy-*"
  ]
}
```

---

## DynamoDB for Dynamic Configuration

### Purpose

Runtime configuration that changes without redeployment.

**Use Cases**:
- Feature flags (enable/disable features)
- Circuit breaker state (shared across Lambda instances)
- Rate limits (per-user quotas)
- A/B testing configuration

---

### Feature Flags Table (Optional)

**Table Name**: `rez-agent-feature-flags-{stage}`

**Schema**:
```json
{
  "feature_name": "manual_message_creation", // PK
  "enabled": true,
  "stage": "dev",
  "enabled_for": ["user:alice@example.com"], // Optional: user whitelist
  "description": "Allow manual message creation via Web API",
  "updated_at": "2025-10-21T14:30:00Z",
  "updated_by": "admin@example.com"
}
```

**Go Code** (Feature Flag Check):
```go
package featureflags

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type FeatureFlags struct {
	client    *dynamodb.Client
	tableName string
}

func (ff *FeatureFlags) IsEnabled(ctx context.Context, featureName, userID string) (bool, error) {
	// Fetch feature flag from DynamoDB
	result, err := ff.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(ff.tableName),
		Key: map[string]types.AttributeValue{
			"feature_name": &types.AttributeValueMemberS{Value: featureName},
		},
	})

	if err != nil || result.Item == nil {
		return false, err
	}

	// Check global enabled flag
	enabled := result.Item["enabled"].(*types.AttributeValueMemberBOOL).Value

	// Check user whitelist (if exists)
	if enabledFor, ok := result.Item["enabled_for"]; ok {
		userList := enabledFor.(*types.AttributeValueMemberL).Value
		// Check if userID in list
		// (implementation omitted)
	}

	return enabled, nil
}
```

**Usage in Web API**:
```go
func (h *Handler) CreateMessage(ctx context.Context, req CreateMessageRequest, userID string) error {
	// Check feature flag
	enabled, err := h.featureFlags.IsEnabled(ctx, "manual_message_creation", userID)
	if err != nil || !enabled {
		return fmt.Errorf("manual message creation is disabled")
	}

	// ... create message
}
```

---

## Configuration per Environment

### Development (dev)

**Characteristics**:
- Verbose logging (DEBUG level)
- Relaxed timeouts (longer for debugging)
- X-Ray 100% sampling
- Short TTL on DynamoDB items (7 days)
- CloudWatch Logs retention: 7 days

**Configuration**:
```bash
STAGE=dev
LOG_LEVEL=DEBUG
MAX_RETRIES=5
CIRCUIT_BREAKER_THRESHOLD=10
XRAY_SAMPLING_RATE=1.0
```

---

### Staging (stage)

**Characteristics**:
- Production-like configuration
- INFO logging
- X-Ray 10% sampling
- DynamoDB TTL: 90 days
- CloudWatch Logs retention: 14 days

**Configuration**:
```bash
STAGE=stage
LOG_LEVEL=INFO
MAX_RETRIES=3
CIRCUIT_BREAKER_THRESHOLD=5
XRAY_SAMPLING_RATE=0.1
```

---

### Production (prod)

**Characteristics**:
- INFO/WARN/ERROR logging only
- Strict timeouts
- X-Ray 5% sampling
- DynamoDB TTL: 90 days
- CloudWatch Logs retention: 30 days
- Enhanced monitoring and alarms

**Configuration**:
```bash
STAGE=prod
LOG_LEVEL=INFO
MAX_RETRIES=3
CIRCUIT_BREAKER_THRESHOLD=5
XRAY_SAMPLING_RATE=0.05
```

---

## Infrastructure as Code (CDK)

### AWS CDK Configuration

**Language**: Go (matches application language)

**Stack Structure**:
```
stacks/
├── dev-stack.go
├── stage-stack.go
└── prod-stack.go

constructs/
├── lambda-function.go
├── dynamodb-table.go
├── sns-topic.go
└── sqs-queue.go

cdk.json (CDK app configuration)
```

**Example Stack** (dev-stack.go):
```go
package stacks

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type RezAgentStackProps struct {
	awscdk.StackProps
	Stage string
}

func NewRezAgentStack(scope constructs.Construct, id string, props *RezAgentStackProps) awscdk.Stack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	// DynamoDB Table
	messagesTable := awsdynamodb.NewTable(stack, jsii.String("MessagesTable"), &awsdynamodb.TableProps{
		TableName:   jsii.String("rez-agent-messages-" + props.Stage),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("message_id"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		BillingMode:   awsdynamodb.BillingMode_PAY_PER_REQUEST,
		PointInTimeRecovery: jsii.Bool(props.Stage == "prod" || props.Stage == "stage"),
		TimeToLiveAttribute: jsii.String("ttl"),
	})

	// GSI: stage-created_date-index
	messagesTable.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
		IndexName: jsii.String("stage-created_date-index"),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("stage"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("created_date"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		ProjectionType: awsdynamodb.ProjectionType_ALL,
	})

	// SNS Topic
	messagesTopic := awssns.NewTopic(stack, jsii.String("MessagesTopic"), &awssns.TopicProps{
		TopicName: jsii.String("rez-agent-messages-" + props.Stage),
	})

	// SQS Queue (with DLQ)
	dlq := awssqs.NewQueue(stack, jsii.String("MessagesDLQ"), &awssqs.QueueProps{
		QueueName:         jsii.String("rez-agent-message-queue-dlq-" + props.Stage),
		RetentionPeriod:   awscdk.Duration_Days(jsii.Number(14)),
	})

	messagesQueue := awssqs.NewQueue(stack, jsii.String("MessagesQueue"), &awssqs.QueueProps{
		QueueName:         jsii.String("rez-agent-message-queue-" + props.Stage),
		VisibilityTimeout: awscdk.Duration_Seconds(jsii.Number(300)),
		RetentionPeriod:   awscdk.Duration_Days(jsii.Number(14)),
		DeadLetterQueue: &awssqs.DeadLetterQueue{
			Queue:           dlq,
			MaxReceiveCount: jsii.Number(3),
		},
	})

	// Subscribe SQS to SNS
	messagesTopic.AddSubscription(awssnssubscriptions.NewSqsSubscription(messagesQueue, nil))

	// Lambda Functions (Scheduler, Web API, Message Processor, Notification Service)
	// (Implementation omitted for brevity)

	// Outputs
	awscdk.NewCfnOutput(stack, jsii.String("MessagesTableName"), &awscdk.CfnOutputProps{
		Value: messagesTable.TableName(),
	})

	awscdk.NewCfnOutput(stack, jsii.String("SNSTopicARN"), &awscdk.CfnOutputProps{
		Value: messagesTopic.TopicArn(),
	})

	return stack
}
```

**Deployment**:
```bash
# Deploy to dev
cdk deploy --context stage=dev

# Deploy to prod
cdk deploy --context stage=prod
```

---

## Configuration Change Management

### Change Process

1. **Propose Change**: Document change in GitHub issue or RFC
2. **Code Review**: Update configuration in CDK/Terraform, review PR
3. **Test in Dev**: Deploy configuration change to dev environment
4. **Validate**: Test functionality with new configuration
5. **Deploy to Stage**: Deploy to staging for final validation
6. **Deploy to Prod**: Deploy to production (with rollback plan)
7. **Monitor**: Watch CloudWatch metrics and logs for issues
8. **Rollback**: Revert configuration if errors detected

---

### Parameter Versioning

**Parameter Store Versioning**: Enabled by default

**View Parameter History**:
```bash
aws ssm get-parameter-history \
  --name /rez-agent/prod/ntfy-api-key \
  --with-decryption
```

**Output**:
```json
{
  "Parameters": [
    {
      "Name": "/rez-agent/prod/ntfy-api-key",
      "Type": "SecureString",
      "Value": "old-api-key",
      "Version": 1,
      "LastModifiedDate": "2025-10-15T10:00:00Z"
    },
    {
      "Name": "/rez-agent/prod/ntfy-api-key",
      "Type": "SecureString",
      "Value": "new-api-key",
      "Version": 2,
      "LastModifiedDate": "2025-10-21T14:00:00Z"
    }
  ]
}
```

**Rollback**: Update parameter to previous version value

---

## Configuration Validation

### Pre-Deployment Validation

**CDK Synth**: Validate CloudFormation template before deployment
```bash
cdk synth --context stage=prod
```

**cfn-lint**: Lint CloudFormation templates for errors
```bash
cfn-lint cdk.out/RezAgentStack-prod.template.json
```

**Go Unit Tests**: Test configuration loading logic
```go
func TestConfigLoad_DevEnvironment(t *testing.T) {
	os.Setenv("STAGE", "dev")
	os.Setenv("DYNAMODB_TABLE_NAME", "test-table")

	cfg, err := config.Load()

	assert.NoError(t, err)
	assert.Equal(t, "dev", cfg.Stage)
	assert.Equal(t, "test-table", cfg.DynamoDBTableName)
}
```

---

### Runtime Validation

**Lambda Init**: Validate configuration on Lambda cold start
```go
func init() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Validate required parameters
	if cfg.DynamoDBTableName == "" {
		log.Fatal("DYNAMODB_TABLE_NAME is required")
	}

	// Store config globally
	globalConfig = cfg
}
```

**Health Check**: Verify configuration in `/api/health` endpoint
```go
func (h *Handler) HealthCheck(ctx context.Context) (*HealthResponse, error) {
	// Check DynamoDB table accessible
	_, err := h.dynamodbClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(h.config.DynamoDBTableName),
	})

	if err != nil {
		return &HealthResponse{Status: "unhealthy", Errors: []string{"DynamoDB table not accessible"}}, nil
	}

	return &HealthResponse{Status: "healthy"}, nil
}
```

---

## Secrets Rotation

### Current State (Manual)

**Process**:
1. Generate new API key (ntfy.sh dashboard)
2. Update Parameter Store: `/rez-agent/prod/ntfy-api-key`
3. Lambda functions fetch new value on next invocation (cached for 5 minutes)
4. Monitor logs for authentication errors
5. Deactivate old API key after 1 hour

**Downtime**: ~5 minutes (cache TTL)

---

### Future Enhancement: Automatic Rotation

**AWS Secrets Manager** (instead of Parameter Store):
- **Automatic rotation**: Lambda function rotates secret on schedule (30/60/90 days)
- **Dual secrets**: Both old and new secrets valid during rotation window
- **Zero downtime**: No service interruption

**Cost**: $0.40/secret/month + $0.05 per 10k API calls (vs. free for Parameter Store)

**Recommendation**: Use Secrets Manager if rotation frequency > quarterly

---

## Configuration Documentation

### Configuration Reference

Maintain up-to-date documentation in `/docs/configuration-reference.md`:

**Contents**:
1. All environment variables (description, required/optional, default value)
2. All Parameter Store parameters (description, type, example value)
3. DynamoDB dynamic configuration (feature flags, circuit breaker thresholds)
4. Per-environment differences (dev vs. stage vs. prod)
5. Configuration change process

**Example Entry**:
```markdown
### DYNAMODB_TABLE_NAME

**Description**: Name of DynamoDB table for message metadata

**Type**: Environment Variable (String)

**Required**: Yes

**Default**: None

**Example**: `rez-agent-messages-dev`

**Used by**: Scheduler Lambda, Web API Lambda, Message Processor Lambda

**Per-Environment Values**:
- dev: `rez-agent-messages-dev`
- stage: `rez-agent-messages-stage`
- prod: `rez-agent-messages-prod`
```

---

## Summary

The rez_agent configuration management strategy provides:

1. **Environment variables**: Non-sensitive Lambda configuration (table names, ARNs)
2. **Parameter Store**: Centralized secrets and shared config (encrypted, access-controlled)
3. **DynamoDB**: Dynamic runtime configuration (feature flags, circuit breaker state)
4. **Infrastructure as Code**: CDK/Terraform for infrastructure configuration (versioned, reviewable)
5. **Caching**: Parameter Store values cached in Lambda globals (reduce API calls)
6. **Validation**: Pre-deployment and runtime validation (catch errors early)
7. **Versioning**: Parameter Store version history (audit trail, rollback)
8. **Per-environment config**: Dev, stage, prod configurations (isolated, testable)
9. **Documentation**: Configuration reference guide (single source of truth)

This approach ensures configuration is secure, auditable, environment-specific, and easy to manage across the Lambda-based serverless architecture.
