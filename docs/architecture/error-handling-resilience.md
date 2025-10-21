# rez_agent Error Handling & Resilience Patterns

## Overview

This document defines error handling strategies and resilience patterns for the rez_agent event-driven messaging system.

**Design Principles**:
1. **Fail gracefully**: Degrade functionality rather than complete failure
2. **Retry intelligently**: Exponential backoff for transient errors
3. **Circuit breakers**: Prevent cascading failures
4. **Idempotency**: Safe retries without duplicate effects
5. **Observability**: Log errors with context for debugging
6. **Dead letter queues**: Capture permanently failed messages

---

## Error Categories

### 1. Transient Errors (Retryable)

**Definition**: Temporary failures that may succeed on retry

**Examples**:
- Network timeouts (ntfy.sh API)
- Rate limiting (429 Too Many Requests)
- AWS service throttling (DynamoDB, SQS)
- Temporary service unavailability (503 Service Unavailable)

**Handling Strategy**: Retry with exponential backoff

---

### 2. Permanent Errors (Non-Retryable)

**Definition**: Errors that will not resolve with retries

**Examples**:
- Invalid message format (400 Bad Request)
- Authentication failure (401 Unauthorized)
- Message not found (404 Not Found)
- Business logic errors (invalid payload schema)

**Handling Strategy**: Log error, update status to "failed", send to DLQ

---

### 3. Validation Errors (Client Errors)

**Definition**: Invalid input from user or upstream service

**Examples**:
- Missing required fields
- Invalid data types (string instead of number)
- Constraint violations (stage not in [dev, stage, prod])

**Handling Strategy**: Return 400 Bad Request with detailed error message

---

### 4. System Errors (Internal Errors)

**Definition**: Unexpected errors in application logic or infrastructure

**Examples**:
- Null pointer dereference
- Database connection failure
- Lambda timeout (exceeds 5 minutes)

**Handling Strategy**: Return 500 Internal Server Error, log with correlation ID, alert on-call

---

## Retry Strategy

### Exponential Backoff with Jitter

**Algorithm**:
```
wait_time = min(max_wait, base * (2 ^ attempt)) + random_jitter
```

**Parameters**:
- `base`: 1 second
- `max_wait`: 32 seconds
- `max_attempts`: 3
- `jitter`: Random 0-500ms (prevent thundering herd)

**Retry Schedule**:
- Attempt 1: 0s (immediate)
- Attempt 2: 1s + jitter (after ~1s)
- Attempt 3: 2s + jitter (after ~2s)
- Attempt 4: 4s + jitter (after ~4s)
- After 3 retries → DLQ

**Go Implementation**:
```go
package retry

import (
	"context"
	"math"
	"math/rand"
	"time"
)

type Config struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func DefaultConfig() Config {
	return Config{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    32 * time.Second,
	}
}

func WithExponentialBackoff(ctx context.Context, cfg Config, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		if !IsRetryable(err) {
			return err // Permanent error, don't retry
		}

		// Calculate backoff with jitter
		if attempt < cfg.MaxAttempts-1 {
			backoff := calculateBackoff(attempt, cfg.BaseDelay, cfg.MaxDelay)
			select {
			case <-time.After(backoff):
				// Continue to next attempt
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return lastErr
}

func calculateBackoff(attempt int, base, max time.Duration) time.Duration {
	exponential := base * time.Duration(math.Pow(2, float64(attempt)))
	if exponential > max {
		exponential = max
	}

	// Add jitter (0-500ms)
	jitter := time.Duration(rand.Int63n(500)) * time.Millisecond
	return exponential + jitter
}

func IsRetryable(err error) bool {
	// Check error type/message for retryable conditions
	// - Network timeouts
	// - 429 Rate Limit
	// - 503 Service Unavailable
	// - DynamoDB throttling errors
	// Return false for 4xx (except 429), validation errors, etc.

	// Example (simplified):
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	if httpErr, ok := err.(*HTTPError); ok {
		return httpErr.StatusCode == 429 || httpErr.StatusCode == 503
	}

	return false
}
```

---

