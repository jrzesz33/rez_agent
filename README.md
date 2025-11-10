# rez_agent

[![Go Version](https://img.shields.io/badge/Go-1.24-blue.svg)](https://golang.org)
[![Python Version](https://img.shields.io/badge/Python-3.12-blue.svg)](https://python.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Infrastructure](https://img.shields.io/badge/IaC-Pulumi-purple.svg)](https://pulumi.com)

**rez_agent** is an event-driven, serverless automation platform built on AWS that orchestrates scheduled tasks, web actions, and intelligent AI agent responses through a scalable messaging infrastructure.

## Overview

rez_agent is designed to automate and orchestrate various tasks including:

- **Scheduled Operations**: Recurring tasks via EventBridge Scheduler
- **Web Action Execution**: OAuth-authenticated REST API calls (weather forecasts, golf tee time reservations)
- **AI Agent Integration**: Claude-powered MCP (Model Context Protocol) server for intelligent task automation
- **Push Notifications**: Real-time alerts via ntfy.sh
- **Dynamic Scheduling**: Runtime schedule creation and management
- **Web Management Interface**: HTTP API for message and schedule management

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  EventBridge    │────▶│   SNS Topics     │────▶│   SQS Queues    │
│   Scheduler     │     │  - Web Actions   │     │  - Processors   │
└─────────────────┘     │  - Notifications │     └─────────────────┘
                        │  - Agent Response│             │
┌─────────────────┐     │  - Schedules     │             ▼
│  API Gateway    │────▶└──────────────────┘     ┌─────────────────┐
│   (HTTP API)    │                              │ Lambda Functions│
└─────────────────┘                              │  - Processor    │
                                                 │  - Scheduler    │
┌─────────────────┐                              │  - WebAction    │
│  MCP Client     │─────────────────────────────▶│  - WebAPI       │
│   (Claude AI)   │        Lambda URL            │  - AI Agent     │
└─────────────────┘                              │  - MCP Server   │
                                                 └─────────────────┘
                                                         │
                                                         ▼
                                                 ┌─────────────────┐
                                                 │   DynamoDB      │
                                                 │  - Messages     │
                                                 │  - Schedules    │
                                                 │  - Web Actions  │
                                                 └─────────────────┘
```

## Features

### Core Capabilities

- **Event-Driven Architecture**: Decoupled components communicating via SNS/SQS
- **Multi-Stage Deployments**: Support for dev, stage, and prod environments
- **Secure Authentication**: OAuth 2.0, JWT verification, AWS Secrets Manager integration
- **SSRF Protection**: Comprehensive URL validation and hostname allowlisting
- **Structured Logging**: JSON-formatted logs with configurable log levels
- **Error Handling**: Automatic retries with exponential backoff via SQS
- **Observability**: Optional AWS X-Ray tracing support

### Supported Web Actions

#### Weather Integration
- Fetch weather forecasts from NOAA Weather API
- Location-based grid point queries
- Automated weather alerts

#### Golf Course Reservations
- Multi-course support (configured via YAML)
- OAuth 2.0 authentication with JWT verification
- Tee time search and booking
- Reservation management
- Price calculation and confirmation

### AI Agent (MCP Server)

The MCP (Model Context Protocol) server enables Claude AI to:
- Search and book golf tee times
- Fetch weather forecasts
- Send notifications
- Create dynamic schedules

## Technology Stack

### Backend Services
- **Language**: Go 1.24
- **Runtime**: AWS Lambda
- **Messaging**: AWS SNS/SQS
- **Database**: Amazon DynamoDB
- **Secrets**: AWS Secrets Manager
- **Scheduling**: Amazon EventBridge Scheduler

### AI Agent
- **Language**: Python 3.12
- **Framework**: Anthropic MCP SDK
- **AI Model**: Claude (via Anthropic API)

### Infrastructure
- **IaC**: Pulumi (Go)
- **Deployment**: GitHub Actions
- **Container**: Development via devcontainers

## Quick Start

### Prerequisites

- **Go** 1.24 or later
- **Python** 3.12 or later (for AI agent)
- **AWS CLI** configured with appropriate credentials
- **Pulumi CLI** for infrastructure deployment
- **Docker** for local development and builds
- **Make** for build automation

### Installation

1. **Clone the repository**:
   ```bash
   git clone https://github.com/jrzesz33/rez_agent.git
   cd rez_agent
   ```

2. **Install dependencies**:
   ```bash
   make dev-env
   ```

3. **Configure Pulumi**:
   ```bash
   make infra-init
   cd infrastructure
   pulumi stack init dev
   pulumi config set ntfyUrl https://ntfy.sh/your-topic-name
   ```

4. **Build Lambda functions**:
   ```bash
   make build
   ```

5. **Deploy infrastructure**:
   ```bash
   make deploy-dev
   ```

## Project Structure

```
rez_agent/
├── cmd/                          # Application entrypoints
│   ├── agent/                   # AI agent Lambda (Python)
│   ├── mcp/                     # MCP server Lambda (Go)
│   ├── processor/               # Message processor Lambda
│   ├── scheduler/               # Scheduler trigger Lambda
│   ├── webaction/              # Web action executor Lambda
│   └── webapi/                 # HTTP API Lambda
├── internal/                    # Private application code
│   ├── httpclient/             # HTTP client with OAuth support
│   ├── logging/                # Structured logging utilities
│   ├── mcp/                    # MCP protocol implementation
│   │   ├── protocol/          # MCP protocol types
│   │   ├── server/            # MCP server implementation
│   │   └── tools/             # MCP tool definitions
│   ├── messaging/              # SNS/SQS messaging logic
│   ├── models/                 # Domain models and types
│   ├── notification/           # ntfy.sh integration
│   ├── repository/             # DynamoDB repositories
│   ├── scheduler/              # EventBridge Scheduler client
│   ├── secrets/                # AWS Secrets Manager client
│   └── webaction/              # Web action handlers
├── pkg/                         # Public libraries
│   ├── config/                 # Configuration management
│   └── courses/                # Golf course definitions
├── infrastructure/              # Pulumi infrastructure code
├── docs/                        # Documentation
│   ├── api/                    # API documentation
│   ├── architecture/           # Architecture diagrams
│   └── design/                 # Design documents
├── tools/                       # Development tools
│   └── mcp-client/            # MCP stdio client
├── Makefile                     # Build automation
└── go.mod                       # Go module definition
```

## Configuration

### Environment Variables

All Lambda functions are configured via environment variables set by Pulumi:

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `STAGE` | Deployment stage (dev/stage/prod) | Yes | dev |
| `AWS_REGION` | AWS region | Yes | us-east-1 |
| `DYNAMODB_TABLE_NAME` | Messages table name | Yes | - |
| `SCHEDULES_TABLE_NAME` | Schedules table name | Yes | - |
| `WEB_ACTION_RESULTS_TABLE_NAME` | Web action results table | Yes | - |
| `WEB_ACTIONS_TOPIC_ARN` | SNS topic for web actions | Yes | - |
| `NOTIFICATIONS_TOPIC_ARN` | SNS topic for notifications | Yes | - |
| `AGENT_RESPONSE_TOPIC_ARN` | SNS topic for agent responses | Yes | - |
| `SCHEDULE_CREATION_TOPIC_ARN` | SNS topic for schedule creation | Yes | - |
| `NTFY_URL` | ntfy.sh topic URL | Yes | - |
| `GOLF_SECRET_NAME` | Secrets Manager secret name | Yes | - |
| `LOG_LEVEL` | Logging level (DEBUG/INFO/WARN/ERROR) | No | INFO |

### Pulumi Configuration

Required Pulumi config values:

```bash
pulumi config set ntfyUrl https://ntfy.sh/your-topic
pulumi config set stage dev
pulumi config set logRetentionDays 7
pulumi config set enableXRay false
pulumi config set schedulerCron "cron(0 12 * * ? *)"
```

### Golf Course Configuration

Golf courses are configured in `pkg/courses/courseInfo.yaml`:

```yaml
courses:
  - courseId: 1
    name: "Birdsfoot Golf Course"
    origin: "https://birdsfoot.cps.golf"
    client-id: "onlineresweb"
    websiteid: "94fa26b7-2e63-4cbc-99e5-08d7d7f41522"
    scope: "openid profile email"
    actions:
      - request:
          name: search-tee-times
          url: "/onlineres/onlineapi/api/v1/onlinereservation/TeeTimes"
      # ... additional actions
```

## Development

### Building

```bash
# Build all Lambda functions
make build

# Build specific function
make build-scheduler
make build-processor
make build-webaction
make build-webapi
make build-agent
make build-mcp

# Quick build (no clean)
make quick-build
```

### Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific package tests
go test ./internal/webaction -v
go test ./internal/mcp/server -v
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run all validation
make validate

# Tidy modules
make tidy
```

### Local Development

```bash
# Start local DynamoDB
make local-dynamodb

# Stop local DynamoDB
make local-dynamodb-stop

# Watch for changes and rebuild
make watch
```

## Deployment

### Development Environment

```bash
make deploy-dev
```

### Production Environment

```bash
make deploy-prod
```

### Manual Deployment

```bash
# Select stack
cd infrastructure
pulumi stack select dev

# Preview changes
pulumi preview

# Deploy
pulumi up

# View outputs
pulumi stack output
```

## Monitoring

### CloudWatch Logs

```bash
# Tail Lambda logs
make lambda-logs-scheduler
make lambda-logs-processor
make lambda-logs-webaction
make lambda-logs-webapi

# Or use AWS CLI directly
aws logs tail /aws/lambda/rez-agent-scheduler-dev --follow
```

### Metrics

All Lambda functions publish custom CloudWatch metrics:
- Message processing duration
- Success/failure rates
- Web action response times
- OAuth authentication latency

### X-Ray Tracing

Enable X-Ray tracing in Pulumi config:

```bash
pulumi config set enableXRay true
```

## API Documentation

### Web API Endpoints

The Web API Lambda exposes an HTTP API via API Gateway:

#### Create Message
```http
POST /api/messages
Content-Type: application/json

{
  "message_type": "web_action",
  "stage": "dev",
  "payload": {
    "version": "1.0",
    "action": "weather",
    "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast"
  }
}
```

#### Create Schedule
```http
POST /api/schedules
Content-Type: application/json

{
  "action": "create",
  "name": "daily-weather",
  "schedule_expression": "cron(0 12 * * ? *)",
  "timezone": "America/New_York",
  "target_type": "web_action",
  "message_type": "web_action",
  "payload": { ... }
}
```

See [API Documentation](docs/api/README.md) for complete endpoint reference.

## Message Schemas

### Message Types

- `hello_world`: Simple test message
- `notify`: Notification message
- `agent_response`: AI agent response
- `scheduled`: Scheduled task message
- `web_action`: Web action request
- `schedule_creation`: Dynamic schedule creation

See [Message Schemas](docs/MESSAGE_SCHEMAS.md) for detailed schemas.

## Security

### Authentication
- OAuth 2.0 password grant flow for external APIs
- JWT verification with JWKS
- AWS IAM roles for Lambda execution
- Secrets Manager for credential storage

### SSRF Prevention
- URL allowlist enforcement
- Hostname validation
- Private IP range blocking
- AWS metadata service protection

### Data Protection
- Encrypted DynamoDB tables
- Encrypted SNS topics and SQS queues
- Sensitive data redaction in logs
- Short-lived credentials via STS

## MCP Server Integration

The MCP server enables Claude AI integration:

### Available Tools

- `golf_search_tee_times`: Search for available golf tee times
- `golf_book_tee_time`: Book a golf tee time
- `golf_fetch_reservations`: Get upcoming reservations
- `get_weather_forecast`: Fetch weather forecast
- `send_notification`: Send push notification

### Usage with Claude Desktop

Add to Claude Desktop config (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "rez-agent": {
      "command": "/path/to/rez-agent-mcp-client",
      "args": ["--lambda-url", "https://your-lambda-url.lambda-url.us-east-1.on.aws/"]
    }
  }
}
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests and validation (`make validate`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

### Development Guidelines

- All new code must include tests
- Maintain test coverage above 70%
- Follow Go standard project layout
- Use structured logging (JSON format)
- Document all public APIs
- Run `make validate` before committing

## Troubleshooting

### Common Issues

**Build Errors**
```bash
# Clean and rebuild
make clean
make build
```

**Deployment Failures**
```bash
# Check Pulumi logs
cd infrastructure
pulumi logs

# Verify AWS credentials
aws sts get-caller-identity
```

**Lambda Errors**
```bash
# Check CloudWatch logs
make lambda-logs-webaction

# Test locally with SAM
sam local invoke WebActionFunction --event event.json
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- [Anthropic](https://anthropic.com) for Claude AI and MCP protocol
- [NOAA](https://weather.gov) for weather data API
- [ntfy.sh](https://ntfy.sh) for push notifications
- AWS for serverless infrastructure
- Pulumi for infrastructure as code

## Support

For issues, questions, or contributions:
- Open an issue on GitHub
- Review the [documentation](docs/)
- Check the [design documents](docs/design/)

---

**Built with** ☕ **by** [@jrzesz33](https://github.com/jrzesz33)
