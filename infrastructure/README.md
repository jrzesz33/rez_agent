# rez_agent Infrastructure

This directory contains the Pulumi Infrastructure as Code (IaC) for the rez_agent event-driven messaging system.

## Overview

The infrastructure provisions a complete serverless architecture on AWS for the rez_agent system, including:

- **DynamoDB**: Message storage with GSIs for querying
- **SNS/SQS**: Event-driven messaging with dead letter queue
- **Lambda Functions**: Three serverless compute functions (Scheduler, Processor, WebAPI)
- **EventBridge**: Scheduled triggers for daily message creation
- **API Gateway HTTP API**: Serverless HTTP endpoint for WebAPI Lambda
- **CloudWatch**: Logging, metrics, and alarms
- **IAM**: Fine-grained permissions for each component
- **Systems Manager**: Parameter Store for configuration

## Prerequisites

### Required Tools

1. **Go 1.24+** - For building Lambda functions and Pulumi code
2. **Pulumi CLI** - For infrastructure deployment
   ```bash
   curl -fsSL https://get.pulumi.com | sh
   ```
3. **AWS CLI** - For AWS credentials and debugging
   ```bash
   # Install AWS CLI v2
   curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
   unzip awscliv2.zip
   sudo ./aws/install
   ```
4. **Docker** - For building Lambda containers (optional)

### AWS Setup

1. **AWS Account** - You need an AWS account with appropriate permissions
2. **AWS Credentials** - Configure your AWS credentials
   ```bash
   aws configure
   ```
   Or use environment variables:
   ```bash
   export AWS_ACCESS_KEY_ID="your-access-key"
   export AWS_SECRET_ACCESS_KEY="your-secret-key"
   export AWS_REGION="us-east-1"
   ```

3. **IAM Permissions** - Your AWS user/role needs permissions for:
   - DynamoDB (create tables, manage GSIs)
   - Lambda (create functions, manage permissions)
   - IAM (create roles and policies)
   - SNS/SQS (create topics and queues)
   - EventBridge (create rules and schedules)
   - CloudWatch (create log groups and alarms)
   - API Gateway v2 (create HTTP APIs, routes, integrations, stages)
   - Systems Manager (create parameters)

### Pulumi Backend

Choose your Pulumi backend:

**Option 1: Pulumi Cloud (Recommended for teams)**
```bash
pulumi login
```

**Option 2: Local Backend (For development)**
```bash
pulumi login --local
```

**Option 3: AWS S3 Backend**
```bash
pulumi login s3://your-bucket-name
```

## Architecture

### Infrastructure Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         rez_agent System                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  EventBridge (24h cron)  ──► Scheduler Lambda                    │
│                                    │                              │
│                                    ▼                              │
│                              DynamoDB Table                       │
│                                    │                              │
│                                    ▼                              │
│                               SNS Topic                           │
│                                    │                              │
│                                    ▼                              │
│                              SQS Queue ──► DLQ                    │
│                                    │                              │
│                                    ▼                              │
│                           Processor Lambda                        │
│                                    │                              │
│                                    ▼                              │
│                           ntfy.sh (HTTP)                          │
│                                                                   │
│  HTTP Client ──► API Gateway ──► WebAPI Lambda ──► DynamoDB/SNS  │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

### Lambda Functions

| Function | Trigger | Timeout | Memory | Purpose |
|----------|---------|---------|--------|---------|
| Scheduler | EventBridge (cron) | 60s | 256MB | Create scheduled messages |
| Processor | SQS (batch=10) | 300s | 512MB | Process messages, send to ntfy.sh |
| WebAPI | API Gateway HTTP | 30s | 256MB | REST API for frontend |

### DynamoDB Schema

**Table**: `rez-agent-messages-{stage}`
- **Partition Key**: `id` (String)
- **Sort Key**: `created_date` (String)
- **GSI 1**: `stage-created_date-index` (stage + created_date)
- **GSI 2**: `status-created_date-index` (status + created_date)
- **TTL**: Enabled on `ttl` attribute (90-day retention)
- **Billing**: Pay-per-request (on-demand)

## Project Structure

```
infrastructure/
├── Pulumi.yaml              # Pulumi project configuration
├── Pulumi.dev.yaml          # Dev stack configuration
├── Pulumi.prod.yaml         # Production stack configuration
├── go.mod                   # Go module dependencies
├── go.sum                   # Go module checksums
├── main.go                  # Main Pulumi program
└── README.md                # This file

../build/                    # Lambda deployment packages (created by make build)
├── scheduler.zip
├── processor.zip
└── webapi.zip
```

## Quick Start

### 1. Initialize Infrastructure

```bash
# From project root
make check-deps          # Verify all dependencies are installed
make dev-env             # Download Go dependencies
```

