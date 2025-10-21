# rez_agent Observability & Monitoring

## Overview

This document defines the observability and monitoring strategy for the rez_agent event-driven messaging system.

**Three Pillars of Observability**:
1. **Logging**: Structured logs with correlation IDs
2. **Metrics**: Application and infrastructure metrics
3. **Tracing**: Distributed tracing across services

**Goals**:
- Detect issues before users report them
- Debug production issues quickly
- Understand system behavior and performance
- Monitor SLIs/SLOs and alert on violations

---

## Logging Strategy

### Structured Logging with Go `log/slog`

**Library**: `log/slog` (Go 1.21+ standard library)

**Format**: JSON (for CloudWatch Logs Insights queries)

**Log Levels**:
- **DEBUG**: Detailed information for troubleshooting (disabled in prod)
- **INFO**: General informational messages (state changes, processing)
- **WARN**: Warning conditions (non-critical errors, fallbacks)
- **ERROR**: Error conditions requiring attention

---

### Log Configuration (Go)

**Initialize Logger** (Lambda init):
```go
package main

import (
	"context"
	"log/slog"
	"os"
)

func init() {
	// Configure JSON handler for CloudWatch
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: getLogLevel(),
		AddSource: true, // Include file:line in logs
	})

	logger := slog.New(jsonHandler)
	slog.SetDefault(logger)
}

func getLogLevel() slog.Level {
	stage := os.Getenv("STAGE")
	if stage == "dev" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
```

---

### Correlation ID Propagation

**Purpose**: Link logs across all services for a single message

**Implementation**:
1. Generate correlation_id when message created (Scheduler/Web API)
2. Include in DynamoDB message record
3. Include in SNS/SQS message
4. Extract in Message Processor and pass to Notification Service
5. Log correlation_id in every log statement

**Go Code** (Context-based propagation):
```go
package correlation

import (
	"context"
	"github.com/google/uuid"
)

type contextKey string

const correlationIDKey contextKey = "correlation_id"

// WithCorrelationID adds correlation ID to context
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey, correlationID)
}

// GetCorrelationID extracts correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if correlationID, ok := ctx.Value(correlationIDKey).(string); ok {
		return correlationID
	}
	return ""
}

// NewCorrelationID generates new correlation ID
func NewCorrelationID() string {
	return uuid.New().String()
}
```

**Logging with Correlation ID**:
```go
func (h *Handler) processMessage(ctx context.Context, messageID string) error {
	correlationID := correlation.GetCorrelationID(ctx)

	slog.InfoContext(ctx, "Processing message started",
		"message_id", messageID,
		"correlation_id", correlationID,
	)

	// ... processing logic

	slog.InfoContext(ctx, "Processing message completed",
		"message_id", messageID,
		"correlation_id", correlationID,
		"duration_ms", duration.Milliseconds(),
	)

	return nil
}
```

---

### Log Events (Key Lifecycle Events)

#### Scheduler Lambda
```go
// Message creation
slog.Info("Scheduled message created",
	"message_id", messageID,
	"correlation_id", correlationID,
	"message_type", "scheduled_hello",
	"stage", stage,
)

// SNS publish
slog.Info("Published message to SNS",
	"message_id", messageID,
	"correlation_id", correlationID,
	"topic_arn", topicARN,
)
```

#### Web API Lambda
```go
// API request received
slog.Info("API request received",
	"method", "POST",
	"path", "/api/messages",
	"correlation_id", correlationID,
	"user_id", userID,
)

// Message created
slog.Info("Manual message created",
	"message_id", messageID,
	"correlation_id", correlationID,
	"created_by", userEmail,
)

// API response
slog.Info("API response sent",
	"status_code", 202,
	"correlation_id", correlationID,
	"duration_ms", duration.Milliseconds(),
)
```

#### Message Processor Lambda
```go
// SQS message received
slog.Info("SQS message received",
	"message_id", messageID,
	"correlation_id", correlationID,
	"attempt", 1,
)

// Status update
slog.Info("Message status updated",
	"message_id", messageID,
	"correlation_id", correlationID,
	"old_status", "created",
	"new_status", "processing",
)

// Processing error
slog.Error("Message processing failed",
	"message_id", messageID,
	"correlation_id", correlationID,
	"error", err.Error(),
	"retry_count", retryCount,
)
```

#### Notification Service Lambda
```go
// Notification send attempt
slog.Info("Sending notification to ntfy.sh",
	"message_id", messageID,
	"correlation_id", correlationID,
	"attempt", attempt,
)

// Circuit breaker state change
slog.Warn("Circuit breaker opened",
	"service", "ntfy-sh",
	"failure_count", failureCount,
	"correlation_id", correlationID,
)

// Notification success
slog.Info("Notification sent successfully",
	"message_id", messageID,
	"correlation_id", correlationID,
	"notification_id", notificationID,
	"duration_ms", duration.Milliseconds(),
)
```

