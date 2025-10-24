# rez_agent Infrastructure Documentation

## Quick Links

- **Infrastructure Code**: `/workspaces/rez_agent/infrastructure/`
- **Infrastructure README**: `/workspaces/rez_agent/infrastructure/README.md`
- **Architecture Documentation**: `/workspaces/rez_agent/docs/architecture/`
- **Build System**: `/workspaces/rez_agent/Makefile`

## Overview

The rez_agent infrastructure is defined using **Pulumi with Go**, providing Infrastructure as Code (IaC) for deploying a complete serverless event-driven messaging system on AWS.

## Quick Start

### One-Command Setup

```bash
./infrastructure/scripts/quick-start.sh
```

This interactive script will:
1. Check prerequisites (Go, Pulumi, AWS CLI)
2. Verify AWS credentials
3. Initialize Pulumi backend
4. Configure your environment (dev/prod)
5. Optionally build and deploy

### Manual Setup

```bash
# 1. Install dependencies
make check-deps

# 2. Build Lambda functions
make build

# 3. Deploy to dev
make deploy-dev

# 4. View outputs
make infra-outputs
```

## Infrastructure Components

### AWS Services Provisioned

| Service | Resource | Purpose |
|---------|----------|---------|
| **DynamoDB** | `rez-agent-messages-{stage}` | Message storage with GSIs |
| **SNS** | `rez-agent-messages-{stage}` | Event publishing |
| **SQS** | `rez-agent-messages-{stage}` | Message queue with DLQ |
| **Lambda** | `rez-agent-scheduler-{stage}` | Create scheduled messages (256MB, 60s) |
| **Lambda** | `rez-agent-processor-{stage}` | Process messages (512MB, 300s) |
| **Lambda** | `rez-agent-webapi-{stage}` | REST API (256MB, 30s) |
| **EventBridge** | `rez-agent-daily-scheduler-{stage}` | Daily cron trigger |
| **ALB** | `rez-agent-alb-{stage}` | HTTP endpoint for WebAPI |
| **CloudWatch** | Log Groups + Alarms | Observability |
| **IAM** | Roles + Policies | Fine-grained permissions |
| **SSM** | Parameter Store | Configuration management |

### Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                         User/Frontend                            │
└────────────┬─────────────────────────────────────────────────────┘
             │ HTTP
             ▼
┌────────────────────────────────────────────────────────────────┐
│                   Application Load Balancer                     │
│                    (rez-agent-alb-{stage})                      │
└────────────┬───────────────────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────────────────┐
│                    WebAPI Lambda (256MB)                        │
│               - GET/POST /api/messages                          │
│               - GET /api/metrics                                │
│               - GET /api/health                                 │
└─────┬──────────────────────────────────────┬──────────────────┘
      │                                       │
      │ Write                                 │ Publish
      ▼                                       ▼
┌─────────────────┐                 ┌──────────────────┐
│  DynamoDB Table │                 │    SNS Topic     │
│  rez-agent-     │                 │  rez-agent-      │
│  messages       │                 │  messages        │
│                 │                 │                  │
│  PK: id         │                 └────────┬─────────┘
│  SK: created_   │                          │
│      date       │                          │ Subscribe
│                 │                          ▼
│  GSI 1: stage- │                 ┌──────────────────┐
│         created │                 │    SQS Queue     │
│  GSI 2: status-│                 │  rez-agent-      │
│         created │                 │  messages        │
└────▲────────────┘                 │                  │
     │                              │  DLQ: 3 retries  │
     │                              └────────┬─────────┘
     │                                       │
     │ Read/Update                           │ Poll (batch=10)
     │                                       ▼
     │                              ┌──────────────────┐
     │                              │ Processor Lambda │
     │                              │     (512MB)      │
     └──────────────────────────────┤                  │
                                    │  - Process msgs  │
                                    │  - Send to ntfy  │
                                    └────────┬─────────┘
                                             │
                                             │ HTTP POST
                                             ▼
                                    ┌──────────────────┐
                                    │   ntfy.sh API    │
                                    │  (External)      │
                                    └──────────────────┘

