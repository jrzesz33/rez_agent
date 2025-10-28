# Event Source Mapping Import Fix

## Issue

When running `pulumi up`, you encountered this error:

```
ResourceConflictException: An event source mapping with SQS arn
("arn:aws:sqs:us-east-1:944945738659:rez-agent-web-actions-dev")
and function ("rez-agent-webaction-dev") already exists.
Please update or delete the existing mapping with UUID 19572a82-9001-4a76-b535-d73fc224e439
```

## Root Cause

The event source mappings already exist in AWS (created manually or by a previous deployment), but Pulumi doesn't know about them. When Pulumi tried to create them, AWS rejected the request because they already exist.

## Solution

Import the existing event source mappings into Pulumi's state using `pulumi.Import()`.

### Changes Made

**File:** `infrastructure/main.go`

**Line 653-659:** Processor Lambda Event Source Mapping
```go
_, err = lambda.NewEventSourceMapping(...,
    pulumi.Import(pulumi.ID("2f7ae690-413e-4be6-8ec2-fd0358f5117e")))
```

**Line 823-829:** WebAction Lambda Event Source Mapping
```go
_, err = lambda.NewEventSourceMapping(...,
    pulumi.Import(pulumi.ID("19572a82-9001-4a76-b535-d73fc224e439")))
```

## Deployment Steps

### Step 1: Deploy with Import

```bash
cd infrastructure
pulumi up
```

Pulumi will:
1. Import the existing event source mappings
2. Update them if needed (remove filter criteria)
3. Continue with the rest of the deployment

**Expected Output:**
```
= aws:lambda:EventSourceMapping rez-agent-processor-sqs-trigger-dev importing
= aws:lambda:EventSourceMapping rez-agent-webaction-sqs-trigger-dev importing
```

### Step 2: Remove Import Statements (IMPORTANT!)

After the **first successful deployment**, you MUST remove the `pulumi.Import()` statements:

**Edit `infrastructure/main.go`:**

```diff
# Line 653-659
_, err = lambda.NewEventSourceMapping(ctx, fmt.Sprintf("rez-agent-processor-sqs-trigger-%s", stage), &lambda.EventSourceMappingArgs{
    EventSourceArn: notificationsQueue.Arn,
    FunctionName:   processorLambda.Arn,
    BatchSize:      pulumi.Int(10),
    Enabled:        pulumi.Bool(true),
-}, pulumi.DependsOn([]pulumi.Resource{queuePolicy}), pulumi.Import(pulumi.ID("2f7ae690-413e-4be6-8ec2-fd0358f5117e")))
+}, pulumi.DependsOn([]pulumi.Resource{queuePolicy}))
```

```diff
# Line 823-829
_, err = lambda.NewEventSourceMapping(ctx, fmt.Sprintf("rez-agent-webaction-sqs-trigger-%s", stage), &lambda.EventSourceMappingArgs{
    EventSourceArn: webActionsQueue.Arn,
    FunctionName:   webactionLambda.Arn,
    BatchSize:      pulumi.Int(1),
    Enabled:        pulumi.Bool(true),
-}, pulumi.DependsOn([]pulumi.Resource{queuePolicy}), pulumi.Import(pulumi.ID("19572a82-9001-4a76-b535-d73fc224e439")))
+}, pulumi.DependsOn([]pulumi.Resource{queuePolicy}))
```

### Step 3: Commit Clean Code

```bash
git add infrastructure/main.go
git commit -m "Remove import statements after successful import"
```

## Verification

After deployment, verify the event source mappings are managed by Pulumi:

```bash
# Check Pulumi state
pulumi stack export | jq '.deployment.resources[] | select(.type == "aws:lambda/eventSourceMapping:EventSourceMapping") | {urn: .urn, id: .id}'
```

**Expected Output:**
```json
{
  "urn": "urn:pulumi:dev::rez_agent::aws:lambda/eventSourceMapping:EventSourceMapping::rez-agent-processor-sqs-trigger-dev",
  "id": "2f7ae690-413e-4be6-8ec2-fd0358f5117e"
}
{
  "urn": "urn:pulumi:dev::rez_agent::aws:lambda/eventSourceMapping:EventSourceMapping::rez-agent-webaction-sqs-trigger-dev",
  "id": "19572a82-9001-4a76-b535-d73fc224e439"
}
```

## Why This Happened

The event source mappings were likely created in one of these ways:
1. Manual creation via AWS Console or CLI
2. Previous Pulumi deployment that wasn't tracked
3. Partial deployment that created resources but didn't update state

## Important Notes

⚠️ **Remove Import Statements After First Deployment**

The `pulumi.Import()` option should **ONLY** be used once to import existing resources. If you leave it in the code:
- Subsequent deployments will fail
- Pulumi will try to re-import resources that are already imported

✅ **Best Practice:**
1. Add `pulumi.Import()` → Deploy → Remove `pulumi.Import()` → Commit

## Alternative Solution (If Import Fails)

If the import approach doesn't work, you can manually delete and recreate:

```bash
# Delete existing event source mappings
aws lambda delete-event-source-mapping --uuid 2f7ae690-413e-4be6-8ec2-fd0358f5117e
aws lambda delete-event-source-mapping --uuid 19572a82-9001-4a76-b535-d73fc224e439

# Remove import statements from infrastructure/main.go

# Deploy (will create new mappings)
pulumi up
```

**Warning:** This will cause a brief interruption in message processing.

---

**Status:** ✅ Fixed - Ready for import deployment
**Last Updated:** 2025-10-28
