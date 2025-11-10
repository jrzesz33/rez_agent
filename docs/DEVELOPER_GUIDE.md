# Developer Guide

## Table of Contents

1. [Getting Started](#getting-started)
2. [Development Environment](#development-environment)
3. [Project Structure](#project-structure)
4. [Development Workflow](#development-workflow)
5. [Testing](#testing)
6. [Code Style and Standards](#code-style-and-standards)
7. [Adding New Features](#adding-new-features)
8. [Debugging](#debugging)
9. [Performance Optimization](#performance-optimization)
10. [Common Tasks](#common-tasks)

## Getting Started

### Prerequisites

Ensure you have the following installed:

- **Go 1.24+**: [Download](https://golang.org/dl/)
- **Python 3.12+**: [Download](https://www.python.org/downloads/)
- **AWS CLI**: [Install Guide](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html)
- **Pulumi CLI**: [Install Guide](https://www.pulumi.com/docs/get-started/install/)
- **Docker**: [Install Guide](https://docs.docker.com/get-docker/)
- **Make**: Usually pre-installed on Unix systems
- **Git**: [Download](https://git-scm.com/downloads)

### Clone and Setup

```bash
# Clone the repository
git clone https://github.com/jrzesz33/rez_agent.git
cd rez_agent

# Install Go dependencies
go mod download

# Install Python dependencies (for AI agent)
pip install -r cmd/agent/requirements.txt

# Check all dependencies
make check-deps
```

### Configure AWS

```bash
# Configure AWS credentials
aws configure

# Verify configuration
aws sts get-caller-identity
```

### Initialize Pulumi

```bash
# Login to Pulumi
cd infrastructure
pulumi login

# Create a new stack
pulumi stack init dev

# Set required configuration
pulumi config set ntfyUrl https://ntfy.sh/your-topic
pulumi config set stage dev
pulumi config set logRetentionDays 7
```

## Development Environment

### Using DevContainers

The project includes a devcontainer configuration for consistent development environments:

```bash
# Open in VS Code with Remote Containers extension
code .

# In VS Code, run: "Remote-Containers: Reopen in Container"
```

The devcontainer includes:
- Go 1.24 on Debian Bookworm
- Python 3.12
- AWS CLI
- Pulumi CLI
- Docker-in-Docker
- GitHub CLI (gh)

### Environment Variables

For local development, create a `.env` file (not committed to git):

```bash
# AWS Configuration
AWS_REGION=us-east-1

# DynamoDB
DYNAMODB_TABLE_NAME=rez-agent-messages-dev
SCHEDULES_TABLE_NAME=rez-agent-schedules-dev
WEB_ACTION_RESULTS_TABLE_NAME=rez-agent-web-action-results-dev

# SNS Topics (get from Pulumi outputs)
WEB_ACTIONS_TOPIC_ARN=arn:aws:sns:...
NOTIFICATIONS_TOPIC_ARN=arn:aws:sns:...
AGENT_RESPONSE_TOPIC_ARN=arn:aws:sns:...
SCHEDULE_CREATION_TOPIC_ARN=arn:aws:sns:...

# SQS Queues
WEB_ACTION_SQS_QUEUE_URL=https://sqs.us-east-1.amazonaws.com/...
NOTIFICATION_SQS_QUEUE_URL=https://sqs.us-east-1.amazonaws.com/...

# Other
NTFY_URL=https://ntfy.sh/your-topic
GOLF_SECRET_NAME=rez-agent/golf/credentials-dev
STAGE=dev
LOG_LEVEL=DEBUG
```

## Project Structure

```
rez_agent/
├── cmd/                      # Lambda function entry points
│   ├── agent/               # AI agent (Python)
│   │   ├── lambda_function.py    # Lambda handler
│   │   ├── requirements.txt      # Python dependencies
│   │   └── ui/                   # UI components
│   ├── mcp/                 # MCP server (Go)
│   │   └── main.go
│   ├── processor/           # Notification processor (Go)
│   │   └── main.go
│   ├── scheduler/           # Event scheduler (Go)
│   │   └── main.go
│   ├── webaction/          # Web action executor (Go)
│   │   └── main.go
│   └── webapi/             # HTTP API (Go)
│       └── main.go
├── internal/                # Private application code
│   ├── httpclient/         # HTTP client with OAuth
│   │   ├── client.go           # Base HTTP client
│   │   └── oauth.go            # OAuth 2.0 client
│   ├── logging/            # Structured logging
│   │   └── logger.go
│   ├── mcp/                # MCP implementation
│   │   ├── protocol/          # MCP types
│   │   ├── server/            # MCP server
│   │   └── tools/             # MCP tool definitions
│   ├── messaging/          # SNS/SQS messaging
│   │   ├── sns.go             # SNS publisher
│   │   └── sqs.go             # SQS batch processor
│   ├── models/             # Domain models
│   │   ├── message.go         # Base message
│   │   ├── webaction.go       # Web action types
│   │   └── golf.go            # Golf-specific models
│   ├── notification/       # ntfy.sh integration
│   │   └── ntfy.go
│   ├── repository/         # Data access layer
│   │   ├── dynamodb.go        # Message repository
│   │   ├── schedule_repository.go
│   │   └── webaction_repository.go
│   ├── scheduler/          # EventBridge Scheduler
│   │   └── eventbridge.go
│   ├── secrets/            # AWS Secrets Manager
│   │   └── manager.go
│   └── webaction/          # Web action handlers
│       ├── handler.go         # Handler interface
│       ├── registry.go        # Handler registry
│       ├── weather_handler.go # Weather handler
│       └── golf_handler.go    # Golf handler
├── pkg/                     # Public libraries
│   ├── config/             # Configuration management
│   │   └── config.go
│   └── courses/            # Golf course definitions
│       ├── loader.go          # Course loader
│       └── courseInfo.yaml    # Course configurations
├── infrastructure/          # Pulumi IaC
│   └── main.go
├── docs/                    # Documentation
├── tools/                   # Development tools
│   └── mcp-client/         # MCP stdio client
├── Makefile                # Build automation
├── go.mod                  # Go module definition
└── CLAUDE.md              # Project instructions
```

## Development Workflow

### 1. Feature Development

```bash
# Create a feature branch
git checkout -b feature/my-awesome-feature

# Make changes
# ...

# Run tests
make test

# Format code
make fmt

# Run linter
make lint

# Build
make build

# Commit changes
git add .
git commit -m "feat: add awesome feature"

# Push to GitHub
git push origin feature/my-awesome-feature
```

### 2. Commit Message Convention

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add new web action handler
fix: resolve OAuth token caching issue
docs: update API documentation
test: add unit tests for golf handler
refactor: simplify message routing logic
chore: update dependencies
```

### 3. Pull Request Process

1. Create PR on GitHub
2. Ensure all tests pass
3. Request code review
4. Address review comments
5. Merge when approved

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run with verbose output
go test -v ./...

# Run specific package
go test ./internal/webaction -v

# Run specific test
go test -run TestGolfHandler_SearchTeeTimes ./internal/webaction -v
```

### Test Coverage

```bash
# Generate coverage report
make test-coverage

# View coverage in browser
open coverage.html
```

### Writing Tests

#### Example: Testing a Handler

```go
package webaction

import (
    "context"
    "testing"

    "github.com/jrzesz33/rez_agent/internal/models"
)

func TestWeatherHandler_Execute(t *testing.T) {
    tests := []struct {
        name    string
        payload *models.WebActionPayload
        want    []string
        wantErr bool
    }{
        {
            name: "valid weather request",
            payload: &models.WebActionPayload{
                Version: "1.0",
                Action:  models.WebActionTypeWeather,
                URL:     "https://api.weather.gov/gridpoints/TOP/31,80/forecast",
            },
            want:    []string{"Weather forecast for..."},
            wantErr: false,
        },
        {
            name: "invalid URL",
            payload: &models.WebActionPayload{
                Version: "1.0",
                Action:  models.WebActionTypeWeather,
                URL:     "http://invalid",
            },
            want:    nil,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            h := NewWeatherHandler(mockHTTPClient, logger)
            got, err := h.Execute(context.Background(), nil, tt.payload)

            if (err != nil) != tt.wantErr {
                t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
                return
            }

            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("Execute() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Integration Tests

```bash
# Start local DynamoDB
make local-dynamodb

# Run integration tests
AWS_REGION=us-east-1 \
DYNAMODB_ENDPOINT=http://localhost:8000 \
go test -tags=integration ./...

# Stop local DynamoDB
make local-dynamodb-stop
```

## Code Style and Standards

### Go Style Guide

Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md):

1. **Error Handling**: Always check errors
```go
// Bad
data, _ := json.Marshal(obj)

// Good
data, err := json.Marshal(obj)
if err != nil {
    return fmt.Errorf("failed to marshal object: %w", err)
}
```

2. **Context Propagation**: Pass context as first parameter
```go
func ProcessMessage(ctx context.Context, msg *models.Message) error {
    // Use ctx for cancellation and timeouts
}
```

3. **Structured Logging**: Use slog with fields
```go
logger.Info("processing message",
    slog.String("message_id", msg.ID),
    slog.String("type", msg.MessageType.String()),
)
```

4. **Error Wrapping**: Use `fmt.Errorf` with `%w`
```go
if err := doSomething(); err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}
```

### Code Organization

1. **Package Structure**: Group by feature, not by layer
```
internal/
  webaction/
    handler.go          # Interface
    registry.go         # Registry implementation
    weather_handler.go  # Weather implementation
    golf_handler.go     # Golf implementation
```

2. **Naming Conventions**:
   - Interfaces: `Handler`, `Repository`
   - Implementations: `WeatherHandler`, `DynamoDBRepository`
   - Files: `snake_case.go`
   - Constants: `UPPER_SNAKE_CASE` or `PascalCase`

3. **Package Comments**: Every package should have a doc comment
```go
// Package webaction provides handlers for executing web actions.
// It supports OAuth authentication, JWT verification, and SSRF protection.
package webaction
```

## Adding New Features

### Adding a New Web Action Type

1. **Define the action type** in `internal/models/webaction.go`:

```go
const (
    // ... existing types
    WebActionTypeNewService WebActionType = "new_service"
)

func (wat WebActionType) IsValid() bool {
    switch wat {
    case WebActionTypeWeather, WebActionTypeGolf, WebActionTypeNewService:
        return true
    default:
        return false
    }
}
```

2. **Create handler** in `internal/webaction/newservice_handler.go`:

```go
package webaction

import (
    "context"
    "github.com/jrzesz33/rez_agent/internal/models"
)

type NewServiceHandler struct {
    httpClient *httpclient.Client
    logger     *slog.Logger
}

func NewNewServiceHandler(client *httpclient.Client, logger *slog.Logger) *NewServiceHandler {
    return &NewServiceHandler{
        httpClient: client,
        logger:     logger,
    }
}

func (h *NewServiceHandler) GetActionType() models.WebActionType {
    return models.WebActionTypeNewService
}

func (h *NewServiceHandler) Execute(ctx context.Context, args map[string]interface{},
                                    payload *models.WebActionPayload) ([]string, error) {
    // Implementation
    return []string{"Success message"}, nil
}
```

3. **Register handler** in `cmd/webaction/main.go`:

```go
newServiceHandler := webaction.NewNewServiceHandler(httpClient, logger)
if err := handlerRegistry.Register(newServiceHandler); err != nil {
    logger.Error("failed to register new service handler", slog.String("error", err.Error()))
    panic(err)
}
```

4. **Add to allowlist** in `internal/models/webaction.go`:

```go
var AllowedHosts = map[string]bool{
    "api.weather.gov":    true,
    "birdsfoot.cps.golf": true,
    "newservice.com":     true,  // Add new host
}
```

5. **Write tests** in `internal/webaction/newservice_handler_test.go`

6. **Update documentation**:
   - README.md
   - docs/api/README.md
   - docs/MESSAGE_SCHEMAS.md

### Adding a New Lambda Function

1. **Create entry point** in `cmd/newfunction/main.go`

2. **Add build target** in `Makefile`:

```makefile
build-newfunction: ## Build newfunction Lambda
	@echo "$(YELLOW)Building newfunction Lambda...$(NC)"
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(BUILD_DIR)/bootstrap ./cmd/newfunction
	@cd $(BUILD_DIR) && zip newfunction.zip bootstrap && rm bootstrap
	@echo "$(GREEN)NewFunction Lambda built: $(BUILD_DIR)/newfunction.zip$(NC)"
```

3. **Add to main build target**:

```makefile
build: clean build-scheduler build-processor build-webaction build-webapi build-agent build-mcp build-newfunction
```

4. **Add infrastructure** in `infrastructure/main.go`:

```go
// Create Lambda function
newFunctionLambda, err := lambda.NewFunction(ctx, fmt.Sprintf("rez-agent-newfunction-%s", stage), &lambda.FunctionArgs{
    // ... configuration
})
```

5. **Deploy and test**

## Debugging

### CloudWatch Logs

```bash
# Tail specific Lambda logs
make lambda-logs-webaction
make lambda-logs-scheduler
make lambda-logs-processor

# Or use AWS CLI
aws logs tail /aws/lambda/rez-agent-webaction-dev --follow --format short
```

### Local Debugging

```go
// Add debug logging
logger.Debug("debug information",
    slog.String("variable", value),
    slog.Any("object", obj),
)

// Set log level to DEBUG
export LOG_LEVEL=DEBUG
```

### X-Ray Tracing

Enable X-Ray for end-to-end tracing:

```bash
cd infrastructure
pulumi config set enableXRay true
pulumi up
```

View traces in AWS X-Ray console.

### Common Issues

**Issue**: Lambda timeout

**Solution**: Increase timeout in `infrastructure/main.go`:

```go
Timeout: pulumi.Int(180), // 3 minutes
```

**Issue**: DynamoDB throughput exceeded

**Solution**: Tables use on-demand capacity mode, should auto-scale. Check for infinite loops.

**Issue**: OAuth authentication failing

**Solution**: Check credentials in Secrets Manager:

```bash
aws secretsmanager get-secret-value \
    --secret-id rez-agent/golf/credentials-dev \
    --query SecretString \
    --output text
```

## Performance Optimization

### Lambda Cold Starts

Minimize cold starts:

1. **Keep functions warm**: Use scheduled pings
2. **Reduce package size**: Only include necessary dependencies
3. **Use provisioned concurrency**: For critical functions

### Connection Pooling

Reuse HTTP clients:

```go
// Bad: Creates new client per invocation
func handler() {
    client := httpclient.NewClient(logger)
    // ...
}

// Good: Reuse client across invocations
var client *httpclient.Client

func init() {
    client = httpclient.NewClient(logger)
}

func handler() {
    // Use client
}
```

### Caching

Cache expensive operations:

```go
var publicKeyCache sync.Map

func getPublicKey(keyID string) (*rsa.PublicKey, error) {
    // Check cache
    if cached, ok := publicKeyCache.Load(keyID); ok {
        return cached.(*rsa.PublicKey), nil
    }

    // Fetch and cache
    key, err := fetchPublicKey(keyID)
    if err != nil {
        return nil, err
    }

    publicKeyCache.Store(keyID, key)
    return key, nil
}
```

### Batch Processing

Process SQS messages in batches:

```go
// Already implemented in internal/messaging/sqs.go
batchSize := 10 // Process 10 messages per invocation
```

## Common Tasks

### Adding a Golf Course

Edit `pkg/courses/courseInfo.yaml`:

```yaml
courses:
  - courseId: 3
    name: "New Golf Course"
    address: "123 Golf Lane"
    description: "A great course"
    origin: "https://newcourse.example.com"
    client-id: "client-id"
    websiteid: "website-id"
    scope: "openid profile email"
    actions:
      - request:
          name: search-tee-times
          url: "/api/tee-times"
      # ... other actions
```

Rebuild and deploy:

```bash
make build-webaction
cd infrastructure
pulumi up
```

### Updating Dependencies

```bash
# Update Go dependencies
go get -u ./...
go mod tidy

# Update Python dependencies
pip install -U -r cmd/agent/requirements.txt
pip freeze > cmd/agent/requirements.txt

# Update infrastructure dependencies
cd infrastructure
go get -u ./...
go mod tidy
```

### Creating a New SNS Topic

In `infrastructure/main.go`:

```go
newTopic, err := sns.NewTopic(ctx, fmt.Sprintf("rez-agent-new-topic-%s", stage), &sns.TopicArgs{
    DisplayName: pulumi.String("New Topic"),
    Tags:        commonTags,
})
```

### Adding Environment Variables

In `infrastructure/main.go`, add to Lambda environment:

```go
Environment: &lambda.FunctionEnvironmentArgs{
    Variables: pulumi.StringMap{
        // ... existing variables
        "NEW_VAR": pulumi.String("value"),
    },
},
```

In `pkg/config/config.go`, add to Config struct and Load function:

```go
type Config struct {
    // ... existing fields
    NewVar string
}

func Load() (*Config, error) {
    newVar := os.Getenv("NEW_VAR")
    // ... validation

    return &Config{
        // ... existing fields
        NewVar: newVar,
    }, nil
}
```

## Best Practices

1. **Always handle errors**: Never ignore error returns
2. **Use context**: Pass and respect context for cancellation
3. **Log structured data**: Use slog with fields, not printf
4. **Validate input**: Check all external input
5. **Test thoroughly**: Aim for >70% coverage
6. **Document code**: Add comments for complex logic
7. **Keep functions small**: One function, one responsibility
8. **Use interfaces**: For testability and flexibility
9. **Avoid globals**: Except for initialization in `init()`
10. **Follow Go conventions**: Read Effective Go and style guides
