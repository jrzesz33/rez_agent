# Logging Configuration

## Overview

All Lambda functions in the rez_agent project now support configurable log levels via the `LOG_LEVEL` environment variable. If the environment variable is not set, the default log level is **Info**.

## Environment Variable

**Variable Name:** `LOG_LEVEL`

**Supported Values (case-insensitive):**
- `DEBUG` - Most verbose logging, includes debug messages
- `INFO` - Standard informational messages (default)
- `WARN` or `WARNING` - Warning messages only
- `ERROR` - Error messages only

## Lambda Functions Updated

All Lambda functions now use the centralized logging configuration:

1. **Processor Lambda** (`cmd/processor/main.go`) - SQS message processor
2. **Scheduler Lambda** (`cmd/scheduler/main.go`) - EventBridge scheduler
3. **MCP Lambda** (`cmd/mcp/main.go`) - MCP server
4. **Web Action Lambda** (`cmd/webaction/main.go`) - Web action processor
5. **Web API Lambda** (`cmd/webapi/main.go`) - Web API handler

## Implementation Details

### Logging Package

A new internal package was created at `internal/logging/config.go` that provides:

- `GetLogLevel()` function that reads the `LOG_LEVEL` environment variable
- Defaults to `slog.LevelInfo` if not set or invalid
- Handles case-insensitive input
- Trims whitespace from the environment variable value

### Usage in Lambda Functions

Each Lambda function initializes its logger using:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: logging.GetLogLevel(),
}))
slog.SetDefault(logger)
```

## Setting Log Level in AWS Lambda

### Via Pulumi Infrastructure

Add the `LOG_LEVEL` environment variable to your Lambda function configuration:

```go
Environment: &lambda.FunctionEnvironmentArgs{
    Variables: pulumi.StringMap{
        "LOG_LEVEL": pulumi.String("DEBUG"),
        // ... other environment variables
    },
},
```

### Via AWS Console

1. Navigate to the Lambda function in the AWS Console
2. Go to **Configuration** → **Environment variables**
3. Click **Edit**
4. Add a new environment variable:
   - **Key**: `LOG_LEVEL`
   - **Value**: `DEBUG`, `INFO`, `WARN`, or `ERROR`
5. Click **Save**

### Via AWS CLI

```bash
aws lambda update-function-configuration \
    --function-name <function-name> \
    --environment "Variables={LOG_LEVEL=DEBUG,OTHER_VAR=value}"
```

## Examples

### Debug Mode for Development
```bash
LOG_LEVEL=DEBUG
```

### Production (Info Level)
```bash
LOG_LEVEL=INFO
```
or simply don't set the variable (it defaults to Info).

### Error-Only Logging
```bash
LOG_LEVEL=ERROR
```

## Testing

The logging configuration includes comprehensive tests in `internal/logging/config_test.go` that verify:

- All supported log level values (DEBUG, INFO, WARN, WARNING, ERROR)
- Case-insensitive handling
- Whitespace trimming
- Default behavior when not set
- Invalid value handling

Run tests with:
```bash
go test ./internal/logging/...
```
