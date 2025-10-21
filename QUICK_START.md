# Rez Agent Quick Start Guide

This guide helps you quickly understand and work with the rez_agent codebase.

## Prerequisites

- Go 1.24+
- AWS account with appropriate permissions
- Basic understanding of AWS Lambda, DynamoDB, SNS/SQS

## Project Overview

Rez Agent is an event-driven system that:
1. **Scheduler Lambda** creates messages on a schedule (every 24 hours)
2. **Messages flow** through SNS → SQS → Processor Lambda
3. **Processor Lambda** sends notifications to ntfy.sh
4. **Web API Lambda** provides REST endpoints for message management
5. **DynamoDB** stores all message metadata

## Quick Commands

```bash
# Install dependencies
go mod download

# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Build all packages
go build ./...

# Format code
go fmt ./...

# Tidy dependencies
go mod tidy
```

## Environment Variables

Set these before running locally or in Lambda:

```bash
# Required
export SNS_TOPIC_ARN="arn:aws:sns:us-east-1:123456789012:rez-agent-topic"
export SQS_QUEUE_URL="https://sqs.us-east-1.amazonaws.com/123456789012/rez-agent-queue"

# Optional (with defaults)
export STAGE="dev"                          # dev, stage, or prod
export AWS_REGION="us-east-1"               # AWS region
export DYNAMODB_TABLE_NAME="rez-agent-messages"
export NTFY_URL="https://ntfy.sh/rzesz-alerts"
```

## Package Overview

### Core Types (`internal/models`)

```go
// Message represents a message in the system
type Message struct {
    ID          string      // Unique identifier
    CreatedDate time.Time   // When created
    CreatedBy   string      // System that created it
    Stage       Stage       // dev, stage, or prod
    MessageType MessageType // hello_world, manual, scheduled
    Status      Status      // created, queued, processing, completed, failed
    Payload     string      // Message content
    UpdatedDate time.Time   // Last update
    ErrorMessage string     // Error details if failed
    RetryCount  int         // Number of retries
}

// Create a new message
msg := models.NewMessage("my-system", models.StageDev, models.MessageTypeManual, "Hello!")

// Update status
msg.MarkQueued()
msg.MarkProcessing()
msg.MarkCompleted()
msg.MarkFailed("error details")
msg.IncrementRetry()
```

### Configuration (`pkg/config`)

```go
// Load configuration from environment
cfg, err := config.Load()

// Or panic on error (for Lambda handlers)
cfg := config.MustLoad()

// Check environment
if cfg.IsDevelopment() {
    // Dev-specific code
}
```

### Repository (`internal/repository`)

```go
// Create repository
repo := repository.NewDynamoDBRepository(dynamoClient, tableName)

// Save message
err := repo.SaveMessage(ctx, message)

// Get message by ID
msg, err := repo.GetMessage(ctx, "msg_123")

// List messages with filters
stage := models.StageDev
status := models.StatusCompleted
messages, err := repo.ListMessages(ctx, &stage, &status, 100)

// Update status
err := repo.UpdateStatus(ctx, "msg_123", models.StatusCompleted, "")
```

### Notification Client (`internal/notification`)

```go
// Create client
client := notification.NewNtfyClient(notification.NtfyClientConfig{
    BaseURL:    "https://ntfy.sh/my-topic",
    Timeout:    30 * time.Second,
    MaxRetries: 3,
    Logger:     logger,
})

// Send notification
err := client.Send(ctx, "Hello, world!")

// Send with title
err := client.SendWithTitle(ctx, "Alert", "Something happened!")
```

### Messaging (`internal/messaging`)

```go
// SNS Publisher
publisher := messaging.NewSNSClient(snsClient, topicArn, logger)
err := publisher.PublishMessage(ctx, message)

// SQS Batch Processor
processor := messaging.NewSQSBatchProcessor(logger)
response, err := processor.ProcessBatch(ctx, event, func(ctx context.Context, msg *models.Message) error {
    // Process each message
    return nil
})
```

## Lambda Handlers

### Scheduler Lambda

Triggered by EventBridge on a schedule:

```go
// Creates a "Hello World" message every 24 hours
// Flow: Create → Save to DynamoDB → Publish to SNS
```

**Environment**: EventBridge scheduled event

### Processor Lambda

Triggered by SQS messages:

```go
// Processes messages from SQS queue
// Flow: Parse SQS → Send to ntfy.sh → Update DynamoDB
```

**Environment**: SQS event source mapping

### Web API Lambda

Triggered by API Gateway:

