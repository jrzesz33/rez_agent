# Fix: WebAction DynamoDB Permissions Issue

## Issue Summary

The webaction Lambda was failing with:
```
failed to save web action result to DynamoDB: operation error DynamoDB: PutItem, 
https response error StatusCode: 400, RequestID: ..., 
api error AccessDeniedException: User: arn:aws:sts::944945738659:assumed-role/rez-agent-webaction-role-dev/rez-agent-webaction-dev 
is not authorized to perform: dynamodb:PutItem on resource: 
arn:aws:dynamodb:us-east-1:944945738659:table/rez-agent-web-action-results-dev 
because no identity-based policy allows the dynamodb:PutItem action
```

## Root Cause

1. The `rez-agent-web-action-results-{stage}` DynamoDB table **did not exist**
2. The webaction Lambda IAM role **had no permissions** to access this table

## Changes Made

### 1. Created Web Action Results DynamoDB Table

**File**: `infrastructure/main.go` (lines 87-119)

Added new DynamoDB table:
- **Name**: `rez-agent-web-action-results-{stage}`
- **Primary Key**: `id` (String)
- **GSI**: `message_id-index` for querying by message ID
- **TTL**: Enabled on `ttl` attribute (3-day retention)
- **Billing**: PAY_PER_REQUEST (auto-scaling)

```go
webActionResultsTable, err := dynamodb.NewTable(ctx, 
    fmt.Sprintf("rez-agent-web-action-results-%s", stage), 
    &dynamodb.TableArgs{
        Name:        pulumi.String(fmt.Sprintf("rez-agent-web-action-results-%s", stage)),
        BillingMode: pulumi.String("PAY_PER_REQUEST"),
        HashKey:     pulumi.String("id"),
        // ... GSI and TTL configuration
    })
```

### 2. Updated WebAction Lambda IAM Policy

**File**: `infrastructure/main.go` (lines 579-638)

Added permissions for webaction Lambda to access the new table:
- `dynamodb:PutItem` - Save web action results
- `dynamodb:GetItem` - Retrieve results
- `dynamodb:Query` - Query by message_id

```go
Policy: pulumi.All(messagesTable.Arn, webActionResultsTable.Arn, ...).ApplyT(...)
```

## Deployment Steps

### Step 1: Preview Infrastructure Changes

```bash
cd infrastructure
pulumi stack select dev
pulumi preview
```

**Expected Changes:**
- ✅ Create new DynamoDB table: `rez-agent-web-action-results-dev`
- ✅ Update IAM policy for webaction Lambda role

### Step 2: Deploy Infrastructure

```bash
pulumi up
```

Review the changes and type "yes" to confirm.

### Step 3: Verify Deployment

```bash
# Check table was created
aws dynamodb describe-table --table-name rez-agent-web-action-results-dev

# Verify IAM permissions
aws iam get-role-policy \
  --role-name rez-agent-webaction-role-dev \
  --policy-name rez-agent-webaction-policy-dev
```

### Step 4: Test WebAction Lambda

Send a test web action message:

```bash
# Get SQS queue URL
QUEUE_URL=$(pulumi stack output webActionQueueUrl)

# Send test message
aws sqs send-message \
  --queue-url "$QUEUE_URL" \
  --message-body '{
    "version": "1.0",
    "action": "weather",
    "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast",
    "arguments": {},
    "auth_config": {
      "type": "none"
    }
  }'

# Monitor logs for success
aws logs tail /aws/lambda/rez-agent-webaction-dev --follow
```

**Expected Log Output:**
```
✅ Successfully saved web action result to DynamoDB
```

## Verification Checklist

- [ ] DynamoDB table `rez-agent-web-action-results-dev` exists
- [ ] Table has `message_id-index` GSI
- [ ] Table has TTL enabled on `ttl` attribute
- [ ] WebAction Lambda has `dynamodb:PutItem` permission
- [ ] WebAction Lambda has `dynamodb:GetItem` permission
- [ ] WebAction Lambda has `dynamodb:Query` permission
- [ ] Test message processes without permission errors
- [ ] Results are saved to DynamoDB

## Rollback (If Needed)

```bash
# Rollback infrastructure
cd infrastructure
pulumi refresh
pulumi stack history
pulumi stack export --version <PREVIOUS_VERSION> > rollback.json
pulumi stack import --file rollback.json
pulumi up
```

## Production Deployment

After verifying in dev:

```bash
cd infrastructure
pulumi stack select prod
pulumi preview
pulumi up
```

## Cost Impact

**New DynamoDB Table:**
- Billing: PAY_PER_REQUEST (no fixed costs)
- Estimated cost: ~$0.25 per million writes
- Expected usage: <1000 writes/day = ~$0.01/day
- Storage: Minimal (3-day TTL auto-deletes old data)

**Total estimated cost**: <$1/month

## Files Modified

1. `infrastructure/main.go`
   - Added webActionResultsTable creation (lines 87-119)
   - Updated webaction Lambda IAM policy (lines 579-638)

## Related Documentation

- [WebAction Architecture](./architecture/web-action-processor-design.md)
- [DynamoDB Schema](./architecture/data-model.md)
- [IAM Policies](./architecture/authentication-authorization.md)

---

**Status**: ✅ Ready for Deployment
**Last Updated**: 2025-10-28