## Circuit Breaker Pattern

### Purpose

Prevent cascading failures when downstream service (ntfy.sh) is unavailable.

**States**:
1. **Closed** (Normal): Requests flow through, monitor failure rate
2. **Open** (Failing): Fail fast without calling downstream service
3. **Half-Open** (Testing): Allow single request to test if service recovered

**State Transitions**:
```
        Success rate > threshold
  ┌──────────────────────────────────┐
  │                                  │
  ▼                                  │
┌─────────┐                    ┌───────────┐
│ Closed  │──Failures exceed──>│   Open    │
│ (Normal)│     threshold      │ (Failing) │
└─────────┘                    └─────┬─────┘
      ▲                              │
      │                              │ Timeout expires
      │                              │ (30 seconds)
      │                              ▼
      │                        ┌────────────┐
      │         Success        │ Half-Open  │
      └────────────────────────│ (Testing)  │
                               └──────┬─────┘
                                      │
                                      │ Failure
                                      └──────> Open
```

**Thresholds**:
- **Open threshold**: 5 failures in 1 minute
- **Success threshold**: 2 consecutive successes (Half-Open → Closed)
- **Timeout**: 30 seconds (Open → Half-Open)

---

### Circuit Breaker Implementation (Go)

**Storage**: DynamoDB table for distributed circuit state (shared across Lambda instances)

**Table**: `rez-agent-circuit-breaker-{stage}`

**Schema**:
```json
{
  "service_name": "ntfy-sh", // PK
  "state": "closed", // closed, open, half_open
  "failure_count": 0,
  "last_failure_time": "2025-10-21T14:30:00Z",
  "last_success_time": "2025-10-21T14:29:00Z",
  "opened_at": null, // When circuit opened
  "ttl": 1737464400 // Auto-delete after 7 days
}
```

**Go Code**:
```go
package circuitbreaker

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	StateClosed   = "closed"
	StateOpen     = "open"
	StateHalfOpen = "half_open"

	FailureThreshold   = 5
	SuccessThreshold   = 2
	OpenTimeout        = 30 * time.Second
	FailureTimeWindow  = 1 * time.Minute
)

var (
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

type CircuitBreaker struct {
	serviceName   string
	tableName     string
	dynamodbClient *dynamodb.Client
}

func New(serviceName, tableName string, client *dynamodb.Client) *CircuitBreaker {
	return &CircuitBreaker{
		serviceName:   serviceName,
		tableName:     tableName,
		dynamodbClient: client,
	}
}

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	// Check current state
	state, err := cb.getState(ctx)
	if err != nil {
		// If can't read state, allow request (fail open)
		return fn()
	}

	// If circuit is open, check if timeout expired
	if state.State == StateOpen {
		if time.Since(state.OpenedAt) > OpenTimeout {
			// Transition to half-open
			cb.setState(ctx, StateHalfOpen)
		} else {
			// Circuit still open, fail fast
			return ErrCircuitOpen
		}
	}

	// Execute function
	err = fn()

	if err != nil {
		// Record failure
		cb.recordFailure(ctx)
		return err
	}

	// Record success
	cb.recordSuccess(ctx)
	return nil
}

func (cb *CircuitBreaker) getState(ctx context.Context) (*CircuitState, error) {
	result, err := cb.dynamodbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(cb.tableName),
		Key: map[string]types.AttributeValue{
			"service_name": &types.AttributeValueMemberS{Value: cb.serviceName},
		},
	})

	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		// No state exists, initialize to closed
		return &CircuitState{State: StateClosed, FailureCount: 0}, nil
	}

	// Parse DynamoDB item to CircuitState
	// (implementation omitted for brevity)
	return parseCircuitState(result.Item), nil
}

func (cb *CircuitBreaker) recordFailure(ctx context.Context) {
	state, _ := cb.getState(ctx)

	state.FailureCount++
	state.LastFailureTime = time.Now()

	// Check if should open circuit
	if state.FailureCount >= FailureThreshold {
		if time.Since(state.LastFailureTime) < FailureTimeWindow {
			state.State = StateOpen
			state.OpenedAt = time.Now()
		}
	}

	cb.updateState(ctx, state)
}

func (cb *CircuitBreaker) recordSuccess(ctx context.Context) {
	state, _ := cb.getState(ctx)

	state.LastSuccessTime = time.Now()

	if state.State == StateHalfOpen {
		// Transition to closed after success
		state.State = StateClosed
		state.FailureCount = 0
		state.OpenedAt = time.Time{}
	} else if state.State == StateClosed {
		// Reset failure count on success
		state.FailureCount = 0
	}

	cb.updateState(ctx, state)
}

type CircuitState struct {
	ServiceName     string
	State           string
	FailureCount    int
	LastFailureTime time.Time
	LastSuccessTime time.Time
	OpenedAt        time.Time
}
```