---

### CloudWatch Logs Configuration

**Log Groups**:
- `/aws/lambda/rez-agent-scheduler-{stage}`
- `/aws/lambda/rez-agent-web-api-{stage}`
- `/aws/lambda/rez-agent-message-processor-{stage}`
- `/aws/lambda/rez-agent-notification-service-{stage}`
- `/aws/lambda/rez-agent-jwt-authorizer-{stage}`

**Retention Period**:
- **dev**: 7 days (cost optimization)
- **stage**: 14 days
- **prod**: 30 days (compliance, debugging)

**Subscription Filters** (Optional):
- Stream ERROR logs to SNS → Slack/Email alert
- Stream all logs to S3 for long-term archival (Glacier)

---

### CloudWatch Logs Insights Queries

#### Query 1: Find all logs for a correlation ID
```sql
fields @timestamp, @message, level, message_id, correlation_id
| filter correlation_id = "660e9500-f39c-52e5-b827-557766551111"
| sort @timestamp asc
```

#### Query 2: Count errors by message_id (last 1 hour)
```sql
fields message_id, @message
| filter level = "ERROR"
| stats count() as error_count by message_id
| sort error_count desc
```

#### Query 3: Average API response time (last 24 hours)
```sql
fields duration_ms
| filter path = "/api/messages" and method = "POST"
| stats avg(duration_ms) as avg_duration_ms, max(duration_ms) as max_duration_ms
```

#### Query 4: Failed notifications (ntfy.sh errors)
```sql
fields @timestamp, message_id, error
| filter @message like "Notification failed"
| sort @timestamp desc
```

---

## Metrics Strategy

### CloudWatch Metrics

#### Lambda Metrics (Automatic)

AWS Lambda automatically publishes these metrics:
- **Invocations**: Number of function invocations
- **Errors**: Number of failed invocations (uncaught errors)
- **Duration**: Execution time (p50, p90, p99, p100)
- **Throttles**: Throttled invocations (concurrency limit)
- **ConcurrentExecutions**: Concurrent function executions
- **IteratorAge**: Age of last record processed (SQS/Kinesis)

**Dashboard**: CloudWatch default Lambda dashboard per function

---

#### Custom Application Metrics

**Go SDK for CloudWatch Metrics**:
```go
package metrics

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type Recorder struct {
	client    *cloudwatch.Client
	namespace string
	stage     string
}

func NewRecorder(client *cloudwatch.Client, namespace, stage string) *Recorder {
	return &Recorder{
		client:    client,
		namespace: namespace,
		stage:     stage,
	}
}

// RecordCount records a count metric
func (r *Recorder) RecordCount(ctx context.Context, metricName string, value float64, dimensions map[string]string) error {
	return r.putMetric(ctx, metricName, value, types.StandardUnitCount, dimensions)
}

// RecordDuration records a duration metric (milliseconds)
func (r *Recorder) RecordDuration(ctx context.Context, metricName string, duration time.Duration, dimensions map[string]string) error {
	return r.putMetric(ctx, metricName, float64(duration.Milliseconds()), types.StandardUnitMilliseconds, dimensions)
}

func (r *Recorder) putMetric(ctx context.Context, metricName string, value float64, unit types.StandardUnit, dimensions map[string]string) error {
	// Convert dimensions to CloudWatch format
	var cwDimensions []types.Dimension
	for k, v := range dimensions {
		cwDimensions = append(cwDimensions, types.Dimension{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	// Add stage dimension
	cwDimensions = append(cwDimensions, types.Dimension{
		Name:  aws.String("Stage"),
		Value: aws.String(r.stage),
	})

	_, err := r.client.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(r.namespace),
		MetricData: []types.MetricDatum{
			{
				MetricName: aws.String(metricName),
				Value:      aws.Float64(value),
				Unit:       unit,
				Timestamp:  aws.Time(time.Now()),
				Dimensions: cwDimensions,
			},
		},
	})

	return err
}
```

---

#### Custom Metrics to Track

**Namespace**: `RezAgent`

