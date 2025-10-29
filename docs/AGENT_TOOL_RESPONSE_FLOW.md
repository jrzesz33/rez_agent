# Agent Tool Response Flow

## Overview

This document describes the complete tool response flow that enables the AI Agent to receive results from asynchronous tool executions.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Tool Response Flow                          │
└─────────────────────────────────────────────────────────────────────┘

1. User Request
   ↓
2. Agent Lambda (API Gateway)
   ├─ Cost Check
   ├─ LLM Reasoning (Bedrock)
   └─ Tool Selection
   ↓
3. Tool Execution Request
   Agent → SNS (web_actions topic) → SQS Queue → WebAction Lambda
   ↓
4. External API Call
   WebAction Lambda → Golf API / Weather API
   ↓
5. Tool Response Routing
   WebAction Lambda → Check created_by field
   ├─ If "ai-agent" → SNS (agent_response topic)
   └─ If other → SNS (notifications topic)
   ↓
6. Agent Response Queue
   SNS (agent_response) → SQS (agent_response_queue)
   ↓
7. Response Polling
   Agent Lambda → Poll SQS → Receive tool responses
   ↓
8. Agent Re-invocation
   Agent → Add tool results to context → LLM Reasoning → Final response
   ↓
9. User Response
   Agent Lambda → API Gateway → User
```

## Components

### 1. Response Handler (`cmd/agent/response_handler.py`)

**Purpose**: Poll and process tool execution results from SQS.

**Key Methods**:
- `poll_responses()` - Long poll for any available responses
- `poll_for_message()` - Poll for specific message response
- `format_response_for_agent()` - Format for LLM consumption
- `get_queue_depth()` - Check pending responses

**Features**:
- Long polling (up to 20 seconds)
- Batch processing (up to 10 messages)
- Automatic message deletion
- Error handling with DLQ

### 2. Main Agent Integration (`cmd/agent/main.py`)

**Dual Handler Support**:
```python
def lambda_handler(event, context):
    # Handle SQS events (tool responses)
    if "Records" in event:
        return handle_sqs_event(event)

    # Handle API Gateway requests (user chat)
    else:
        return handle_chat_request(event)
```

**Tool Response Flow**:
1. Agent invokes tools (publishes to SNS)
2. Tools execute asynchronously
3. Agent polls response queue (30s timeout)
4. If responses found:
   - Format for LLM context
   - Re-invoke agent with results
   - Return final response
5. If no responses (timeout):
   - Return "processing" message
   - User can poll again later

### 3. Message Type Routing

**New Message Type**: `MessageTypeAgentResponse`

**SNS Topic Routing** (`internal/messaging/sns.go`):
```go
func getTopicForMessageType(messageType MessageType) string {
    switch messageType {
    case MessageTypeWebAction:
        return webActionsTopicArn
    case MessageTypeAgentResponse:
        return agentResponseTopicArn  // Routes to agent
    default:
        return notificationsTopicArn  // Routes to ntfy
    }
}
```

### 4. WebAction Response Routing

**Detection Logic** (`cmd/webaction/main.go`):
```go
isAgentMessage := originalMessage.CreatedBy == "ai-agent"

