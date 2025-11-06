# Bedrock Converse API Message Sequencing Fix

## Issue Summary

**Error**: `ValidationException` - `tool_use` ids found without corresponding `tool_result` blocks
**Trigger**: Multi-tool sequences (e.g., weather tool → tee times tool)
**Impact**: Agent conversation breaks after first tool invocation
**Root Cause**: System message was re-inserted on every agent_node invocation, breaking message sequence

## Problem Details

### Error Message
```
ValidationException: The model returned the following errors:
messages.1: `tool_use` ids were found without `tool_result` blocks
immediately after: tooluse_OBPyxl4XSUS4GrLDpsEt0w.
Each `tool_use` block must have a corresponding `tool_result` block
in the next message.
```

### Bedrock Converse API Requirements

The Bedrock Converse API has strict message sequencing requirements:
1. Messages must alternate between user and assistant
2. Each `tool_use` block must be immediately followed by a `tool_result` block
3. System messages should only appear at the start of the conversation
4. Message indices are validated for proper pairing

### Root Cause Analysis

**The Bug**: In the original code at `cmd/agent/main.py:177`:
```python
# Old code (BUGGY)
def agent_node(state: AgentState) -> AgentState:
    # Create system message with context
    system_msg = SystemMessage(...)

    # This re-inserts the system message EVERY TIME
    messages = [system_msg] + state.messages
    response = llm_with_tools.invoke(messages)
```

**Why it failed**:

**Invocation 1** (weather request):
```
messages = [SystemMsg, HumanMsg("what's the weather?")]
→ Agent responds with AIMsg(tool_use: weather_tool)
→ Tool executes, adds ToolMessage
state.messages = [HumanMsg, AIMsg(tool_use), ToolMessage]
```

**Invocation 2** (tee times request):
```
messages = [SystemMsg] + [HumanMsg, AIMsg(tool_use), ToolMessage, HumanMsg("get tee times")]
                                      ↑
                        PROBLEM: AIMsg with tool_use is now at index 2,
                        but Bedrock expects tool_result immediately after
```

The **system message re-insertion** shifts all message indices, breaking the tool_use/tool_result pairing that Bedrock validates.

## Solution

### Fix Applied

**Modified `agent_node` function** to only add system message on first invocation:

```python
def agent_node(state: AgentState) -> AgentState:
    """Main agent reasoning node"""
    logger.info(f"Agent node processing with {len(state.messages)} messages")

    # Only add system message if this is the first invocation
    messages = state.messages
    if not messages or not isinstance(messages[0], SystemMessage):
        system_msg = SystemMessage(content=f"""...""")
        messages = [system_msg] + messages
        logger.info("Added system message (first invocation)")
    else:
        logger.info("System message already present, reusing existing messages")

    # Debug logging
    logger.info(f"Message sequence before LLM invocation: {[type(msg).__name__ for msg in messages]}")

    # Invoke LLM
    response = invoke_llm()

    # CRITICAL: Update state.messages to include system message
    state.messages = messages
    state.messages.append(response)

    return state
```

### Key Changes

1. **Conditional System Message Addition**
   - Check if messages list is empty or first message is not SystemMessage
   - Only add system message when needed
   - Preserves system message across tool invocations

2. **State Persistence**
   - `state.messages = messages` ensures system message persists
   - Maintains proper message sequence across graph cycles

3. **Debug Logging**
   - Log message types before/after each invocation
   - Track message count for troubleshooting
   - Helps diagnose future sequencing issues

## Message Flow (Fixed)

**Invocation 1** (weather request):
```
Initial: messages = []
→ Add system message: [SystemMsg, HumanMsg("weather?")]
→ Agent: AIMsg(tool_use: weather)
→ Tool: ToolMessage(result: "sunny")
state.messages = [SystemMsg, HumanMsg, AIMsg(tool_use), ToolMessage]
```

**Invocation 2** (tee times request - same conversation):
```
Initial: messages = [SystemMsg, HumanMsg, AIMsg(tool_use), ToolMessage, HumanMsg("tee times")]
→ System message already present, skip adding
→ Messages stay in order: [SystemMsg, HumanMsg, AIMsg(tool_use), ToolMessage, HumanMsg]
→ Agent: AIMsg(tool_use: search_tee_times)
→ Tool: ToolMessage(result: "available times...")
state.messages = [SystemMsg, ..., AIMsg(tool_use), ToolMessage]
```

**Result**: Proper tool_use/tool_result pairing maintained ✓

## Testing

### Manual Testing

1. **Single Tool Call** (baseline)
   ```bash
   curl -X POST https://your-api/agent \
     -d '{"message": "What is the weather at Pebble Beach?", "session_id": "test1"}'

   # Expected: Success, weather returned
   ```

