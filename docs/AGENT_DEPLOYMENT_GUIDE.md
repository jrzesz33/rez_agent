# AI Agent Deployment Guide

## Prerequisites

1. **AWS Account Setup**:
   - AWS CLI configured with appropriate credentials
   - Bedrock access enabled in us-east-1 region
   - Claude 3.5 Sonnet model access enabled in Bedrock

2. **Development Tools**:
   - Go 1.24+
   - Python 3.12+
   - Pulumi CLI
   - pip (Python package manager)

3. **Existing Infrastructure**:
   - Golf API credentials stored in Secrets Manager: `rez-agent/golf/credentials-{stage}`

## Deployment Steps

### Step 1: Build All Components

```bash
# From project root
make build
```

This will:
- Build all Go Lambda functions (scheduler, processor, webaction, webapi)
- Build Python agent Lambda with dependencies
- Create deployment packages in `build/` directory

### Step 2: Verify Build Artifacts

```bash
ls -lh build/
```

You should see:
- `scheduler.zip`
- `processor.zip`
- `webaction.zip`
- `webapi.zip`
- `agent.zip` (largest file due to Python dependencies)

### Step 3: Deploy Infrastructure

```bash
cd infrastructure
pulumi stack select dev  # or prod
pulumi up
```

Review the preview and confirm deployment. This will create:
- Agent Lambda function
- Agent Session DynamoDB table
- Agent Response SNS topic and SQS queue
- API Gateway route for `/agent` endpoint
- Updated WebAction Lambda with agent response routing

### Step 4: Verify Deployment

```bash
# Get the API Gateway endpoint
pulumi stack output apiGatewayEndpoint

# Get agent Lambda ARN
pulumi stack output agentLambdaArn

# Verify agent session table
pulumi stack output agentSessionTableName
```

### Step 5: Test the Agent

#### Using curl:

```bash
export API_ENDPOINT=$(cd infrastructure && pulumi stack output apiGatewayEndpoint)

curl -X POST $API_ENDPOINT/agent \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Hello, what can you help me with?",
    "session_id": "test_'$(date +%s)'"
  }'
```

Expected response:
```json
{
  "session_id": "test_1234567890",
  "message": "Hello! I'm your golf reservation assistant. I can help you with: ..."
}
```

#### Test Golf Operations:

```bash
# Check reservations
curl -X POST $API_ENDPOINT/agent \
  -H "Content-Type: application/json" \
  -d '{
    "message": "What are my upcoming reservations at Birdsfoot Golf Course?",
    "session_id": "test_'$(date +%s)'"
  }'

# Search for tee times
curl -X POST $API_ENDPOINT/agent \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Search for tee times at Totteridge on November 4th, 2025 between 9 AM and 2 PM for 2 players",
    "session_id": "test_'$(date +%s)'"
  }'

# Get weather
curl -X POST $API_ENDPOINT/agent \
  -H "Content-Type: application/json" \
  -d '{
    "message": "What is the weather forecast for Birdsfoot Golf Course?",
    "session_id": "test_'$(date +%s)'"
  }'
```

### Step 6: Access Web UI

Open browser to: `https://<api-gateway-endpoint>/agent/ui` (if configured)

Or use the standalone HTML file:
1. Open `cmd/agent/ui/index.html` in a text editor
2. Update the `API_ENDPOINT` constant to your API Gateway URL
3. Open in browser

## Monitoring

### CloudWatch Logs

View agent logs:
```bash
aws logs tail /aws/lambda/rez-agent-agent-dev --follow
```

View webaction logs:
```bash
aws logs tail /aws/lambda/rez-agent-webaction-dev --follow
```

### DynamoDB Tables

Check agent sessions:
```bash
aws dynamodb scan --table-name rez-agent-sessions-dev --max-items 10
```

Check messages:
```bash
aws dynamodb scan --table-name rez-agent-messages-dev --max-items 10
```

### SQS Queues

Monitor agent response queue:
```bash
aws sqs get-queue-attributes \
  --queue-url $(cd infrastructure && pulumi stack output agentResponseQueueUrl) \
  --attribute-names All
```

## Troubleshooting

### Issue: Agent not responding

**Check Lambda execution:**
```bash
aws lambda invoke \
  --function-name rez-agent-agent-dev \
  --payload '{"body": "{\"message\": \"test\", \"session_id\": \"test\"}"}' \
  response.json
cat response.json
```

