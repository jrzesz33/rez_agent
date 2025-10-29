# AI Agent - Golf Reservation Assistant

## Overview

AI-powered golf reservation assistant built with LangGraph and AWS Bedrock. The agent helps users manage golf reservations, search for tee times, get weather forecasts, and send notifications.

## Features

✅ **LangGraph Agent Framework** - Advanced agent orchestration
✅ **AWS Bedrock Integration** - Claude 3.5 Sonnet for reasoning
✅ **Session Management** - Persistent conversation history
✅ **5 Specialized Tools** - Reservations, tee times, weather, notifications
✅ **Cost Management** - $5 daily spending cap
✅ **A2A Integration** - Agent card for agent-to-agent communication
✅ **Simple Web UI** - Interactive chat interface

## Quick Start

### Build
```bash
# From project root
make build-agent
```

### Deploy
```bash
cd infrastructure
pulumi up
```

### Test
```bash
export API_ENDPOINT=$(cd infrastructure && pulumi stack output apiGatewayEndpoint)

curl -X POST $API_ENDPOINT/agent \
  -H "Content-Type: application/json" \
  -d '{
    "message": "What are my reservations at Birdsfoot?",
    "session_id": "test_123"
  }'
```

## Agent Card (A2A)

Discover agent capabilities:
```bash
curl $API_ENDPOINT/agent/card
```

## Cost Management

Check current usage:
```bash
curl -X POST $API_ENDPOINT/agent \
  -H "Content-Type: application/json" \
  -d '{"message": "cost", "session_id": "usage_check"}'
```

**Daily Cap**: $5
**Reset**: Midnight UTC
**Blocked Response**: HTTP 429

## Tools

### 1. Get Reservations
Fetch upcoming golf reservations.

**Example**:
```
"What are my reservations at Totteridge?"
```

### 2. Search Tee Times
Search for available tee times with optional auto-booking.

**Example**:
```
"Search for tee times at Birdsfoot on Nov 4, 2025 between 9 AM and 2 PM for 2 players"
```

### 3. Book Tee Time
Book a specific tee time.

**Example**:
```
"Book tee time ID 141593 at Birdsfoot"
```

### 4. Get Weather
Get weather forecast for golf courses.

**Example**:
```
"What's the weather at Totteridge for the next 3 days?"
```

### 5. Send Notification
Send push notifications.

**Example**:
```
"Send notification: Your tee time is confirmed"
```

## Files

```
cmd/agent/
├── main.py              # Lambda handler and agent logic
├── agent_tools.py       # Tool implementations
├── course_config.py     # Course configuration loader
├── cost_limiter.py      # Cost management
├── agent_card.json      # A2A agent card
├── requirements.txt     # Python dependencies
├── ui/
│   └── index.html       # Web UI
└── README.md           # This file
```

## Configuration

### Environment Variables
- `STAGE` - dev/stage/prod
- `DYNAMODB_TABLE_NAME` - Messages table
- `AGENT_SESSION_TABLE_NAME` - Session storage
- `WEB_ACTIONS_TOPIC_ARN` - SNS topic for web actions
- `NOTIFICATIONS_TOPIC_ARN` - SNS topic for notifications
- `AGENT_RESPONSE_TOPIC_ARN` - SNS topic for tool responses

### Course Configuration
Courses are defined in `pkg/courses/courseInfo.yaml`.

### Adjust Daily Spending Cap
Edit `cost_limiter.py`:
```python
DAILY_SPENDING_CAP = Decimal("10.00")  # Change to $10/day
```

## Architecture

```
User → API Gateway → Agent Lambda → Bedrock (Claude 3.5 Sonnet)
                              ↓
                         Cost Check ($5/day cap)
                              ↓
                         Tool Execution
                              ↓
                    SNS Topic (web_actions)
                              ↓
                    WebAction Lambda
                              ↓
                    Golf API / Weather API
                              ↓
                    Agent Response Topic
```

## Session Management

- Each conversation has a unique `session_id`
- Messages stored in DynamoDB
- Full conversation history maintained
- Sessions persist across requests

## Cost Tracking

**Pricing (Claude 3.5 Sonnet v2)**:
- Input: $0.003 per 1K tokens
- Output: $0.015 per 1K tokens

**Request Capacity ($5/day)**:
- Simple queries: ~1,250 requests
- Complex conversations: ~119 requests

**Storage**:
- DynamoDB record: `bedrock_cost_tracker_{stage}`
- Automatic daily reset at midnight UTC

## Monitoring

### CloudWatch Logs
```bash
aws logs tail /aws/lambda/rez-agent-agent-dev --follow
```

### Cost Tracking
```bash
aws dynamodb get-item \
  --table-name rez-agent-messages-dev \
  --key '{"id": {"S": "bedrock_cost_tracker_dev"}}'
```

### Agent Sessions
```bash
aws dynamodb scan \
  --table-name rez-agent-sessions-dev \
  --max-items 10
```

## Troubleshooting

### Issue: "Daily spending limit reached"
**Solution**: Wait until midnight UTC or increase cap in `cost_limiter.py`

### Issue: "Bedrock access denied"
**Solution**: Verify IAM permissions and Bedrock model access:
```bash
aws bedrock list-foundation-models --region us-east-1 | grep claude-3-5
```

### Issue: Tools not executing
**Solution**: Check SNS topic permissions and SQS queue configuration

### Issue: Sessions not persisting
**Solution**: Verify DynamoDB table exists and Lambda has write permissions

## Web UI

Open `ui/index.html` in browser after updating `API_ENDPOINT` constant.

**Features**:
- Real-time chat interface
- Session management
- Loading indicators
- Error handling

## A2A Integration

External agents can:
1. Discover capabilities via agent card
2. Invoke tools with structured requests
3. Monitor rate limits and quotas

**Rate Limits**:
- 60 requests/minute
- 1,000 requests/hour
- 10,000 requests/day

**SLA**:
- 99.9% availability
- P95: 5 seconds
- P99: 10 seconds

## Documentation

- **Implementation**: `/docs/AI_AGENT_IMPLEMENTATION.md`
- **Deployment**: `/docs/AGENT_DEPLOYMENT_GUIDE.md`
- **Cost Management**: `/docs/COST_MANAGEMENT.md`
- **A2A & Cost**: `/docs/A2A_AND_COST_SUMMARY.md`

## Support

For issues or questions:
1. Check documentation in `/docs`
2. Review CloudWatch logs
3. Verify DynamoDB cost tracking
4. Test with curl examples above

## License

Part of the rez_agent project.
