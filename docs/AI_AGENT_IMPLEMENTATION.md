# AI Agent Implementation Summary

## Overview
This document summarizes the implementation of an AI Agent component for the rez_agent system. The agent integrates with AWS Bedrock, uses LangGraph for agent orchestration, and provides tools for managing golf reservations.

## Components Created

### 1. Python AI Agent (`cmd/agent/`)
- **main.py**: Lambda handler implementing the LangGraph agent workflow
- **agent_tools.py**: Tool implementations for golf operations and notifications
- **course_config.py**: Course configuration loader from YAML
- **requirements.txt**: Python dependencies (LangGraph, Bedrock, boto3)
- **ui/index.html**: Simple web UI for interacting with the agent

### 2. Agent Tools
The agent has the following capabilities:

#### Golf Course Operations
- **get_reservations_tool**: Fetch user's upcoming golf reservations
- **search_tee_times_tool**: Search for available tee times with optional auto-booking
- **book_tee_time_tool**: Book a specific tee time
- **get_weather_tool**: Get weather forecast for golf courses

#### Notification
- **send_notification_tool**: Send push notifications via ntfy.sh

### 3. Infrastructure Updates

#### New AWS Resources (in `infrastructure/main.go`):
- **Agent Response Topic**: SNS topic for tool responses back to agent
- **Agent Response Queue**: SQS queue subscribed to response topic
- **Agent Session Table**: DynamoDB table for storing conversation sessions
- **Agent Lambda**: Python 3.12 Lambda function running the agent
- **API Gateway Route**: `/agent` POST endpoint for agent interaction

#### Updated Resources:
- **WebAction Lambda**: Now routes responses to agent topic when message was created by agent
- **WebAction IAM Role**: Added permissions for agent response topic

### 4. Configuration Updates

#### `pkg/config/config.go`:
- Added `AgentResponseTopicArn` field
- Loads `AGENT_RESPONSE_TOPIC_ARN` environment variable

#### `cmd/webaction/main.go`:
- Added `agentResponseTopicArn` field to Handler struct
- Updated `publishNotification` to route responses based on `created_by` field
- Checks if message was created by "ai-agent" and routes accordingly

### 5. Build System Updates

#### `Makefile`:
- Added `build-agent` target for Python Lambda
- Installs dependencies with pip
- Creates agent.zip deployment package
- Includes courseInfo.yaml in package

## Architecture

### Message Flow

1. **User → Agent**:
   ```
   User → API Gateway → Agent Lambda → Agent processes request
   ```

2. **Agent → Tools**:
   ```
   Agent → SNS (web_actions or notifications topic) → SQS Queue → WebAction Lambda
   ```

3. **Tool Response → Agent** ✅ **COMPLETE**:
   ```
   WebAction Lambda → Agent Response Topic → Agent Response Queue → Agent (polling with response_handler.py)
   ```

   **Implementation Details**:
   - `response_handler.py` polls SQS queue with 30s timeout
   - Long polling (up to 20s per request) for efficiency
   - Batch processing (up to 10 messages)
   - Automatic message deletion after processing
   - Agent re-invokes LLM with tool results
   - Graceful timeout handling if no response

4. **Agent → User**:
   ```
   Agent → API Gateway → User
   ```

### Session Management
- Each conversation has a unique `session_id`
- Sessions are stored in DynamoDB with full message history
- Agent maintains context across multiple interactions

### Tool Integration
- Agent publishes messages to SNS topics with `created_by: "ai-agent"`
- WebAction processor detects agent-created messages via `created_by` field
- Responses are routed to agent response topic instead of notification topic

## LangGraph Agent Workflow

The agent uses a StateGraph with the following nodes:

1. **Agent Node**: Main reasoning node using Claude 3.5 Sonnet via Bedrock
2. **Tool Node**: Executes tool calls
3. **Conditional Routing**: Determines if more tool calls needed or if conversation is complete

```
┌──────┐
│Entry │
└──┬───┘
   │
   v
┌──────────┐      Yes      ┌──────┐
│  Agent   ├──────────────>│Tools │
└────┬─────┘                └──┬───┘
     │                         │
     │ No                      │
     │                         │
     v                         v
   ┌───┐                   ┌───────┐
   │END│<──────────────────┤ Agent │
   └───┘                   └───────┘
```

## Environment Variables

### Agent Lambda:
- `DYNAMODB_TABLE_NAME`: Messages table
- `AGENT_SESSION_TABLE_NAME`: Session storage table
- `WEB_ACTIONS_TOPIC_ARN`: Topic for web action requests
- `NOTIFICATIONS_TOPIC_ARN`: Topic for notifications
- `AGENT_RESPONSE_TOPIC_ARN`: Topic for receiving tool responses
- `AGENT_RESPONSE_QUEUE_URL`: Queue URL for tool responses
- `STAGE`: Deployment environment (dev/prod)

### WebAction Lambda (updated):
- Added `AGENT_RESPONSE_TOPIC_ARN`: For routing agent responses

## Course Configuration