### 2. Build Lambda Functions

```bash
make build               # Build all Lambda functions (creates build/*.zip)
```

### 3. Initialize Pulumi

```bash
cd infrastructure
pulumi login             # Login to Pulumi backend
pulumi stack init dev    # Create dev stack
pulumi config set aws:region us-east-1  # Set AWS region
```

### 4. Deploy to Dev Environment

```bash
# From project root
make deploy-dev

# Or manually from infrastructure directory
cd infrastructure
pulumi up --stack dev
```

### 5. Verify Deployment

```bash
# Get infrastructure outputs
make infra-outputs

# Expected outputs:
# - dynamodbTableName: rez-agent-messages-dev
# - snsTopicArn: arn:aws:sns:...
# - sqsQueueUrl: https://sqs.us-east-1.amazonaws.com/...
# - apiGatewayId: abc123xyz
# - apiGatewayEndpoint: https://abc123xyz.execute-api.us-east-1.amazonaws.com
# - webapiUrl: https://abc123xyz.execute-api.us-east-1.amazonaws.com
```

### 6. Test the System

```bash
# Test WebAPI health endpoint
curl $(pulumi stack output webapiUrl --cwd infrastructure)/api/health

# Watch Lambda logs
make lambda-logs-scheduler  # In one terminal
make lambda-logs-processor  # In another terminal
make lambda-logs-webaction  # In another terminal
make lambda-logs-webapi     # In another terminal
```

## Configuration

### Stack Configuration

Each stack (dev, prod) has its own configuration file (`Pulumi.{stack}.yaml`):

```yaml
config:
  aws:region: us-east-1                        # AWS region
  rez-agent-infrastructure:stage: dev          # Stage name (dev/prod)
  rez-agent-infrastructure:ntfyUrl: https://ntfy.sh/rzesz-alerts
  rez-agent-infrastructure:logRetentionDays: 7 # CloudWatch log retention
  rez-agent-infrastructure:enableXRay: true    # Enable AWS X-Ray tracing
  rez-agent-infrastructure:schedulerCron: "cron(0 12 * * ? *)"  # Daily at 12:00 UTC
```

### Modifying Configuration

```bash
cd infrastructure

# Set configuration values
pulumi config set stage prod
pulumi config set logRetentionDays 30
pulumi config set schedulerCron "cron(0 8 * * ? *)"  # Change to 8:00 UTC

# View configuration
pulumi config

# Preview changes
pulumi preview

# Apply changes
pulumi up
```

### Environment-Specific Settings

| Setting | Dev | Prod |
|---------|-----|------|
| Log Retention | 7 days | 30 days |
| X-Ray Tracing | Enabled | Enabled |
| DynamoDB Billing | On-Demand | On-Demand |
| Scheduler Cron | 12:00 UTC | 12:00 UTC |

## Deployment

### Deploy to Development

```bash
make deploy-dev
```

This will:
1. Select the `dev` stack
2. Build all Lambda functions
3. Deploy infrastructure via `pulumi up`

### Deploy to Production

```bash
make deploy-prod
```

This will:
1. Select the `prod` stack
2. Build all Lambda functions
3. Deploy infrastructure via `pulumi up`

### Manual Deployment

```bash
# 1. Build Lambda functions
make build

# 2. Select stack
cd infrastructure
pulumi stack select dev  # or prod

# 3. Preview changes
pulumi preview

# 4. Deploy
pulumi up

# 5. View outputs
pulumi stack output
```

## Infrastructure Management

### Stack Operations

```bash
cd infrastructure

# List stacks
pulumi stack ls

# Select stack
pulumi stack select dev

# View stack outputs
pulumi stack output

# View specific output
pulumi stack output webapiUrl

# Export stack state
pulumi stack export > stack-backup.json

# Import stack state
pulumi stack import < stack-backup.json
```

### Update Infrastructure

```bash
# 1. Modify main.go or configuration
vim main.go
# or
pulumi config set logRetentionDays 14

# 2. Preview changes
pulumi preview

# 3. Apply changes
pulumi up

# 4. Rollback if needed
pulumi stack history    # View deployment history
pulumi stack rollback   # Rollback to previous state
```

### Destroy Infrastructure

```bash
# From project root
make infra-destroy

# Or manually
cd infrastructure
pulumi destroy --stack dev
```

**Warning**: This will delete all resources including data in DynamoDB!

### Resource Tagging

All resources are tagged with:
```
Project: rez-agent
Stage: dev/prod
ManagedBy: pulumi
Environment: dev/prod
```

## Monitoring and Debugging

### CloudWatch Logs

