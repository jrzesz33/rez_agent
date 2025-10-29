# Bedrock Cost Management

## Overview

The AI Agent includes a comprehensive cost management system to prevent unexpected Bedrock expenses. The system enforces a **$5 daily spending cap** and automatically blocks LLM calls once the limit is reached.

## How It Works

### 1. Cost Tracking
- All Bedrock API calls are tracked in DynamoDB
- Costs are calculated based on actual token usage:
  - **Input tokens**: $3.00 per million tokens ($0.003 per 1K tokens)
  - **Output tokens**: $15.00 per million tokens ($0.015 per 1K tokens)

### 2. Daily Cap Enforcement
- **Default limit**: $5.00 per day
- **Reset time**: Midnight UTC daily
- **Enforcement**: Pre-call estimation blocks requests that would exceed cap

### 3. Cost Calculation
```python
# Claude 3.5 Sonnet v2 Pricing
input_cost = (input_tokens / 1000) * $0.003
output_cost = (output_tokens / 1000) * $0.015
total_cost = input_cost + output_cost
```

## Features

### Pre-Request Cost Estimation
Before each LLM call, the system:
1. Estimates token usage (conservative estimate)
2. Calculates projected cost
3. Checks if projected cost would exceed daily cap
4. Blocks request if cap would be exceeded
5. Returns HTTP 429 (Too Many Requests) with retry-after header

### Post-Request Cost Adjustment
After each LLM call, the system:
1. Extracts actual token usage from Bedrock response
2. Recalculates actual cost
3. Updates DynamoDB with precise figures
4. Logs token usage and cost

### Usage Tracking
The system tracks:
- Total daily cost
- Request count
- Input token count
- Output token count
- Percentage of budget used
- Time until reset

## Usage

### Query Current Usage
Send "cost", "usage", "spending", or "budget" as the message:

```bash
curl -X POST https://<api-endpoint>/agent \
  -H "Content-Type: application/json" \
  -d '{
    "message": "cost",
    "session_id": "check_usage"
  }'
```

Response:
```json
{
  "session_id": "check_usage",
  "message": "Current Bedrock usage today:\n- Cost: $2.35 / $5.00\n- Remaining budget: $2.65\n- Requests: 47\n- Tokens: 125000 input, 85000 output\n- Resets at: 2025-01-29 23:59:59 UTC",
  "usage": {
    "date": "2025-01-29",
    "total_cost": 2.35,
    "daily_cap": 5.0,
    "remaining_budget": 2.65,
    "percentage_used": 47.0,
    "request_count": 47,
    "input_tokens": 125000,
    "output_tokens": 85000,
    "reset_time": "2025-01-29 23:59:59 UTC"
  }
}
```

### Cap Exceeded Response
When daily cap is reached:

```json
{
  "error": "Daily spending limit reached",
  "message": "Daily spending cap of $5.00 would be exceeded. Current usage: $4.95, Estimated request cost: $0.08. Resets at midnight UTC (2025-01-29 23:59:59 UTC).",
  "cost_info": {
    "current_cost": 4.95,
    "estimated_cost": 0.08,
    "projected_cost": 5.03,
    "daily_cap": 5.0,
    "remaining_budget": 0.05,
    "request_count": 98,
    "reset_time": "2025-01-29 23:59:59 UTC"
  }
}
```

HTTP Status: `429 Too Many Requests`
Header: `Retry-After: 86400` (24 hours in seconds)

## Configuration

### Changing the Daily Cap

Edit `cmd/agent/cost_limiter.py`:

```python
DAILY_SPENDING_CAP = Decimal("10.00")  # Change to $10 per day
```

### Updating Pricing

If Bedrock pricing changes, update `cmd/agent/cost_limiter.py`:

```python
CLAUDE_3_5_SONNET_PRICING = {
    "input_per_1k_tokens": Decimal("0.003"),   # Update with new price
    "output_per_1k_tokens": Decimal("0.015"),  # Update with new price
}
```

### Per-Stage Caps

To set different caps per stage, modify the constructor:

```python
class CostLimiter:
    def __init__(self, dynamodb_table_name: str, stage: str):
        # Different caps per stage
        caps = {
            "dev": Decimal("2.00"),
            "stage": Decimal("5.00"),
            "prod": Decimal("20.00")
        }
        self.daily_cap = caps.get(stage, Decimal("5.00"))
```

## Monitoring

### CloudWatch Metrics

Add custom metrics for cost tracking:

```python
import boto3

cloudwatch = boto3.client('cloudwatch')

cloudwatch.put_metric_data(
    Namespace='RezAgent/Bedrock',
    MetricData=[
        {
            'MetricName': 'DailyCost',
            'Value': float(current_cost),
            'Unit': 'None',
            'Dimensions': [
                {'Name': 'Stage', 'Value': stage}
            ]
        }
    ]
)
```

### CloudWatch Alarms

