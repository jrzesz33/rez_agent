# Logging Configuration

This document describes the logging setup and configuration for the rez_agent system.

## Overview

All Lambda functions in rez_agent use structured JSON logging via Go's `log/slog` package. Logs are automatically sent to Amazon CloudWatch Logs and can be queried using CloudWatch Logs Insights.

## Log Levels

The system supports the following log levels (in order of increasing severity):

- **DEBUG**: Detailed diagnostic information useful for troubleshooting
- **INFO**: General informational messages about normal operations (default)
- **WARN**: Warning messages about potentially problematic situations
- **ERROR**: Error messages about failures that don't stop execution

## Configuration

### Environment Variable

Set the `LOG_LEVEL` environment variable to control logging verbosity:

```bash
LOG_LEVEL=DEBUG  # Show all logs including debug
LOG_LEVEL=INFO   # Show info, warn, and error (default)
LOG_LEVEL=WARN   # Show only warnings and errors
LOG_LEVEL=ERROR  # Show only errors
```

### Implementation

Logging is configured in each Lambda function's `main.go` using the `internal/logging` package:

```go
import (
    "log/slog"
    "os"
    "github.com/jrzesz33/rez_agent/internal/logging"
)

func main() {
    // Setup structured logging
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: logging.GetLogLevel(),
    }))
    slog.SetDefault(logger)

    // Use logger
    logger.Info("Lambda starting", slog.String("stage", stage))
}
```

### Helper Function

The `logging.GetLogLevel()` function reads the `LOG_LEVEL` environment variable and returns the appropriate `slog.Level`:

```go
package logging

import (
    "log/slog"
    "os"
    "strings"
)

// GetLogLevel returns the log level from environment variable LOG_LEVEL
// Defaults to INFO if not set or invalid
func GetLogLevel() slog.Level {
    level := strings.ToUpper(os.Getenv("LOG_LEVEL"))
    switch level {
    case "DEBUG":
        return slog.LevelDebug
    case "INFO":
        return slog.LevelInfo
    case "WARN", "WARNING":
        return slog.LevelWarn
    case "ERROR":
        return slog.LevelError
    default:
        return slog.LevelInfo
    }
}
```

## Log Format

All logs are output in JSON format with structured fields:

```json
{
  "time": "2024-01-15T10:30:45.123Z",
  "level": "INFO",
  "msg": "processing message",
  "message_id": "msg_20240115103045_123456",
  "message_type": "web_action",
  "stage": "prod"
}
```

### Common Fields

- `time`: ISO 8601 timestamp
- `level`: Log level (DEBUG, INFO, WARN, ERROR)
- `msg`: Human-readable message
- Additional context fields (message_id, user_id, etc.)

## Context-Aware Logging

Use context-aware logging methods to automatically include request context:

```go
// InfoContext includes context fields automatically
logger.InfoContext(ctx, "processing message",
    slog.String("message_id", msgID),
    slog.String("operation", "booking"),
)

// ErrorContext for errors
logger.ErrorContext(ctx, "failed to process message",
    slog.String("message_id", msgID),
    slog.String("error", err.Error()),
)
```

## CloudWatch Logs

### Log Groups

Each Lambda function has its own CloudWatch log group:

- `/aws/lambda/rez-agent-webapi-{stage}`
- `/aws/lambda/rez-agent-processor-{stage}`
- `/aws/lambda/rez-agent-scheduler-{stage}`
- `/aws/lambda/rez-agent-webaction-{stage}`
- `/aws/lambda/rez-agent-mcp-{stage}`
- `/aws/lambda/rez-agent-agent-{stage}` (Python)

### Log Retention

- **Development**: 7 days
- **Staging**: 14 days
- **Production**: 30 days

Configured in Pulumi infrastructure:

```go
LogGroupRetentionDays: pulumi.Int(7),  // Dev
LogGroupRetentionDays: pulumi.Int(30), // Prod
```

## CloudWatch Logs Insights Queries

### View Recent Errors

```cloudwatch
fields @timestamp, level, msg, error, message_id
| filter level = "ERROR"
| sort @timestamp desc
| limit 100
```

### Track Message Processing

```cloudwatch
fields @timestamp, msg, message_id, message_type, status
| filter message_id = "msg_20240115103045_123456"
| sort @timestamp asc
```

### Monitor Performance

```cloudwatch
fields @timestamp, msg, @duration, message_type
| filter msg like /completed/
| stats avg(@duration), max(@duration), min(@duration) by message_type
```

### Golf Booking Operations

