"""
AI Agent Lambda Handler
This Lambda function implements an AI agent using LangGraph and AWS Bedrock.
"""
import json
import logging
import os
from datetime import datetime
from typing import Any, Dict, List

import boto3
from langchain_aws import ChatBedrockConverse
from langchain_aws import ChatBedrock
from langchain_core.messages import HumanMessage, AIMessage, SystemMessage, ToolMessage
from langgraph.graph import StateGraph, END
from langgraph.prebuilt import ToolNode
from pydantic import BaseModel, Field
# The LangChain MCP Adapter can be used to import tools from a remote MCP server, making them accessible to your LangGraph agent.
# Import Bedrock configuration for rate limiting and retry logic
from bedrock_config import (
    create_bedrock_client,
    get_rate_limiter,
    with_rate_limit,
    with_exponential_backoff
)

# Configure logging (must be before any logger usage)
logger = logging.getLogger()
logger.setLevel(logging.INFO)

# Import LangChain MCP Adapter for remote MCP server integration
from langchain_mcp_adapters.client import MultiServerMCPClient
from course_config import load_course_config
from cost_limiter import CostLimiter
from response_handler import ResponseHandler
# Environment variables
STAGE = os.environ.get("STAGE", "dev")
DYNAMODB_TABLE_NAME = os.environ.get("DYNAMODB_TABLE_NAME")
SESSION_TABLE_NAME = os.environ.get("AGENT_SESSION_TABLE_NAME")
AGENT_RESPONSE_TOPIC_ARN = os.environ.get("AGENT_RESPONSE_TOPIC_ARN")
AGENT_RESPONSE_QUEUE_URL = os.environ.get("AGENT_RESPONSE_QUEUE_URL")
MCP_SERVER_URL = os.environ.get("MCP_SERVER_URL")

# Bedrock LLM Configuration
BEDROCK_MODEL_ID = os.environ.get(
    "BEDROCK_MODEL_ID",
    "arn:aws:bedrock:us-east-1:944945738659:inference-profile/global.anthropic.claude-sonnet-4-20250514-v1:0"
)
BEDROCK_PROVIDER = os.environ.get("BEDROCK_PROVIDER", "Anthropic")
BEDROCK_REGION = os.environ.get("BEDROCK_REGION", "us-east-1")
BEDROCK_TEMPERATURE = float(os.environ.get("BEDROCK_TEMPERATURE", "0.0"))
BEDROCK_MAX_TOKENS = int(os.environ.get("BEDROCK_MAX_TOKENS", "4096"))

# Bedrock Throttling Configuration
BEDROCK_RATE_LIMIT = int(os.environ.get("BEDROCK_RATE_LIMIT", "30"))  # requests per minute
BEDROCK_MAX_RETRIES = int(os.environ.get("BEDROCK_MAX_RETRIES", "10"))
BEDROCK_BASE_DELAY = float(os.environ.get("BEDROCK_BASE_DELAY", "1.0"))
BEDROCK_MAX_DELAY = float(os.environ.get("BEDROCK_MAX_DELAY", "60.0"))
BEDROCK_APP_RETRIES = int(os.environ.get("BEDROCK_APP_RETRIES", "5"))
BEDROCK_APP_BASE_DELAY = float(os.environ.get("BEDROCK_APP_BASE_DELAY", "2.0"))
BEDROCK_APP_MAX_DELAY = float(os.environ.get("BEDROCK_APP_MAX_DELAY", "30.0"))

# Initialize AWS clients
dynamodb = boto3.resource("dynamodb")
sns_client = boto3.client("sns")

# Initialize cost limiter
cost_limiter = CostLimiter(DYNAMODB_TABLE_NAME, STAGE)

# Initialize response handler
response_handler = ResponseHandler(AGENT_RESPONSE_QUEUE_URL) if AGENT_RESPONSE_QUEUE_URL else None

# Set global rate limit from environment
from bedrock_config import set_rate_limit
set_rate_limit(BEDROCK_RATE_LIMIT)

