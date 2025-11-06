# Dynamic Schedule Creation - Design Document

## Overview
Add dynamic EventBridge Scheduler management to rez_agent, allowing schedules to be created via SNS events and REST API.

## Requirements

### Functional Requirements
1. **Event-Driven Schedule Creation**: Scheduler Lambda receives SNS events to create EventBridge Schedules
2. **REST API Endpoint**: POST /api/schedules creates new schedules
3. **Schedule Persistence**: Store schedule metadata in DynamoDB
4. **Flexible Scheduling**: Support cron and rate expressions with timezone awareness
5. **Action Routing**: Schedules can trigger different action types (web actions, notifications, custom)

### Non-Functional Requirements
- Low latency (<500ms for API endpoint)
- Idempotent schedule creation
- Proper error handling and logging
- IAM least privilege principle
- Schedule validation before creation

## Architecture

### Components

#### 1. DynamoDB Table: `rez-agent-schedules-{stage}`
Stores schedule metadata and configuration.

**Schema**:
```go
type Schedule struct {
    ID                string    // Primary key: unique schedule ID
    Name              string    // Human-readable name
    ScheduleExpression string  // Cron or rate expression
    Timezone          string    // IANA timezone (default: UTC)
    TargetType        string    // Type of action (web_action, notification, custom)
    TargetTopicArn    string    // SNS topic to publish to
    Payload           map[string]interface{} // Action-specific payload
    CreatedBy         string    // User/system that created schedule
    CreatedDate       time.Time
    UpdatedDate       time.Time
    Status            string    // active, paused, deleted
    EventBridgeArn    string    // ARN of created EventBridge Schedule
    LastTriggered     time.Time // Optional: last execution time
}
```

**Indexes**:
- GSI: `status-created_date-index` - Query active schedules
- GSI: `created_by-index` - Query schedules by creator

#### 2. SNS Topic: `rez-agent-schedule-creation-{stage}`
Receives schedule creation requests.

**Message Format**:
```json
{
  "action": "create",
  "schedule": {
    "name": "daily-tee-time-check",
    "schedule_expression": "cron(0 6 * * ? *)",
    "timezone": "America/New_York",
    "target_type": "web_action",
    "payload": {
      "action": "check_tee_times",
      "course_id": "pebble-beach",
      "date_offset_days": 1,
      "preferred_time": "09:00",
      "auto_book": true
    }
  }
}
```

#### 3. Enhanced Scheduler Lambda
**New Functionality**:
- Subscribe to schedule creation SNS topic
- Create EventBridge Schedules via AWS SDK
- Save schedule metadata to DynamoDB
- Validate schedule expressions
- Handle errors and send notifications

**Handler Types**:
1. **EventBridge Trigger** (existing): Execute scheduled actions
2. **SNS Trigger** (new): Create/update/delete schedules

#### 4. Web API Endpoint: POST /api/schedules
**Request**:
```json
POST /api/schedules
Content-Type: application/json

{
  "name": "daily-tee-time-check",
  "schedule_expression": "cron(0 6 * * ? *)",
  "timezone": "America/New_York",
  "target_type": "web_action",
  "payload": {
    "action": "check_tee_times",
    "course_id": "pebble-beach",
    "date_offset_days": 1,
    "preferred_time": "09:00",
    "auto_book": true
  }
}
```

**Response (Success)**:
```json
{
  "schedule_id": "sched_abc123",
  "name": "daily-tee-time-check",
  "status": "active",
  "eventbridge_arn": "arn:aws:scheduler:us-east-1:...",
  "next_execution": "2025-11-07T06:00:00Z",
  "created_at": "2025-11-06T20:00:00Z"
}
```

**Response (Error)**:
```json
{
  "error": "invalid_schedule_expression",
  "message": "Cron expression is invalid",
  "details": "Expression must have 6 fields for EventBridge Scheduler"
}
```

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| /api/schedules | POST | Create new schedule |
| /api/schedules | GET | List all schedules |
| /api/schedules/{id} | GET | Get schedule details |
| /api/schedules/{id} | PUT | Update schedule |
| /api/schedules/{id} | DELETE | Delete schedule |
| /api/schedules/{id}/pause | POST | Pause schedule |
| /api/schedules/{id}/resume | POST | Resume schedule |

## Data Flow

### Schedule Creation Flow

```
User/System
    |
    | POST /api/schedules
    v
Web API Lambda
    |
    | Validate request
    | Generate schedule ID
    | Publish to SNS
    v
schedule-creation-topic
    |
    | SNS event
    v
Scheduler Lambda
    |
    | Validate schedule expression
    | Create EventBridge Schedule
    | Save to DynamoDB
    | Return result
    v
User receives response
```

### Schedule Execution Flow

```
EventBridge Scheduler
    |
    | Cron/Rate trigger
    v
Scheduler Lambda
    |
    | Load schedule from DynamoDB
    | Build action payload
    | Publish to target SNS topic
    v
Web Actions / Notifications Topic
    |
    v
Downstream processors
```

## Example Use Cases

### Use Case 1: Daily Tee Time Check
**Goal**: Check for available tee times every day at 1 AM EST and book if found.

**Schedule Creation**:
```json
{
  "name": "daily-pebble-beach-tee-time",
  "schedule_expression": "cron(0 6 * * ? *)",
  "timezone": "America/New_York",
  "target_type": "web_action",
  "payload": {
    "action": "check_and_book_tee_times",
    "course_id": "pebble-beach",
    "date_offset_days": 1,
    "preferred_times": ["09:00", "10:00", "11:00"],
    "party_size": 4,
    "auto_book": true,
    "notify_on_success": true
  }
}
```

