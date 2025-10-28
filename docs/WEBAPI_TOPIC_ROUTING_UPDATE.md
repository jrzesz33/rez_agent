# WebAPI Lambda Topic Routing Update

## Summary

Updated the WebAPI Lambda to use **topic-based routing** with `TopicRoutingSNSClient` instead of publishing to a single legacy topic.

---

## Changes Made

### 1. Config Package (`pkg/config/config.go`)

**Added New Fields:**
```go
// SNS Configuration
SNSTopicArn              string // Legacy topic (for backward compatibility)
WebActionsSNSTopicArn    string // Topic for web action messages
NotificationsSNSTopicArn string // Topic for notification messages
```

**Added Environment Variable Reading:**
```go
// Topic-based routing (for webapi Lambda)
webActionsSNSTopicArn := os.Getenv("WEB_ACTIONS_TOPIC_ARN")
notificationsSNSTopicArn := os.Getenv("NOTIFICATIONS_TOPIC_ARN")
```

**Updated Struct Initialization:**
```go
return &Config{
    SNSTopicArn:              snsTopicArn,
    WebActionsSNSTopicArn:    webActionsSNSTopicArn,
    NotificationsSNSTopicArn: notificationsSNSTopicArn,
    // ...
}
```

### 2. WebAPI Lambda Code (`cmd/webapi/main.go`)

**Replaced Single-Topic Publisher with Topic Routing:**

**Before:**
```go
publisher := messaging.NewSNSClient(snsClient, cfg.SNSTopicArn, logger)
```

**After:**
```go
// Use topic routing if both topics are configured, otherwise fall back to legacy
var publisher messaging.SNSPublisher
if cfg.WebActionsSNSTopicArn != "" && cfg.NotificationsSNSTopicArn != "" {
    publisher = messaging.NewTopicRoutingSNSClient(
        snsClient,
        cfg.WebActionsSNSTopicArn,
        cfg.NotificationsSNSTopicArn,
        logger,
    )
    logger.Info("using topic-routing SNS client",
        slog.String("web_actions_topic", cfg.WebActionsSNSTopicArn),
        slog.String("notifications_topic", cfg.NotificationsSNSTopicArn),
    )
} else {
    publisher = messaging.NewSNSClient(snsClient, cfg.SNSTopicArn, logger)
    logger.Info("using legacy SNS client",
        slog.String("topic", cfg.SNSTopicArn),
    )
}
```

**Benefits:**
- Automatically routes messages based on `message_type`
- Falls back to legacy topic if new topics not configured (backward compatibility)
- Logs which routing mode is active for debugging

### 3. Infrastructure (`infrastructure/main.go`)

**Added Environment Variables to WebAPI Lambda:**

**Lines 671-680:**
```go
Environment: &lambda.FunctionEnvironmentArgs{
    Variables: pulumi.StringMap{
        "DYNAMODB_TABLE_NAME":      messagesTable.Name,
        "SNS_TOPIC_ARN":            messagesTopic.Arn,        // Legacy (backward compat)
        "WEB_ACTIONS_TOPIC_ARN":    webActionsTopic.Arn,     // New: web actions
        "NOTIFICATIONS_TOPIC_ARN":  notificationsTopic.Arn,  // New: notifications
        "SQS_QUEUE_URL":            messagesQueue.Url,
        "STAGE":                    pulumi.String(stage),
    },
},
```

---

## Message Routing Logic

The `TopicRoutingSNSClient` automatically routes messages to the correct topic:

```go
switch messageType {
case MessageTypeWebAction:
    → web-actions-topic → web-actions-queue → webaction Lambda

default: // scheduled, manual, hello_world
    → notifications-topic → notifications-queue → processor Lambda
}
```

---

## Deployment Impact

### Environment Variables Added

| Lambda | New Variables |
|--------|--------------|
| **webapi** | `WEB_ACTIONS_TOPIC_ARN`, `NOTIFICATIONS_TOPIC_ARN` |

### Expected Changes