# Log throttling configuration on startup
logger.info(
    f"Bedrock throttling configuration: "
    f"rate_limit={BEDROCK_RATE_LIMIT} req/min, "
    f"max_retries={BEDROCK_MAX_RETRIES}, "
    f"app_retries={BEDROCK_APP_RETRIES}"
)

# Agent State
class AgentState(BaseModel):
    """State for the AI agent"""
    messages: List[Any] = Field(default_factory=list)
    session_id: str = ""
    course_info: Dict[str, Any] = Field(default_factory=dict)
    current_time: str = ""


async def create_agent_graph():
    """Create the LangGraph agent workflow with MCP tools"""

    # Load course configuration
    course_config = load_course_config()

    # Validate MCP server URL is configured
    if not MCP_SERVER_URL:
        raise ValueError("MCP_SERVER_URL environment variable must be set")

    # Initialize MCP client to connect to remote MCP server
    mcp_client = MultiServerMCPClient(
        {
            "rez-agent-mcp": {
                "transport": "streamable_http",
                "url": MCP_SERVER_URL,
            }
        }
    )

    # Get tools from MCP server - returns LangChain-compatible tools
    tools = await mcp_client.get_tools()

    logger.info(f"Loaded {len(tools)} tools from MCP server: {[tool.name for tool in tools]}")

    # Initialize Bedrock LLM with tools
    # ChatBedrockConverse supports tool binding via bind_tools()
    # Create a custom boto3 client with optimized retry configuration
    bedrock_client = create_bedrock_client(
        region_name=BEDROCK_REGION,
        max_retries=BEDROCK_MAX_RETRIES,
        base_delay=BEDROCK_BASE_DELAY,
        max_delay=BEDROCK_MAX_DELAY
    )

    llm = ChatBedrockConverse(
        model_id=BEDROCK_MODEL_ID,
        region_name=BEDROCK_REGION,
        temperature=BEDROCK_TEMPERATURE,
        max_tokens=BEDROCK_MAX_TOKENS,
        client=bedrock_client  # Use our configured client
    )

    # Bind tools to LLM
    llm_with_tools = llm.bind_tools(tools)

    # Get rate limiter instance
    rate_limiter = get_rate_limiter()

    # Define agent node
    def agent_node(state: AgentState) -> AgentState:
        """Main agent reasoning node"""
        logger.info(f"Agent node processing with {len(state.messages)} messages")

        # Only add system message if this is the first invocation (no messages yet)
        # or if the first message is not a SystemMessage
        messages = state.messages
        if not messages or not isinstance(messages[0], SystemMessage):
            system_msg = SystemMessage(content=f"""You are a helpful golf reservation assistant.
Current date and time: {state.current_time}

Available Golf Courses:
{json.dumps(state.course_info, indent=2)}

You can help users with:
1. Checking their existing golf reservations
2. Searching for available tee times
3. Booking tee times
4. Getting weather forecasts for golf courses
5. Sending push notifications

Always be friendly, clear, and confirm actions with users before booking.
When searching for tee times, ask for the date, time range, and number of players if not provided.
""")
            messages = [system_msg] + messages
            logger.info("Added system message (first invocation)")
        else:
            logger.info("System message already present, reusing existing messages")

        # Debug: Log message structure for troubleshooting
        logger.info(f"Message sequence before LLM invocation: {[type(msg).__name__ for msg in messages]}")

        # Invoke LLM with tools (with rate limiting and retry logic)

        # Acquire rate limit token before making request
        if not rate_limiter.acquire(timeout=30.0):
            logger.error("Rate limiter timeout - too many concurrent requests")
            raise Exception("Rate limit exceeded: too many concurrent requests to Bedrock")

        # Wrap the LLM invocation with exponential backoff for throttling errors
        @with_exponential_backoff(
            max_retries=BEDROCK_APP_RETRIES,
            base_delay=BEDROCK_APP_BASE_DELAY,
            max_delay=BEDROCK_APP_MAX_DELAY
        )
        def invoke_llm():
            return llm_with_tools.invoke(messages)

        try:
            response = invoke_llm()
            logger.info(f"Invoking LLM with :{messages}")
        except Exception as e:
            logger.error(f"Error invoking LLM after retries: {e}", exc_info=True)
            logger.info(e)
            # Create a user-friendly error response
            error_msg = (
                "I'm experiencing an issue right now. "
                "Please try again later."
            )
            response = AIMessage(content=error_msg)

        # Update state with the new messages list (including system message if it was added)
        # This ensures the system message persists across tool invocations
        state.messages = messages

        # Add response to messages
        state.messages.append(response)

        logger.info(f"Agent node complete, message count: {len(state.messages)}")
        return state

    # Define custom tool node for Bedrock Converse API compatibility
    def tool_node(state: AgentState) -> AgentState:
        """Execute tools and return properly formatted results for Converse API"""
        last_message = state.messages[-1]

        if not hasattr(last_message, 'tool_calls') or not last_message.tool_calls:
            logger.info("No tool calls to execute")
            return state

        logger.info(f"Executing {len(last_message.tool_calls)} tool calls")
        logger.info(f"Message count before tool execution: {len(state.messages)}")

        # Create a mapping of tool names to tool functions
        tools_by_name = {tool.name: tool for tool in tools}

        # Execute each tool call and create tool result messages
        for tool_call in last_message.tool_calls:
            tool_name = tool_call.get('name')
            tool_args = tool_call.get('args', {})
            tool_call_id = tool_call.get('id')

            logger.info(f"Executing tool: {tool_name} with args: {tool_args}")

            try:
                # Get the tool and execute it
                tool_func = tools_by_name.get(tool_name)
                if tool_func:
                    result = tool_func.invoke(tool_args)
                    logger.info(f"Tool {tool_name} result: {result}")

                    # Create a ToolMessage with the result
                    tool_message = ToolMessage(
                        content=str(result),
                        tool_call_id=tool_call_id,
                        name=tool_name
                    )
                    state.messages.append(tool_message)
                else:
                    logger.error(f"Tool not found: {tool_name}")
                    # Add error message
                    tool_message = ToolMessage(
                        content=f"Error: Tool {tool_name} not found",
                        tool_call_id=tool_call_id,
                        name=tool_name
                    )
                    state.messages.append(tool_message)
            except Exception as e:
                logger.error(f"Error executing tool {tool_name}: {e}", exc_info=True)
                # Add error message
                tool_message = ToolMessage(
                    content=f"Error executing tool: {str(e)}",
                    tool_call_id=tool_call_id,
                    name=tool_name
                )
                state.messages.append(tool_message)

        logger.info(f"Tool execution complete, message count: {len(state.messages)}")
        logger.info(f"Message sequence after tool execution: {[type(msg).__name__ for msg in state.messages[-5:]]}")
        return state

    # Should continue to tools or end?
    def should_continue(state: AgentState) -> str:
        """Determine if we should continue to tools or end"""
        last_message = state.messages[-1]

        # If there are tool calls, continue to tools
        if hasattr(last_message, 'tool_calls') and last_message.tool_calls:
            logger.info(f"Continuing to tools: {len(last_message.tool_calls)} tool calls")
            return "tools"

        # Otherwise, end
        logger.info("Ending agent workflow")
        return "end"

    # Build graph
    workflow = StateGraph(AgentState)

    # Add nodes
    workflow.add_node("agent", agent_node)
    workflow.add_node("tools", tool_node)

    # Set entry point
    workflow.set_entry_point("agent")

    # Add conditional edges
    workflow.add_conditional_edges(
        "agent",
        should_continue,
        {
            "tools": "tools",
            "end": END
        }
    )

    # After tools, always go back to agent
    workflow.add_edge("tools", "agent")

    # Compile graph
    return workflow.compile()


