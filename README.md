# Rez Agent

An event-driven system built in Go that processes messages and manages tasks through a scalable messaging infrastructure.

## Overview

Rez Agent is an event-driven application that processes messages and either completes tasks or produces additional messages for further processing. The system is designed to be scalable, maintainable, and cloud-native, leveraging AWS services for infrastructure and Go for robust service implementation.

## Features

### Core Functionality
- **Event-Driven Architecture**: Process messages asynchronously through AWS SQS/SNS
- **Scheduled Tasks**: EventBridge Scheduler for recurring jobs (initially 24-hour intervals)
- **Message Metadata Tracking**:
  - Created date: Timestamp when message was created
  - Created by: System that created the message
  - Stage: Target environment (dev, stage, prod)
- **Push Notifications**: Integration with ntfy.sh for system alerts

### Web Interface
A Lambda-based frontend application providing:
- Message metrics and monitoring dashboard
- Manual message creation interface
- OAuth2 authentication for secure access

## Architecture

### System Components

```
┌─────────────────┐
│  EventBridge    │──┐
│   Scheduler     │  │
└─────────────────┘  │
                     ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   SNS Topic     │────▶│   SQS Queue     │────▶│ Lambda Services │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                          │
                                                          ▼
                                                 ┌─────────────────┐
                                                 │   ntfy.sh API   │
                                                 └─────────────────┘

┌─────────────────┐     ┌─────────────────┐
│ Load Balancer   │────▶│  Lambda (Web)   │
└─────────────────┘     └─────────────────┘
```

### Infrastructure Stack
- **Messaging**: Amazon SQS (queue) and SNS (pub/sub)
- **Scheduling**: Amazon EventBridge Scheduler
- **Compute**: AWS Lambda functions
- **Frontend Hosting**: AWS Lambda + Application Load Balancer
- **IaC**: Pulumi for infrastructure automation

### External Integrations
- **ntfy.sh**: Push notifications endpoint at `https://ntfy.sh/rzesz-alerts`

## Initial Implementation

### Phase 1: Hello World Service
1. EventBridge Scheduler configured for 24-hour intervals
2. Lambda function to send "hello world" message to ntfy.sh
3. Message metadata tracking (created date, created by, stage)

### Phase 2: Web Interface
1. Lambda-based web application
2. OAuth2 authentication
3. Dashboard for message metrics
4. Manual message creation form

## Technology Stack

- **Language**: Go 1.24 (primary language for all services)
- **Infrastructure as Code**: Pulumi
- **Cloud Provider**: AWS
- **CI/CD**: GitHub Actions + Pulumi
- **Notifications**: ntfy.sh
- **Authentication**: OAuth2

## Development Setup

### Prerequisites
- Go 1.24+
- AWS CLI configured with appropriate credentials
- Pulumi CLI
- Docker (for local testing)

### Dev Container
This project includes a devcontainer configuration with:
- Go 1.24 on Debian Bookworm
- AWS CLI
- Docker-in-Docker with Buildx
- GitHub CLI (gh)

### Getting Started

```bash
# Clone the repository
git clone <repository-url>
cd rez_agent

# Install dependencies
go mod download

# Run tests
go test ./...

# Build the project
go build ./...

# Format code
go fmt ./...

# Tidy dependencies
go mod tidy
```

## Project Structure

```
rez_agent/
├── cmd/                    # Application entrypoints
│   ├── scheduler/         # EventBridge scheduler handler
│   └── web/              # Web application
├── internal/              # Private application code
│   ├── messaging/        # SQS/SNS messaging logic
│   ├── notification/     # ntfy.sh integration
│   └── models/          # Message metadata models
├── pkg/                   # Public libraries
├── infrastructure/        # Pulumi infrastructure code
├── docs/                  # Documentation
│   └── design/           # Design documents
├── .devcontainer/        # Dev container configuration
├── .github/              # GitHub Actions workflows
└── README.md
```

## Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...

# Run specific test
go test -run <TestName> ./path/to/package
```

## Deployment

### Infrastructure Deployment
The project uses Pulumi for infrastructure automation. Deployment is handled through GitHub Actions.

```bash
# Preview infrastructure changes
pulumi preview

# Deploy infrastructure
pulumi up

# Destroy infrastructure
pulumi destroy
```

### CI/CD Pipeline
GitHub Actions workflows handle:
1. Automated testing on pull requests
2. Code quality checks
3. Infrastructure deployment via Pulumi
4. Lambda function deployment

## Message Metadata Schema

All messages in the system include the following metadata:

```go
type MessageMetadata struct {
    CreatedDate time.Time `json:"created_date"` // When message was created
    CreatedBy   string    `json:"created_by"`   // System that created the message
    Stage       string    `json:"stage"`        // Target environment (dev/stage/prod)
}
```

## Roadmap

- [x] Project setup and initial architecture
- [ ] EventBridge Scheduler implementation
- [ ] SQS/SNS messaging layer
- [ ] Lambda service for ntfy.sh integration
- [ ] Message metadata tracking
- [ ] Web frontend with OAuth2
- [ ] Metrics dashboard
- [ ] Manual message creation UI
- [ ] Complete CI/CD pipeline
- [ ] Production deployment

## Contributing

1. Create a feature branch from `main`
2. Make your changes with appropriate tests
3. Ensure all tests pass: `go test ./...`
4. Format code: `go fmt ./...`
5. Submit a pull request

## License

[License information to be added]

## Support

For issues and questions, please open an issue in the GitHub repository.