if isAgentMessage {
    // Route to agent response topic
    notificationMsg.MessageType = MessageTypeAgentResponse
} else {
    // Route to notifications (ntfy)
    notificationMsg.MessageType = MessageTypeNotification
}
```

## Message Flow Examples

### Example 1: Get Reservations

**Request**:
```bash
POST /agent
{
  "message": "What are my reservations at Birdsfoot?",
  "session_id": "session_123"
}
```

**Flow**:
1. Agent Lambda receives request
2. LLM decides to use `get_reservations_tool`
3. Agent publishes to `web_actions` topic:
   ```json
   {
     "id": "msg_20250129143522_abc123",
     "created_by": "ai-agent",
     "message_type": "web_action",
     "payload": {
       "operation": "fetch_reservations",
       "course": "Birdsfoot"
     }
   }
   ```
4. WebAction Lambda receives message
5. Fetches reservations from Golf API
6. Detects `created_by: "ai-agent"`
7. Publishes response to `agent_response` topic:
   ```json
   {
     "id": "msg_20250129143525_def456",
     "created_by": "web-action-processor",
     "message_type": "agent_response",
     "payload": "You have 2 upcoming reservations:\n1. Nov 4, 9:00 AM\n2. Nov 5, 2:00 PM"
   }
   ```
8. Message arrives in `agent_response_queue`
9. Agent polls queue (detects message)
10. Agent adds result to context:
    ```
    Tool Results:
    You have 2 upcoming reservations:
    1. Nov 4, 9:00 AM
    2. Nov 5, 2:00 PM
    ```
11. Agent re-invokes LLM with results
12. LLM formulates natural response:
    ```
    "You have 2 upcoming reservations at Birdsfoot Golf Course:
    - November 4th at 9:00 AM
    - November 5th at 2:00 PM

    Would you like me to help with anything else?"
    ```

### Example 2: Search and Book Tee Time

**Request**:
```bash
POST /agent
{
  "message": "Search for tee times at Totteridge on Nov 10 around 10 AM and book the first available",
  "session_id": "session_456"
}
```

**Flow**:
1. Agent identifies `search_tee_times_tool` with `auto_book: true`
2. Publishes search request
3. Polls for 30 seconds
4. Receives search results (with booking confirmation)
5. Formats response with booking details
6. Returns to user

## Polling Strategy

### Configuration

```python
response_handler = ResponseHandler(
    queue_url=AGENT_RESPONSE_QUEUE_URL,
    max_messages=10,      # Batch size
    wait_time=5           # Long polling wait
)
```

### Polling Behavior

**Long Polling**:
- SQS waits up to `wait_time` seconds for messages
- Reduces empty responses
- More efficient than short polling

**Timeout**:
- Agent polls for up to 30 seconds
- Checks multiple times within timeout
- Breaks early if responses received

**Batch Processing**:
- Receives up to 10 messages per poll
- Processes all in batch
- Deletes successfully processed messages

## Error Handling

### No Response (Timeout)

```json
{
  "session_id": "session_123",
  "message": "I've submitted your request for processing. The results should be available shortly."
}
```

User can retry request to get results.

### Invalid Response

- SQS batch failure returned
- Message remains in queue
- Retried up to 3 times
- Moved to DLQ after max retries

### DLQ Monitoring

```bash
# Check DLQ depth
aws sqs get-queue-attributes \
  --queue-url <dlq-url> \
  --attribute-names ApproximateNumberOfMessages
```

## Performance

### Latency Breakdown

| Step | Time | Notes |
|------|------|-------|
| User → Agent | 100ms | API Gateway + Lambda cold start |
| Agent → LLM | 2-5s | Bedrock inference |
| Agent → Tool | 500ms | SNS → SQS propagation |
| Tool → API | 1-3s | External API call |
| API → Response | 500ms | SNS → SQS propagation |
| Response → Agent | 0-30s | Polling timeout |
| Agent → LLM | 2-5s | Second inference with results |
| **Total** | **6-44s** | Depends on tool execution time |

### Optimization Tips

1. **Reduce polling timeout** for fast APIs:
   ```python
   responses = response_handler.poll_responses(timeout_seconds=10)
   ```

2. **Implement response caching** for repeated queries

3. **Use exponential backoff** for polling:
   ```python
   for wait in [1, 2, 4, 8, 15]:
       responses = poll_responses(timeout_seconds=wait)
       if responses:
           break
   ```

4. **WebSocket support** for real-time updates (future)

## Monitoring

### CloudWatch Metrics

**Custom Metrics**:
```python
cloudwatch.put_metric_data(
    Namespace='RezAgent/Agent',
    MetricData=[
        {
            'MetricName': 'ToolResponseLatency',
            'Value': latency_ms,
            'Unit': 'Milliseconds'
        },
        {
            'MetricName': 'ToolResponseSuccess',
            'Value': 1 if success else 0,
            'Unit': 'Count'
        }
    ]
)
```

### CloudWatch Logs

**Search for response polling**:
```bash
aws logs filter-pattern "Polling for" \
  --log-group-name /aws/lambda/rez-agent-agent-dev
