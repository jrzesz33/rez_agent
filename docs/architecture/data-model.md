# rez_agent Data Model

## Storage Strategy

### Primary Data Store: Amazon DynamoDB

**Rationale**: See service-architecture.md for full comparison. Key benefits for serverless event-driven systems:
- No connection pooling issues with Lambda
- Auto-scaling without capacity planning
- Single-digit millisecond latency
- Pay-per-request pricing

---

## Table Design

### Messages Table

**Table Name**: `rez-agent-messages-{stage}`

**Primary Key**:
- **Partition Key (PK)**: `message_id` (String, UUID)
- **No Sort Key**: Single-item access pattern

**Attributes**:

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `message_id` | String (UUID) | Yes | Unique identifier (PK) |
| `created_date` | String (ISO 8601) | Yes | Message creation timestamp |
| `created_by` | String | Yes | Creator (e.g., "scheduler", "user:email@example.com") |
| `stage` | String | Yes | Environment: "dev", "stage", "prod" |
| `message_type` | String | Yes | Type of message/job (e.g., "scheduled_hello", "manual_notification") |
| `status` | String | Yes | Lifecycle state: "created", "queued", "processing", "completed", "failed" |
| `payload` | Map | Yes | Message content (JSON object) |
| `updated_date` | String (ISO 8601) | Yes | Last update timestamp |
| `processing_started_at` | String (ISO 8601) | No | When processing began |
| `processing_completed_at` | String (ISO 8601) | No | When processing finished |
| `error_message` | String | No | Error details if status="failed" |
| `retry_count` | Number | No | Number of retry attempts |
| `notification_id` | String | No | ntfy.sh notification identifier |
| `correlation_id` | String (UUID) | Yes | For distributed tracing (X-Ray) |
| `ttl` | Number (Unix timestamp) | No | Auto-delete after 90 days |

**Item Example**:
```json
{
  "message_id": "550e8400-e29b-41d4-a716-446655440000",
  "created_date": "2025-10-21T14:30:00.000Z",
  "created_by": "scheduler",
  "stage": "dev",
  "message_type": "scheduled_hello",
  "status": "completed",
  "payload": {
    "text": "hello world",
    "priority": "default",
    "tags": ["scheduled", "daily"]
  },
  "updated_date": "2025-10-21T14:30:15.000Z",
  "processing_started_at": "2025-10-21T14:30:05.000Z",
  "processing_completed_at": "2025-10-21T14:30:15.000Z",
  "retry_count": 0,
  "notification_id": "ntfy_abc123",
  "correlation_id": "660e9500-f39c-52e5-b827-557766551111",
  "ttl": 1737464400
}
```

---

### Global Secondary Indexes (GSIs)

#### GSI 1: Stage-CreatedDate Index

**Purpose**: Query messages by stage and creation date (dashboard, filtering)

**Index Name**: `stage-created_date-index`

**Key Schema**:
- **Partition Key**: `stage` (String)
- **Sort Key**: `created_date` (String, ISO 8601)

**Projected Attributes**: ALL (allows filtering without additional reads)

**Access Patterns**:
- Get all messages for a stage: `Query(PK=stage, SK begins_with "2025")`
- Get messages in date range: `Query(PK=stage, SK between "2025-10-01" and "2025-10-31")`
- List recent messages: `Query(PK=stage, ScanIndexForward=false, Limit=100)`

**Example Query** (Go SDK v2):
```go
result, err := dynamodbClient.Query(ctx, &dynamodb.QueryInput{
    TableName:              aws.String("rez-agent-messages-dev"),
    IndexName:              aws.String("stage-created_date-index"),
    KeyConditionExpression: aws.String("stage = :stage AND created_date > :date"),
    ExpressionAttributeValues: map[string]types.AttributeValue{
        ":stage": &types.AttributeValueMemberS{Value: "dev"},
        ":date":  &types.AttributeValueMemberS{Value: "2025-10-01T00:00:00Z"},
    },
    ScanIndexForward: aws.Bool(false), // Descending order
    Limit:            aws.Int32(50),
})
```

---

#### GSI 2: Status-CreatedDate Index

**Purpose**: Query messages by status (e.g., all failed messages, processing messages)

**Index Name**: `status-created_date-index`

**Key Schema**:
- **Partition Key**: `status` (String)
- **Sort Key**: `created_date` (String, ISO 8601)

**Projected Attributes**: ALL