| Metric Name | Type | Dimensions | Description |
|-------------|------|------------|-------------|
| `MessagesCreated` | Count | Stage, MessageType, CreatedBy | Messages created (scheduled/manual) |
| `MessagesProcessed` | Count | Stage, Status | Messages processed (completed/failed) |
| `NotificationsSent` | Count | Stage, Status | Notifications sent (success/failure) |
| `NotificationDuration` | Duration (ms) | Stage | Time to send notification |
| `ProcessingDuration` | Duration (ms) | Stage, MessageType | Message processing time |
| `CircuitBreakerState` | Count | Stage, Service, State | Circuit breaker state (0=closed, 1=open, 0.5=half-open) |
| `DLQMessageCount` | Count | Stage | Messages in DLQ |
| `APIRequestCount` | Count | Stage, Endpoint, StatusCode | API requests by endpoint |
| `APIResponseTime` | Duration (ms) | Stage, Endpoint | API response time |

**Usage Example**:
```go
func (h *Handler) processMessage(ctx context.Context, message *Message) error {
	start := time.Now()

	// ... processing logic

	duration := time.Since(start)

	// Record metrics
	h.metrics.RecordCount(ctx, "MessagesProcessed", 1, map[string]string{
		"Status": "completed",
	})

	h.metrics.RecordDuration(ctx, "ProcessingDuration", duration, map[string]string{
		"MessageType": message.MessageType,
	})

	return nil
}
```

---

### SQS Metrics (Automatic)

**Queue Metrics** (published by SQS):
- **ApproximateNumberOfMessagesVisible**: Messages available to receive
- **ApproximateNumberOfMessagesNotVisible**: Messages in flight (processing)
- **ApproximateAgeOfOldestMessage**: Age of oldest message in queue
- **NumberOfMessagesSent**: Messages sent to queue
- **NumberOfMessagesReceived**: Messages received from queue
- **NumberOfMessagesDeleted**: Messages deleted (successfully processed)

**DLQ Metrics**:
- **ApproximateNumberOfMessagesVisible**: Messages in DLQ (alarm on > 0)

---

### DynamoDB Metrics (Automatic)

**Table Metrics**:
- **ConsumedReadCapacityUnits**: Read capacity consumed (on-demand)
- **ConsumedWriteCapacityUnits**: Write capacity consumed
- **UserErrors**: Client-side errors (validation, conditional check failures)
- **SystemErrors**: Server-side errors (throttling, internal errors)
- **ThrottledRequests**: Throttled requests (rare with on-demand)
- **SuccessfulRequestLatency**: Request latency (p50, p90, p99)

---

## Distributed Tracing (AWS X-Ray)

### Purpose

Visualize request flow across Lambda functions, DynamoDB, SNS, SQS, and external services (ntfy.sh).

**Use Cases**:
- Identify bottlenecks (slow DynamoDB queries, ntfy.sh latency)
- Understand end-to-end message processing time
- Debug failures in distributed system
- Analyze service dependencies

---

### X-Ray Configuration

**Enable X-Ray Tracing** (Lambda):
- **Active Tracing**: Enabled on all Lambda functions
- **IAM Permissions**: `xray:PutTraceSegments`, `xray:PutTelemetryRecords`

**X-Ray SDK** (Go):
```go
package main

import (
	"context"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/aws/aws-sdk-go-v2/config"
)

func main() {
	// Initialize AWS config with X-Ray instrumentation
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		panic(err)
	}

	// Instrument AWS SDK clients
	dynamodbClient := dynamodb.NewFromConfig(cfg)
	xray.AWS(dynamodbClient.Client) // Automatically traces DynamoDB calls

	snsClient := sns.NewFromConfig(cfg)
	xray.AWS(snsClient.Client) // Automatically traces SNS calls

	// ... rest of Lambda handler
}
```

**Instrument HTTP Client** (for ntfy.sh calls):
```go
import (
	"net/http"
	"github.com/aws/aws-xray-sdk-go/xray"
)

func NewHTTPClient() *http.Client {
	return xray.Client(&http.Client{
		Timeout: 10 * time.Second,
	})
}
```

---

### X-Ray Trace Example

**Trace for Scheduled Message**:
```
[EventBridge Scheduler] → [Scheduler Lambda]
    ├─ [DynamoDB PutItem] (messages table)
    └─ [SNS Publish] (rez-agent-messages topic)
         └─ [SQS SendMessage] (rez-agent-message-queue)
              └─ [Message Processor Lambda]
                   ├─ [DynamoDB UpdateItem] (status → processing)
                   ├─ [DynamoDB GetItem] (fetch message)
                   ├─ [Lambda Invoke] (Notification Service)
                   │    ├─ [DynamoDB GetItem] (circuit breaker state)
                   │    ├─ [HTTP POST] (ntfy.sh API)
                   │    └─ [DynamoDB UpdateItem] (circuit breaker state)
                   └─ [DynamoDB UpdateItem] (status → completed)
```

