# Topic-Based Routing Migration Guide

## Overview

This document describes the migration from SNS filter-based message routing to **dedicated SNS topics** for different message types. This architectural change eliminates the complexity of SQS filter criteria and provides better separation of concerns.

## Why This Change?

### Previous Architecture (Filter-Based)
```
┌─────────────┐
│  Scheduler  │
│   Lambda    │──┐
└─────────────┘  │
                 │
┌─────────────┐  │     ┌──────────────────┐     ┌────────────────┐
│   WebAPI    │──┼────→│  messages-topic  │────→│ messages-queue │
│   Lambda    │  │     │      (SNS)       │     │     (SQS)      │
└─────────────┘  │     └──────────────────┘     └────────────────┘
                 │                                        │
                 │                                        ├──[Filter: message_type != "web_action"]──→ Processor Lambda
                 │                                        │
                 │                                        └──[Filter: message_type == "web_action"]──→ WebAction Lambda
```

**Problems:**
- Complex filter criteria on SQS event source mappings
- Single point of failure (one queue for all message types)
- Difficult to debug message routing
- Limited flexibility for future message types

### New Architecture (Topic-Based)
```
┌─────────────┐
│  Scheduler  │────→│ notifications-topic │────→│ notifications-queue │────→ Processor Lambda
│   Lambda    │     │        (SNS)        │     │        (SQS)        │
└─────────────┘     └─────────────────────┘     └─────────────────────┘

┌─────────────┐
│   WebAPI    │────→│ web-actions-topic   │────→│ web-actions-queue   │────→ WebAction Lambda
│   Lambda    │     │        (SNS)        │     │        (SQS)        │
└─────────────┘     └─────────────────────┘     └─────────────────────┘
```

**Benefits:**
- ✅ No filter criteria needed (simpler architecture)
- ✅ Clear separation of message types
- ✅ Each queue serves a single purpose
- ✅ Easier to debug and monitor
- ✅ Better scalability for future message types
- ✅ Independent DLQs per message type

---

## Architecture Changes

### 1. SNS Topics

| Topic Name | Purpose | Subscribers |
|------------|---------|-------------|
| `rez-agent-web-actions-{stage}` | Web action requests (HTTP REST API calls) | WebAction Lambda |
| `rez-agent-notifications-{stage}` | Scheduled tasks, manual messages, hello_world | Processor Lambda |
| `rez-agent-messages-{stage}` | Legacy topic (kept for backward compatibility) | (None - deprecated) |

### 2. SQS Queues

| Queue Name | Source Topic | Consumer | DLQ |
|------------|--------------|----------|-----|
| `rez-agent-web-actions-{stage}` | web-actions-topic | webaction Lambda | web-actions-dlq |
| `rez-agent-notifications-{stage}` | notifications-topic | processor Lambda | notifications-dlq |
| `rez-agent-messages-{stage}` | messages-topic (legacy) | (None) | messages-dlq |

### 3. Lambda Event Source Mappings

**Before:**
```go
// Processor Lambda - with filter
EventSourceArn: messagesQueue.Arn
FilterCriteria: {"body": {"message_type": [{"anything-but": ["web_action"]}]}}

// WebAction Lambda - with filter
EventSourceArn: messagesQueue.Arn
FilterCriteria: {"body": {"message_type": ["web_action"]}}
```

**After:**
```go
// Processor Lambda - no filter needed
EventSourceArn: notificationsQueue.Arn
// No FilterCriteria

// WebAction Lambda - no filter needed
EventSourceArn: webActionsQueue.Arn
// No FilterCriteria
```

---

## Code Changes

### 1. Infrastructure (`infrastructure/main.go`)

**Created:**
- New SNS topics: `webActionsTopic`, `notificationsTopic`
- New SQS queues: `webActionsQueue`, `notificationsQueue`
- New DLQs: `webActionsDlq`, `notificationsDlq`
- Topic-to-queue subscriptions for each pair
- Queue policies to allow SNS publishing

**Updated:**
- Scheduler Lambda IAM policy: now publishes to `notificationsTopic`
- Scheduler Lambda environment: `SNS_TOPIC_ARN` → notifications topic
- Processor Lambda IAM policy: now consumes from `notificationsQueue`
- Processor Lambda event source: connected to `notificationsQueue`
- WebAction Lambda IAM policy: now consumes from `webActionsQueue`
- WebAction Lambda event source: connected to `webActionsQueue`
- WebAPI Lambda IAM policy: can publish to both topics

**Stack Outputs Added:**
```yaml
webActionsTopicArn
notificationsTopicArn
webActionsQueueUrl
webActionsQueueArn
notificationsQueueUrl
notificationsQueueArn
webActionsDlqUrl
webActionsDlqArn
notificationsDlqUrl
notificationsDlqArn
```

### 2. Messaging Package (`internal/messaging/sns.go`)

**Added:**
- `TopicRoutingSNSClient` - routes messages to topics based on message type
- `NewTopicRoutingSNSClient()` - constructor for topic-routing client
- `getTopicForMessageType()` - determines which topic to use