The agent loads golf course information from `pkg/courses/courseInfo.yaml`:
- Course names and addresses
- API endpoints for each operation
- Authentication URLs
- Weather API endpoints

## Security Considerations

1. **IAM Permissions**:
   - Agent has Bedrock access for LLM invocations
   - Limited SNS publish permissions to specific topics
   - DynamoDB access scoped to messages and sessions tables

2. **Message Attribution**:
   - All agent-created messages tagged with `created_by: "ai-agent"`
   - Enables proper response routing and auditing

3. **Secrets Management**:
   - Golf API credentials stored in AWS Secrets Manager
   - Retrieved by WebAction Lambda, not directly by agent

## Testing Considerations

### Manual Testing:
1. Deploy infrastructure: `make build && cd infrastructure && pulumi up`
2. Access API Gateway endpoint
3. Send POST request to `/agent` with JSON body:
   ```json
   {
     "message": "What are my upcoming reservations at Birdsfoot?",
     "session_id": "test_session_123"
   }
   ```
4. Verify agent response includes tool execution results

### Integration Testing:
- Test each tool independently
- Verify message routing to correct topics
- Confirm session persistence across requests
- Validate response routing back to agent

## Future Enhancements

1. **Response Polling**: Implement SQS polling in agent to receive tool responses asynchronously
2. **Streaming Responses**: Add streaming support for real-time agent responses
3. **Multi-Agent Communication**: Implement A2A (Agent-to-Agent) capabilities
4. **Enhanced UI**: Build more sophisticated web interface with chat history
5. **Tool Result Parsing**: Parse tool responses and integrate into agent context
6. **Error Handling**: Add retry logic and graceful degradation
7. **Metrics**: Add CloudWatch metrics for agent performance monitoring

## Files Modified

### New Files:
- `cmd/agent/main.py`
- `cmd/agent/agent_tools.py`
- `cmd/agent/course_config.py`
- `cmd/agent/requirements.txt`
- `cmd/agent/ui/index.html`

### Modified Files:
- `infrastructure/main.go` (added agent infrastructure)
- `Makefile` (added build-agent target)
- `pkg/config/config.go` (added AgentResponseTopicArn)
- `cmd/webaction/main.go` (added agent response routing)

## Deployment

### Build All Components:
```bash
make build
```

### Deploy Infrastructure:
```bash
cd infrastructure
pulumi up
```

### Get API Endpoint:
```bash
cd infrastructure
pulumi stack output apiGatewayEndpoint
```

### Test Agent:
```bash
curl -X POST https://<api-gateway-url>/agent \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Search for tee times at Birdsfoot on 2025-11-04 between 9 AM and 2 PM",
    "session_id": "test_session"
  }'
```

## Dependencies

### Python:
- boto3: AWS SDK
- langgraph: Agent orchestration framework
- langchain-aws: AWS Bedrock integration
- langchain-core: Core LangChain functionality
- pydantic: Data validation
- pyyaml: YAML configuration parsing

### Go:
- No new dependencies added (uses existing AWS SDK v2)

## Agent-to-Agent (A2A) Integration

### Agent Card
The agent exposes a standardized **Agent Card** for A2A discovery:

**Endpoints**:
- `GET /agent/card` - Standard agent card endpoint
- `GET /agent/.well-known/agent-card` - Well-known discovery path

**Agent Card Location**: `cmd/agent/agent_card.json`

**Features**:
- Complete tool documentation with parameters
- Protocol specifications (HTTP/JSON)
- Rate limits and SLA guarantees
- Authentication requirements
- Request/response schemas

**Usage**:
```bash
curl https://<api-endpoint>/agent/card
```

External agents can discover capabilities and invoke tools programmatically.

## Cost Management

### Daily Spending Cap: $5
The agent enforces a **hard $5 daily limit** on Bedrock API costs.

**Implementation**: `cmd/agent/cost_limiter.py`

**Features**:
- ✅ Pre-request cost estimation and blocking
- ✅ Post-request actual cost tracking
- ✅ Daily automatic reset at midnight UTC
- ✅ Detailed usage statistics
- ✅ Query current usage via chat

**Pricing** (Claude 3.5 Sonnet v2):
- Input: $0.003 per 1K tokens
- Output: $0.015 per 1K tokens

**Request Capacity**:
- Simple queries: ~1,250 requests/day
- Complex conversations: ~119 requests/day

**Check Usage**:
```bash
curl -X POST https://<api-endpoint>/agent \
  -d '{"message": "cost", "session_id": "test"}'
```

**When Cap Reached**:
- Returns HTTP 429 (Too Many Requests)
- Includes retry-after header (24 hours)
- Shows current usage and reset time

**Documentation**: See `docs/COST_MANAGEMENT.md` for full details

## Notes

- The agent uses Claude 3.5 Sonnet via AWS Bedrock
- Temperature set to 0.0 for deterministic responses
- Session data expires via DynamoDB TTL
- Agent response queue has DLQ for failed message handling
- All components follow existing naming conventions (rez-agent-*)
- **A2A ready**: Agent card enables agent-to-agent communication
- **Cost protected**: Hard $5/day cap prevents unexpected bills