```go
// REST API for message management
// Endpoints:
//   GET  /api/health
//   GET  /api/messages?stage=dev&status=completed
//   POST /api/messages
//   GET  /api/metrics
```

**Environment**: API Gateway proxy integration

## REST API Examples

```bash
# Health check
curl https://api.example.com/api/health

# List messages
curl "https://api.example.com/api/messages?stage=dev&status=completed&limit=10"

# Create message
curl -X POST https://api.example.com/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "payload": "Test message",
    "message_type": "manual",
    "stage": "dev"
  }'

# Get metrics
curl https://api.example.com/api/metrics
```

## Common Tasks

### Adding a New Message Type

1. Add enum to `internal/models/message.go`:
```go
const (
    MessageTypeHelloWorld MessageType = "hello_world"
    MessageTypeManual MessageType = "manual"
    MessageTypeScheduled MessageType = "scheduled"
    MessageTypeMyNew MessageType = "my_new"  // Add this
)
```

2. Update validation:
```go
func (mt MessageType) IsValid() bool {
    switch mt {
    case MessageTypeHelloWorld, MessageTypeManual, MessageTypeScheduled, MessageTypeMyNew:
        return true
    default:
        return false
    }
}
```

3. Add tests in `internal/models/message_test.go`

### Testing Locally

Since this uses AWS services, local testing requires mocks or local AWS services:

```go
// Example: Mock repository for testing
type MockRepository struct {
    messages map[string]*models.Message
}

func (m *MockRepository) SaveMessage(ctx context.Context, msg *models.Message) error {
    m.messages[msg.ID] = msg
    return nil
}
```

For integration testing:
- Use AWS DynamoDB Local
- Use LocalStack for SNS/SQS
- Use testcontainers-go for containerized testing

### Debugging

Enable verbose logging:
```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,  // Change from LevelInfo to LevelDebug
}))
```

## Message Flow Diagram

```
EventBridge (24h schedule)
    ↓
Scheduler Lambda
    ↓
Save to DynamoDB (status: created)
    ↓
Update DynamoDB (status: queued)
    ↓
Publish to SNS Topic
    ↓
SNS → SQS Queue
    ↓
Processor Lambda (triggered by SQS)
    ↓
Update DynamoDB (status: processing)
    ↓
Send to ntfy.sh
    ↓
Update DynamoDB (status: completed or failed)
```

## Building for AWS Lambda

```bash
# Build for Lambda (Linux AMD64)
cd cmd/scheduler
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap main.go
zip scheduler.zip bootstrap

cd ../processor
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap main.go
zip processor.zip bootstrap

cd ../webapi
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap main.go
zip webapi.zip bootstrap
```

## Troubleshooting

### Tests Failing

```bash
# Clean cache and rebuild
go clean -testcache
go test ./...

# Check for missing dependencies
go mod tidy
go mod verify
```

### Build Errors

```bash
# Update dependencies
go get -u ./...
go mod tidy

# Check Go version
go version  # Should be 1.24+
```

### AWS Errors

Common issues:
- **"Missing required env var"**: Set SNS_TOPIC_ARN and SQS_QUEUE_URL
- **"Access denied"**: Check IAM permissions for Lambda execution role
- **"Table not found"**: Ensure DynamoDB table exists with correct name

## Best Practices

1. **Always use context**: Pass `context.Context` to all AWS SDK calls
2. **Log structured data**: Use slog with key-value pairs
3. **Handle errors explicitly**: Never ignore errors
4. **Test edge cases**: Use table-driven tests
5. **Validate inputs**: Check enum values with IsValid()
6. **Use interfaces**: Enable mocking and testing
7. **Follow Go idioms**: Keep code simple and readable

## Next Steps

1. Review the design documentation: `/workspaces/rez_agent/docs/design/README.md`
2. Check the full implementation summary: `/workspaces/rez_agent/IMPLEMENTATION_SUMMARY.md`
3. Set up Pulumi for infrastructure deployment
4. Configure GitHub Actions for CI/CD
5. Add OAuth2 authentication to Web API
6. Build frontend admin interface

## Support

For issues or questions:
- Check test files for usage examples
- Review CloudWatch Logs for Lambda errors
- Use DynamoDB console to inspect message data
- Test ntfy.sh integration at https://ntfy.sh/rzesz-alerts

## Resources

- Go 1.24 Documentation: https://go.dev/doc/
- AWS SDK for Go v2: https://aws.github.io/aws-sdk-go-v2/
- AWS Lambda Go: https://github.com/aws/aws-lambda-go
- ntfy.sh Documentation: https://docs.ntfy.sh/