**Access Patterns**:
- Get failed messages: `Query(PK="failed", ScanIndexForward=false)`
- Get processing messages: `Query(PK="processing")`
- Get completed messages in date range: `Query(PK="completed", SK between dates)`

**Example Query**:
```go
result, err := dynamodbClient.Query(ctx, &dynamodb.QueryInput{
    TableName:              aws.String("rez-agent-messages-dev"),
    IndexName:              aws.String("status-created_date-index"),
    KeyConditionExpression: aws.String("#status = :status"),
    ExpressionAttributeNames: map[string]string{
        "#status": "status", // Reserved keyword, use placeholder
    },
    ExpressionAttributeValues: map[string]types.AttributeValue{
        ":status": &types.AttributeValueMemberS{Value: "failed"},
    },
    Limit: aws.Int32(100),
})
```

---

#### GSI 3 (Optional): MessageType-CreatedDate Index

**Purpose**: Query messages by type (future extensibility for multiple job types)

**Index Name**: `message_type-created_date-index`

**Key Schema**:
- **Partition Key**: `message_type` (String)
- **Sort Key**: `created_date` (String, ISO 8601)

**Projected Attributes**: ALL

**Access Patterns**:
- Get all scheduled_hello messages
- Get all manual_notification messages
- Analytics on message type distribution

**Note**: Add this GSI when you have 3+ message types to justify the cost.

---

## DynamoDB Configuration

### Capacity Mode
**Recommendation**: On-Demand (pay-per-request)

**Rationale**:
- Unpredictable traffic patterns (scheduled + manual messages)
- No capacity planning required
- Handles spikes automatically
- Cost-effective for low/variable usage

**When to switch to Provisioned**:
- Consistent, predictable traffic
- High sustained throughput (cheaper at scale)
- Cost optimization after 6+ months of usage data

---

### Time-to-Live (TTL)

**Enabled**: Yes

**TTL Attribute**: `ttl` (Number, Unix timestamp)

**Configuration**:
- Set `ttl` to `created_date + 90 days` on item creation
- DynamoDB automatically deletes expired items (within 48 hours)
- No read/write cost for TTL deletions

**Go Code Example**:
```go
import "time"

ttl := time.Now().Add(90 * 24 * time.Hour).Unix()

item := map[string]types.AttributeValue{
    "message_id":   &types.AttributeValueMemberS{Value: messageID},
    "created_date": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
    "ttl":          &types.AttributeValueMemberN{Value: strconv.FormatInt(ttl, 10)},
    // ... other attributes
}
```

---

### Point-in-Time Recovery (PITR)

**Enabled**: Yes (for stage and prod)

**Rationale**:
- Recover from accidental deletes or corruption
- No performance impact
- Minimal cost (adds ~20% to storage cost)

**Disabled for**: dev (cost optimization)

---

### Encryption

**Encryption at Rest**: AWS-managed KMS key (default)

**Rationale**:
- No additional cost
- Meets compliance requirements
- Automatic key rotation

**For higher security**: Customer-managed KMS key (allows audit logging)

---

## Data Access Patterns

### Pattern 1: Get Message by ID
**Operation**: `GetItem`

**Key**: `{message_id: "uuid"}`

**Use Case**: Web API endpoint `GET /api/messages/:id`

**Performance**: Single-digit millisecond latency

---

### Pattern 2: List Messages (with filters)
**Operation**: `Query` (on GSI)

**Index**: `stage-created_date-index` or `status-created_date-index`

**Filters**:
- By stage: `stage = "dev"`
- By status: `status = "completed"`
- By date range: `created_date BETWEEN "2025-10-01" AND "2025-10-31"`

**Pagination**: Use `LastEvaluatedKey` for cursor-based pagination

**Use Case**: Web API endpoint `GET /api/messages?stage=dev&status=completed`

---

### Pattern 3: Update Message Status
**Operation**: `UpdateItem`

**Key**: `{message_id: "uuid"}`

**Update Expression**:
```go
updateExpr := "SET #status = :status, updated_date = :updated_date"
if status == "processing" {
    updateExpr += ", processing_started_at = :started_at"
} else if status == "completed" || status == "failed" {
    updateExpr += ", processing_completed_at = :completed_at"
}

_, err := dynamodbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
    TableName: aws.String(tableName),
    Key: map[string]types.AttributeValue{
        "message_id": &types.AttributeValueMemberS{Value: messageID},
    },
    UpdateExpression: aws.String(updateExpr),
    ExpressionAttributeNames: map[string]string{
        "#status": "status",
    },
    ExpressionAttributeValues: map[string]types.AttributeValue{
        ":status":       &types.AttributeValueMemberS{Value: status},
        ":updated_date": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
        // ... conditional attributes
    },
})
```