Create alarms for cost thresholds:

```bash
aws cloudwatch put-metric-alarm \
  --alarm-name rez-agent-bedrock-cost-80-percent \
  --alarm-description "Alert when 80% of daily Bedrock budget is used" \
  --metric-name DailyCost \
  --namespace RezAgent/Bedrock \
  --statistic Average \
  --period 300 \
  --threshold 4.0 \
  --comparison-operator GreaterThanThreshold \
  --evaluation-periods 1
```

### DynamoDB Cost Records

Query cost tracking records:

```bash
aws dynamodb get-item \
  --table-name rez-agent-messages-dev \
  --key '{"id": {"S": "bedrock_cost_tracker_dev"}}'
```

## Cost Estimation Examples

### Average Request Costs

| Request Type | Input Tokens | Output Tokens | Estimated Cost |
|--------------|--------------|---------------|----------------|
| Simple query | 500 | 200 | $0.004 |
| Tool execution | 2000 | 1000 | $0.021 |
| Complex conversation | 4000 | 2000 | $0.042 |
| Max single request | 10000 | 4000 | $0.090 |

### Daily Request Capacity

With $5/day cap:
- **Simple queries**: ~1,250 requests
- **Tool executions**: ~238 requests
- **Complex conversations**: ~119 requests
- **Max requests**: ~55 requests

## Best Practices

### 1. Monitor Usage Proactively
- Check usage regularly via the `/agent` endpoint
- Set up CloudWatch alarms at 50%, 75%, and 90% thresholds
- Review daily cost trends

### 2. Optimize Token Usage
- Keep system prompts concise
- Use shorter tool descriptions
- Implement response caching for repeated queries
- Use context summarization for long conversations

### 3. Implement Graceful Degradation
When cap is reached:
- Return cached responses for common queries
- Provide pre-computed answers for FAQs
- Direct users to alternative information sources
- Schedule non-urgent requests for next day

### 4. User Communication
- Inform users about service limitations
- Display current usage in UI
- Show estimated cost per request (optional)
- Provide alternative contact methods

## Troubleshooting

### Issue: Cost tracking not updating

**Check DynamoDB permissions:**
```bash
aws iam get-role-policy \
  --role-name rez-agent-agent-role-dev \
  --policy-name rez-agent-agent-policy-dev
```

**Verify table exists:**
```bash
aws dynamodb describe-table --table-name rez-agent-messages-dev
```

### Issue: Incorrect cost calculations

**Check pricing values:**
```python
# Verify against current AWS Bedrock pricing
# https://aws.amazon.com/bedrock/pricing/
```

**Review token counts in logs:**
```bash
aws logs filter-pattern "Updated actual cost" \
  --log-group-name /aws/lambda/rez-agent-agent-dev \
  --start-time $(date -d '1 hour ago' +%s)000
```

### Issue: Cap reached too quickly

**Review usage patterns:**
```bash
aws dynamodb get-item \
  --table-name rez-agent-messages-dev \
  --key '{"id": {"S": "bedrock_cost_tracker_dev"}}'
```

**Check for abnormal token usage:**
```bash
aws logs filter-pattern "input_tokens output_tokens" \
  --log-group-name /aws/lambda/rez-agent-agent-dev
```

## Alternative Models

To reduce costs, consider using different Bedrock models:

### Claude 3 Haiku (Cheaper Alternative)
```python
# In cmd/agent/main.py
llm = ChatBedrock(
    model_id="anthropic.claude-3-haiku-20240307-v1:0",
    # Pricing: $0.00025/1K input, $0.00125/1K output
)
```

Cost comparison:
- **Haiku**: ~$0.003 per complex conversation
- **Sonnet**: ~$0.042 per complex conversation
- **Savings**: 14x cheaper

## Future Enhancements

1. **Per-User Caps**: Track and limit spending per user
2. **Rate Limiting**: Requests per minute/hour limits
3. **Priority Queuing**: Premium users get higher limits
4. **Cost Analytics**: Dashboard for cost trends and predictions
5. **Auto-scaling Caps**: Adjust caps based on usage patterns
6. **Model Selection**: Automatically use cheaper models when near cap

## Compliance

### Cost Tracking Retention
- Cost records stored in DynamoDB
- Automatic daily reset
- Historical data retention configurable via DynamoDB TTL

### Audit Trail
- All cost decisions logged to CloudWatch
- Request count and token usage tracked
- Cost calculations transparent and verifiable

## Summary

The cost management system provides:
✅ **Hard cap enforcement** - No unexpected bills
✅ **Transparent tracking** - Know exactly what you're spending
✅ **Graceful degradation** - Clear error messages when cap reached
✅ **Daily reset** - Fresh budget every day at midnight UTC
✅ **Query interface** - Check usage anytime
✅ **Configurable** - Easy to adjust caps and pricing

The system ensures Bedrock costs stay within budget while maintaining service quality.
