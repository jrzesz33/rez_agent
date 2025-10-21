# rez_agent Message Schemas

## Overview

This document defines the JSON message schemas for SNS and SQS messages in the rez_agent system.

**Design Principles**:
1. **Generic envelope**: Message structure is independent of message_type (extensibility)
2. **Minimal payload**: Only message_id in SNS/SQS (full data in DynamoDB)
3. **Idempotency**: message_id allows deduplication
4. **Traceability**: correlation_id for distributed tracing
5. **Versioning**: schema_version for future evolution

---

## SNS Message Schema

### SNS Topic: `rez-agent-messages-{stage}`

**Purpose**: Event notification when new message is created (by Scheduler or Web API)

**Message Attributes** (SNS metadata):
- `event_type` (String): Type of event (e.g., "message_created")
- `message_type` (String): Type of message (e.g., "scheduled_hello", "manual_notification")
- `stage` (String): Environment (dev, stage, prod)
- `source` (String): Event source (scheduler, web-api)

**Message Body** (JSON):

```json
{
  "schema_version": "1.0",
  "event_type": "message_created",
  "event_id": "770f9600-g49d-62f6-c938-668877662222",
  "timestamp": "2025-10-21T14:30:00.000Z",
  "source": "scheduler",
  "message": {
    "message_id": "550e8400-e29b-41d4-a716-446655440000",
    "message_type": "scheduled_hello",
    "stage": "dev",
    "created_by": "scheduler",
    "correlation_id": "660e9500-f39c-52e5-b827-557766551111"
  },
  "metadata": {
    "region": "us-east-1",
    "environment": "dev"
  }
}
```

### Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | String | Yes | Schema version (for future evolution) |
| `event_type` | String | Yes | Event type (always "message_created" for now) |
| `event_id` | String (UUID) | Yes | Unique event identifier (different from message_id) |
| `timestamp` | String (ISO 8601) | Yes | Event timestamp (when SNS message published) |
| `source` | String | Yes | Event source: "scheduler", "web-api" |
| `message.message_id` | String (UUID) | Yes | Unique message identifier (DynamoDB PK) |
| `message.message_type` | String | Yes | Type of message (e.g., "scheduled_hello") |
| `message.stage` | String | Yes | Environment stage (dev, stage, prod) |
| `message.created_by` | String | Yes | Creator (scheduler, user:email@example.com) |
| `message.correlation_id` | String (UUID) | Yes | Distributed tracing ID (X-Ray) |
| `metadata.region` | String | Yes | AWS region |
| `metadata.environment` | String | Yes | Environment name |

### Example: Scheduled Message

```json
{
  "schema_version": "1.0",
  "event_type": "message_created",
  "event_id": "770f9600-g49d-62f6-c938-668877662222",
  "timestamp": "2025-10-21T14:30:00.000Z",
  "source": "scheduler",
  "message": {
    "message_id": "550e8400-e29b-41d4-a716-446655440000",
    "message_type": "scheduled_hello",
    "stage": "dev",
    "created_by": "scheduler",
    "correlation_id": "660e9500-f39c-52e5-b827-557766551111"
  },
  "metadata": {
    "region": "us-east-1",
    "environment": "dev"
  }
}
```

### Example: Manual Message (Web API)

```json
{
  "schema_version": "1.0",
  "event_type": "message_created",
  "event_id": "880g0700-h50e-73g7-d049-779988773333",
  "timestamp": "2025-10-21T15:00:00.000Z",
  "source": "web-api",
  "message": {
    "message_id": "660e9500-f39c-52e5-b827-557766551111",
    "message_type": "manual_notification",
    "stage": "dev",
    "created_by": "user:alice@example.com",
    "correlation_id": "660e9500-f39c-52e5-b827-557766551111"
  },
  "metadata": {
    "region": "us-east-1",
    "environment": "dev"
  }
}
```

---

## SQS Message Schema

### SQS Queue: `rez-agent-message-queue-{stage}`

**Purpose**: Message queue for processing pipeline (subscribed to SNS topic)

