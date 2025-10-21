# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based project named "rez_agent". The repository is currently in early development stages.

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

The project structure and architecture are still being established. This section will be updated as the codebase develops.