**Usage in Notification Service**:
```go
func (ns *NotificationService) SendNotification(ctx context.Context, message *Message) error {
	circuitBreaker := circuitbreaker.New("ntfy-sh", "rez-agent-circuit-breaker-dev", ns.dynamodbClient)

	err := circuitBreaker.Execute(ctx, func() error {
		return ns.sendToNtfy(ctx, message)
	})

	if err == circuitbreaker.ErrCircuitOpen {
		// Circuit is open, fail gracefully
		return fmt.Errorf("ntfy.sh service unavailable (circuit open): %w", err)
	}

	return err
}
```

---

## Timeout Management

### Lambda Timeout Configuration

| Lambda Function | Timeout | Rationale |
|-----------------|---------|-----------|
| Scheduler | 30s | Simple message creation + SNS publish |
| Web API | 15s | API response time SLA (p99 < 1s) |
| Message Processor | 5 minutes | Matches SQS visibility timeout |
| Notification Service | 30s | ntfy.sh API call + retries |
| JWT Authorizer | 10s | JWT validation (JWKS fetch + parse) |

**Why 5 minutes for Message Processor?**
- Matches SQS visibility timeout (prevents duplicate processing)
- Allows time for 3 retries with exponential backoff (1s + 2s + 4s = 7s)
- Buffer for DynamoDB operations and Lambda cold start

---

### HTTP Client Timeouts

**Go http.Client Configuration**:
```go
package httpclient

import (
	"net"
	"net/http"
	"time"
)

func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second, // Overall request timeout
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,  // Connection timeout
				KeepAlive: 30 * time.Second, // Keep-alive
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second, // Time to receive headers
		},
	}
}
```

**Timeouts**:
- **Connection timeout**: 5s (time to establish TCP connection)
- **TLS handshake**: 5s (time to complete TLS handshake)
- **Response headers**: 5s (time to receive HTTP headers)
- **Overall request**: 10s (total request duration)

---

## SQS Dead Letter Queue (DLQ)

### Configuration

**DLQ Name**: `rez-agent-message-queue-dlq-{stage}`

**Redrive Policy** (on main queue):
```json
{
  "deadLetterTargetArn": "arn:aws:sqs:us-east-1:123456789012:rez-agent-message-queue-dlq-dev",
  "maxReceiveCount": 3
}
```

**Max Receive Count**: 3 (after 3 failed processing attempts → DLQ)

---

### DLQ Processing

**CloudWatch Alarm**:
- **Metric**: `ApproximateNumberOfMessagesVisible` (DLQ)
- **Threshold**: > 0 (alert immediately)
- **Action**: SNS topic → Email/Slack notification to on-call engineer

**Manual Investigation**:
1. Query DLQ messages (AWS Console or CLI)
2. Inspect message body and attributes
3. Check CloudWatch Logs for error details (search by correlation_id)
4. Identify root cause:
   - Permanent error (invalid payload) → Fix code, redeploy
   - Transient error (ntfy.sh down) → Redrive messages after service recovery