2. **Multi-Tool Sequence** (bug scenario)
   ```bash
   # First request
   curl -X POST https://your-api/agent \
     -d '{"message": "What is the weather?", "session_id": "test2"}'

   # Second request (same session)
   curl -X POST https://your-api/agent \
     -d '{"message": "Show me tee times for tomorrow", "session_id": "test2"}'

   # Expected: Success, tee times returned (previously failed)
   ```

3. **Multiple Tool Cycles**
   ```bash
   # Session with 3+ tool calls
   # Request 1: Weather
   # Request 2: Tee times
   # Request 3: Book tee time

   # Expected: All succeed
   ```

### CloudWatch Logs Verification

Look for these log entries:

**First invocation**:
```
Agent node processing with 0 messages
Added system message (first invocation)
Message sequence before LLM invocation: ['SystemMessage', 'HumanMessage']
```

**Subsequent invocations**:
```
Agent node processing with 4 messages
System message already present, reusing existing messages
Message sequence before LLM invocation: ['SystemMessage', 'HumanMessage', 'AIMessage', 'ToolMessage', 'HumanMessage']
```

**Tool execution**:
```
Executing 1 tool calls
Message count before tool execution: 3
Tool execution complete, message count: 4
Message sequence after tool execution: ['SystemMessage', 'HumanMessage', 'AIMessage', 'ToolMessage']
```

### Automated Testing

Create integration test:

```python
def test_multi_tool_sequence():
    """Test that multi-tool sequences work correctly"""
    session_id = "test_multi_tool"

    # Request 1: Weather
    response1 = invoke_agent({
        "message": "What's the weather at Pebble Beach?",
        "session_id": session_id
    })
    assert response1["statusCode"] == 200

    # Request 2: Tee times (same session)
    response2 = invoke_agent({
        "message": "Show me tee times for tomorrow",
        "session_id": session_id
    })
    assert response2["statusCode"] == 200
    # Previously would get ValidationException here

    # Request 3: Another tool call
    response3 = invoke_agent({
        "message": "Book the 9am time",
        "session_id": session_id
    })
    assert response3["statusCode"] == 200
```

## Deployment

### Pre-Deployment Checklist
- [x] Code changes reviewed
- [x] Debug logging added
- [x] System message persistence verified
- [ ] Integration tests pass
- [ ] Manual testing complete

### Deployment Steps

1. **Deploy to Lambda**
   ```bash
   cd /workspaces/rez_agent
   # Build and deploy via Pulumi or your deployment method
   pulumi up
   ```

2. **Verify in CloudWatch Logs**
   ```bash
   aws logs tail /aws/lambda/agent-function --follow
   ```

3. **Test Multi-Tool Flow**
   - Make weather request
   - Make tee times request in same session
   - Verify no ValidationException

### Rollback Plan

If issues occur:
```bash
# Revert to previous Lambda version
aws lambda update-alias \
  --function-name agent-function \
  --name production \
  --function-version <previous-version>
```

## Monitoring

### CloudWatch Alarms

1. **ValidationException Errors**
   ```
   Filter: "ValidationException"
   Threshold: > 0 in 5 minutes
   Action: Alert team
   ```

2. **Tool Execution Failures**
   ```
   Filter: "Error executing tool"
   Threshold: > 5 in 5 minutes
   Action: Alert team
   ```

### Metrics to Track

- Error rate by error type
- Successful multi-tool sequences
- Average message count per session
- Session duration

## Related Documentation

- [Bedrock Converse API Documentation](https://docs.aws.amazon.com/bedrock/latest/APIReference/API_Converse.html)
- [LangGraph Message Types](https://langchain.com/docs/langgraph/concepts/messages)
- [Bedrock Throttling Mitigation](./bedrock-throttling-mitigation.md)

## Future Improvements

1. **Message Validation**
   - Add pre-invocation validation of message sequence
   - Check tool_use/tool_result pairing
   - Warn before sending to Bedrock

2. **Session State Management**
   - Store message history in DynamoDB
   - Implement message pruning for long sessions
   - Add session reset functionality

3. **Better Error Recovery**
   - Catch ValidationException
   - Attempt to repair message sequence
   - Provide user-friendly error messages

## Lessons Learned

1. **Bedrock Converse API is strict about message sequencing** - Unlike some other LLM APIs, Bedrock validates message structure rigorously

2. **Stateful agents require careful message management** - When cycling through a graph, ensure state persistence is correct

3. **System messages should be added once** - Re-inserting system messages breaks message indices

4. **Debug logging is critical** - Message type logging helped identify the root cause quickly

5. **Test multi-turn conversations** - Single-turn tests may not catch sequencing bugs