**Trace Metadata**:
- **Correlation ID**: Attached as annotation (searchable)
- **Message ID**: Attached as annotation
- **Message Type**: Attached as annotation
- **Status**: Attached as metadata

**Annotations vs Metadata**:
- **Annotations**: Indexed, searchable (e.g., correlation_id, message_id)
- **Metadata**: Not indexed, for context (e.g., payload, error details)

**Go Code** (Adding annotations):
```go
import "github.com/aws/aws-xray-sdk-go/xray"

func (h *Handler) processMessage(ctx context.Context, messageID, correlationID string) error {
	// Add annotations (searchable in X-Ray console)
	xray.AddAnnotation(ctx, "message_id", messageID)
	xray.AddAnnotation(ctx, "correlation_id", correlationID)

	// Add metadata (not searchable, but visible in trace details)
	xray.AddMetadata(ctx, "message_type", "scheduled_hello")

	// ... processing logic

	return nil
}
```

---

### X-Ray Sampling

**Default Sampling Rule**:
- **Reservoir**: 1 request/second (always trace)
- **Rate**: 5% of additional requests

**Custom Sampling Rule** (prod):
- **Priority**: 100 (lower = higher priority)
- **Reservoir**: 1/second
- **Rate**: 5%
- **Service Type**: AWS::Lambda::Function
- **URL Path**: * (all paths)

**Dev Environment**: 100% sampling (trace everything for debugging)

**Cost Optimization**: 5% sampling in prod reduces X-Ray costs while maintaining visibility

---

## Alerting Strategy

### CloudWatch Alarms

#### Alarm 1: DLQ Has Messages

**Metric**: `ApproximateNumberOfMessagesVisible` (DLQ)

**Threshold**: > 0

**Period**: 1 minute

**Action**: SNS topic → Email/Slack notification

**Severity**: High (messages permanently failed)

---

#### Alarm 2: Lambda Errors

**Metric**: `Errors` (per Lambda function)

**Threshold**: > 5 errors in 5 minutes

**Period**: 5 minutes

**Action**: SNS topic → PagerDuty/Slack

**Severity**: Medium (investigate if sustained)

---

#### Alarm 3: Lambda Duration (p99)

**Metric**: `Duration` (p99)

**Threshold**: > 4 minutes (Message Processor)

**Period**: 5 minutes

**Action**: SNS topic → Email

**Severity**: Medium (approaching timeout)

---

#### Alarm 4: Circuit Breaker Open

**Metric**: `CircuitBreakerState` (custom metric)

**Threshold**: = 1 (open state)

**Period**: 1 minute

**Action**: SNS topic → PagerDuty

**Severity**: High (ntfy.sh unavailable)

---

#### Alarm 5: API Error Rate

**Metric**: `APIRequestCount` (StatusCode >= 500)

**Threshold**: > 10 errors in 5 minutes

**Period**: 5 minutes

**Action**: SNS topic → Slack

**Severity**: Medium

---

#### Alarm 6: SQS Queue Backlog

**Metric**: `ApproximateAgeOfOldestMessage` (main queue)

**Threshold**: > 300 seconds (5 minutes)

**Period**: 5 minutes

**Action**: SNS topic → Email

**Severity**: Medium (processing lag)

---

### Alarm Actions (SNS Topics)

**SNS Topics**:
- `rez-agent-alerts-critical-{stage}`: PagerDuty integration
- `rez-agent-alerts-warning-{stage}`: Slack webhook
- `rez-agent-alerts-info-{stage}`: Email only