```bash
# Tail logs for each Lambda function
make lambda-logs-scheduler
make lambda-logs-processor
make lambda-logs-webaction
make lambda-logs-webapi

# Or manually with AWS CLI
aws logs tail /aws/lambda/rez-agent-scheduler-dev --follow
aws logs tail /aws/lambda/rez-agent-processor-dev --follow
aws logs tail /aws/lambda/rez-agent-webapi-dev --follow
```

### CloudWatch Alarms

The infrastructure creates the following alarms:

1. **DLQ Messages Alarm** - Alerts when messages appear in the dead letter queue
2. **Processor Errors Alarm** - Alerts when processor Lambda has errors (>5 in 10 minutes)

View alarms:
```bash
aws cloudwatch describe-alarms --alarm-name-prefix "rez-agent"
```

### Metrics

View custom metrics in CloudWatch:
```bash
aws cloudwatch list-metrics --namespace "rez-agent"
```

### X-Ray Tracing

When enabled, view traces in AWS X-Ray console:
```bash
# Open X-Ray console
aws xray get-trace-summaries --start-time $(date -u -d '1 hour ago' +%s) --end-time $(date -u +%s)
```

## Troubleshooting

### Common Issues

#### 1. Lambda Build Fails

**Problem**: `make build` fails with "cannot find package"

**Solution**:
```bash
go mod tidy
go mod download
make build
```

#### 2. Pulumi Up Fails - Missing Dependencies

**Problem**: `pulumi up` fails with "missing go.sum entry"

**Solution**:
```bash
cd infrastructure
go mod tidy
pulumi up
```

#### 3. Lambda Deployment Package Too Large

**Problem**: Lambda deployment fails with "Unzipped size must be smaller than..."

**Solution**:
```bash
# Verify build flags in Makefile
# Ensure CGO_ENABLED=0 and proper GOOS/GOARCH
cat Makefile | grep "go build"

# Rebuild with optimizations
make clean
make build
```

#### 4. API Gateway 403 Forbidden

**Problem**: API Gateway returns 403 Forbidden

**Solution**:
```bash
# Verify Lambda permissions for API Gateway
aws lambda get-policy --function-name rez-agent-webapi-dev

# Check if permission exists for apigateway.amazonaws.com
# Should see a statement with Principal: {"Service": "apigateway.amazonaws.com"}

# Test API endpoint
curl $(pulumi stack output webapiUrl --cwd infrastructure)/api/health

# Check API Gateway logs
aws logs tail /aws/lambda/rez-agent-webapi-dev --follow
```

#### 5. EventBridge Not Triggering Scheduler

**Problem**: Scheduler Lambda not being invoked

**Solution**:
```bash
# Check EventBridge rule
aws scheduler get-schedule --name rez-agent-daily-scheduler-dev

# Check EventBridge permissions
aws events list-targets-by-rule --rule rez-agent-daily-scheduler-dev

# Manually invoke Lambda for testing
aws lambda invoke --function-name rez-agent-scheduler-dev response.json
cat response.json
```

#### 6. SQS Messages Not Processing

**Problem**: Messages stuck in SQS queue

**Solution**:
```bash
# Check queue status
aws sqs get-queue-attributes \
  --queue-url $(pulumi stack output sqsQueueUrl --cwd infrastructure) \
  --attribute-names All

# Check DLQ
aws sqs get-queue-attributes \
  --queue-url $(pulumi stack output dlqUrl --cwd infrastructure) \
  --attribute-names ApproximateNumberOfMessages

# Check processor Lambda logs
make lambda-logs-processor
```

### Debug Mode

Enable verbose logging:

```bash
# Set environment variable for Pulumi
export PULUMI_DEBUG_COMMANDS=true

# Run with verbose output
pulumi up -v=3

# Lambda environment variables (update in main.go)
# Add "LOG_LEVEL": "DEBUG" to Lambda environment variables
```

## Cost Estimation

### Monthly Cost Breakdown (1,000 messages/day)

| Service | Usage | Monthly Cost |
|---------|-------|--------------|
| Lambda (Scheduler) | 30 invocations/month × 256MB × 1s | $0.00 |
| Lambda (Processor) | 30,000 invocations/month × 512MB × 2s | $0.86 |
| Lambda (WebAPI) | 10,000 invocations/month × 256MB × 0.5s | $0.10 |
| DynamoDB | 1GB storage + 100K reads/writes | $0.37 |
| SNS | 30,000 requests | $0.00 |
| SQS | 30,000 requests | $0.00 |
| API Gateway HTTP | 10,000 requests | $0.01 |
| CloudWatch Logs | 1GB ingestion + 1GB storage | $0.50 |
| CloudWatch Alarms | 2 alarms | $0.20 |
| **Total** | | **~$2.04/month** |

**Note**: Actual costs may vary. Use AWS Cost Explorer for precise tracking.

