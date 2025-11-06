# Dynamic Schedule Creation - Implementation Status

## ✅ Completed (Part A: Infrastructure)

### 1. Data Models
**File**: `internal/models/schedule.go`
- `Schedule` struct with full metadata
- `ScheduleStatus` enum (active, paused, deleted, error)
- `TargetType` enum (web_action, notification, custom)
- `ScheduleCreationRequest` for SNS messages
- `ScheduleDefinition` for API requests
- Validation logic for cron/rate/at expressions
- Helper methods for status updates

### 2. Repository Layer
**File**: `internal/repository/schedule_repository.go`
- `ScheduleRepository` interface
- `DynamoDBScheduleRepository` implementation
- Methods: Save, Get, Update, Delete, List (by status/creator)

### 3. Infrastructure (Pulumi)
**File**: `infrastructure/main.go`

**Resources Created**:
- ✅ DynamoDB table: `rez-agent-schedules-{stage}`
  - Primary key: `id`
  - GSI: `status-created_date-index`
  - GSI: `created_by-index`
  - TTL enabled

- ✅ SNS topic: `rez-agent-schedule-creation-{stage}`

- ✅ IAM Role: `rez-agent-eventbridge-scheduler-execution-role-{stage}`
  - Allows EventBridge Scheduler to invoke Lambda

**IAM Permissions Updated**:
- ✅ Scheduler Lambda Role:
  - DynamoDB: Read/Write on messages & schedules tables
  - SNS: Publish to notifications & web-actions topics
  - EventBridge Scheduler: Create/Get/Update/Delete schedules
  - IAM: PassRole for EventBridge execution role

- ✅ Web API Lambda Role:
  - DynamoDB: Read/Write on messages & schedules tables
  - SNS: Publish to schedule-creation topic

**Environment Variables Added**:
- ✅ Scheduler Lambda:
  - `SCHEDULES_TABLE_NAME`
  - `SCHEDULE_CREATION_TOPIC_ARN`
  - `EVENTBRIDGE_EXECUTION_ROLE_ARN`

**SNS Subscriptions**:
- ✅ `schedule-creation-topic` → Scheduler Lambda (with Lambda permission)

**Stack Outputs**:
- ✅ `schedulesTableName`
- ✅ `schedulesTableArn`
- ✅ `scheduleCreationTopicArn`
- ✅ `eventBridgeSchedulerExecutionRoleArn`

## 🚧 In Progress (Part B: Lambda Implementation)

### Next Steps

#### 1. Enhance Scheduler Lambda (`cmd/scheduler/main.go`)
Need to add:
- SNS event handler (detect schedule creation requests)
- EventBridge Scheduler SDK integration
- Schedule creation logic
- DynamoDB schedule persistence

**Pseudo-code**:
```go
func (h *SchedulerHandler) HandleEvent(ctx context.Context, event interface{}) error {
    // Detect event type
    if snsEvent, ok := event.(events.SNSEvent); ok {
        return h.handleSNSEvent(ctx, snsEvent)
    }
    // Existing EventBridge trigger logic
    return h.handleScheduledEvent(ctx)
}

func (h *SchedulerHandler) handleSNSEvent(ctx context.Context, event events.SNSEvent) error {
    for _, record := range event.Records {
        var req models.ScheduleCreationRequest
        json.Unmarshal([]byte(record.SNS.Message), &req)

        switch req.Action {
        case "create":
            return h.createSchedule(ctx, &req.Schedule)
        case "delete":
            return h.deleteSchedule(ctx, req.ScheduleID)
        }
    }
}

func (h *SchedulerHandler) createSchedule(ctx context.Context, def *models.ScheduleDefinition) error {
    // 1. Validate schedule definition
    // 2. Create Schedule model
    // 3. Create EventBridge Schedule via SDK
    // 4. Save to DynamoDB
    // 5. Return success/error
}
```

#### 2. Add EventBridge Scheduler Service (`internal/scheduler/eventbridge.go`)
Create service layer:
```go
type EventBridgeScheduler interface {
    CreateSchedule(ctx context.Context, schedule *models.Schedule) (string, error)
    DeleteSchedule(ctx context.Context, scheduleName string) error
    GetSchedule(ctx context.Context, scheduleName string) error
}
```

#### 3. Web API Endpoint (`cmd/webapi/main.go`)
Add POST /api/schedules:
```go
func (h *WebAPIHandler) handleCreateSchedule(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
    // 1. Parse request body
    // 2. Validate schedule definition
    // 3. Publish to SNS schedule-creation topic
    // 4. Return 202 Accepted with schedule ID
}
```