**Redrive Messages** (AWS CLI):
```bash
# Get DLQ messages
aws sqs receive-message \
  --queue-url https://sqs.us-east-1.amazonaws.com/123456789012/rez-agent-message-queue-dlq-dev \
  --max-number-of-messages 10

# Redrive to main queue (after fixing issue)
aws sqs start-message-move-task \
  --source-arn arn:aws:sqs:us-east-1:123456789012:rez-agent-message-queue-dlq-dev \
  --destination-arn arn:aws:sqs:us-east-1:123456789012:rez-agent-message-queue-dev
```

---

## Idempotency

### Why Idempotency Matters

**Problem**: SQS delivers messages at-least-once (duplicates possible)

**Solution**: Ensure processing is idempotent (safe to execute multiple times)

---

### Idempotent DynamoDB Updates

**Conditional Update** (only update if status is expected):
```go
func (h *Handler) updateMessageStatus(ctx context.Context, messageID, newStatus string) error {
	_, err := h.dynamodbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(h.tableName),
		Key: map[string]types.AttributeValue{
			"message_id": &types.AttributeValueMemberS{Value: messageID},
		},
		UpdateExpression: aws.String("SET #status = :new_status, updated_date = :updated_date"),
		ConditionExpression: aws.String("#status IN (:expected1, :expected2)"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":new_status":  &types.AttributeValueMemberS{Value: newStatus},
			":updated_date": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
			":expected1":   &types.AttributeValueMemberS{Value: "created"},
			":expected2":   &types.AttributeValueMemberS{Value: "queued"},
		},
	})

	if err != nil {
		var ccfe *types.ConditionalCheckFailedException
		if errors.As(err, &ccfe) {
			// Already processed (status not "created" or "queued")
			return nil // Idempotent: not an error
		}
		return err
	}

	return nil
}
```

**Result**: If message processed twice, second attempt is no-op (status already "completed")

---

### Idempotent Notification Delivery

**Problem**: Same message sent to ntfy.sh multiple times

**Solution**: Track `notification_id` in DynamoDB
```go
func (ns *NotificationService) SendNotification(ctx context.Context, message *Message) error {
	// Check if already sent (notification_id exists)
	if message.NotificationID != "" {
		// Already sent, skip
		return nil
	}

	// Send to ntfy.sh
	notificationID, err := ns.sendToNtfy(ctx, message)
	if err != nil {
		return err
	}

	// Record notification_id in DynamoDB (atomic update)
	_, err = ns.dynamodbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(ns.tableName),
		Key: map[string]types.AttributeValue{
			"message_id": &types.AttributeValueMemberS{Value: message.MessageID},
		},
		UpdateExpression: aws.String("SET notification_id = :notification_id"),
		ConditionExpression: aws.String("attribute_not_exists(notification_id)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":notification_id": &types.AttributeValueMemberS{Value: notificationID},
		},
	})

	return err
}
```

**Result**: If Lambda retries, second attempt skips sending (notification_id already set)

---

## Error Response Format (Web API)

### Structured Error Responses

**Schema** (from OpenAPI spec):
```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": {
      "field": "stage",
      "reason": "must be one of [dev, stage, prod]"
    }
  }
}
```

**Error Codes**:
- `VALIDATION_ERROR`: Invalid request parameters (400)
- `UNAUTHORIZED`: Missing or invalid JWT token (401)
- `FORBIDDEN`: Insufficient permissions (403)
- `MESSAGE_NOT_FOUND`: Message ID not found (404)
- `RATE_LIMIT_EXCEEDED`: Rate limit exceeded (429)
- `INTERNAL_SERVER_ERROR`: Unexpected server error (500)
- `SERVICE_UNAVAILABLE`: Downstream service unavailable (503)

---

### Go Error Response Helper

```go
package api

import (
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func ErrorResponseJSON(statusCode int, code, message string, details map[string]interface{}) events.APIGatewayProxyResponse {
	errorResp := ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
			Details: details,
		},
	}

	body, _ := json.Marshal(errorResp)

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}

// Usage
return ErrorResponseJSON(400, "VALIDATION_ERROR", "Invalid stage parameter", map[string]interface{}{
	"field":  "stage",
	"reason": "must be one of [dev, stage, prod]",
})
```

---

## Graceful Degradation

### Fallback Strategies

#### 1. Metrics Endpoint (GET /api/metrics)