# Initialize agent graph
agent_graph = None


async def get_agent():
    """Get or create agent graph (lazy async initialization)"""
    global agent_graph
    if agent_graph is None:
        agent_graph = await create_agent_graph()
    return agent_graph


def get_or_create_session(session_id: str) -> Dict[str, Any]:
    """Get or create an agent session"""
    table = dynamodb.Table(SESSION_TABLE_NAME)

    try:
        response = table.get_item(Key={"session_id": session_id})
        if "Item" in response:
            return response["Item"]
    except Exception as e:
        logger.error(f"Error retrieving session: {e}")

    # Create new session
    session = {
        "session_id": session_id,
        "created_at": datetime.utcnow().isoformat(),
        "messages": [],
    }

    try:
        table.put_item(Item=session)
    except Exception as e:
        logger.error(f"Error creating session: {e}")

    return session


def save_session(session_id: str, messages: List[Dict]) -> None:
    """Save session to DynamoDB"""
    table = dynamodb.Table(SESSION_TABLE_NAME)

    try:
        table.update_item(
            Key={"session_id": session_id},
            UpdateExpression="SET messages = :messages, updated_at = :updated_at",
            ExpressionAttributeValues={
                ":messages": messages,
                ":updated_at": datetime.utcnow().isoformat(),
            }
        )
    except Exception as e:
        logger.error(f"Error saving session: {e}")