```cloudwatch
fields @timestamp, msg, course_name, operation, tee_sheet_id
| filter operation = "book_tee_time"
| sort @timestamp desc
```

### MCP Tool Calls

```cloudwatch
fields @timestamp, msg, tool_name, execution_time
| filter msg like /MCP tool/
| stats count(), avg(execution_time) by tool_name
```

## Best Practices

### 1. Use Structured Logging

Always use structured fields instead of string interpolation:

```go
// Good
logger.Info("user login",
    slog.String("user_id", userID),
    slog.String("ip", ipAddr),
)

// Bad
logger.Info(fmt.Sprintf("user %s logged in from %s", userID, ipAddr))
```

### 2. Include Context

Always include relevant identifiers:

```go
logger.InfoContext(ctx, "processing payment",
    slog.String("user_id", userID),
    slog.String("order_id", orderID),
    slog.Float64("amount", amount),
)
```

### 3. Log Errors with Stack Traces

When logging errors, include the full error message:

```go
if err != nil {
    logger.ErrorContext(ctx, "failed to save record",
        slog.String("record_id", id),
        slog.String("error", err.Error()),
    )
    return err
}
```

### 4. Use Appropriate Log Levels

- **DEBUG**: Internal state, variables, detailed flow
- **INFO**: Important business events (message received, booking created)
- **WARN**: Recoverable issues (retry, fallback)
- **ERROR**: Failures requiring attention

### 5. Avoid Logging Sensitive Data

Never log:
- Passwords or API keys
- Credit card numbers
- Personal identifiable information (PII) unless required
- Full authentication tokens

## Agent Event Handler Logging

The scheduled agent event handler (`internal/scheduler/agentevent.go`) includes comprehensive logging:

```go
// Execution start
logger.InfoContext(ctx, "starting scheduled agent event execution",
    slog.String("schedule_id", event.ScheduleID),
    slog.String("user_prompt", event.UserPrompt),
)

// Retry attempts
logger.WarnContext(ctx, "agent execution attempt failed",
    slog.Int("attempt", attempt),
    slog.String("error", err.Error()),
)

// MCP tool calls
logger.Info("fetching weather forecast",
    slog.String("location", location),
    slog.Int("days", numDays),
)

// Completion
logger.InfoContext(ctx, "agent event execution completed successfully")
```

## Monitoring and Alerts

### CloudWatch Alarms

Set up alarms for:

1. **Error Rate**: Alert when error count exceeds threshold
2. **Latency**: Alert on slow Lambda executions
3. **Dead Letter Queue**: Alert on messages in DLQ
4. **Throttling**: Alert on Lambda throttles

### Metrics Filters

Create metric filters for custom metrics:

```cloudwatch
[time, level=ERROR, msg, error]
```

### Dashboard

Create CloudWatch dashboard with:
- Error rates by Lambda function
- Invocation counts
- Duration percentiles (p50, p95, p99)
- Message processing throughput

## Troubleshooting

### Enable Debug Logging

For a specific Lambda:

```bash
aws lambda update-function-configuration \
  --function-name rez-agent-webaction-dev \
  --environment "Variables={LOG_LEVEL=DEBUG,STAGE=dev,...}"
```

### View Logs in Real-Time

```bash
aws logs tail /aws/lambda/rez-agent-webaction-dev --follow
```

### Search for Specific Message

```bash
aws logs filter-log-events \
  --log-group-name /aws/lambda/rez-agent-processor-dev \
  --filter-pattern "msg_20240115103045_123456"
```

## Performance Considerations

- Structured logging has minimal performance impact
- CloudWatch Logs Insights queries are fast for recent logs
- Consider log sampling for very high-volume operations
- Use DEBUG level sparingly in production

## Future Enhancements

- [ ] Add distributed tracing with AWS X-Ray
- [ ] Implement log sampling for high-volume endpoints
- [ ] Add custom metrics for business KPIs
- [ ] Create automated log analysis with AWS Lambda
- [ ] Implement log forwarding to external systems (Datadog, Splunk)

## References

- [Go slog documentation](https://pkg.go.dev/log/slog)
- [CloudWatch Logs documentation](https://docs.aws.amazon.com/cloudwatch/latest/logs/)
- [CloudWatch Logs Insights query syntax](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/CWL_QuerySyntax.html)
- [AWS Lambda logging best practices](https://docs.aws.amazon.com/lambda/latest/dg/golang-logging.html)

---

**Last Updated**: 2024-01-15
**Version**: 1.0
**Maintained By**: [@jrzesz33](https://github.com/jrzesz33)