**Failure**: DynamoDB query timeout

**Fallback**: Return cached metrics (stale data acceptable) or partial metrics
```go
func (h *Handler) GetMetrics(ctx context.Context) (*Metrics, error) {
	metrics, err := h.queryMetrics(ctx)
	if err != nil {
		// Fallback: Return cached metrics (Redis or in-memory)
		cachedMetrics, cacheErr := h.getCachedMetrics()
		if cacheErr == nil {
			return cachedMetrics, nil
		}

		// No cache available, return error
		return nil, err
	}

	// Cache metrics for future fallback
	h.cacheMetrics(metrics)
	return metrics, nil
}
```

---

#### 2. Notification Service (ntfy.sh unavailable)

**Failure**: Circuit breaker open (ntfy.sh down)

**Fallback**: Update message status to "failed", log error, alert on-call
```go
func (ns *NotificationService) SendNotification(ctx context.Context, message *Message) error {
	err := ns.circuitBreaker.Execute(ctx, func() error {
		return ns.sendToNtfy(ctx, message)
	})

	if err == circuitbreaker.ErrCircuitOpen {
		// Fallback: Log error, update status, alert
		log.Error("ntfy.sh circuit open, message failed", "message_id", message.MessageID)
		ns.updateMessageStatus(ctx, message.MessageID, "failed", "ntfy.sh service unavailable")
		ns.sendAlert(ctx, "ntfy.sh circuit breaker open")
		return err
	}

	return err
}
```

---

## Partial Batch Failures (SQS)

### Lambda Response Format

**Purpose**: Return only failed messages to SQS (successful messages deleted)

**Go Code**:
```go
func (h *Handler) HandleSQSEvent(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	var failedMessageIDs []events.SQSBatchItemFailure

	for _, record := range event.Records {
		if err := h.processMessage(ctx, record); err != nil {
			// Failed processing, return to queue
			failedMessageIDs = append(failedMessageIDs, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
		}
	}

	// Return partial batch failures
	return events.SQSEventResponse{
		BatchItemFailures: failedMessageIDs,
	}, nil
}
```

**SQS Behavior**:
- Successful messages: Deleted from queue
- Failed messages: Returned to queue (retry after visibility timeout)
- After 3 retries → DLQ

**Lambda Configuration**:
- **ReportBatchItemFailures**: Enabled (required for partial batch failures)

---

## Error Logging Best Practices

### Structured Error Logging

**Use `log/slog` with structured fields**:
```go
import "log/slog"

slog.Error("Failed to process message",
	"error", err,
	"message_id", messageID,
	"correlation_id", correlationID,
	"retry_count", retryCount,
	"status_code", statusCode,
)
```

**Output** (JSON):
```json
{
  "time": "2025-10-21T14:30:00Z",
  "level": "ERROR",
  "msg": "Failed to process message",
  "error": "ntfy.sh API returned 503",
  "message_id": "550e8400-e29b-41d4-a716-446655440000",
  "correlation_id": "660e9500-f39c-52e5-b827-557766551111",
  "retry_count": 2,
  "status_code": 503
}
```

**Benefits**:
- Searchable in CloudWatch Logs Insights
- Correlation ID links logs across services
- Error context for debugging

---

## Summary

The rez_agent error handling and resilience strategy provides:

1. **Intelligent retries**: Exponential backoff with jitter for transient errors
2. **Circuit breaker**: Prevent cascading failures when ntfy.sh unavailable
3. **Timeout management**: Appropriate timeouts for each Lambda and HTTP call
4. **Dead letter queue**: Capture permanently failed messages for investigation
5. **Idempotency**: Safe retries without duplicate effects (conditional DynamoDB updates)
6. **Graceful degradation**: Fallback strategies for non-critical failures
7. **Partial batch failures**: SQS returns only failed messages to queue
8. **Structured error responses**: Consistent API error format with error codes
9. **Observability**: Structured logging with correlation IDs for debugging

This design ensures the system is resilient to failures, recovers gracefully, and provides clear visibility into errors for debugging and alerting.
