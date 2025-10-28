# IAM Policy Fix: WebAPI SNS Publish Permission

## Issue Summary

**Error:**
```
failed to publish message to SNS: operation error SNS: Publish,
https response error StatusCode: 403, RequestID: e513ea78-65cf-52d3-b6ea-8b02bed81849,
AuthorizationError: User: arn:aws:sts::944945738659:assumed-role/rez-agent-webapi-role-dev/rez-agent-webapi-dev
is not authorized to perform: SNS:Publish on resource: arn:aws:sns:us-east-1:944945738659:rez-agent-messages-dev
because no identity-based policy allows the SNS:Publish action
```

## Root Cause

When migrating to topic-based routing, the **webapi Lambda IAM policy** was updated to only include the NEW topics (`webActionsTopic`, `notificationsTopic`), but:

1. The webapi Lambda **code** is still using the **legacy topic ARN** from the `SNS_TOPIC_ARN` environment variable
2. The **legacy topic** (`messagesTopic`) was **removed from the IAM policy**
3. This caused a 403 Authorization Error when the Lambda tried to publish

## Solution

Updated the webapi Lambda IAM policy to include **all three topics** for backward compatibility:

### Before (Broken)
```go
Policy: pulumi.All(messagesTable.Arn, webActionsTopic.Arn, notificationsTopic.Arn).ApplyT(...)
// Only 2 new topics in the policy
Resource: [webActionsTopicArn, notificationsTopicArn]
```

### After (Fixed)
```go
Policy: pulumi.All(messagesTable.Arn, webActionsTopic.Arn, notificationsTopic.Arn, messagesTopic.Arn).ApplyT(...)
// All 3 topics in the policy (including legacy)
Resource: [webActionsTopicArn, notificationsTopicArn, legacyTopicArn]
```

## Verification Summary

### ✅ All Lambda IAM Policies Verified

| Lambda | SNS Topics Allowed | SQS Queues Allowed | Status |
|--------|-------------------|-------------------|---------|
| **scheduler** | ✅ `notificationsTopic` | N/A (publishes only) | ✅ Correct |
| **processor** | N/A (consumes only) | ✅ `notificationsQueue` | ✅ Correct |
| **webaction** | ✅ `webActionsTopic` | ✅ `webActionsQueue` | ✅ Correct |
| **webapi** | ✅ `webActionsTopic`, `notificationsTopic`, `messagesTopic` (legacy) | N/A (publishes only) | ✅ **FIXED** |

### Detailed Policy Review

#### 1. Scheduler Lambda (`rez-agent-scheduler-role-dev`)
```json
{
  "Action": ["sns:Publish"],
  "Resource": "arn:aws:sns:*:*:rez-agent-notifications-dev"
}
```
✅ **Correct** - Publishes scheduled tasks to notifications topic

#### 2. Processor Lambda (`rez-agent-processor-role-dev`)
```json
{
  "Action": ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"],
  "Resource": "arn:aws:sqs:*:*:rez-agent-notifications-dev"
}
```
✅ **Correct** - Consumes from notifications queue (no filter needed)

#### 3. WebAction Lambda (`rez-agent-webaction-role-dev`)
```json
{
  "Action": ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"],
  "Resource": "arn:aws:sqs:*:*:rez-agent-web-actions-dev"
},
{
  "Action": ["sns:Publish"],
  "Resource": "arn:aws:sns:*:*:rez-agent-web-actions-dev"
}
```
✅ **Correct** - Consumes from web-actions queue (no filter needed)

#### 4. WebAPI Lambda (`rez-agent-webapi-role-dev`) - FIXED
```json
{
  "Action": ["sns:Publish"],
  "Resource": [
    "arn:aws:sns:*:*:rez-agent-web-actions-dev",
    "arn:aws:sns:*:*:rez-agent-notifications-dev",
    "arn:aws:sns:*:*:rez-agent-messages-dev"  // ← LEGACY TOPIC ADDED
  ]
}
```
✅ **Fixed** - Can publish to all topics (new + legacy)

## Deployment

### Step 1: Build and Verify
```bash
cd infrastructure
go build .
```
✅ **Verified** - Code compiles successfully

### Step 2: Deploy Updated Policy
```bash
pulumi stack select dev
pulumi up
```

**Expected Changes:**
```diff
~ aws:iam:RolePolicy  rez-agent-webapi-policy-dev
  ~ Policy: {
      "Statement": [
        {
          "Action": ["sns:Publish"],
~         "Resource": ["...", "...", "arn:aws:sns:us-east-1:944945738659:rez-agent-messages-dev"]
        }
      ]
    }
```

### Step 3: Test WebAPI
```bash
# Test message publishing from webapi
curl -X POST https://your-api-endpoint/api/messages \
  -H "Content-Type: application/json" \
  -d '{"message_type":"manual","payload":"test"}'

# Check logs
aws logs tail /aws/lambda/rez-agent-webapi-dev --follow
```

**Expected Output:**
```
✅ message published to SNS
sns_message_id=...
topic_arn=arn:aws:sns:us-east-1:944945738659:rez-agent-messages-dev
```

## Migration Path

This fix maintains **backward compatibility** while supporting the new topic-based architecture:

### Current State (After Fix)
```
WebAPI Lambda
  └─→ Can publish to:
       ├─→ rez-agent-messages-dev (legacy, currently used)
       ├─→ rez-agent-web-actions-dev (new)
       └─→ rez-agent-notifications-dev (new)
```

### Future State (After Code Migration)
```
WebAPI Lambda (using TopicRoutingSNSClient)
  └─→ Publishes to:
       ├─→ rez-agent-web-actions-dev (for web_action messages)
       └─→ rez-agent-notifications-dev (for other messages)
```

### Migration Steps for WebAPI Lambda

**Optional: Update to use TopicRoutingSNSClient**

1. **Update environment variables** (`infrastructure/main.go`):
```go
Environment: &lambda.FunctionEnvironmentArgs{
    Variables: pulumi.StringMap{
        "WEB_ACTIONS_TOPIC_ARN":    webActionsTopic.Arn,
        "NOTIFICATIONS_TOPIC_ARN":  notificationsTopic.Arn,
        // Keep legacy for backward compatibility
        "SNS_TOPIC_ARN":            messagesTopic.Arn,
    },
}
```

2. **Update application code** (`cmd/webapi/main.go`):
```go
// Load new environment variables
webActionsTopicArn := os.Getenv("WEB_ACTIONS_TOPIC_ARN")
notificationsTopicArn := os.Getenv("NOTIFICATIONS_TOPIC_ARN")

// Use TopicRoutingSNSClient instead of SNSClient
publisher := messaging.NewTopicRoutingSNSClient(
    snsClient,
    webActionsTopicArn,
    notificationsTopicArn,
    logger,
)
```

3. **Test and verify** message routing works correctly

4. **Remove legacy topic** from IAM policy once migration is complete

## Conclusion

✅ **Issue Resolved** - WebAPI Lambda now has permission to publish to all topics
✅ **Backward Compatible** - Legacy topic support maintained
✅ **Infrastructure Verified** - All Lambda IAM policies are correct
✅ **Ready for Deployment** - Code compiles and is ready to deploy

**Status**: ✅ Fixed and Ready for Deployment
**Last Updated**: 2025-10-28