┌────────────────────────────────────────────────────────────────┐
│                      EventBridge Scheduler                      │
│                   cron(0 12 * * ? *) - Daily                    │
└────────────┬───────────────────────────────────────────────────┘
             │
             │ Trigger
             ▼
┌────────────────────────────────────────────────────────────────┐
│                   Scheduler Lambda (256MB)                      │
│                  - Create daily message                         │
│                  - Write to DynamoDB                            │
│                  - Publish to SNS                               │
└─────┬──────────────────────────────────────┬──────────────────┘
      │                                       │
      │ Write                                 │ Publish
      ▼                                       ▼
 (DynamoDB)                               (SNS Topic)
```

## File Structure

```
/workspaces/rez_agent/
├── infrastructure/
│   ├── main.go                      # Main Pulumi program
│   ├── api-gateway-alternative.go   # Alternative API Gateway implementation
│   ├── go.mod                       # Pulumi Go dependencies
│   ├── Pulumi.yaml                  # Pulumi project configuration
│   ├── Pulumi.dev.yaml              # Dev environment config
│   ├── Pulumi.prod.yaml             # Prod environment config
│   ├── Pulumi.example.yaml          # Example configuration template
│   ├── .gitignore                   # Git ignore for infrastructure
│   ├── README.md                    # Comprehensive infrastructure guide
│   └── scripts/
│       ├── deploy.sh                # Deployment automation script
│       └── quick-start.sh           # Interactive setup wizard
├── build/                           # Lambda deployment packages (generated)
│   ├── scheduler.zip
│   ├── processor.zip
│   └── webapi.zip
├── Makefile                         # Build and deployment automation
├── .gitignore                       # Project-level git ignore
└── INFRASTRUCTURE.md                # This file
```

## Common Commands

### Build Commands

```bash
make build                 # Build all Lambda functions
make build-scheduler       # Build scheduler Lambda only
make build-processor       # Build processor Lambda only
make build-webapi          # Build webapi Lambda only
make clean                 # Clean build artifacts
```

### Infrastructure Commands

```bash
make infra-preview         # Preview infrastructure changes
make infra-up              # Deploy infrastructure
make infra-destroy         # Destroy infrastructure
make infra-outputs         # Show stack outputs
make infra-stack-dev       # Select dev stack
make infra-stack-prod      # Select prod stack
```

### Deployment Commands

```bash
make deploy-dev            # Build + deploy to dev
make deploy-prod           # Build + deploy to prod
```

### Monitoring Commands

```bash
make lambda-logs-scheduler # Tail scheduler Lambda logs
make lambda-logs-processor # Tail processor Lambda logs
make lambda-logs-webaction # Tail webaction Lambda logs
make lambda-logs-webapi    # Tail webapi Lambda logs
```

### Development Commands

```bash
make dev-env              # Set up development environment
make test                 # Run tests
make test-coverage        # Run tests with coverage
make fmt                  # Format Go code
make lint                 # Run linter
make validate             # Run all validation checks
```

### Utility Commands

```bash
make help                 # Show all available commands
make check-deps           # Check if dependencies are installed
```

## Configuration

### Environment Variables (Lambda Functions)

Each Lambda function receives these environment variables:

**Scheduler Lambda:**
- `DYNAMODB_TABLE`: DynamoDB table name
- `SNS_TOPIC_ARN`: SNS topic ARN
- `STAGE`: Environment stage (dev/prod)

**Processor Lambda:**
- `DYNAMODB_TABLE`: DynamoDB table name
- `NTFY_URL`: ntfy.sh notification URL
- `STAGE`: Environment stage (dev/prod)

**WebAPI Lambda:**
- `DYNAMODB_TABLE`: DynamoDB table name
- `SNS_TOPIC_ARN`: SNS topic ARN
- `STAGE`: Environment stage (dev/prod)

### Pulumi Configuration

Configuration is stored in `Pulumi.{stack}.yaml`:

```yaml
config:
  aws:region: us-east-1
  rez-agent-infrastructure:stage: dev
  rez-agent-infrastructure:ntfyUrl: https://ntfy.sh/rzesz-alerts
  rez-agent-infrastructure:logRetentionDays: 7
  rez-agent-infrastructure:enableXRay: true
  rez-agent-infrastructure:schedulerCron: "cron(0 12 * * ? *)"