**Use Case**: Message Processor updating status transitions

---

### Pattern 4: Get Metrics (Dashboard)
**Operation**: `Query` (on multiple GSIs) + aggregation in Lambda

**Queries**:
1. Count by status: Query `status-created_date-index` for each status
2. Recent messages: Query `stage-created_date-index` with `Limit=10`
3. Failed messages: Query `status-created_date-index` where `status="failed"`

**Optimization**: Cache metrics in ElastiCache (future) or compute with DynamoDB Streams

**Example Aggregation**:
```go
// Count completed messages (last 24 hours)
completed, _ := queryByStatus(ctx, "completed", time.Now().Add(-24*time.Hour))
// Count failed messages
failed, _ := queryByStatus(ctx, "failed", time.Now().Add(-24*time.Hour))

metrics := Metrics{
    CompletedCount: len(completed.Items),
    FailedCount:    len(failed.Items),
    SuccessRate:    float64(len(completed.Items)) / float64(len(completed.Items) + len(failed.Items)),
}
```

---

## Status State Machine

```
┌─────────┐
│ created │  Initial state (Scheduler/Web API)
└────┬────┘
     │
     ▼
┌─────────┐
│ queued  │  Published to SNS → SQS (optional tracking state)
└────┬────┘
     │
     ▼
┌────────────┐
│ processing │  Message Processor starts work
└─────┬──────┘
      │
      ├─────── Success ──────┐
      │                       ▼
      │                  ┌───────────┐
      │                  │ completed │  Final success state
      │                  └───────────┘
      │
      └─────── Failure ──────┐
                              ▼
                         ┌────────┐
                         │ failed │  Final error state (+ error_message)
                         └────────┘
```

**State Transitions**:
- `created` → `queued` (when published to SNS, optional)
- `created`/`queued` → `processing` (when Lambda starts processing)
- `processing` → `completed` (successful notification delivery)
- `processing` → `failed` (max retries exhausted, ntfy.sh error)

**No Backwards Transitions**: States only move forward (idempotency)

---

## Message Payload Schema

The `payload` attribute is a flexible JSON object (DynamoDB Map type) that varies by `message_type`.

### Scheduled Hello Payload
```json
{
  "text": "hello world",
  "priority": "default",
  "tags": ["scheduled", "daily"]
}
```

### Manual Notification Payload (future)
```json
{
  "text": "User-defined message",
  "title": "Optional title",
  "priority": "high",
  "tags": ["manual", "urgent"],
  "click_url": "https://example.com"
}
```

### Extensibility
Add new message types by defining new payload schemas:
- `deployment_notification`: CI/CD integration
- `alert_notification`: Monitoring alerts
- `reminder`: Scheduled reminders

**Validation**: Implement payload schema validation in Web API and Scheduler (use Go `encoding/json` or `go-playground/validator`)

---

## Data Retention and Archival

### Active Data (DynamoDB)
- **Retention**: 90 days (via TTL)
- **Access**: Real-time queries, metrics, dashboard
- **Cost**: On-demand pricing (read/write units)

### Archived Data (S3 + Athena - Optional Future)
If long-term analytics required:
1. **DynamoDB Streams** → Lambda → S3 (Parquet format)
2. **AWS Glue Crawler** creates Athena table schema
3. **Athena** for SQL queries on historical data
4. **S3 Lifecycle**: Transition to Glacier after 1 year

**Cost-Benefit**: Only implement if >1 year retention needed.

---

## Data Consistency Model

### Strong Consistency
**When**: `GetItem`, `Query`, `Scan` with `ConsistentRead=true`

**Use Case**: Immediately after write (e.g., create message → read back for confirmation)

**Cost**: 2x read capacity units

### Eventual Consistency
**When**: GSI queries (always eventually consistent)

**Use Case**: Dashboard queries, list messages (acceptable lag: <1 second typically)

**Cost**: 1x read capacity units

**For rez_agent**: Eventual consistency is acceptable for all use cases.

---

## Indexing Strategy Summary

