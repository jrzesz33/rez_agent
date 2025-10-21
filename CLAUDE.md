# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**rez_agent** is an event-driven system that processes messages and manages tasks through a scalable AWS-based messaging infrastructure. The application is designed to:

- Process messages asynchronously through SQS/SNS
- Execute scheduled tasks via EventBridge Scheduler
- Send push notifications to ntfy.sh
- Provide a web interface for message management and metrics
- Track message metadata (created date, created by, stage)

### Key Components
- **Backend Services**: Lambda functions written in Go for message processing
- **Frontend**: Lambda-based web application with OAuth2 authentication
- **Messaging Layer**: AWS SQS queues and SNS topics
- **Scheduling**: EventBridge Scheduler for recurring jobs
- **Infrastructure**: Managed via Pulumi (IaC)
- **Deployment**: GitHub Actions + Pulumi

## Development Environment

- **Language**: Go 1.24
- **Container**: Development is done using devcontainers with Go 1.24 on Debian Bookworm
- **Tools Available**:
  - AWS CLI (latest)
  - Docker-in-Docker with Buildx and Compose v2
  - GitHub CLI (gh)

## Common Commands

### Go Development
```bash
# Build the project
go build ./...

# Run tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run a specific test
go test -run <TestName> ./path/to/package

# Run tests with coverage
go test -cover ./...

# Format code
go fmt ./...

# Lint (requires golangci-lint installation)
golangci-lint run

# Tidy dependencies
go mod tidy

# Download dependencies
go mod download
```

## Architecture

### System Design

The system follows an event-driven architecture with the following flow:

```
EventBridge Scheduler → SNS Topic → SQS Queue → Lambda Services → ntfy.sh

Load Balancer → Lambda (Web Frontend)
```

### AWS Services
- **EventBridge Scheduler**: Triggers scheduled tasks (initially 24-hour intervals)
- **SNS (Simple Notification Service)**: Pub/sub messaging for event distribution
- **SQS (Simple Queue Service)**: Message queuing for reliable processing
- **Lambda**: Serverless compute for both backend services and web frontend
- **Application Load Balancer**: Routes traffic to web frontend Lambda

### External Integrations
- **ntfy.sh**: Push notification endpoint at `https://ntfy.sh/rzesz-alerts`

### Project Structure
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
└── .github/              # GitHub Actions workflows
```

### Message Metadata Schema
All messages must include:
- **created_date**: Timestamp when message was created
- **created_by**: System identifier that created the message
- **stage**: Environment designation (dev, stage, prod)

## Development Guidelines

### Code Organization
- Use Go standard project layout
- Keep business logic in `internal/` packages
- Shared libraries go in `pkg/` if they're meant to be imported externally
- Lambda handlers go in `cmd/` directories

### Testing Requirements
- Unit tests are mandatory for all new code
- Test files should be co-located with the code they test
- Aim for meaningful test coverage (focus on critical paths)
- Use table-driven tests where appropriate

### Pulumi Infrastructure
- All AWS resources must be defined in Pulumi
- Keep infrastructure code in `infrastructure/` directory
- Use Pulumi stacks for environment separation (dev, stage, prod)

### Deployment Process
- GitHub Actions handles CI/CD
- All PRs must pass tests before merging
- Pulumi deployments are triggered on merge to main
- Use feature flags for gradual rollouts when appropriate