**SQS Message Structure**:
```json
{
  "Type": "Notification",
  "MessageId": "sqs-message-id-123",
  "TopicArn": "arn:aws:sns:us-east-1:123456789012:rez-agent-messages-dev",
  "Subject": null,
  "Message": "{...SNS message body...}",
  "Timestamp": "2025-10-21T14:30:00.000Z",
  "SignatureVersion": "1",
  "Signature": "...",
  "SigningCertURL": "...",
  "UnsubscribeURL": "...",
  "MessageAttributes": {
    "event_type": {
      "Type": "String",
      "Value": "message_created"
    },
    "message_type": {
      "Type": "String",
      "Value": "scheduled_hello"
    },
    "stage": {
      "Type": "String",
      "Value": "dev"
    },
    "source": {
      "Type": "String",
      "Value": "scheduler"
    }
  }
}
```

**Note**: SQS wraps SNS message in `Message` field (JSON string). Lambda must parse this.

### Parsed Message Body

After parsing `Message` field, the body is identical to SNS schema:

```json
{
  "schema_version": "1.0",
  "event_type": "message_created",
  "event_id": "770f9600-g49d-62f6-c938-668877662222",
  "timestamp": "2025-10-21T14:30:00.000Z",
  "source": "scheduler",
  "message": {
    "message_id": "550e8400-e29b-41d4-a716-446655440000",
    "message_type": "scheduled_hello",
    "stage": "dev",
    "created_by": "scheduler",
    "correlation_id": "660e9500-f39c-52e5-b827-557766551111"
  },
  "metadata": {
    "region": "us-east-1",
    "environment": "dev"
  }
}
```

---

## Go Struct Definitions

### SNS/SQS Event Structs

```go
package models

import (
	"time"
)

// MessageEvent represents the SNS/SQS message body
type MessageEvent struct {
	SchemaVersion string            `json:"schema_version"`
	EventType     string            `json:"event_type"` // "message_created"
	EventID       string            `json:"event_id"`   // UUID
	Timestamp     time.Time         `json:"timestamp"`
	Source        string            `json:"source"` // "scheduler", "web-api"
	Message       MessageEventData  `json:"message"`
	Metadata      MessageMetadata   `json:"metadata"`
}

// MessageEventData contains message identifiers
type MessageEventData struct {
	MessageID     string `json:"message_id"`
	MessageType   string `json:"message_type"`
	Stage         string `json:"stage"`
	CreatedBy     string `json:"created_by"`
	CorrelationID string `json:"correlation_id"`
}

// MessageMetadata contains event metadata
type MessageMetadata struct {
	Region      string `json:"region"`
	Environment string `json:"environment"`
}

// SQSMessageWrapper represents the SQS wrapper around SNS message
// (automatically handled by AWS Lambda SQS event source mapping)
type SQSMessageWrapper struct {
	Type              string                       `json:"Type"`
	MessageID         string                       `json:"MessageId"`
	TopicArn          string                       `json:"TopicArn"`
	Message           string                       `json:"Message"` // JSON string (parse to MessageEvent)
	Timestamp         time.Time                    `json:"Timestamp"`
	MessageAttributes map[string]MessageAttribute  `json:"MessageAttributes"`
}

// MessageAttribute represents SQS/SNS message attribute
type MessageAttribute struct {
	Type  string `json:"Type"`
	Value string `json:"Value"`
}
```

---

## Message Processing Flow

### 1. Scheduler/Web API Publishes to SNS

**Go Code (Scheduler Lambda)**:
```go
package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/google/uuid"
)

func publishMessageEvent(ctx context.Context, snsClient *sns.Client, topicARN string, message MessageEventData) error {
	event := MessageEvent{
		SchemaVersion: "1.0",
		EventType:     "message_created",
		EventID:       uuid.New().String(),
		Timestamp:     time.Now().UTC(),
		Source:        "scheduler",
		Message:       message,
		Metadata: MessageMetadata{
			Region:      "us-east-1",
			Environment: "dev",
		},
	}

	messageBody, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = snsClient.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(topicARN),
		Message:  aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"event_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String("message_created"),
			},
			"message_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String(message.MessageType),
			},
			"stage": {
				DataType:    aws.String("String"),
				StringValue: aws.String(message.Stage),
			},
			"source": {
				DataType:    aws.String("String"),
				StringValue: aws.String("scheduler"),
			},
		},
	})

	return err
}
```

---

### 2. Message Processor Consumes from SQS