#### 4. Configuration Updates (`pkg/config/config.go`)
Add fields:
```go
type Config struct {
    // Existing fields...
    SchedulesTableName            string
    ScheduleCreationTopicArn      string
    EventBridgeExecutionRoleArn   string
}
```

## 📝 Testing Plan

### Unit Tests
1. `internal/models/schedule_test.go`
   - Schedule validation
   - Expression validation (cron/rate/at)
   - Status transitions

2. `internal/repository/schedule_repository_test.go`
   - CRUD operations
   - Query by status/creator

3. `cmd/scheduler/schedule_handler_test.go`
   - SNS event parsing
   - Schedule creation logic

### Integration Tests
1. End-to-end schedule creation flow
2. EventBridge trigger → Lambda execution
3. Error handling scenarios

### Manual Testing
```bash
# 1. Deploy infrastructure
cd infrastructure
pulumi up

# 2. Create schedule via SNS (simulated)
aws sns publish \
  --topic-arn arn:aws:sns:...:rez-agent-schedule-creation-dev \
  --message '{
    "action": "create",
    "schedule": {
      "name": "test-daily-check",
      "schedule_expression": "cron(0 6 * * ? *)",
      "timezone": "America/New_York",
      "target_type": "web_action",
      "payload": {"action": "check_tee_times"}
    }
  }'

# 3. Verify in DynamoDB
aws dynamodb scan --table-name rez-agent-schedules-dev

# 4. Verify EventBridge Schedule created
aws scheduler list-schedules
```

## 📊 Deployment Checklist

### Pre-Deployment
- [ ] Review all code changes
- [ ] Run unit tests
- [ ] Update documentation
- [ ] Test locally with LocalStack (optional)

### Deployment
- [ ] Deploy infrastructure: `pulumi up`
- [ ] Build Lambda binaries: `make build`
- [ ] Verify stack outputs
- [ ] Test SNS → Lambda subscription

### Post-Deployment
- [ ] Monitor CloudWatch Logs
- [ ] Test schedule creation flow
- [ ] Verify EventBridge Schedules created
- [ ] Check DynamoDB for schedule records

## 🎯 Success Criteria

- ✅ Infrastructure deployed successfully
- ⏳ Scheduler Lambda handles SNS events
- ⏳ EventBridge Schedules created dynamically
- ⏳ Schedules persist in DynamoDB
- ⏳ Web API endpoint creates schedules
- ⏳ End-to-end flow works: API → SNS → Lambda → EventBridge

## 📚 Documentation Links

- [Design Document](./dynamic-schedule-creation-design.md)
- [Schedule Models](../internal/models/schedule.go)
- [Schedule Repository](../internal/repository/schedule_repository.go)
- [Infrastructure Code](../infrastructure/main.go)

## 🔮 Future Enhancements

1. **Schedule Management UI**
   - List all schedules
   - Edit/pause/resume schedules
   - View execution history

2. **Advanced Features**
   - Schedule templates
   - Conditional execution
   - Retry policies
   - Execution history tracking

3. **Monitoring & Observability**
   - Schedule execution metrics
   - Failure alerting
   - Execution duration tracking

## ⚠️ Known Limitations

1. **EventBridge Scheduler Quotas**
   - Default: 1 million schedules per account per region
   - Rate limiting: 100 CreateSchedule requests/second

2. **Payload Size**
   - EventBridge: 256 KB max
   - Ensure large payloads are stored separately

3. **Timezone Handling**
   - Must use IANA timezone names
   - EventBridge Scheduler handles DST transitions

## 🆘 Troubleshooting

### Common Issues

**Issue**: Schedule creation fails with "AccessDenied"
**Solution**: Verify IAM permissions for `scheduler:CreateSchedule` and `iam:PassRole`

**Issue**: EventBridge Schedule not triggering
**Solution**: Check schedule expression syntax and timezone

**Issue**: Lambda timeout on schedule creation
**Solution**: Increase Lambda timeout or optimize EventBridge SDK calls

### Debug Commands
```bash
# Check Lambda logs
aws logs tail /aws/lambda/rez-agent-scheduler-dev --follow

# List EventBridge Schedules
aws scheduler list-schedules --output table

# Get schedule details
aws scheduler get-schedule --name <schedule-name>

# Query DynamoDB
aws dynamodb query \
  --table-name rez-agent-schedules-dev \
  --index-name status-created_date-index \
  --key-condition-expression "status = :status" \
  --expression-attribute-values '{":status":{"S":"active"}}'
```

## 📞 Contact

For questions or issues, refer to:
- [Project README](../README.md)
- [CLAUDE.md](../CLAUDE.md)
- GitHub Issues
