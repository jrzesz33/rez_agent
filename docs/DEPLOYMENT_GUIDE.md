# Deployment Guide

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Initial Setup](#initial-setup)
4. [Deployment Environments](#deployment-environments)
5. [Deployment Process](#deployment-process)
6. [Configuration](#configuration)
7. [Secrets Management](#secrets-management)
8. [Monitoring and Verification](#monitoring-and-verification)
9. [Rollback Procedures](#rollback-procedures)
10. [Troubleshooting](#troubleshooting)
11. [CI/CD Pipeline](#cicd-pipeline)

## Overview

rez_agent uses Pulumi for infrastructure as code (IaC) and AWS Lambda for serverless compute. The deployment process is automated and supports multiple environments (dev, stage, prod).

### Deployment Architecture

```
GitHub Repository
    ↓
Build Process (Make)
    ↓
Lambda ZIP Packages
    ↓
S3 Deployment Bucket
    ↓
Pulumi Deployment
    ↓
AWS Resources
```

### Deployed Resources

Each deployment creates:

- **6 Lambda Functions**: scheduler, processor, webaction, webapi, agent, mcp
- **4 SNS Topics**: web-actions, notifications, agent-response, schedule-creation
- **2 SQS Queues**: web-actions, notifications
- **3 DynamoDB Tables**: messages, schedules, web-action-results
- **1 API Gateway HTTP API**: for Web API Lambda
- **1 Lambda Function URL**: for MCP Server
- **EventBridge Scheduler**: for recurring tasks
- **CloudWatch Log Groups**: for Lambda logs
- **IAM Roles and Policies**: for Lambda execution

## Prerequisites

### Tools

Ensure you have installed:

- **Go 1.24+**
- **Python 3.12+**
- **AWS CLI** (configured with credentials)
- **Pulumi CLI**
- **Docker**
- **Make**
- **Git**

### AWS Account Setup

1. **Create AWS Account**: [Sign up](https://aws.amazon.com/)

2. **Create IAM User** with permissions:
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": [
           "lambda:*",
           "dynamodb:*",
           "sns:*",
           "sqs:*",
           "s3:*",
           "iam:*",
           "apigateway:*",
           "scheduler:*",
           "events:*",
           "logs:*",
           "cloudwatch:*",
           "secretsmanager:*"
         ],
         "Resource": "*"
       }
     ]
   }
   ```

   **Note**: For production, use more restrictive policies.

3. **Configure AWS CLI**:
   ```bash
   aws configure
   # AWS Access Key ID: YOUR_ACCESS_KEY
   # AWS Secret Access Key: YOUR_SECRET_KEY
   # Default region name: us-east-1
   # Default output format: json
   ```

4. **Verify configuration**:
   ```bash
   aws sts get-caller-identity
   ```

### Pulumi Account Setup

1. **Create account**: [Sign up](https://app.pulumi.com/signup)

2. **Login**:
   ```bash
   pulumi login
   ```

3. **Create organization** (optional for team deployments)

## Initial Setup

### 1. Clone Repository

```bash
git clone https://github.com/jrzesz33/rez_agent.git
cd rez_agent
```

### 2. Install Dependencies

```bash
# Install Go dependencies
go mod download

# Install infrastructure dependencies
cd infrastructure
go mod download
cd ..

# Check all dependencies
make check-deps
```

### 3. Create Pulumi Stack

```bash
cd infrastructure

# Initialize Pulumi (first time only)
pulumi login

# Create dev stack
pulumi stack init dev

# Or select existing stack
pulumi stack select dev
```

### 4. Configure Stack

```bash
# Required configuration
pulumi config set ntfyUrl https://ntfy.sh/your-unique-topic
pulumi config set stage dev

# Optional configuration
pulumi config set logRetentionDays 7
pulumi config set enableXRay false
pulumi config set schedulerCron "cron(0 12 * * ? *)"
```

### 5. Create Secrets

```bash
# Create golf credentials secret
aws secretsmanager create-secret \
    --name rez-agent/golf/credentials-dev \
    --secret-string '{
      "username": "your-email@example.com",
      "password": "your-password"
    }' \
    --region us-east-1
```

## Deployment Environments

### Development (dev)

**Purpose**: Active development and testing

**Characteristics**:
- Lower costs (minimal log retention)
- Relaxed rate limits
- Test data only
- Frequent deployments

**Configuration**:
```bash
pulumi stack select dev
pulumi config set stage dev
pulumi config set logRetentionDays 3
```

### Staging (stage)

**Purpose**: Pre-production testing and validation

**Characteristics**:
- Production-like configuration
- Test data with production schema
- Performance testing
- Integration testing

**Configuration**:
```bash
pulumi stack select stage
pulumi config set stage stage
pulumi config set logRetentionDays 7
```

### Production (prod)

**Purpose**: Live production environment

**Characteristics**:
- Maximum reliability
- Extended log retention
- X-Ray tracing enabled
- Provisioned concurrency (optional)

**Configuration**:
```bash
pulumi stack select prod
pulumi config set stage prod
pulumi config set logRetentionDays 30
pulumi config set enableXRay true
```

## Deployment Process

### Quick Deployment

```bash
# Deploy to dev environment
make deploy-dev

# Deploy to prod environment
make deploy-prod
```

### Step-by-Step Deployment

#### 1. Build Lambda Functions

```bash
make build
```

This creates ZIP files in `build/`:
- `scheduler.zip`
- `processor.zip`
- `webaction.zip`
- `webapi.zip`
- `agent.zip`
- `mcp.zip`

#### 2. Preview Infrastructure Changes

```bash
cd infrastructure
pulumi preview
```

Review the changes Pulumi will make.

#### 3. Deploy Infrastructure

```bash
pulumi up
```

**Prompts**:
- Review the plan
- Confirm with "yes"
- Wait for deployment (typically 3-5 minutes)

#### 4. Verify Deployment

```bash
# Get stack outputs
pulumi stack output

# Outputs:
# WebApiUrl: https://xxxxx.execute-api.us-east-1.amazonaws.com
# McpServerUrl: https://xxxxx.lambda-url.us-east-1.on.aws/
# MessagesTableName: rez-agent-messages-dev
# SchedulesTableName: rez-agent-schedules-dev
```

#### 5. Test Deployment

```bash
# Test Web API
curl -X POST $(pulumi stack output WebApiUrl)/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "notify",
    "stage": "dev",
    "payload": {
      "message": "Deployment test"
    }
  }'

# Check CloudWatch logs
aws logs tail /aws/lambda/rez-agent-processor-dev --follow
```

### Deployment Order

Pulumi handles dependency ordering automatically:

1. **DynamoDB Tables**: Created first
2. **SNS Topics**: Created next
3. **SQS Queues**: Subscribe to SNS topics
4. **IAM Roles**: For Lambda execution
5. **Lambda Functions**: Deployed with code
6. **API Gateway**: Routes to Web API Lambda
7. **EventBridge Schedules**: Configured last

## Configuration

### Pulumi Configuration Files

Configuration is stored in `infrastructure/Pulumi.{stack}.yaml`:

```yaml
# Pulumi.dev.yaml
config:
  rez_agent:ntfyUrl: https://ntfy.sh/rzesz-dev
  rez_agent:stage: dev
  rez_agent:logRetentionDays: "7"
  rez_agent:enableXRay: "false"
  rez_agent:schedulerCron: cron(0 12 * * ? *)
```

### Environment-Specific Settings

| Setting | Dev | Stage | Prod |
|---------|-----|-------|------|
| Log Retention | 3 days | 7 days | 30 days |
| X-Ray Tracing | Disabled | Enabled | Enabled |
| Lambda Memory | 256 MB | 512 MB | 512 MB |
| Reserved Concurrency | None | None | Optional |
| Alarms | Minimal | Basic | Comprehensive |

### Updating Configuration

```bash
# Change configuration
pulumi config set logRetentionDays 14

# Preview changes
pulumi preview

# Apply changes
pulumi up
```

## Secrets Management

### Creating Secrets

```bash
# Golf credentials
aws secretsmanager create-secret \
    --name rez-agent/golf/credentials-{stage} \
    --secret-string '{
      "username": "email@example.com",
      "password": "password123"
    }' \
    --region us-east-1

# API keys (if needed)
aws secretsmanager create-secret \
    --name rez-agent/api-keys/service-name-{stage} \
    --secret-string '{"api_key": "your-api-key"}' \
    --region us-east-1
```

### Updating Secrets

```bash
aws secretsmanager update-secret \
    --secret-id rez-agent/golf/credentials-dev \
    --secret-string '{
      "username": "new-email@example.com",
      "password": "new-password"
    }' \
    --region us-east-1
```

### Viewing Secrets

```bash
aws secretsmanager get-secret-value \
    --secret-id rez-agent/golf/credentials-dev \
    --query SecretString \
    --output text
```

### Secret Rotation

For production:

1. **Enable automatic rotation**:
   ```bash
   aws secretsmanager rotate-secret \
       --secret-id rez-agent/golf/credentials-prod \
       --rotation-lambda-arn arn:aws:lambda:... \
       --rotation-rules AutomaticallyAfterDays=30
   ```

2. **Create rotation Lambda** (separate from rez_agent)

## Monitoring and Verification

### CloudWatch Logs

```bash
# Tail specific Lambda logs
aws logs tail /aws/lambda/rez-agent-scheduler-dev --follow
aws logs tail /aws/lambda/rez-agent-processor-dev --follow
aws logs tail /aws/lambda/rez-agent-webaction-dev --follow
aws logs tail /aws/lambda/rez-agent-webapi-dev --follow

# Or use Make targets
make lambda-logs-scheduler
make lambda-logs-processor
make lambda-logs-webaction
make lambda-logs-webapi
```

### CloudWatch Metrics

Monitor in AWS Console:

1. **Lambda Invocations**: Requests per minute
2. **Lambda Errors**: Error rate
3. **Lambda Duration**: Execution time
4. **SQS Queue Depth**: Messages waiting
5. **DynamoDB Throttles**: Capacity issues

### Health Checks

```bash
# Test Web API
curl $(pulumi stack output WebApiUrl)/health

# Test MCP Server
curl $(pulumi stack output McpServerUrl)/health

# Check DynamoDB
aws dynamodb describe-table \
    --table-name $(pulumi stack output MessagesTableName)

# Check SNS topics
aws sns list-topics | grep rez-agent
```

### X-Ray Tracing

If enabled, view traces in AWS X-Ray console:

1. Open [X-Ray Console](https://console.aws.amazon.com/xray/)
2. Select time range
3. View service map
4. Analyze traces

## Rollback Procedures

### Rollback to Previous Deployment

Pulumi maintains deployment history:

```bash
# View deployment history
pulumi history

# Rollback to specific version
pulumi stack export --version 42 > rollback.json
pulumi stack import < rollback.json
pulumi up
```

### Emergency Rollback

```bash
# Disable all schedules
aws scheduler list-schedules --query "Schedules[?starts_with(Name, 'rez-agent')].Name" \
    | jq -r '.[]' \
    | xargs -I {} aws scheduler update-schedule \
        --name {} \
        --state DISABLED

# Stop processing new messages
aws lambda update-function-configuration \
    --function-name rez-agent-processor-dev \
    --reserved-concurrent-executions 0

# Purge SQS queues
aws sqs purge-queue \
    --queue-url $(pulumi stack output | grep QueueUrl | awk '{print $2}')
```

### Gradual Rollout

For production deployments:

1. **Deploy to canary** (10% of traffic):
   ```go
   // In infrastructure/main.go
   Aliases: &lambda.FunctionAliasArgs{
       Name:    pulumi.String("live"),
       FunctionVersion: newVersion.Version,
       RoutingConfig: &lambda.FunctionAliasRoutingConfigArgs{
           AdditionalVersionWeights: pulumi.Float64Map{
               previousVersion.Version: pulumi.Float64(0.9),
           },
       },
   }
   ```

2. **Monitor metrics** for 30 minutes

3. **Shift 100% traffic** if healthy

## Troubleshooting

### Common Issues

#### Issue: Pulumi deployment fails

**Error**: `error: update failed`

**Solutions**:

1. Check AWS credentials:
   ```bash
   aws sts get-caller-identity
   ```

2. Verify IAM permissions:
   ```bash
   aws iam get-user
   ```

3. Check Pulumi state:
   ```bash
   pulumi refresh
   pulumi up
   ```

#### Issue: Lambda function timeout

**Error**: `Task timed out after 30.00 seconds`

**Solutions**:

1. Increase timeout in `infrastructure/main.go`:
   ```go
   Timeout: pulumi.Int(180), // 3 minutes
   ```

2. Check for infinite loops in code

3. Optimize slow operations

#### Issue: DynamoDB capacity exceeded

**Error**: `ProvisionedThroughputExceededException`

**Solutions**:

Tables use on-demand capacity mode, so this shouldn't occur. If it does:

1. Check for infinite retry loops
2. Review batch sizes
3. Add exponential backoff

#### Issue: OAuth authentication failing

**Error**: `OAuth authentication failed: invalid credentials`

**Solutions**:

1. Verify credentials in Secrets Manager:
   ```bash
   aws secretsmanager get-secret-value \
       --secret-id rez-agent/golf/credentials-dev
   ```

2. Test credentials manually

3. Check if credentials expired

#### Issue: SSRF protection blocking valid requests

**Error**: `host not in allowlist: newhost.com`

**Solutions**:

1. Add host to allowlist in `internal/models/webaction.go`:
   ```go
   var AllowedHosts = map[string]bool{
       "api.weather.gov":    true,
       "birdsfoot.cps.golf": true,
       "newhost.com":        true,
   }
   ```

2. Rebuild and redeploy:
   ```bash
   make build-webaction
   pulumi up
   ```

### Debug Mode

Enable debug logging:

```bash
# Set in Pulumi config
pulumi config set logLevel DEBUG

# Or set environment variable in Lambda
aws lambda update-function-configuration \
    --function-name rez-agent-webaction-dev \
    --environment Variables={LOG_LEVEL=DEBUG}
```

### Support Channels

- **GitHub Issues**: Report bugs
- **CloudWatch Logs**: View Lambda logs
- **X-Ray**: Trace requests
- **Pulumi Logs**: `pulumi logs`

## CI/CD Pipeline

### GitHub Actions Workflow

Create `.github/workflows/deploy.yml`:

```yaml
name: Deploy

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.24'

      - name: Run tests
        run: make test

      - name: Run linter
        run: make lint

  deploy-dev:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.24'

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1

      - name: Build
        run: make build

      - name: Install Pulumi CLI
        uses: pulumi/setup-pulumi@v2

      - name: Deploy to dev
        run: |
          cd infrastructure
          pulumi stack select dev
          pulumi up --yes
        env:
          PULUMI_ACCESS_TOKEN: ${{ secrets.PULUMI_ACCESS_TOKEN }}
```

### Required GitHub Secrets

Add these secrets in GitHub repository settings:

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `PULUMI_ACCESS_TOKEN`

### Deployment Workflow

1. **Push to feature branch** → Run tests
2. **Open PR** → Run tests + preview
3. **Merge to main** → Deploy to dev
4. **Tag release** → Deploy to prod

### Manual Production Deployment

```bash
# After dev deployment succeeds
git tag v1.0.0
git push origin v1.0.0

# Deploy to prod
cd infrastructure
pulumi stack select prod
pulumi up
```

## Best Practices

1. **Always preview before deploy**: Use `pulumi preview`
2. **Deploy to dev first**: Test in dev before prod
3. **Monitor after deployment**: Watch logs and metrics
4. **Use stack outputs**: Don't hardcode URLs
5. **Version your deployments**: Tag releases in Git
6. **Document changes**: Update CHANGELOG.md
7. **Rotate secrets regularly**: Every 30-90 days
8. **Enable X-Ray in prod**: For better debugging
9. **Set up alarms**: Monitor error rates
10. **Backup DynamoDB**: Enable point-in-time recovery

## Maintenance

### Regular Tasks

**Weekly**:
- Review CloudWatch logs for errors
- Check Lambda execution times
- Monitor costs in AWS Cost Explorer

**Monthly**:
- Review and rotate secrets
- Update dependencies
- Check for AWS service updates
- Review and archive old DynamoDB data

**Quarterly**:
- Review IAM policies
- Audit security settings
- Performance optimization
- Cost optimization review

### Cleanup

Remove old resources:

```bash
# Destroy dev environment
cd infrastructure
pulumi stack select dev
pulumi destroy

# Delete stack
pulumi stack rm dev
```

## Additional Resources

- [AWS Lambda Best Practices](https://docs.aws.amazon.com/lambda/latest/dg/best-practices.html)
- [Pulumi Documentation](https://www.pulumi.com/docs/)
- [AWS Well-Architected Framework](https://aws.amazon.com/architecture/well-architected/)
- [Go Project Layout](https://github.com/golang-standards/project-layout)