**Go Code (Message Processor Lambda)**:
```go
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type Handler struct {
	// Dependencies (DynamoDB, Notification Service)
}

func (h *Handler) HandleSQSEvent(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	var failedMessageIDs []events.SQSBatchItemFailure

	for _, record := range event.Records {
		// Parse SQS wrapper (SNS notification)
		var snsWrapper SQSMessageWrapper
		if err := json.Unmarshal([]byte(record.Body), &snsWrapper); err != nil {
			// Invalid message format, send to DLQ
			failedMessageIDs = append(failedMessageIDs, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
			continue
		}

		// Parse SNS message body
		var messageEvent MessageEvent
		if err := json.Unmarshal([]byte(snsWrapper.Message), &messageEvent); err != nil {
			failedMessageIDs = append(failedMessageIDs, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
			continue
		}

		// Process message
		if err := h.processMessage(ctx, messageEvent); err != nil {
			// Failed processing, return to queue for retry
			failedMessageIDs = append(failedMessageIDs, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
			continue
		}
	}

	// Return partial batch failures (only failed messages re-queued)
	return events.SQSEventResponse{
		BatchItemFailures: failedMessageIDs,
	}, nil
}

func (h *Handler) processMessage(ctx context.Context, event MessageEvent) error {
	messageID := event.Message.MessageID

	// 1. Update status to "processing" in DynamoDB
	if err := h.updateMessageStatus(ctx, messageID, "processing"); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// 2. Fetch full message data from DynamoDB
	message, err := h.getMessageByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("failed to fetch message: %w", err)
	}

	// 3. Invoke Notification Service
	if err := h.sendNotification(ctx, message); err != nil {
		// Update status to "failed" with error message
		h.updateMessageStatus(ctx, messageID, "failed", err.Error())
		return fmt.Errorf("notification failed: %w", err)
	}

	// 4. Update status to "completed"
	if err := h.updateMessageStatus(ctx, messageID, "completed"); err != nil {
		return fmt.Errorf("failed to update final status: %w", err)
	}

	return nil
}

func main() {
	handler := &Handler{
		// Initialize dependencies
	}
	lambda.Start(handler.HandleSQSEvent)
}
```

---

## Message Filtering (SNS Subscription Filter)

### Use Case: Route Different Message Types to Different Queues (Future)

**Example**: Route `alert_notification` to high-priority queue

**SNS Subscription Filter Policy** (JSON):
```json
{
  "message_type": ["scheduled_hello", "manual_notification"]
}
```

**High-Priority Queue Filter**:
```json
{
  "message_type": ["alert_notification"]
}
```

**Benefits**:
- Separate processing pipelines for different message types
- Priority-based processing (different Lambda concurrency limits)
- Cost optimization (process low-priority messages less frequently)

---

## SQS Queue Configuration

### Standard Queue Settings

| Setting | Value | Rationale |
|---------|-------|-----------|
| **Visibility Timeout** | 5 minutes (300s) | Matches Lambda timeout, prevents duplicate processing |
| **Message Retention** | 14 days | AWS maximum, allows debugging failed messages |
| **Delivery Delay** | 0 seconds | Immediate processing |
| **Receive Wait Time** | 20 seconds | Long polling (reduces costs, improves efficiency) |
| **Maximum Message Size** | 256 KB | AWS default (sufficient for message IDs) |
| **Batch Size** | 10 messages | Lambda event source mapping batch size |
| **Dead Letter Queue** | Enabled | After 3 retries, send to DLQ |
| **Max Receive Count** | 3 | Number of retries before DLQ |

### Dead Letter Queue (DLQ) Settings

| Setting | Value | Rationale |
|---------|-------|-----------|
| **Message Retention** | 14 days | Maximum retention for investigation |
| **Redrive** | Manual | Replay messages after fixing issues |
| **Alarm** | CloudWatch alarm on message count > 0 | Alert on-call engineer |

---

## Message Size Optimization

### Why Minimal Payload?

**Design Decision**: Only include message_id in SNS/SQS, not full message payload

**Rationale**:
1. **SNS/SQS limits**: 256 KB max message size
2. **Cost**: SNS/SQS charged per 64 KB chunk
3. **Flexibility**: Payload can change in DynamoDB without re-publishing
4. **Single source of truth**: DynamoDB is authoritative, not SQS

**Trade-off**: Extra DynamoDB read (GetItem) in Message Processor

**Cost Analysis**:
- **DynamoDB GetItem**: $0.25 per million reads (negligible)
- **Benefit**: Decoupling, smaller SQS messages, single source of truth

---