**Cost Optimization Tips**:
- API Gateway HTTP API is significantly cheaper than ALB (~$0.01/10K requests vs $16.20/month base)
- Reduce log retention (7 days in dev)
- Enable X-Ray sampling (5% instead of 100%)
- Use Reserved Concurrency only if needed

## API Gateway Features

The infrastructure uses **API Gateway HTTP API** which provides:

- **Low Cost**: Pay-per-request pricing (~$1 per million requests)
- **Automatic Scaling**: Handles traffic spikes automatically
- **Built-in Logging**: Access logs to CloudWatch
- **Lambda Proxy Integration**: Automatic request/response mapping
- **Default Route**: Catch-all route (`$default`) forwards all requests to WebAPI Lambda

### Access Logs

API Gateway access logs are automatically sent to the WebAPI CloudWatch log group and include:
- Request ID
- Source IP
- Request time
- HTTP method
- Route key
- Status code
- Protocol
- Response length

## Security Considerations

### IAM Least Privilege

Each Lambda function has its own IAM role with minimal permissions:
- Scheduler: DynamoDB PutItem, SNS Publish
- Processor: DynamoDB GetItem/UpdateItem, SQS Receive/Delete, SSM GetParameter
- WebAPI: DynamoDB Query/PutItem, SNS Publish

### Secrets Management

- ntfy.sh URL stored in SSM Parameter Store
- Future: OAuth credentials in SSM SecureString with KMS encryption

### Network Security

- API Gateway is publicly accessible (managed by AWS)
- Lambda functions are not in VPC (no VPC configuration required)
- Future: Add custom domain with ACM certificate for HTTPS
- Future: Use Lambda in VPC with VPC endpoints for AWS services

### Encryption

- DynamoDB: Encryption at rest (AWS managed keys)
- CloudWatch Logs: Encryption at rest (AWS managed keys)
- Future: KMS customer managed keys for enhanced security

## Advanced Topics

### Multi-Region Deployment

To deploy to multiple regions:

```bash
# Create stacks for each region
pulumi stack init us-east-1-dev
pulumi stack init us-west-2-dev

# Configure each stack
pulumi stack select us-east-1-dev
pulumi config set aws:region us-east-1

pulumi stack select us-west-2-dev
pulumi config set aws:region us-west-2

# Deploy to each region
pulumi up --stack us-east-1-dev
pulumi up --stack us-west-2-dev
```

### Blue-Green Deployment

Use Lambda aliases and weighted routing:

```bash
# Update Lambda code and create new version
aws lambda update-function-code --function-name rez-agent-processor-dev --zip-file fileb://build/processor.zip
aws lambda publish-version --function-name rez-agent-processor-dev

# Update alias to point to new version with weighted routing
aws lambda update-alias \
  --function-name rez-agent-processor-dev \
  --name live \
  --routing-config AdditionalVersionWeights={"2"=0.1}  # 10% traffic to new version

# Gradually increase traffic
aws lambda update-alias \
  --function-name rez-agent-processor-dev \
  --name live \
  --routing-config AdditionalVersionWeights={"2"=0.5}  # 50% traffic

# Full cutover
aws lambda update-alias \
  --function-name rez-agent-processor-dev \
  --name live \
  --function-version 2
```

### CI/CD Integration

Example GitHub Actions workflow:

```yaml
name: Deploy Infrastructure

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v1
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1

      - name: Setup Go
        uses: actions/setup-go@v2
        with:
          go-version: 1.24

      - name: Build Lambda functions
        run: make build

      - name: Setup Pulumi
        uses: pulumi/actions@v3
        with:
          command: up
          stack-name: dev
        env:
          PULUMI_ACCESS_TOKEN: ${{ secrets.PULUMI_ACCESS_TOKEN }}
```

## Support and Resources

### Documentation

- Project Architecture: `/workspaces/rez_agent/docs/architecture/README.md`
- API Specification: `/workspaces/rez_agent/docs/api/openapi.yaml`
- Implementation Summary: `/workspaces/rez_agent/IMPLEMENTATION_SUMMARY.md`

### AWS Resources

- [Lambda Go Runtime](https://docs.aws.amazon.com/lambda/latest/dg/golang-handler.html)
- [DynamoDB Best Practices](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/best-practices.html)
- [EventBridge Scheduler](https://docs.aws.amazon.com/scheduler/latest/UserGuide/what-is-scheduler.html)

### Pulumi Resources

- [Pulumi AWS Provider](https://www.pulumi.com/registry/packages/aws/)
- [Pulumi Go SDK](https://www.pulumi.com/docs/languages-sdks/go/)

### Community

- GitHub Issues: https://github.com/yourusername/rez_agent/issues
- Discussions: https://github.com/yourusername/rez_agent/discussions

## License

See project root LICENSE file.
