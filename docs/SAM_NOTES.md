# SAM Template Notes

## Important Changes from Pulumi

The SAM template differs from the Pulumi infrastructure in a few key ways:

### 1. AWS_REGION Environment Variable

**Issue**: `AWS_REGION` is a reserved Lambda environment variable and cannot be set explicitly.

**Solution**: Lambda automatically provides `AWS_REGION` at runtime. Your Go code can access it via `os.Getenv("AWS_REGION")` or use the AWS SDK's default region configuration.

If you need to override the region for testing, use the `AWS::Region` pseudo-parameter in CloudFormation templates.

### 2. EventBridge Scheduler Execution Role

**Issue**: Circular dependency between `SchedulerFunction` and `SchedulerExecutionRole`.

**Original Pattern** (Pulumi):
```go
// SchedulerFunction references SchedulerExecutionRole.Arn
Environment: map[string]string{
    "EVENTBRIDGE_EXECUTION_ROLE_ARN": schedulerExecutionRole.Arn
}

// SchedulerExecutionRole references SchedulerFunction.Arn
Policies: []iam.Policy{
    {Resource: schedulerFunction.Arn}
}
```

This creates a circular dependency in CloudFormation.

**SAM Solution**:

1. **Remove environment variable**: Don't pass `EVENTBRIDGE_EXECUTION_ROLE_ARN` to SchedulerFunction
2. **Use wildcard in role**: The execution role uses a wildcard pattern instead of specific function ARN:
   ```yaml
   Resource:
     - !Sub 'arn:aws:lambda:${AWS::Region}:${AWS::AccountId}:function:rez-agent-*-local'
   ```
3. **Use stack output**: After deployment, get the role ARN from stack outputs:
   ```bash
   aws cloudformation describe-stacks \
     --stack-name rez-agent-local \
     --query 'Stacks[0].Outputs[?OutputKey==`SchedulerExecutionRoleArn`].OutputValue' \
     --output text
   ```

4. **Update scheduler code**: The scheduler Lambda should:
   - Either use a fixed naming convention for the role
   - Or query the CloudFormation stack to get the role ARN
   - Or use SSM Parameter Store to store/retrieve the role ARN

### Example Code Update

**Option 1: Use SSM Parameter (Recommended)**

After deploying the SAM stack, store the role ARN:

```bash
# Get role ARN from stack outputs
ROLE_ARN=$(aws cloudformation describe-stacks \
  --stack-name rez-agent-local \
  --query 'Stacks[0].Outputs[?OutputKey==`SchedulerExecutionRoleArn`].OutputValue' \
  --output text)

# Store in SSM
aws ssm put-parameter \
  --name "/rez-agent/scheduler-execution-role-arn" \
  --value "$ROLE_ARN" \
  --type String \
  --overwrite
```

In your Go code:

```go
import (
    "github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Get role ARN from SSM
ssmClient := ssm.NewFromConfig(awsCfg)
param, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
    Name: aws.String("/rez-agent/scheduler-execution-role-arn"),
})
if err != nil {
    return fmt.Errorf("failed to get scheduler role ARN: %w", err)
}
roleArn := *param.Parameter.Value
```

**Option 2: Use CloudFormation Describe Stack**

```go
import (
    "github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

func getSchedulerRoleArn(ctx context.Context, cfnClient *cloudformation.Client) (string, error) {
    stacks, err := cfnClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
        StackName: aws.String("rez-agent-local"),
    })
    if err != nil {
        return "", err
    }

    for _, output := range stacks.Stacks[0].Outputs {
        if *output.OutputKey == "SchedulerExecutionRoleArn" {
            return *output.OutputValue, nil
        }
    }

    return "", fmt.Errorf("SchedulerExecutionRoleArn not found in stack outputs")
}
```

**Option 3: Use Naming Convention**

```go
// Construct role ARN using naming convention
roleArn := fmt.Sprintf(
    "arn:aws:iam::%s:role/rez-agent-scheduler-execution-%s",
    accountId,
    stage,
)
```

### 3. Table Names vs ARNs

SAM uses table names for DynamoDB policies, while Pulumi often uses ARNs. The SAM template uses:

```yaml
Policies:
  - DynamoDBCrudPolicy:
      TableName: !Ref MessagesTable
```

This is equivalent to Pulumi's table ARN reference but more concise.

### 4. Local Testing Differences

When using `sam local invoke`, some behaviors differ from deployed Lambda:

- **AWS_REGION**: Defaults to `us-east-1` unless overridden
- **DynamoDB**: Use `--docker-network` and DynamoDB Local
- **Secrets Manager**: Real AWS credentials needed (or mock)
- **SNS/SQS**: Real AWS resources needed (or use LocalStack)

### 5. Using the Template

**Validate**:
```bash
make sam-validate
```

**Build**:
```bash
make sam-build
```

**Deploy to AWS** (for realistic local-like environment):
```bash
make sam-deploy-local
```

**Test Locally** (without deploying):
```bash
make sam-start-api
```

**Get Role ARN After Deployment**:
```bash
aws cloudformation describe-stacks \
  --stack-name rez-agent-local \
  --query 'Stacks[0].Outputs[?OutputKey==`SchedulerExecutionRoleArn`].OutputValue' \
  --output text
```

## Recommendations

1. **Use SSM Parameter Store**: Store the scheduler execution role ARN in SSM after deployment
2. **Add IAM permissions**: Give SchedulerFunction permission to read from SSM
3. **Update scheduler code**: Retrieve role ARN from SSM instead of environment variable
4. **Document the process**: Add to deployment checklist

## Alternative: Keep Environment Variable (Not Recommended)

If you really need the environment variable, you can set it manually after deployment:

```bash
# Get role ARN
ROLE_ARN=$(aws cloudformation describe-stacks \
  --stack-name rez-agent-local \
  --query 'Stacks[0].Outputs[?OutputKey==`SchedulerExecutionRoleArn`].OutputValue' \
  --output text)

# Update function
aws lambda update-function-configuration \
  --function-name rez-agent-scheduler-local \
  --environment "Variables={EVENTBRIDGE_EXECUTION_ROLE_ARN=$ROLE_ARN,...}"
```

However, this is not recommended as it requires manual steps after each deployment.