## Schema Versioning Strategy

### Current Version: 1.0

**Fields**: All fields described above

### Future Evolution (Example: 2.0)

**New Fields** (hypothetical):
- `priority`: Message priority (low, normal, high)
- `retry_policy`: Custom retry configuration per message

**Migration Strategy**:
1. **Additive changes**: Add new optional fields (no breaking change)
2. **Version detection**: Message Processor checks `schema_version`
3. **Backward compatibility**: Support both 1.0 and 2.0 schemas
4. **Deprecation**: Announce v1.0 deprecation 6 months before removal

**Go Code (Version Handling)**:
```go
func (h *Handler) processMessage(ctx context.Context, event MessageEvent) error {
	switch event.SchemaVersion {
	case "1.0":
		return h.processV1(ctx, event)
	case "2.0":
		return h.processV2(ctx, event)
	default:
		return fmt.Errorf("unsupported schema version: %s", event.SchemaVersion)
	}
}
```

---

## Error Message Schema (DLQ)

When message fails and sent to DLQ, SQS includes error metadata:

**SQS Message Attributes** (automatically added):
- `ApproximateReceiveCount`: Number of retry attempts
- `SentTimestamp`: Original message timestamp
- `ApproximateFirstReceiveTimestamp`: When first received

**Message Body**: Same as original (unchanged)

**Investigation**:
1. Query DLQ messages via AWS Console or CLI
2. Check `error_message` field in DynamoDB for failure details
3. Fix underlying issue (e.g., ntfy.sh API down)
4. Redrive messages from DLQ to main queue (manual or script)

---

## Idempotency and Deduplication

### SQS Deduplication (FIFO Queue - Not Used)

**Standard Queue** (used in rez_agent):
- No built-in deduplication
- Same message can be delivered multiple times (at-least-once delivery)

**Idempotency Strategy**:
1. **Check status before processing**: If status != "created"/"queued", skip (already processed)
2. **Conditional DynamoDB update**: Use `ConditionExpression` to prevent duplicate status updates
3. **Notification Service idempotency**: Track notification_id in DynamoDB

**Go Code (Idempotent Processing)**:
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
		// ConditionalCheckFailedException = already processed
		var ccfe *types.ConditionalCheckFailedException
		if errors.As(err, &ccfe) {
			// Idempotent: message already processed, not an error
			return nil
		}
		return err
	}

	return nil
}
```

---

## Testing Message Schemas

### Unit Test Example (Go)

```go
package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageEventSerialization(t *testing.T) {
	event := MessageEvent{
		SchemaVersion: "1.0",
		EventType:     "message_created",
		EventID:       "event-uuid",
		Timestamp:     time.Date(2025, 10, 21, 14, 30, 0, 0, time.UTC),
		Source:        "scheduler",
		Message: MessageEventData{
			MessageID:     "message-uuid",
			MessageType:   "scheduled_hello",
			Stage:         "dev",
			CreatedBy:     "scheduler",
			CorrelationID: "correlation-uuid",
		},
		Metadata: MessageMetadata{
			Region:      "us-east-1",
			Environment: "dev",
		},
	}

	// Serialize
	data, err := json.Marshal(event)
	require.NoError(t, err)

	// Deserialize
	var parsed MessageEvent
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// Validate
	assert.Equal(t, "1.0", parsed.SchemaVersion)
	assert.Equal(t, "message_created", parsed.EventType)
	assert.Equal(t, "message-uuid", parsed.Message.MessageID)
	assert.Equal(t, "dev", parsed.Message.Stage)
}
```

---

## Message Schema Summary

| Schema | Purpose | Size | Lifetime |
|--------|---------|------|----------|
| **SNS Message** | Event notification (message created) | ~500 bytes | Ephemeral (seconds) |
| **SQS Message** | Processing queue (includes SNS wrapper) | ~800 bytes | 14 days max retention |
| **DynamoDB Item** | Full message metadata and payload | 1-5 KB | 90 days (TTL) |

**Key Design Decisions**:
1. **Minimal SNS/SQS payload**: Only message_id (not full payload)
2. **Single source of truth**: DynamoDB for message data
3. **Idempotency**: message_id for deduplication
4. **Extensibility**: schema_version for future evolution
5. **Traceability**: correlation_id for distributed tracing

This design provides a foundation for reliable, scalable, and maintainable event-driven messaging.