```

Modify configuration:
```bash
cd infrastructure
pulumi config set stage prod
pulumi config set logRetentionDays 30
pulumi config set schedulerCron "cron(0 8 * * ? *)"
```

## Deployment Workflows

### Initial Deployment

```bash
# 1. Run quick start wizard
./infrastructure/scripts/quick-start.sh

# Or manually:

# 2. Check dependencies
make check-deps

# 3. Initialize Pulumi
cd infrastructure
pulumi login
pulumi stack init dev

# 4. Configure stack
pulumi config set aws:region us-east-1
pulumi config set stage dev
pulumi config set ntfyUrl https://ntfy.sh/rzesz-alerts
pulumi config set logRetentionDays 7
pulumi config set enableXRay true
pulumi config set schedulerCron "cron(0 12 * * ? *)"

# 5. Build and deploy
cd ..
make deploy-dev

# 6. Verify deployment
make infra-outputs
curl $(pulumi stack output webapiUrl --cwd infrastructure)/api/health
```

### Update Deployment

```bash
# 1. Make changes to infrastructure code or Lambda functions

# 2. Preview changes
make infra-preview

# 3. Deploy changes
make deploy-dev  # or make deploy-prod

# 4. Monitor deployment
make lambda-logs-webapi
```

### Rollback Deployment

```bash
cd infrastructure
pulumi stack history       # View deployment history
pulumi stack rollback      # Rollback to previous state
```

### Destroy Infrastructure

```bash
make infra-destroy

# Or with confirmation:
cd infrastructure
pulumi destroy --stack dev
```

## Stack Outputs

After deployment, the following outputs are available:

```bash
make infra-outputs

# Outputs:
# - dynamodbTableName: rez-agent-messages-dev
# - dynamodbTableArn: arn:aws:dynamodb:...
# - snsTopicArn: arn:aws:sns:...
# - sqsQueueUrl: https://sqs.us-east-1.amazonaws.com/...
# - sqsQueueArn: arn:aws:sqs:...
# - dlqUrl: https://sqs.us-east-1.amazonaws.com/...
# - dlqArn: arn:aws:sqs:...
# - schedulerLambdaArn: arn:aws:lambda:...
# - processorLambdaArn: arn:aws:lambda:...
# - webapiLambdaArn: arn:aws:lambda:...
# - albDnsName: rez-agent-alb-dev-123456789.us-east-1.elb.amazonaws.com
# - albArn: arn:aws:elasticloadbalancing:...
# - webapiUrl: http://rez-agent-alb-dev-123456789.us-east-1.elb.amazonaws.com
```

Use outputs in scripts:
```bash
# Get WebAPI URL
WEBAPI_URL=$(pulumi stack output webapiUrl --cwd infrastructure)

# Test health endpoint
curl $WEBAPI_URL/api/health

# Get DynamoDB table name
TABLE_NAME=$(pulumi stack output dynamodbTableName --cwd infrastructure)