```diff
~ aws:lambda:Function  rez-agent-webapi-dev
  ~ Environment:
    ~ Variables: {
        "DYNAMODB_TABLE_NAME": "rez-agent-messages-dev"
        "SNS_TOPIC_ARN": "arn:aws:sns:us-east-1:...:rez-agent-messages-dev"
+       "WEB_ACTIONS_TOPIC_ARN": "arn:aws:sns:us-east-1:...:rez-agent-web-actions-dev"
+       "NOTIFICATIONS_TOPIC_ARN": "arn:aws:sns:us-east-1:...:rez-agent-notifications-dev"
        "SQS_QUEUE_URL": "https://sqs.us-east-1.amazonaws.com/.../rez-agent-messages-dev"
        "STAGE": "dev"
      }
```

---

## Backward Compatibility

✅ **Fully Backward Compatible**

The code gracefully falls back to legacy mode if new topics aren't configured:

```go
if cfg.WebActionsSNSTopicArn != "" && cfg.NotificationsSNSTopicArn != "" {
    // Use topic routing
} else {
    // Use legacy single topic
}
```

This means:
- Old deployments continue to work
- Gradual migration is supported
- No breaking changes

---

## Testing

### Test Topic Routing

After deployment, publish a test message via WebAPI:

```bash
# Test web action message
curl -X POST https://$(pulumi stack output webapiUrl)/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "web_action",
    "payload": "{\"action\":\"golf\",\"operation\":\"fetch_reservations\"}"
  }'

# Test notification message
curl -X POST https://$(pulumi stack output webapiUrl)/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "manual",
    "payload": "test notification"
  }'
```

### Verify Routing

Check CloudWatch logs for webapi Lambda:

```bash
aws logs tail /aws/lambda/rez-agent-webapi-dev --follow
```

**Expected Log Output:**
```
using topic-routing SNS client
  web_actions_topic=arn:aws:sns:us-east-1:...:rez-agent-web-actions-dev
  notifications_topic=arn:aws:sns:us-east-1:...:rez-agent-notifications-dev

message published to topic-routed SNS
  message_type=web_action
  topic_arn=arn:aws:sns:us-east-1:...:rez-agent-web-actions-dev

message published to topic-routed SNS
  message_type=manual
  topic_arn=arn:aws:sns:us-east-1:...:rez-agent-notifications-dev
```

---

## Verification Checklist

After deployment:

- [ ] WebAPI Lambda environment has `WEB_ACTIONS_TOPIC_ARN`
- [ ] WebAPI Lambda environment has `NOTIFICATIONS_TOPIC_ARN`
- [ ] WebAPI logs show "using topic-routing SNS client"
- [ ] Web action messages route to `web-actions-topic`
- [ ] Manual/scheduled messages route to `notifications-topic`
- [ ] WebAction Lambda processes web action messages
- [ ] Processor Lambda processes notification messages

---

## Files Modified

1. **`pkg/config/config.go`**
   - Added `WebActionsSNSTopicArn` and `NotificationsSNSTopicArn` fields
   - Added environment variable reading
   - Updated struct initialization

2. **`cmd/webapi/main.go`**
   - Replaced `NewSNSClient` with conditional `NewTopicRoutingSNSClient`
   - Added fallback logic for backward compatibility
   - Added logging for routing mode

3. **`infrastructure/main.go`**
   - Added `WEB_ACTIONS_TOPIC_ARN` environment variable
   - Added `NOTIFICATIONS_TOPIC_ARN` environment variable
   - Kept legacy `SNS_TOPIC_ARN` for backward compatibility

---

## Benefits

✅ **Clean Architecture** - Messages routed by type, not filters
✅ **No Filter Complexity** - Simple topic-based routing
✅ **Backward Compatible** - Falls back to legacy if needed
✅ **Better Observability** - Logs show which routing mode is active
✅ **Separation of Concerns** - Web actions and notifications isolated
✅ **Scalable** - Easy to add new message types/topics

---

## Migration Path

### Current State
```
WebAPI → topic-routing client → {
    web_action → web-actions-topic
    manual/scheduled → notifications-topic
}
```

### Future State (After Legacy Cleanup)
```
WebAPI → topic-routing client only (remove legacy topic support)
```

To complete migration:
1. Verify all messages route correctly for 1-2 weeks
2. Remove `SNS_TOPIC_ARN` fallback logic from `cmd/webapi/main.go`
3. Remove legacy topic from IAM policy
4. Remove legacy topic/queue resources

---

**Status**: ✅ Complete and Ready for Deployment
**Last Updated**: 2025-10-28
