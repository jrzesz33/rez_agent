# SAM Local Testing - Quick Start Guide

## One-Command Setup

```bash
# Setup complete local testing environment
make local-setup
```

This command:
- Creates Docker network (`rez-agent-local`)
- Starts DynamoDB Local on port 8000
- Creates all required tables

## Quick Commands

### Start API Gateway

```bash
# This automatically creates the Docker network and builds the functions
make sam-start-api
```

Access API at: `http://localhost:3000`

**Note**: The first time you run this, it will:
1. Create the Docker network `rez-agent-local`
2. Build all Lambda functions
3. Start the API Gateway

### Test Individual Functions

```bash
# Weather action
make sam-invoke-webaction

# Golf action
make sam-invoke-webaction-golf

# Notification processor
make sam-invoke-processor

# Scheduler
make sam-invoke-scheduler

# Web API
make sam-invoke-webapi

# MCP server
make sam-invoke-mcp
```

### Test API Endpoints

```bash
# Create a message
curl -X POST http://localhost:3000/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "notify",
    "stage": "dev",
    "payload": {
      "message": "Test from SAM",
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

## DynamoDB Local Commands

```bash
# List tables
make dynamodb-local-list-tables

# Scan messages table
aws dynamodb scan \
  --table-name rez-agent-messages-local \
  --endpoint-url http://localhost:8000

# Stop DynamoDB
make dynamodb-local-stop

# Restart DynamoDB
make dynamodb-local-start
```

## Cleanup

```bash
# Teardown everything
make local-teardown
```

## Troubleshooting

### Docker network not found

**Error**: `network rez-agent-local not found`

**Solution**:
```bash
# Create the network
make docker-network-create

# Or the Make commands will create it automatically
make sam-start-api
```

### Port 8000 already in use

```bash
# Find and kill process
lsof -ti:8000 | xargs kill -9

# Or use different port
docker run -d -p 8001:8000 --name dynamodb-local amazon/dynamodb-local
```

### Docker permission denied

```bash
# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

### Function not found

```bash
# Rebuild
make build
make sam-build
```

## Full Documentation

See [docs/LOCAL_TESTING.md](docs/LOCAL_TESTING.md) for complete documentation.

## Common Workflows

### Development Workflow

```bash
# 1. Setup environment (first time only)
make local-setup

# 2. Start API Gateway
make sam-start-api

# 3. In another terminal, make code changes
# ...

# 4. Rebuild and test
make build
make sam-invoke-webaction

# 5. When done, cleanup
make local-teardown
```

### Quick Function Test

```bash
# Build and test in one go
make build && make sam-invoke-webaction
```

### API Integration Test

```bash
# Start API
make sam-start-api

# In another terminal, run tests
curl http://localhost:3000/api/messages -X POST -H "Content-Type: application/json" -d @events/webapi-create-message.json
```

## Available Make Targets

Run `make help` to see all available commands:

```
SAM Local Testing:
  sam-validate              Validate SAM template
  sam-build                Build SAM application
  sam-start-api            Start local API Gateway
  sam-invoke-scheduler     Invoke scheduler function
  sam-invoke-webaction     Invoke webaction function
  sam-invoke-processor     Invoke processor function
  sam-invoke-webapi        Invoke webapi function
  sam-invoke-mcp           Invoke MCP function

DynamoDB Local:
  dynamodb-local-start     Start DynamoDB Local
  dynamodb-local-stop      Stop DynamoDB Local
  dynamodb-local-create-tables  Create tables
  dynamodb-local-list-tables    List tables

Environment:
  local-setup              Setup complete environment
  local-teardown           Cleanup everything
```