**Execution**: At 1 AM EST (6 AM UTC), EventBridge triggers the schedule, which publishes the payload to the web-actions topic. The web action processor checks tee times and books if available.

### Use Case 2: Weekly Reservation Summary
**Goal**: Send a notification every Sunday at 8 PM with upcoming reservations.

**Schedule Creation**:
```json
{
  "name": "weekly-reservation-summary",
  "schedule_expression": "cron(0 0 * * SUN *)",
  "timezone": "America/Los_Angeles",
  "target_type": "notification",
  "payload": {
    "action": "send_reservation_summary",
    "lookback_days": 7,
    "lookahead_days": 14,
    "notification_channel": "ntfy"
  }
}
```

### Use Case 3: One-Time Booking Reminder
**Goal**: Send a reminder 24 hours before a specific tee time.

**Schedule Creation**:
```json
{
  "name": "reminder-pebble-beach-2025-11-15",
  "schedule_expression": "at(2025-11-14T09:00:00)",
  "timezone": "America/Los_Angeles",
  "target_type": "notification",
  "payload": {
    "action": "send_reminder",
    "reservation_id": "res_xyz789",
    "message": "Reminder: Tee time at Pebble Beach tomorrow at 9 AM"
  }
}
```

## Implementation Plan

### Phase 1: Infrastructure & Data Models
1. ✅ Create DynamoDB table for schedules
2. ✅ Create SNS topic for schedule creation
3. ✅ Define Go structs for schedule models
4. ✅ Update IAM roles with EventBridge Scheduler permissions

### Phase 2: Scheduler Lambda Enhancement
1. Add SNS event handler for schedule creation
2. Implement EventBridge Scheduler SDK integration
3. Add schedule validation logic
4. Implement DynamoDB operations for schedules
5. Add error handling and logging

### Phase 3: Web API Implementation
1. Add POST /api/schedules endpoint
2. Implement request validation
3. Add SNS publishing logic
4. Implement additional CRUD endpoints
5. Add API documentation

### Phase 4: Testing & Documentation
1. Unit tests for schedule validation
2. Integration tests for schedule creation
3. E2E tests for full workflow
4. API documentation
5. User guides

## Security Considerations

### IAM Permissions
**Scheduler Lambda Role**:
- `scheduler:CreateSchedule`
- `scheduler:GetSchedule`
- `scheduler:UpdateSchedule`
- `scheduler:DeleteSchedule`
- `scheduler:ListSchedules`
- `iam:PassRole` (for EventBridge execution role)
- `sns:Publish` (to target topics)
- `dynamodb:PutItem`, `GetItem`, `Query`, `UpdateItem`

**Web API Lambda Role**:
- `sns:Publish` (to schedule-creation topic)
- `dynamodb:Query` (to list schedules)

### Validation
- Schedule expression syntax validation
- Timezone validation (IANA database)
- Payload size limits (<256 KB)
- Rate limiting on API endpoint
- Schedule name uniqueness

### Data Protection
- Encrypt sensitive data in payload
- Audit log for schedule creation/deletion
- TTL on old schedules (90 days after deletion)

## Monitoring & Observability

### CloudWatch Metrics
- Schedule creation success/failure rate
- Schedule execution count per schedule
- API endpoint latency
- Error rates by error type

### CloudWatch Alarms
- Alert on schedule creation failures
- Alert on high API error rate
- Alert on EventBridge Scheduler quota limits

### Logs
- Structured JSON logging
- Include schedule ID in all log entries
- Log schedule creation parameters
- Log execution results

## Cost Estimation

**AWS EventBridge Scheduler**: $1.00 per million invocations
- Example: 10 schedules running daily = 3,650 invocations/year = $0.004/year

**DynamoDB**: Pay-per-request
- Example: 10 schedules with 100 reads/day = $0.025/month

**SNS**: $0.50 per million publishes
- Negligible cost for schedule creation events

**Lambda**: Pay-per-invocation + duration
- Negligible incremental cost

**Total Estimated Cost**: <$5/month for moderate usage

## Rollout Strategy

1. **Phase 1**: Deploy infrastructure (DynamoDB table, SNS topic)
2. **Phase 2**: Deploy scheduler Lambda enhancements
3. **Phase 3**: Deploy web API endpoint
4. **Phase 4**: Gradual rollout with feature flag
5. **Phase 5**: Monitor and iterate

## Future Enhancements

1. **Schedule Templates**: Pre-defined schedule patterns
2. **Schedule Groups**: Organize related schedules
3. **Conditional Execution**: Only execute if conditions met
4. **Retry Logic**: Configurable retry policies
5. **Schedule History**: Track execution history
6. **UI Dashboard**: Visual schedule management
7. **Schedule Sharing**: Share schedules between users
8. **Schedule Marketplace**: Community-contributed schedules

## References

- [AWS EventBridge Scheduler Documentation](https://docs.aws.amazon.com/scheduler/latest/UserGuide/what-is-scheduler.html)
- [EventBridge Scheduler API Reference](https://docs.aws.amazon.com/scheduler/latest/APIReference/Welcome.html)
- [Cron Expression Reference](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-create-rule-schedule.html)