**Routing Logic:**
```go
switch messageType {
case MessageTypeWebAction:
    return webActionsTopicArn
default:
    // scheduled, manual, hello_world → notifications
    return notificationsTopicArn
}
```

**Backward Compatibility:**
- `SNSClient` and `NewSNSClient()` remain unchanged
- Existing code continues to work with single topics

---

## Deployment Steps

### Step 1: Build Infrastructure

```bash
cd infrastructure
go build .
```

### Step 2: Preview Changes

```bash
pulumi stack select dev
pulumi preview
```

**Expected Changes:**
```diff
+ Creating (create)
  + aws:sns:Topic              rez-agent-web-actions-dev
  + aws:sns:Topic              rez-agent-notifications-dev
  + aws:sqs:Queue              rez-agent-web-actions-dev
  + aws:sqs:Queue              rez-agent-notifications-dev
  + aws:sqs:Queue              rez-agent-web-actions-dlq-dev
  + aws:sqs:Queue              rez-agent-notifications-dlq-dev
  + aws:sns:TopicSubscription  rez-agent-web-actions-subscription-dev
  + aws:sns:TopicSubscription  rez-agent-notifications-subscription-dev
  + aws:sqs:QueuePolicy        rez-agent-web-actions-queue-policy-dev
  + aws:sqs:QueuePolicy        rez-agent-notifications-queue-policy-dev

~ Updating (update)
  ~ aws:iam:RolePolicy         rez-agent-scheduler-policy-dev (topic ARN changed)
  ~ aws:iam:RolePolicy         rez-agent-processor-policy-dev (queue ARN changed)
  ~ aws:iam:RolePolicy         rez-agent-webaction-policy-dev (queue/topic ARNs changed)
  ~ aws:iam:RolePolicy         rez-agent-webapi-policy-dev (topic ARNs changed)
  ~ aws:lambda:Function        rez-agent-scheduler-dev (env vars changed)
  ~ aws:lambda:EventSourceMapping  rez-agent-processor-sqs-trigger-dev (queue + no filter)
  ~ aws:lambda:EventSourceMapping  rez-agent-webaction-sqs-trigger-dev (queue + no filter)
```

### Step 3: Deploy Infrastructure

```bash
pulumi up
```

Review the plan and type "yes" to confirm.

### Step 4: Verify Deployment

```bash
# Check new topics exist
aws sns list-topics | grep -E '(web-actions|notifications)'

# Check new queues exist
aws sqs list-queues | grep -E '(web-actions|notifications)'

# Verify Lambda event source mappings
aws lambda list-event-source-mappings --function-name rez-agent-processor-dev
aws lambda list-event-source-mappings --function-name rez-agent-webaction-dev
```

**Expected Output:**
- Each Lambda should have ONE event source mapping (no filters)
- Processor Lambda → notifications queue
- WebAction Lambda → web-actions queue

### Step 5: Test Message Flow

#### Test Notifications (Scheduler → Processor)

```bash
# Trigger scheduler manually (via EventBridge)
aws events put-events --entries '[
  {
    "Source": "manual.test",
    "DetailType": "Scheduled Event",
    "Detail": "{}"
  }
]'

# Monitor processor logs
aws logs tail /aws/lambda/rez-agent-processor-dev --follow
```

**Expected Log Output:**
```
Processing message from notifications queue
message_type=scheduled
Successfully processed notification
```

#### Test Web Actions

```bash
# Send test web action to notifications topic
WEB_ACTIONS_TOPIC=$(pulumi stack output webActionsTopicArn)

aws sns publish \
  --topic-arn "$WEB_ACTIONS_TOPIC" \
  --message '{"message_type":"web_action","action":"golf","operation":"fetch_reservations"}'

# Monitor webaction logs
aws logs tail /aws/lambda/rez-agent-webaction-dev --follow
```

**Expected Log Output:**
```
Processing web action message
action=golf operation=fetch_reservations
Successfully processed web action
```

---

## Application Code Migration (Optional)

If you want to use the new `TopicRoutingSNSClient` in your application code:

### Before (Single Topic)
```go
publisher := messaging.NewSNSClient(snsClient, cfg.SNSTopicArn, logger)
```

### After (Topic Routing)
```go
publisher := messaging.NewTopicRoutingSNSClient(
    snsClient,
    cfg.WebActionsTopicArn,
    cfg.NotificationsTopicArn,
    logger,
)
```

**When to Use:**
- **Single Topic**: When Lambda only publishes one message type (e.g., scheduler → notifications)
- **Topic Routing**: When Lambda publishes multiple message types (e.g., webapi → both types)

### Example: WebAPI Lambda

**Update `cmd/webapi/main.go`:**