| Access Pattern | Operation | Index | Notes |
|----------------|-----------|-------|-------|
| Get by message_id | GetItem | Primary key | Fastest, most common |
| List by stage | Query | stage-created_date-index | Dashboard, filtering |
| List by status | Query | status-created_date-index | Failed messages, monitoring |
| List by message_type | Query | message_type-created_date-index | Optional, add later |
| Get metrics | Multiple queries + aggregation | Various GSIs | Cache results in Lambda |

---

## Data Model Versioning

### Schema Evolution
DynamoDB is schema-less, allowing flexible evolution:

**Adding Fields**:
- Add new attributes to items (no migration needed)
- Old items lack the field (check for existence in code)

**Changing Field Types**:
- Avoid (DynamoDB enforces type consistency per attribute name)
- Use new attribute name (e.g., `status_v2`) and backfill

**Removing Fields**:
- Stop writing the field
- Old items retain it (no cleanup needed unless TTL expires)

### Version Tracking (Optional)
Add `schema_version` attribute for explicit versioning:
```json
{
  "message_id": "uuid",
  "schema_version": "1.0",
  ...
}
```

Increment when payload structure changes significantly.

---

## Performance Optimization

### Batch Operations
**BatchGetItem**: Retrieve up to 100 items in one request (for bulk fetching)

**BatchWriteItem**: Write up to 25 items in one request (for bulk inserts)

**Use Case**: Backfilling data, bulk imports

### Pagination Best Practices
**Limit**: Always set `Limit` to prevent large responses (e.g., 50-100 items)

**LastEvaluatedKey**: Use for cursor-based pagination (more efficient than offset)

**Example**:
```go
var allItems []map[string]types.AttributeValue
var lastKey map[string]types.AttributeValue

for {
    result, err := dynamodbClient.Query(ctx, &dynamodb.QueryInput{
        TableName:         aws.String(tableName),
        IndexName:         aws.String("stage-created_date-index"),
        KeyConditionExpression: aws.String("stage = :stage"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":stage": &types.AttributeValueMemberS{Value: "dev"},
        },
        Limit:             aws.Int32(100),
        ExclusiveStartKey: lastKey,
    })

    allItems = append(allItems, result.Items...)

    if result.LastEvaluatedKey == nil {
        break // No more pages
    }
    lastKey = result.LastEvaluatedKey
}
```

### Conditional Writes (Optimistic Locking)
Prevent race conditions when updating status:

```go
_, err := dynamodbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
    TableName: aws.String(tableName),
    Key: map[string]types.AttributeValue{
        "message_id": &types.AttributeValueMemberS{Value: messageID},
    },
    UpdateExpression: aws.String("SET #status = :new_status"),
    ConditionExpression: aws.String("#status = :expected_status"),
    ExpressionAttributeNames: map[string]string{
        "#status": "status",
    },
    ExpressionAttributeValues: map[string]types.AttributeValue{
        ":new_status":      &types.AttributeValueMemberS{Value: "completed"},
        ":expected_status": &types.AttributeValueMemberS{Value: "processing"},
    },
})
```

If condition fails → `ConditionalCheckFailedException` (status already changed)

---

## Cost Estimation (Example)

**Assumptions**:
- 1,000 messages/day (scheduled + manual)
- 90-day retention (90,000 active items)
- Average item size: 1 KB
- On-demand mode

**DynamoDB Costs** (monthly):
- **Write**: 1,000 messages/day × 30 days × $1.25 per million writes = $0.04
- **Read**: 10,000 queries/day × 30 days × $0.25 per million reads = $0.08
- **Storage**: 90,000 items × 1 KB × $0.25/GB = $0.02
- **Total**: ~$0.14/month (negligible)

**At 100x scale** (100,000 messages/day):
- **Total**: ~$14/month (still very cost-effective)

**Note**: Real costs depend on query patterns, GSI usage, and item size.

---

## Summary

The DynamoDB data model for rez_agent provides:

1. **Flexible schema**: Accommodates future message types and payload variations
2. **Efficient queries**: GSIs for common access patterns (stage, status, date range)
3. **Automatic cleanup**: TTL for 90-day retention
4. **Strong performance**: Single-digit millisecond latency, auto-scaling
5. **Cost-effective**: On-demand pricing for variable workload
6. **Observability**: Correlation IDs for distributed tracing
7. **State management**: Clear status state machine with timestamps

This design supports the current "hello world" use case and provides extensibility for complex multi-job-type workflows without schema migrations.