# Query DynamoDB
aws dynamodb scan --table-name $TABLE_NAME
```

## Cost Estimation

### Monthly Costs (1,000 messages/day)

| Service | Cost |
|---------|------|
| Lambda (all functions) | $0.96 |
| DynamoDB (on-demand) | $0.37 |
| SNS/SQS | $0.50 |
| ALB | $16.20 |
| CloudWatch | $0.70 |
| **Total** | **~$18.23** |

### Cost Optimization

**Option 1: Use API Gateway instead of ALB**
- Replace ALB with API Gateway HTTP API
- Savings: ~$13/month
- See: `infrastructure/api-gateway-alternative.go`

**Option 2: Reduce log retention**
- Dev: 7 days, Prod: 30 days
- Already configured by default

**Option 3: Lambda optimization**
- Right-size memory allocations
- Use ARM64 Graviton2 processors (20% savings)

## Alternative: API Gateway

For lower-traffic applications, API Gateway is more cost-effective than ALB:

**Comparison:**
- ALB: $16.20/month fixed + data processing
- API Gateway: $3.50/month for 1M requests

**To use API Gateway:**
1. See implementation in `infrastructure/api-gateway-alternative.go`
2. Replace ALB section in `main.go` with API Gateway code
3. Redeploy: `make deploy-dev`

## Security Best Practices

### IAM Least Privilege
- Each Lambda has its own role
- Minimal permissions granted
- No wildcard permissions

### Secrets Management
- ntfy.sh URL in SSM Parameter Store
- Future: OAuth credentials in SecureString with KMS

### Network Security
- ALB security group restricts traffic
- Future: Add HTTPS with ACM certificate
- Future: VPC endpoints for AWS services

### Encryption
- DynamoDB encrypted at rest (AWS managed keys)
- CloudWatch Logs encrypted at rest
- Future: KMS customer managed keys

## Monitoring and Observability

### CloudWatch Logs

Each Lambda function has its own log group:
- `/aws/lambda/rez-agent-scheduler-{stage}`
- `/aws/lambda/rez-agent-processor-{stage}`
- `/aws/lambda/rez-agent-webapi-{stage}`

Retention: 7 days (dev), 30 days (prod)

### CloudWatch Alarms

1. **DLQ Messages Alarm**
   - Triggers when messages appear in DLQ
   - Indicates permanent failures

2. **Processor Errors Alarm**
   - Triggers on >5 errors in 10 minutes
   - Indicates processing issues

### AWS X-Ray

When enabled (`enableXRay: true`):
- Distributed tracing across all Lambdas
- Visualize request flow
- Identify performance bottlenecks

View traces:
```bash
# AWS Console
open https://console.aws.amazon.com/xray/home

# Or CLI
aws xray get-trace-summaries --start-time $(date -u -d '1 hour ago' +%s) --end-time $(date -u +%s)
```

## Troubleshooting

See comprehensive troubleshooting guide in:
- `/workspaces/rez_agent/infrastructure/README.md` (Troubleshooting section)

Common issues:
1. Lambda build fails → Run `go mod tidy`
2. Pulumi fails → Check AWS credentials
3. ALB health check fails → Verify `/api/health` endpoint
4. EventBridge not triggering → Check scheduler permissions
5. SQS messages stuck → Check processor Lambda logs

## CI/CD Integration

### GitHub Actions Example

See `/workspaces/rez_agent/infrastructure/README.md` for complete GitHub Actions workflow.

### GitLab CI Example

```yaml
stages:
  - build
  - deploy

build:
  stage: build
  script:
    - make build
  artifacts:
    paths:
      - build/

deploy:
  stage: deploy
  script:
    - cd infrastructure
    - pulumi up --yes --stack dev
  only:
    - main
```

## Multi-Region Deployment

Deploy to multiple AWS regions:

```bash
# Create region-specific stacks
pulumi stack init us-east-1-prod
pulumi stack init us-west-2-prod

# Configure each stack
pulumi stack select us-east-1-prod
pulumi config set aws:region us-east-1

pulumi stack select us-west-2-prod
pulumi config set aws:region us-west-2

# Deploy to each region
pulumi up --stack us-east-1-prod
pulumi up --stack us-west-2-prod
```

## Additional Resources

### Documentation
- [Architecture Overview](/workspaces/rez_agent/ARCHITECTURE_SUMMARY.md)
- [Implementation Summary](/workspaces/rez_agent/IMPLEMENTATION_SUMMARY.md)
- [Quick Start Guide](/workspaces/rez_agent/QUICK_START.md)
- [Infrastructure README](/workspaces/rez_agent/infrastructure/README.md)

### External Links
- [Pulumi AWS Provider](https://www.pulumi.com/registry/packages/aws/)
- [AWS Lambda Go Runtime](https://docs.aws.amazon.com/lambda/latest/dg/golang-handler.html)
- [EventBridge Scheduler](https://docs.aws.amazon.com/scheduler/latest/UserGuide/what-is-scheduler.html)

## Support

For issues or questions:
1. Check documentation in `/workspaces/rez_agent/docs/`
2. Review troubleshooting guide in infrastructure README
3. Open GitHub issue: https://github.com/yourusername/rez_agent/issues

---

**Last Updated**: 2025-10-21
**Pulumi Version**: v3.144.1
**AWS Provider Version**: v6.67.0
**Go Version**: 1.24