```go
// Load configuration
type Config struct {
    // ... existing fields
    WebActionsTopicArn    string
    NotificationsTopicArn string
}

func loadConfig() (*Config, error) {
    return &Config{
        // ... existing fields
        WebActionsTopicArn:    os.Getenv("WEB_ACTIONS_TOPIC_ARN"),
        NotificationsTopicArn: os.Getenv("NOTIFICATIONS_TOPIC_ARN"),
    }, nil
}

// Initialize publisher
publisher := messaging.NewTopicRoutingSNSClient(
    snsClient,
    cfg.WebActionsTopicArn,
    cfg.NotificationsTopicArn,
    logger,
)
```

**Update infrastructure environment variables:**
```go
Environment: &lambda.FunctionEnvironmentArgs{
    Variables: pulumi.StringMap{
        "WEB_ACTIONS_TOPIC_ARN":    webActionsTopic.Arn,
        "NOTIFICATIONS_TOPIC_ARN":  notificationsTopic.Arn,
        // ... other vars
    },
}
```

---

## Monitoring and Observability

### CloudWatch Metrics

Monitor queue depths separately:
```bash
# Web actions queue depth
aws cloudwatch get-metric-statistics \
  --namespace AWS/SQS \
  --metric-name ApproximateNumberOfMessagesVisible \
  --dimensions Name=QueueName,Value=rez-agent-web-actions-dev \
  --start-time 2025-10-28T00:00:00Z \
  --end-time 2025-10-28T23:59:59Z \
  --period 300 \
  --statistics Average

# Notifications queue depth
aws cloudwatch get-metric-statistics \
  --namespace AWS/SQS \
  --metric-name ApproximateNumberOfMessagesVisible \
  --dimensions Name=QueueName,Value=rez-agent-notifications-dev \
  --start-time 2025-10-28T00:00:00Z \
  --end-time 2025-10-28T23:59:59Z \
  --period 300 \
  --statistics Average
```

### Dead Letter Queues

Check DLQs for failed messages:
```bash
# Web actions DLQ
aws sqs receive-message \
  --queue-url $(pulumi stack output webActionsDlqUrl) \
  --max-number-of-messages 10

# Notifications DLQ
aws sqs receive-message \
  --queue-url $(pulumi stack output notificationsDlqUrl) \
  --max-number-of-messages 10
```

---

## Rollback Plan

If issues arise, you can rollback using Pulumi:

```bash
# View stack history
pulumi stack history

# Export previous version
pulumi stack export --version <PREVIOUS_VERSION> > rollback.json

# Import and deploy
pulumi stack import --file rollback.json
pulumi up
```

**Note:** Rollback will restore filter-based routing. Ensure no messages are in-flight during rollback.

---

## Cost Impact

**New Resources:**
- 2 additional SNS topics: ~$0 (first 1M requests free)
- 2 additional SQS queues: ~$0 (first 1M requests free)
- 2 additional DLQs: ~$0 (minimal usage expected)

**Expected Additional Cost:** < $1/month

**Cost Optimization:**
- Messages are still processed with the same Lambda invocations
- No additional compute costs
- Minimal increase in AWS service costs

---

## Troubleshooting

### Messages Not Reaching Lambda

**Check 1: Verify Queue Subscriptions**
```bash
aws sns list-subscriptions-by-topic --topic-arn $(pulumi stack output webActionsTopicArn)
```

Expected: Subscription to `webActionsQueue` with `RawMessageDelivery: true`

**Check 2: Verify Lambda Event Source**
```bash
aws lambda get-event-source-mapping --uuid <MAPPING_UUID>
```

Expected: `State: Enabled`, no `FilterCriteria`

**Check 3: Check Queue Permissions**
```bash
aws sqs get-queue-attributes \
  --queue-url $(pulumi stack output webActionsQueueUrl) \
  --attribute-names Policy
```

Expected: Policy allows SNS to send messages

### Messages Stuck in Queue

**Check Lambda Errors:**
```bash
aws logs filter-log-events \
  --log-group-name /aws/lambda/rez-agent-webaction-dev \
  --filter-pattern "ERROR"
```

**Check Lambda Concurrency:**
```bash
aws lambda get-function-concurrency --function-name rez-agent-webaction-dev
```

---

## Future Enhancements

### Adding New Message Types

To add a new message type (e.g., `email_notification`):

1. **Add to models** (`internal/models/message.go`):
```go
const (
    // ... existing types
    MessageTypeEmailNotification MessageType = "email_notification"
)
```

2. **Create new topic/queue** (`infrastructure/main.go`):
```go
emailTopic, err := sns.NewTopic(ctx, fmt.Sprintf("rez-agent-emails-%s", stage), ...)
emailQueue, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-emails-%s", stage), ...)
```

3. **Update routing** (`internal/messaging/sns.go`):
```go
case models.MessageTypeEmailNotification:
    return s.emailTopicArn
```

4. **Create new Lambda** with event source mapping to email queue

---

## Summary

This migration provides a cleaner, more scalable messaging architecture by:

✅ Eliminating complex SQS filter criteria
✅ Providing dedicated topics and queues per message type
✅ Improving observability and debugging
✅ Maintaining backward compatibility
✅ Enabling easier addition of new message types

**Status**: ✅ Ready for Deployment
**Last Updated**: 2025-10-28
**Version**: 1.0
