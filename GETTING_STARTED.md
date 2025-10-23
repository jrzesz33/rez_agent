# Getting Started with Rez Agent

This guide will help you get the rez_agent system up and running from scratch.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Local Development](#local-development)
- [Deployment](#deployment)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### Required Tools

1. **Go 1.24+**
   ```bash
   go version
   # Should show go1.24 or higher
   ```

2. **AWS CLI**
   ```bash
   aws --version
   # Configure with: aws configure
   ```

3. **Pulumi CLI**
   ```bash
   curl -fsSL https://get.pulumi.com | sh
   pulumi version
   ```

4. **Make**
   ```bash
   make --version
   ```

### AWS Account Setup

1. Create an AWS account if you don't have one
2. Create an IAM user with programmatic access
3. Attach these policies:
   - AWSLambdaFullAccess
   - AmazonDynamoDBFullAccess
   - AmazonSNSFullAccess
   - AmazonSQSFullAccess
   - AmazonEventBridgeFullAccess
   - ElasticLoadBalancingFullAccess
   - IAMFullAccess
   - CloudWatchLogsFullAccess

4. Configure AWS credentials:
   ```bash
   aws configure
   # Enter your AWS Access Key ID
   # Enter your AWS Secret Access Key
   # Enter your default region (e.g., us-east-1)
   # Enter default output format (json)
   ```

### Pulumi Account Setup

1. Create a free account at https://app.pulumi.com
2. Get your access token from https://app.pulumi.com/account/tokens
3. Login to Pulumi:
   ```bash
   pulumi login
   # Or set PULUMI_ACCESS_TOKEN environment variable
   ```

## Quick Start

### Option 1: Interactive Setup (Recommended)

Run the interactive setup wizard:

```bash
./infrastructure/scripts/quick-start.sh
```

This will:
- Check all prerequisites
- Validate AWS credentials
- Initialize Pulumi
- Configure the stack
- Build Lambda functions
- Deploy infrastructure
- Run health checks

### Option 2: Manual Setup

1. **Clone and Setup**
   ```bash
   cd /workspaces/rez_agent
   go mod download
   ```

2. **Build Lambda Functions**
   ```bash
   make build
   ```

3. **Initialize Pulumi**
   ```bash
   cd infrastructure
   pulumi stack init dev
   pulumi config set aws:region us-east-1
   pulumi config set stage dev
   ```

4. **Deploy Infrastructure**
   ```bash
   make deploy-dev
   ```

5. **Verify Deployment**
   ```bash
   make infra-outputs
   ```

## Local Development

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run tests verbosely
go test -v ./...

# Run specific test
go test -run TestMessageModel ./internal/models
```

### Building Lambda Functions

```bash
# Build all Lambda functions
make build

# Build specific Lambda
make build-scheduler
make build-processor
make build-webapi

# Clean build artifacts
make clean
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run go vet
make vet

# Run all checks
make check
```

## Deployment

### Development Environment

```bash
# Preview changes
make infra-preview

# Deploy to dev
make deploy-dev

# View outputs
make infra-outputs

# View logs
make lambda-logs-scheduler
make lambda-logs-processor
make lambda-logs-webapi
```

### Production Environment

```bash
# Switch to production stack
cd infrastructure
pulumi stack select prod

# Configure production settings
pulumi config set logRetentionDays 30
pulumi config set schedulerCron "cron(0 8 * * ? *)"

# Preview production deployment
pulumi preview

# Deploy to production (requires confirmation)
make deploy-prod
```

### CI/CD Deployment

The project includes GitHub Actions workflows for automated deployment:

1. **CI Workflow** (`.github/workflows/ci.yml`)
   - Runs on every PR and push to main
   - Executes tests, builds, linting, and security scans

2. **Dev Deployment** (`.github/workflows/deploy-dev.yml`)
   - Automatically deploys to dev on push to main
   - Runs health checks after deployment

3. **Prod Deployment** (`.github/workflows/deploy-prod.yml`)
   - Triggered by releases or manual workflow dispatch
   - Requires manual approval
   - Includes comprehensive testing

#### Required GitHub Secrets

Configure these secrets in your GitHub repository:

```
AWS_ACCESS_KEY_ID         # AWS access key
AWS_SECRET_ACCESS_KEY     # AWS secret key
AWS_REGION                # AWS region (e.g., us-east-1)
PULUMI_ACCESS_TOKEN       # Pulumi access token
SLACK_WEBHOOK_URL         # (Optional) For deployment notifications
```

## Testing

### Unit Tests

```bash
# Run all unit tests
go test ./...

# Run with race detector
go test -race ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Integration Tests

After deployment, test the API endpoints:

```bash
# Get the WebAPI URL from outputs
WEBAPI_URL=$(cd infrastructure && pulumi stack output webapiUrl)

# Test health endpoint
curl $WEBAPI_URL/api/health

# Test metrics endpoint
curl $WEBAPI_URL/api/metrics

# List messages
curl "$WEBAPI_URL/api/messages?stage=dev"

# Create a message
curl -X POST $WEBAPI_URL/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "manual",
    "payload": "Test message from curl",
    "stage":"dev"
  }'
  
```

### Manual Testing

1. **Test Scheduler Lambda**
   ```bash
   aws lambda invoke \
     --function-name rez-agent-scheduler-dev \
     --payload '{}' \
     response.json
   cat response.json
   ```

2. **Check DynamoDB**
   ```bash
   TABLE_NAME=$(cd infrastructure && pulumi stack output dynamodbTableName)
   aws dynamodb scan --table-name $TABLE_NAME --limit 10
   ```

3. **Check SQS Messages**
   ```bash
   QUEUE_URL=$(cd infrastructure && pulumi stack output sqsQueueUrl)
   aws sqs receive-message --queue-url $QUEUE_URL
   ```

4. **View CloudWatch Logs**
   ```bash
   make lambda-logs-scheduler
   # Or use AWS Console
   ```

## Monitoring

### CloudWatch Dashboards

Access CloudWatch in AWS Console to monitor:
- Lambda invocation counts
- Error rates
- Duration metrics
- DynamoDB read/write capacity
- SQS queue depth

### Alarms

The infrastructure creates alarms for:
- Dead Letter Queue messages
- Processor Lambda errors

Configure SNS notifications for these alarms:

```bash
cd infrastructure
pulumi config set alarmEmail your-email@example.com
pulumi up
```

### Logs

```bash
# Tail logs in real-time
make lambda-logs-scheduler
make lambda-logs-processor
make lambda-logs-webapi

# Or use AWS CLI
aws logs tail /aws/lambda/rez-agent-scheduler-dev --follow
```

## Troubleshooting

### Common Issues

#### 1. Build Failures

**Error:** `package not found`
```bash
# Solution: Download dependencies
go mod download
go mod tidy
```

**Error:** `GOOS/GOARCH mismatch`
```bash
# Solution: Ensure building for Linux
GOOS=linux GOARCH=amd64 go build -o build/bootstrap ./cmd/scheduler
```

#### 2. Deployment Issues

**Error:** `ResourceConflictException: Stack already exists`
```bash
# Solution: Select existing stack or use different name
pulumi stack select dev
# or
pulumi stack init dev-v2
```

**Error:** `AWS credentials not configured`
```bash
# Solution: Configure AWS CLI
aws configure
# Verify credentials
aws sts get-caller-identity
```

#### 3. Runtime Errors

**Error:** `Lambda timeout`
```bash
# Solution: Increase timeout in infrastructure/main.go
# Or check CloudWatch Logs for the actual error
make lambda-logs-processor
```

**Error:** `DynamoDB AccessDeniedException`
```bash
# Solution: Check IAM role permissions
# Verify the Lambda execution role has DynamoDB permissions
```

### Getting Help

1. **Check Documentation**
   - [README.md](./README.md) - Project overview
   - [INFRASTRUCTURE.md](./INFRASTRUCTURE.md) - Infrastructure details
   - [infrastructure/README.md](./infrastructure/README.md) - Detailed deployment guide

2. **View Logs**
   ```bash
   make lambda-logs-scheduler
   make lambda-logs-processor
   make lambda-logs-webapi
   ```

3. **Check Stack Status**
   ```bash
   cd infrastructure
   pulumi stack
   pulumi stack output
   ```

4. **Verify AWS Resources**
   ```bash
   # List Lambda functions
   aws lambda list-functions --query 'Functions[?starts_with(FunctionName, `rez-agent`)].FunctionName'

   # List DynamoDB tables
   aws dynamodb list-tables --query 'TableNames[?starts_with(@, `rez-agent`)]'

   # List SQS queues
   aws sqs list-queues --query 'QueueUrls[?contains(@, `rez-agent`)]'
   ```

## Next Steps

After successful deployment:

1. **Configure OAuth2** (for web frontend authentication)
   - Choose OAuth provider (AWS Cognito, Auth0, Google)
   - Add OAuth configuration to infrastructure
   - Update web API Lambda with authentication middleware

2. **Set up Monitoring**
   - Configure CloudWatch alarms
   - Set up SNS email notifications
   - Create custom dashboards

3. **Customize Message Types**
   - Add new message types in `internal/models/message.go`
   - Implement custom processors in `cmd/processor/main.go`
   - Update web API endpoints as needed

4. **Scale the System**
   - Adjust Lambda memory/timeout based on usage
   - Configure DynamoDB auto-scaling
   - Add caching layer if needed

5. **Production Hardening**
   - Enable AWS X-Ray tracing
   - Set up backup/recovery procedures
   - Implement disaster recovery plan
   - Configure multi-region deployment

## Useful Commands Reference

```bash
# Development
make help                  # Show all available commands
make build                 # Build all Lambda functions
make test                  # Run all tests
make fmt                   # Format code

# Deployment
make deploy-dev            # Deploy to development
make deploy-prod           # Deploy to production
make infra-preview         # Preview infrastructure changes
make infra-outputs         # Show deployed resource details

# Monitoring
make lambda-logs-scheduler # View scheduler logs
make lambda-logs-processor # View processor logs
make lambda-logs-webapi    # View web API logs

# Cleanup
make clean                 # Clean build artifacts
make infra-destroy         # Destroy infrastructure
```

## Support

For issues or questions:
- Check the [INFRASTRUCTURE.md](./INFRASTRUCTURE.md) for detailed documentation
- Review [infrastructure/DEPLOYMENT_CHECKLIST.md](./infrastructure/DEPLOYMENT_CHECKLIST.md)
- Open an issue in the GitHub repository