**Check CloudWatch Logs:**
```bash
aws logs tail /aws/lambda/rez-agent-agent-dev --since 10m
```

### Issue: Bedrock access denied

**Verify Bedrock model access:**
```bash
aws bedrock list-foundation-models --region us-east-1 | grep claude-3-5
```

**Check IAM permissions:**
```bash
aws lambda get-function --function-name rez-agent-agent-dev | jq .Configuration.Role
aws iam get-role-policy --role-name rez-agent-agent-role-dev --policy-name rez-agent-agent-policy-dev
```

### Issue: Tools not executing

**Check SNS topic permissions:**
```bash
aws sns get-topic-attributes --topic-arn $(cd infrastructure && pulumi stack output webActionsTopicArn)
```

**Check SQS queue messages:**
```bash
aws sqs receive-message \
  --queue-url $(cd infrastructure && pulumi stack output agentResponseQueueUrl) \
  --max-number-of-messages 10
```

### Issue: Dependencies missing in Lambda

**Rebuild agent package:**
```bash
make clean
make build-agent
```

**Verify package contents:**
```bash
unzip -l build/agent.zip | head -20
```

### Issue: Session not persisting

**Check DynamoDB table:**
```bash
aws dynamodb describe-table --table-name rez-agent-sessions-dev
```

**Scan for sessions:**
```bash
aws dynamodb scan --table-name rez-agent-sessions-dev
```

## Performance Tuning

### Lambda Memory
Current: 1024 MB for agent Lambda

Adjust in `infrastructure/main.go`:
```go
MemorySize: pulumi.Int(2048), // Increase for faster execution
```

### Lambda Timeout
Current: 300 seconds (5 minutes)

Adjust in `infrastructure/main.go`:
```go
Timeout: pulumi.Int(600), // Increase for long-running operations
```

### Bedrock Model
Current: Claude 3.5 Sonnet v2

Change in `cmd/agent/main.py`:
```python
model_id="anthropic.claude-3-5-sonnet-20241022-v2:0"
```

## Cost Optimization

### Estimated Costs (per 1000 requests):
- **Agent Lambda**: ~$0.20 (1024 MB, 30s avg)
- **Bedrock (Claude 3.5 Sonnet)**: ~$15 (input) + ~$75 (output)
- **DynamoDB**: ~$0.01 (on-demand)
- **SNS/SQS**: ~$0.50
- **API Gateway**: ~$3.50

**Total**: ~$94 per 1000 requests

### Optimization Tips:
1. Use smaller Bedrock models for simple queries
2. Implement caching for common responses
3. Use provisioned concurrency for predictable traffic
4. Set DynamoDB TTL to auto-expire old sessions

## Security Best Practices

1. **API Gateway**:
   - Add API key authentication
   - Implement rate limiting
   - Use WAF for DDoS protection

2. **Lambda**:
   - Enable X-Ray tracing for debugging
   - Use VPC for sensitive operations
   - Rotate IAM credentials regularly

3. **Secrets**:
   - Never log sensitive data
   - Use Secrets Manager for all credentials
   - Enable secret rotation

4. **Monitoring**:
   - Set up CloudWatch alarms for errors
   - Monitor Lambda invocation counts
   - Track Bedrock usage and costs

## Rollback Procedure

If deployment fails or issues occur:

```bash
cd infrastructure
pulumi stack select dev
pulumi cancel  # Cancel in-progress update
pulumi refresh # Sync state with AWS
pulumi up      # Redeploy with fixes
```

Or rollback to previous version:
```bash
pulumi stack history
pulumi stack export --version <version-number> | pulumi stack import
```

## Next Steps

1. **Production Deployment**:
   ```bash
   pulumi stack select prod
   make build
   cd infrastructure && pulumi up
   ```

2. **Custom Domain**:
   - Set up Route53 domain
   - Add ACM certificate
   - Configure API Gateway custom domain

3. **Monitoring Dashboard**:
   - Create CloudWatch dashboard
   - Set up SNS alerts
   - Configure CloudWatch Insights queries

4. **CI/CD Pipeline**:
   - Set up GitHub Actions
   - Automate testing
   - Deploy on merge to main

## Support

For issues or questions:
1. Check CloudWatch Logs
2. Review Pulumi state: `pulumi stack`
3. Verify AWS resource status in Console
4. Check project documentation in `docs/`