**Subscriptions**:
- **Critical**: PagerDuty (on-call engineer), Slack
- **Warning**: Slack channel (#rez-agent-alerts)
- **Info**: Team email distribution list

---

## Dashboards

### CloudWatch Dashboard

**Dashboard Name**: `rez-agent-{stage}-overview`

**Widgets**:

1. **Messages Created (24h)**: Line chart, `MessagesCreated` by MessageType
2. **Messages Processed (24h)**: Line chart, `MessagesProcessed` by Status
3. **Success Rate (24h)**: Single value, `(completed / total) * 100`
4. **Lambda Invocations**: Line chart, all Lambda functions
5. **Lambda Errors**: Stacked area chart, errors by function
6. **Lambda Duration (p99)**: Line chart, p99 duration by function
7. **SQS Queue Depth**: Line chart, `ApproximateNumberOfMessagesVisible`
8. **DLQ Message Count**: Single value, `ApproximateNumberOfMessagesVisible` (DLQ)
9. **API Response Time (p95)**: Line chart, `APIResponseTime` by endpoint
10. **Notification Duration (avg)**: Line chart, `NotificationDuration`
11. **Circuit Breaker State**: Line chart, `CircuitBreakerState`
12. **DynamoDB Throttles**: Line chart, `ThrottledRequests`

**Refresh**: Auto-refresh every 1 minute

---

### Grafana Dashboard (Optional Advanced)

**Data Source**: CloudWatch

**Benefits**:
- More flexible visualizations
- Cross-service correlation
- Custom variables (stage, time range)
- Templating for multi-environment view

**Example Panels**:
- Heatmap: Message processing duration distribution
- Gauge: Current SQS queue depth
- Table: Recent errors with correlation IDs (clickable to logs)

---

## Service Level Objectives (SLOs)

### SLI/SLO Definitions

**SLI**: Service Level Indicator (measurable metric)

**SLO**: Service Level Objective (target threshold)

| SLI | SLO | Measurement |
|-----|-----|-------------|
| **API Availability** | 99.9% (30d rolling) | `(successful_requests / total_requests) * 100` |
| **API Response Time** | p95 < 500ms, p99 < 1000ms | CloudWatch metric: `APIResponseTime` |
| **Message Processing Success Rate** | 99.5% (30d rolling) | `(completed / total_processed) * 100` |
| **Message Processing Latency** | p95 < 10 seconds (end-to-end) | X-Ray trace duration (EventBridge → completed) |
| **Notification Delivery Success Rate** | 99% (7d rolling) | `(successful_notifications / total_notifications) * 100` |

**Error Budget**: Based on SLO, calculate allowed downtime/errors
- **99.9% availability**: 43 minutes/month downtime allowed
- **99.5% success rate**: 500 failures/100k messages allowed

---

## Runbook Integration

### CloudWatch Alarms → Runbooks

**Alarm Description**: Include link to runbook (Confluence, GitHub Wiki)

**Example**:
```
Alarm: DLQ Has Messages
Description: Messages have been sent to Dead Letter Queue after max retries.
Runbook: https://github.com/rez-agent/runbooks/blob/main/dlq-investigation.md
```

**Runbook Sections**:
1. **Symptoms**: What alert indicates
2. **Impact**: User/system impact
3. **Investigation Steps**: How to debug (CloudWatch Logs Insights queries, X-Ray traces)
4. **Resolution Steps**: Common fixes, redrive procedure
5. **Escalation**: When to escalate, who to contact

---

## Cost Optimization

### Logging Costs

**CloudWatch Logs Pricing**:
- **Ingestion**: $0.50/GB
- **Storage**: $0.03/GB/month

**Optimization**:
- Reduce log retention (7 days dev, 30 days prod)
- Filter debug logs in prod (only INFO/WARN/ERROR)
- Use log sampling for high-volume logs (e.g., 10% of successful API requests)

---

### Metrics Costs

**CloudWatch Metrics Pricing**:
- **Custom Metrics**: $0.30/metric/month (first 10k)
- **API Requests**: $0.01/1000 GetMetricStatistics requests

**Optimization**:
- Limit custom metrics to critical business metrics (10-20 metrics)
- Use high-resolution metrics (1-second granularity) only when needed
- Aggregate metrics in Lambda before publishing (batch PutMetricData calls)

---

### X-Ray Costs

**X-Ray Pricing**:
- **Traces**: $5/million traces recorded
- **Scanned**: $0.50/million traces scanned (retrieved)

**Optimization**:
- **Sampling**: 5% in prod (reduces cost 95%)
- **Retention**: 30 days (default, no cost)
- **Filter traces**: Only scan traces with errors (filter in X-Ray console)

**Cost Example** (100k messages/day):
- **100% sampling**: 100k traces/day × 30 days × $5/million = $15/month
- **5% sampling**: 5k traces/day × 30 days × $5/million = $0.75/month

---

## Summary

The rez_agent observability and monitoring strategy provides:

1. **Structured logging**: JSON logs with correlation IDs for cross-service tracing
2. **CloudWatch Logs Insights**: Query logs for debugging and analysis
3. **Custom metrics**: Track business and operational metrics (messages, notifications, processing time)
4. **Distributed tracing**: X-Ray for visualizing request flow and identifying bottlenecks
5. **CloudWatch alarms**: Proactive alerting on critical issues (DLQ, errors, circuit breaker)
6. **Dashboards**: Real-time visibility into system health and performance
7. **SLI/SLOs**: Measurable service level objectives with error budgets
8. **Cost optimization**: Sampling, filtering, and retention policies to control costs

This comprehensive observability approach ensures the system is transparent, debuggable, and proactively monitored for production reliability.
