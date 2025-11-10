# Local Testing with AWS SAM

This guide explains how to test rez_agent Lambda functions locally using AWS SAM (Serverless Application Model).

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [SAM Setup](#sam-setup)
3. [Building for Local Testing](#building-for-local-testing)
4. [Deploying Local Stack](#deploying-local-stack)
5. [Invoking Functions Locally](#invoking-functions-locally)
6. [Testing the API Gateway](#testing-the-api-gateway)
7. [Testing with DynamoDB Local](#testing-with-dynamodb-local)
8. [Debugging](#debugging)
9. [Common Issues](#common-issues)

## Prerequisites

### Required Tools

- **AWS SAM CLI**: [Installation Guide](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/install-sam-cli.html)
- **Docker**: Required by SAM for local execution
- **AWS CLI**: Configured with credentials
- **Make**: For build automation

### Verify Installation

```bash
# Check SAM version
sam --version

# Check Docker
docker --version

# Check AWS credentials
aws sts get-caller-identity
```

## SAM Setup

### Project Files

The project includes these SAM-related files:

- **template.yaml**: SAM template defining all resources
- **samconfig.toml**: SAM CLI configuration
- **events/**: Test event JSON files for each function

### Template Structure

The `template.yaml` defines:
- 6 Lambda functions (scheduler, processor, webaction, webapi, agent, mcp)
- 3 DynamoDB tables (messages, schedules, web-action-results)
- 4 SNS topics (web-actions, notifications, agent-response, schedule-creation)
- 2 SQS queues (web-actions, notifications)
- IAM roles and policies
- API Gateway HTTP API

## Building for Local Testing

### 1. Build Lambda Functions

```bash
# Build all functions
make build

# Or build specific functions
make build-scheduler
make build-processor
make build-webaction
make build-webapi
make build-agent
make build-mcp
```

This creates ZIP files in `build/`:
- `scheduler.zip`
- `processor.zip`
- `webaction.zip`
- `webapi.zip`
- `agent.zip`
- `mcp.zip`

### 2. Validate SAM Template

```bash
sam validate --lint
```

### 3. Build SAM Application

```bash
sam build
```

This command:
- Validates the template
- Builds Docker images for Lambda functions
- Prepares the application for deployment

## Deploying Local Stack

### Deploy to AWS (Optional)

For a complete local-like environment in AWS:

```bash
# Deploy with guided setup
sam deploy --guided

# Or use saved config
sam deploy
```

This creates:
- All Lambda functions
- DynamoDB tables with local-specific names
- SNS topics and SQS queues
- API Gateway endpoints

### Stack Outputs

```bash
# View stack outputs
sam list stack-outputs

# Or use AWS CLI
aws cloudformation describe-stacks \
    --stack-name rez-agent-local \
    --query 'Stacks[0].Outputs'
```

## Invoking Functions Locally

### Local Invoke (Individual Function)

```bash
# Invoke scheduler function
sam local invoke SchedulerFunction \
    --event events/scheduler-event.json

# Invoke webaction function
sam local invoke WebActionFunction \
    --event events/webaction-sqs-event.json

# Invoke processor function
sam local invoke ProcessorFunction \
    --event events/processor-sqs-event.json

# Invoke webapi function
sam local invoke WebApiFunction \
    --event events/webapi-create-message.json

# Invoke MCP function
sam local invoke McpFunction \
    --event events/mcp-request.json
```

### Invoke with Custom Environment Variables

```bash
sam local invoke WebActionFunction \
    --event events/webaction-golf-event.json \
    --env-vars env.json

# env.json example:
{
  "WebActionFunction": {
    "LOG_LEVEL": "DEBUG",
    "STAGE": "dev"
  }
}
```

### Invoke with Docker Network

```bash
# Start Docker network
docker network create rez-agent-local

# Invoke with network
sam local invoke WebActionFunction \
    --event events/webaction-sqs-event.json \
    --docker-network rez-agent-local
```

## Testing the API Gateway

### Start Local API Gateway

```bash
# Start API Gateway on port 3000
sam local start-api
```

The API will be available at: `http://localhost:3000`

### Test Web API Endpoints

```bash
# Create a message
curl -X POST http://localhost:3000/api/messages \
    -H "Content-Type: application/json" \
    -d '{
        "message_type": "notify",
        "stage": "dev",
        "payload": {
            "message": "Test from SAM local",
            "title": "SAM Test"
        }
    }'

# Create a schedule
curl -X POST http://localhost:3000/api/schedules \
    -H "Content-Type: application/json" \
    -d '{
        "action": "create",
        "name": "test-schedule",
        "schedule_expression": "cron(0 12 * * ? *)",
        "timezone": "America/New_York",
        "target_type": "web_action",
        "message_type": "web_action",
        "payload": {
            "version": "1.0",
            "action": "weather",
            "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast"
        }
    }'
```

### Test MCP Endpoint

```bash
curl -X POST http://localhost:3000/ \
    -H "Content-Type: application/json" \
    -d '{
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/call",
        "params": {
            "name": "get_weather_forecast",
            "arguments": {
                "location": "Kansas"
            }
        }
    }'
```

### Start API with Hot Reload

```bash
# Watch for changes and auto-reload
sam sync --watch
```

## Testing with DynamoDB Local

### 1. Start DynamoDB Local

```bash
# Start DynamoDB in Docker
docker run -d -p 8000:8000 --name dynamodb-local \
    --network rez-agent-local \
    amazon/dynamodb-local

# Verify it's running
aws dynamodb list-tables \
    --endpoint-url http://localhost:8000 \
    --region us-east-1
```

### 2. Create Local Tables

```bash
# Create messages table
aws dynamodb create-table \
    --table-name rez-agent-messages-local \
    --attribute-definitions AttributeName=id,AttributeType=S \
    --key-schema AttributeName=id,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST \
    --endpoint-url http://localhost:8000 \
    --region us-east-1

# Create schedules table
aws dynamodb create-table \
    --table-name rez-agent-schedules-local \
    --attribute-definitions AttributeName=id,AttributeType=S \
    --key-schema AttributeName=id,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST \
    --endpoint-url http://localhost:8000 \
    --region us-east-1

# Create web action results table
aws dynamodb create-table \
    --table-name rez-agent-web-action-results-local \
    --attribute-definitions AttributeName=id,AttributeType=S \
    --key-schema AttributeName=id,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST \
    --endpoint-url http://localhost:8000 \
    --region us-east-1
```

### 3. Configure Lambda to Use Local DynamoDB

Create `env.json`:

```json
{
  "Parameters": {
    "DYNAMODB_ENDPOINT": "http://dynamodb-local:8000"
  }
}
```

Invoke with:

```bash
sam local invoke WebApiFunction \
    --event events/webapi-create-message.json \
    --env-vars env.json \
    --docker-network rez-agent-local
```

### 4. Query Local Tables

```bash
# Scan messages table
aws dynamodb scan \
    --table-name rez-agent-messages-local \
    --endpoint-url http://localhost:8000 \
    --region us-east-1

# Get specific message
aws dynamodb get-item \
    --table-name rez-agent-messages-local \
    --key '{"id": {"S": "msg_20240115120000_123456"}}' \
    --endpoint-url http://localhost:8000 \
    --region us-east-1
```

## Debugging

### Enable Debug Mode

```bash
# Start API with debug port
sam local start-api --debug-port 5858

# Invoke function with debug port
sam local invoke WebActionFunction \
    --event events/webaction-sqs-event.json \
    --debug-port 5858
```

### Attach Debugger (VS Code)

Add to `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Attach to SAM Local",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "remotePath": "",
      "port": 5858,
      "host": "localhost",
      "showLog": true,
      "trace": "verbose"
    }
  ]
}
```

Start debugging:
1. Run `sam local invoke` with `--debug-port 5858`
2. In VS Code, press F5 or start "Attach to SAM Local"
3. Set breakpoints in your code

### View Lambda Logs

```bash
# SAM logs are printed to stdout
# Add --log-file to save logs

sam local invoke WebActionFunction \
    --event events/webaction-sqs-event.json \
    --log-file sam-local.log
```

### Tail Logs During API Testing

```bash
# In one terminal, start API
sam local start-api

# Logs appear in same terminal
# Or redirect to file:
sam local start-api 2>&1 | tee api-logs.txt
```

## Common Issues

### Issue: Function timeout

**Error**: `Function timed out after 180 seconds`

**Solutions**:

1. Increase timeout in `template.yaml`:
   ```yaml
   Globals:
     Function:
       Timeout: 300
   ```

2. Rebuild:
   ```bash
   sam build
   ```

### Issue: Docker permission denied

**Error**: `Cannot connect to the Docker daemon`

**Solutions**:

1. Start Docker Desktop
2. Add user to docker group:
   ```bash
   sudo usermod -aG docker $USER
   newgrp docker
   ```

### Issue: Port already in use

**Error**: `Port 3000 is already in use`

**Solutions**:

1. Use different port:
   ```bash
   sam local start-api --port 3001
   ```

2. Kill process using port:
   ```bash
   lsof -ti:3000 | xargs kill -9
   ```

### Issue: Unable to import module

**Error**: `Runtime.ImportModuleError: Unable to import module 'bootstrap'`

**Solutions**:

1. Rebuild Lambda functions:
   ```bash
   make clean
   make build
   sam build
   ```

2. Verify ZIP file contents:
   ```bash
   unzip -l build/webaction.zip
   ```

### Issue: DynamoDB connection refused

**Error**: `ResourceNotFoundException: Cannot do operations on a non-existent table`

**Solutions**:

1. Verify DynamoDB Local is running:
   ```bash
   docker ps | grep dynamodb-local
   ```

2. Verify Docker network:
   ```bash
   docker network inspect rez-agent-local
   ```

3. Create tables (see [Testing with DynamoDB Local](#testing-with-dynamodb-local))

### Issue: Secrets Manager access denied

**Error**: `AccessDeniedException: User is not authorized`

**Solutions**:

1. Create local secrets:
   ```bash
   aws secretsmanager create-secret \
       --name rez-agent/golf/credentials-dev \
       --secret-string '{"username":"test","password":"test"}'
   ```

2. Or use environment variables instead of Secrets Manager for local testing

## Test Event Files

The `events/` directory contains test event files:

### Scheduler Events

- **scheduler-event.json**: EventBridge schedule trigger

### SQS Events

- **webaction-sqs-event.json**: Weather action request
- **webaction-golf-event.json**: Golf tee time search
- **processor-sqs-event.json**: Notification message

### API Gateway Events

- **webapi-create-message.json**: Create message via API
- **webapi-create-schedule.json**: Create schedule via API
- **mcp-request.json**: MCP tool call request

### Creating Custom Test Events

```bash
# Generate SQS event
sam local generate-event sqs receive-message > events/custom-sqs.json

# Generate API Gateway event
sam local generate-event apigateway aws-proxy > events/custom-api.json

# Generate EventBridge event
sam local generate-event eventbridge schedule > events/custom-schedule.json
```

## Advanced Testing

### Load Testing

```bash
# Install Apache Bench
sudo apt-get install apache2-utils

# Test API endpoint
ab -n 1000 -c 10 \
    -p events/webapi-create-message.json \
    -T application/json \
    http://localhost:3000/api/messages
```

### Integration Testing

```bash
# Start all services
docker-compose up -d  # If you have docker-compose.yml

# Or manually:
# 1. Start DynamoDB Local
docker run -d -p 8000:8000 --name dynamodb-local amazon/dynamodb-local

# 2. Start SAM API
sam local start-api --docker-network rez-agent-local &

# 3. Run integration tests
go test ./tests/integration -v

# 4. Cleanup
docker stop dynamodb-local
docker rm dynamodb-local
```

### Continuous Testing

```bash
# Watch mode - rebuild and test on file changes
sam sync --watch --stack-name rez-agent-local
```

## Cleanup

### Stop Local Services

```bash
# Stop SAM API (Ctrl+C)

# Stop DynamoDB Local
docker stop dynamodb-local
docker rm dynamodb-local

# Remove Docker network
docker network rm rez-agent-local
```

### Delete Local Stack

```bash
# Delete CloudFormation stack
sam delete

# Or
aws cloudformation delete-stack --stack-name rez-agent-local
```

## Best Practices

1. **Always rebuild before testing**: Run `make build && sam build`
2. **Use Docker networks**: Connect all local services
3. **Test incrementally**: Test one function at a time
4. **Check logs**: Monitor SAM output for errors
5. **Use environment variables**: Override config for local testing
6. **Mock external services**: Use local alternatives where possible
7. **Clean up resources**: Stop containers when done

## Additional Resources

- [AWS SAM Documentation](https://docs.aws.amazon.com/serverless-application-model/)
- [SAM CLI Command Reference](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-sam-cli-command-reference.html)
- [DynamoDB Local Guide](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html)
- [Lambda Local Testing](https://docs.aws.amazon.com/lambda/latest/dg/testing-functions.html)