```

**Search for received responses**:
```bash
aws logs filter-pattern "Received tool response" \
  --log-group-name /aws/lambda/rez-agent-agent-dev
```

### SQS Metrics

**Queue depth**:
```bash
aws cloudwatch get-metric-statistics \
  --namespace AWS/SQS \
  --metric-name ApproximateNumberOfMessagesVisible \
  --dimensions Name=QueueName,Value=rez-agent-agent-responses-dev \
  --start-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 300 \
  --statistics Average
```

## Testing

### Test Tool Response Flow

```bash
# 1. Send agent request that uses a tool
curl -X POST https://<api-endpoint>/agent \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Get my reservations at Birdsfoot",
    "session_id": "test_response_flow"
  }'

# 2. Check CloudWatch logs for:
# - "Polling for async tool responses"
# - "Received N tool responses"
# - "Re-invoking agent with tool responses"

# 3. Verify response includes actual reservation data
```

### Test Direct SQS Handler

```bash
# Invoke with SQS event
aws lambda invoke \
  --function-name rez-agent-agent-dev \
  --payload '{
    "Records": [{
      "messageId": "test-123",
      "body": "{\"id\": \"msg_test\", \"created_by\": \"web-action-processor\", \"message_type\": \"agent_response\", \"payload\": \"Test response\"}"
    }]
  }' \
  response.json
```

### Test Polling Timeout

```bash
# Send request that won't get a response
# Should timeout gracefully after 30s
curl -X POST https://<api-endpoint>/agent \
  -d '{"message": "test timeout", "session_id": "timeout_test"}'
```

## Future Enhancements

1. **WebSocket Support**:
   - Real-time response streaming
   - Eliminate polling delay
   - Better user experience

2. **Response Correlation**:
   - Add correlation IDs to match requests/responses
   - Support parallel tool execution
   - Handle out-of-order responses

3. **Caching Layer**:
   - Cache common queries (weather, reservations)
   - Reduce API calls
   - Faster responses

4. **Streaming Responses**:
   - Stream tool results as they arrive
   - Progressive disclosure
   - Improved perceived performance

5. **Async Callback**:
   - Send results to callback URL
   - Support long-running tools
   - Decouple request/response

## Troubleshooting

### Issue: No responses received

**Check**:
1. Tool execution logs in WebAction Lambda
2. Agent response queue depth
3. Message routing (created_by field)
4. SNS subscription configuration

**Solution**:
```bash
# Check queue for stuck messages
aws sqs receive-message \
  --queue-url <queue-url> \
  --max-number-of-messages 10

# Check WebAction logs
aws logs tail /aws/lambda/rez-agent-webaction-dev --follow
```

### Issue: Duplicate responses

**Cause**: Message not deleted after processing

**Solution**:
- Check receipt handle validity
- Verify delete_message call
- Check visibility timeout

### Issue: Slow response time

**Cause**: Long polling timeout

**Solution**:
- Reduce timeout for fast tools
- Implement adaptive timeout
- Add response caching

## Summary

The tool response flow is now **complete and functional**:

✅ **Response Handler** - Polls SQS for tool results
✅ **Message Routing** - Routes agent responses correctly
✅ **Dual Lambda Handler** - Supports both API Gateway and SQS
✅ **Integration** - Agent re-invokes with tool results
✅ **Error Handling** - Graceful timeout and retries
✅ **Monitoring** - CloudWatch logs and metrics
✅ **Testing** - Multiple test scenarios

The agent can now successfully execute tools and receive results!