async def async_lambda_handler(event: Dict[str, Any], context: Any) -> Dict[str, Any]:
    """
    Async Lambda handler for AI agent with MCP integration.
    Handles API Gateway requests with LangChain MCP Adapter for tool execution.
    """
    logger.info(f"Received event: {json.dumps(event)}")

    try:
        # Check if requesting agent card (for A2A discovery)
        request_path = event.get("rawPath", "")
        if request_path == "/agent/card" or request_path == "/agent/.well-known/agent-card":
            try:
                with open("agent_card.json", "r") as f:
                    agent_card = json.load(f)
                return {
                    "statusCode": 200,
                    "headers": {
                        "Content-Type": "application/json",
                        "Access-Control-Allow-Origin": "*",
                    },
                    "body": json.dumps(agent_card)
                }
            except Exception as e:
                logger.error(f"Error loading agent card: {e}")
                return {
                    "statusCode": 500,
                    "body": json.dumps({"error": "Failed to load agent card"})
                }

        # Serve UI interface
        if request_path == "/agent/ui":
            try:
                with open("ui/index.html", "r") as f:
                    ui_html = f.read()
                return {
                    "statusCode": 200,
                    "headers": {
                        "Content-Type": "text/html",
                        "Access-Control-Allow-Origin": "*",
                    },
                    "body": ui_html
                }
            except Exception as e:
                logger.error(f"Error loading UI: {e}")
                return {
                    "statusCode": 500,
                    "body": json.dumps({"error": "Failed to load UI"})
                }

        # Parse request
        body = json.loads(event.get("body", "{}"))
        user_message = body.get("message", "")
        session_id = body.get("session_id", f"session_{datetime.utcnow().timestamp()}")

        # Check if this is a cost usage query
        if user_message.lower() in ["cost", "usage", "spending", "budget"]:
            usage = cost_limiter.get_current_usage()
            return {
                "statusCode": 200,
                "headers": {
                    "Content-Type": "application/json",
                    "Access-Control-Allow-Origin": "*",
                },
                "body": json.dumps({
                    "session_id": session_id,
                    "message": f"Current Bedrock usage today:\n"
                               f"- Cost: ${usage['total_cost']:.2f} / ${usage['daily_cap']:.2f}\n"
                               f"- Remaining budget: ${usage['remaining_budget']:.2f}\n"
                               f"- Requests: {usage['request_count']}\n"
                               f"- Tokens: {usage['input_tokens']} input, {usage['output_tokens']} output\n"
                               f"- Resets at: {usage['reset_time']}",
                    "usage": usage,
                })
            }

        if not user_message:
            return {
                "statusCode": 400,
                "body": json.dumps({"error": "Message is required"})
            }

        # Check spending cap BEFORE making LLM call
        allowed, cap_message, cost_info = cost_limiter.check_and_update_cost(
            estimated_input_tokens=len(user_message.split()) * 1.5,  # Rough estimate
            estimated_output_tokens=2000  # Conservative estimate
        )

        if not allowed:
            logger.warning(f"Request blocked due to spending cap: {cap_message}")
            return {
                "statusCode": 429,  # Too Many Requests
                "headers": {
                    "Content-Type": "application/json",
                    "Access-Control-Allow-Origin": "*",
                    "Retry-After": "86400",  # Retry after 24 hours
                },
                "body": json.dumps({
                    "error": "Daily spending limit reached",
                    "message": cap_message,
                    "cost_info": cost_info,
                })
            }

        # Load course configuration
        course_config = load_course_config()

        # Get or create session
        session = get_or_create_session(session_id)

        # Create initial state
        messages = []

        # Restore previous messages from session
        if session.get("messages"):
            for msg in session["messages"]:
                if msg["role"] == "user":
                    messages.append(HumanMessage(content=msg["content"]))
                elif msg["role"] == "assistant":
                    messages.append(AIMessage(content=msg["content"]))

        # Add new user message
        messages.append(HumanMessage(content=user_message))

        # Create agent state
        state = AgentState(
            messages=messages,
            session_id=session_id,
            course_info=course_config,
            current_time=datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S UTC")
        )

        # Run agent with MCP tools (synchronous execution via remote MCP server)
        agent = await get_agent()
        result = await agent.ainvoke(state)

        logger.info("MCP tools executed successfully - synchronous execution, results in messages")

        # Extract final response
        final_message = result['messages'][-1]
        response_content = final_message.content if hasattr(final_message, 'content') else str(final_message)

        # Update actual cost based on token usage (if available from response metadata)
        # Note: LangChain/Bedrock should provide token counts in response metadata
        # This is a placeholder - actual implementation depends on LangChain response structure
        try:
            # Try to get actual token counts from LLM response
            # This varies by LangChain version and provider
            if hasattr(final_message, 'response_metadata'):
                metadata = final_message.response_metadata
                input_tokens = metadata.get('usage', {}).get('input_tokens', 0)
                output_tokens = metadata.get('usage', {}).get('output_tokens', 0)
                if input_tokens and output_tokens:
                    cost_limiter.update_actual_cost(input_tokens, output_tokens)
                    logger.info(f"Updated actual cost: {input_tokens} input, {output_tokens} output tokens")
        except Exception as e:
            logger.warning(f"Could not update actual cost: {e}")
            # Continue - we already tracked estimated cost

        # Save session with updated messages
        session_messages = []
        for msg in result['messages']:
            if isinstance(msg, HumanMessage):
                session_messages.append({"role": "user", "content": msg.content})
            elif isinstance(msg, AIMessage):
                session_messages.append({"role": "assistant", "content": msg.content})

        save_session(session_id, session_messages)

        # Return response
        return {
            "statusCode": 200,
            "headers": {
                "Content-Type": "application/json",
                "Access-Control-Allow-Origin": "*",
            },
            "body": json.dumps({
                "session_id": session_id,
                "message": response_content,
            })
        }

    except Exception as e:
        logger.error(f"Error processing request: {e}", exc_info=True)

        # Determine appropriate status code and message based on error type
        error_message = str(e)
        status_code = 500

        # Check if it's a throttling/rate limit error
        if "ThrottlingException" in error_message or "TooManyRequests" in error_message:
            status_code = 429
            error_message = (
                "The service is experiencing high traffic. "
                "Please wait a moment and try again."
            )
        elif "Rate limit" in error_message:
            status_code = 429
            error_message = (
                "Too many requests. Please wait a moment before trying again."
            )
        elif "timeout" in error_message.lower():
            status_code = 504
            error_message = "Request timed out. Please try again."

        return {
            "statusCode": status_code,
            "headers": {
                "Content-Type": "application/json",
                "Access-Control-Allow-Origin": "*",
                "Retry-After": "60" if status_code == 429 else None,
            },
            "body": json.dumps({
                "error": error_message,
                "details": str(e) if status_code == 500 else None
            })
        }


def lambda_handler(event: Dict[str, Any], context: Any) -> Dict[str, Any]:
    """
    Synchronous Lambda handler wrapper for async MCP agent.
    AWS Lambda requires a synchronous entry point.
    """
    import asyncio

    # Run the async handler in an event loop
    loop = asyncio.get_event_loop()
    if loop.is_running():
        # If loop is already running (shouldn't happen in Lambda), create new one
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)

    return loop.run_until_complete(async_lambda_handler(event, context))
