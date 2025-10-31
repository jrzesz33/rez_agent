# MCP Server Architecture Design

## Executive Summary

This document outlines the technical architecture for adding Model Context Protocol (MCP) server capabilities to the rez_agent system. The implementation includes an AWS Lambda-based MCP server, a stdio client for Claude Desktop, and integration with existing golf course and weather APIs.

## 1. System Architecture Overview

### 1.1 Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Claude Desktop                              │
│                    (MCP Client Host)                             │
└────────────────────┬────────────────────────────────────────────┘
                     │ stdio (JSON-RPC)
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│              MCP Stdio Client (Local Process)                    │
│                   (Go Binary)                                    │
│  • JSON-RPC stdio handler                                        │
│  • HTTP client for Lambda                                        │
│  • Async response polling (SSE/Long-poll)                        │
└────────────────────┬────────────────────────────────────────────┘
                     │ HTTPS (JSON-RPC over HTTP)
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                    API Gateway HTTP API                          │
│                  POST /mcp                                       │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                 MCP Lambda Function (Go)                         │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  JSON-RPC Server                                        │    │
│  │  • Tool Registry                                        │    │
│  │  • Request Router                                       │    │
│  │  • Response Correlation                                 │    │
│  └─────────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  MCP Tools Implementation                               │    │
│  │  • send_push_notification                               │    │
│  │  • golf_get_reservations                                │    │
│  │  • golf_search_tee_times                                │    │
│  │  • golf_book_tee_time                                   │    │
│  │  • get_weather                                          │    │
│  └─────────────────────────────────────────────────────────┘    │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ├─► SNS (mcp-responses topic)
                     ├─► DynamoDB (message tracking)
                     ├─► External Golf API (HTTPS)
                     ├─► External Weather API (HTTPS)
                     └─► ntfy.sh (push notifications)
```

### 1.2 Asynchronous Response Flow

For long-running operations (e.g., booking a tee time):

```
1. Client sends JSON-RPC request → Lambda
2. Lambda accepts request, returns requestId immediately
3. Lambda processes async in background (or via SNS → SQS → WebAction Lambda)
4. Result published to SNS mcp-responses topic
5. Client polls /mcp/responses/{requestId} or uses SSE
6. Client receives result and returns to Claude Desktop
```

## 2. MCP Protocol Implementation

### 2.1 Transport Layer

**Primary Transport: HTTP with SSE**
- Lambda functions cannot use stdio transport (designed for subprocesses)
- HTTP transport allows:
  - API Gateway integration
  - Scalable, stateless serverless architecture
  - SSE for async response streaming
  - Standard HTTPS security

**Stdio Transport (Client Side)**
- MCP Stdio Client communicates with Claude Desktop via stdio
- Translates stdio JSON-RPC to HTTP JSON-RPC
- Maintains request/response correlation

### 2.2 JSON-RPC Message Format

All messages follow JSON-RPC 2.0 specification:

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": "uuid-1234",
  "method": "tools/call",
  "params": {
    "name": "golf_search_tee_times",
    "arguments": {
      "date": "2025-11-01",
      "time_range_start": "08:00",
      "time_range_end": "12:00",
      "players": 4
    }
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": "uuid-1234",
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Found 3 available tee times:\n1. 8:30 AM - $65\n2. 10:00 AM - $75\n3. 11:30 AM - $70"
      }
    ]
  }
}
```

**Async Response (Initial):**
```json
{
  "jsonrpc": "2.0",
  "id": "uuid-1234",
  "result": {
    "async": true,
    "requestId": "req-5678",
    "statusUrl": "/mcp/status/req-5678"
  }
}
```

## 3. Technical Design Details

### 3.1 MCP Lambda Function Design

**File Structure:**
```
cmd/mcp/
  ├── main.go              # Lambda handler entrypoint
  └── server.go            # MCP server initialization

internal/mcp/
  ├── server/
  │   ├── jsonrpc.go       # JSON-RPC 2.0 server
  │   ├── transport.go     # HTTP transport handling
  │   └── correlation.go   # Request/response correlation
  ├── tools/
  │   ├── registry.go      # Tool registration
  │   ├── notification.go  # send_push_notification tool
  │   ├── golf.go          # Golf course tools
  │   └── weather.go       # Weather tool
  └── protocol/
      ├── types.go         # MCP protocol types
      ├── messages.go      # JSON-RPC message types
      └── errors.go        # MCP error codes

tools/mcp-client/
  ├── main.go              # Stdio client entrypoint
  ├── stdio.go             # Stdio JSON-RPC handler
  ├── http_client.go       # HTTP client for Lambda
  └── config.go            # Configuration (Lambda URL, auth)
```

**Lambda Environment Variables:**
```
MCP_SERVER_NAME=rez-agent-mcp
MCP_SERVER_VERSION=1.0.0
DYNAMODB_TABLE_NAME=rez-agent-messages-{stage}
MCP_TOPIC_ARN=arn:aws:sns:...:rez-agent-mcp-responses-{stage}
GOLF_SECRET_NAME=rez-agent/golf/credentials-{stage}
WEATHER_API_KEY_SECRET=rez-agent/weather/api-key-{stage}
NTFY_URL=https://ntfy.sh/rzesz-alerts
STAGE=dev
```

### 3.2 MCP Tool Definitions

#### Tool: send_push_notification

```json
{
  "name": "send_push_notification",
  "description": "Send a push notification via ntfy.sh",
  "inputSchema": {
    "type": "object",
    "properties": {
      "title": {"type": "string", "description": "Notification title"},
      "message": {"type": "string", "description": "Notification message"},
      "priority": {"type": "string", "enum": ["low", "default", "high"], "default": "default"}
    },
    "required": ["message"]
  }
}
```

#### Tool: golf_get_reservations

```json
{
  "name": "golf_get_reservations",
  "description": "Get current golf course reservations for the user",
  "inputSchema": {
    "type": "object",
    "properties": {
      "user_id": {"type": "string", "description": "User identifier"},
      "date_from": {"type": "string", "format": "date", "description": "Start date (YYYY-MM-DD)"},
      "date_to": {"type": "string", "format": "date", "description": "End date (YYYY-MM-DD)"}
    },
    "required": ["user_id"]
  }
}
```

#### Tool: golf_search_tee_times

```json
{
  "name": "golf_search_tee_times",
  "description": "Search for available tee times and optionally book the earliest one",
  "inputSchema": {
    "type": "object",
    "properties": {
      "date": {"type": "string", "format": "date", "description": "Date to search (YYYY-MM-DD)"},
      "time_range_start": {"type": "string", "description": "Earliest time (HH:MM)"},
      "time_range_end": {"type": "string", "description": "Latest time (HH:MM)"},
      "players": {"type": "integer", "minimum": 1, "maximum": 4, "description": "Number of players"},
      "auto_book": {"type": "boolean", "default": false, "description": "Automatically book earliest available time"}
    },
    "required": ["date", "players"]
  }
}
```

#### Tool: golf_book_tee_time

```json
{
  "name": "golf_book_tee_time",
  "description": "Book a specific tee time",
  "inputSchema": {
    "type": "object",
    "properties": {
      "tee_time_id": {"type": "string", "description": "Tee time identifier from search"},
      "date": {"type": "string", "format": "date"},
      "time": {"type": "string", "description": "Time (HH:MM)"},
      "players": {"type": "integer", "minimum": 1, "maximum": 4}
    },
    "required": ["tee_time_id", "date", "time", "players"]
  }
}
```

#### Tool: get_weather

```json
{
  "name": "get_weather",
  "description": "Get current weather and forecast for a location",
  "inputSchema": {
    "type": "object",
    "properties": {
      "location": {"type": "string", "description": "City name or coordinates"},
      "units": {"type": "string", "enum": ["metric", "imperial"], "default": "imperial"}
    },
    "required": ["location"]
  }
}
```

### 3.3 Stdio Client Design

**Configuration File: `~/.config/rez-agent-mcp/config.json`**
```json
{
  "mcp_server_url": "https://api-endpoint.execute-api.us-east-1.amazonaws.com/mcp",
  "auth_token": "optional-jwt-token",
  "stage": "dev",
  "poll_interval_ms": 1000,
  "request_timeout_ms": 30000
}
```

**Claude Desktop Configuration: `~/Library/Application Support/Claude/claude_desktop_config.json`**
```json
{
  "mcpServers": {
    "rez-agent": {
      "command": "/path/to/rez-agent-mcp-client",
      "args": [],
      "env": {
        "RZ_MCP_CONFIG": "/Users/username/.config/rez-agent-mcp/config.json"
      }
    }
  }
}
```

## 4. AWS Infrastructure Design

### 4.1 New Resources

**SNS Topic: mcp-responses**
```
Name: rez-agent-mcp-responses-{stage}
Purpose: Deliver asynchronous MCP tool responses
Subscriptions:
  - SQS: rez-agent-mcp-responses-queue (for polling)
  - HTTP: Optional webhook for SSE push
```

**SQS Queue: mcp-responses-queue**
```
Name: rez-agent-mcp-responses-queue-{stage}
VisibilityTimeout: 300s
MessageRetention: 14 days
DLQ: rez-agent-mcp-responses-dlq-{stage}
```

**Lambda Function: rez-agent-mcp**
```
Name: rez-agent-mcp-{stage}
Runtime: provided.al2 (Go)
Memory: 512 MB
Timeout: 30s (synchronous), 300s (async via SQS)
Handler: bootstrap
Triggers:
  - API Gateway: POST /mcp
  - API Gateway: GET /mcp/status/{requestId}
  - API Gateway: GET /mcp/stream (SSE)
```

**DynamoDB Table: mcp-request-tracking**
```
Name: rez-agent-mcp-requests-{stage}
HashKey: request_id (String)
Attributes:
  - request_id: UUID
  - status: pending|processing|completed|failed
  - created_at: Timestamp
  - tool_name: String
  - result: String (JSON)
  - error: String
TTL: 24 hours (ttl attribute)
```

### 4.2 IAM Permissions

**MCP Lambda Role Permissions:**
```yaml
Permissions:
  - SNS:Publish (mcp-responses topic, notifications topic)
  - DynamoDB:PutItem, GetItem, UpdateItem (messages table, mcp-requests table)
  - SecretsManager:GetSecretValue (golf credentials, weather API key)
  - Logs:CreateLogGroup, CreateLogStream, PutLogEvents
  - XRay:PutTraceSegments, PutTelemetryRecords
```

### 4.3 API Gateway Routes

```
POST   /mcp                      → MCP Lambda (JSON-RPC endpoint)
GET    /mcp/status/{requestId}   → MCP Lambda (polling endpoint)
GET    /mcp/stream                → MCP Lambda (SSE endpoint)
GET    /mcp/info                  → MCP Lambda (server info)
```

## 5. External API Integration

### 5.1 Golf Course API

**Provider Selection:** TBD (Requirements gather during implementation)
- Options: GolfNow API, Tee Times API, EZLinks, Supreme Golf
- Requirements:
  - OAuth2 authentication
  - Tee time search/booking capabilities
  - User reservation retrieval
  - Rate limiting: ~100 req/hour

**Secrets Management:**
```json
{
  "secret_name": "rez-agent/golf/credentials-dev",
  "value": {
    "client_id": "...",
    "client_secret": "...",
    "api_base_url": "https://api.golfprovider.com",
    "user_id": "..."
  }
}
```

### 5.2 Weather API

**Provider: OpenWeatherMap**
- Free tier: 1000 calls/day
- Current weather + 5-day forecast
- Simple API key authentication

**Secrets Management:**
```json
{
  "secret_name": "rez-agent/weather/api-key-dev",
  "value": "your-openweather-api-key"
}
```

### 5.3 Caching Strategy

**Weather Data:**
- Cache TTL: 30 minutes
- Cache key: `weather:{location}:{units}`
- Storage: DynamoDB or Lambda memory

**Golf Tee Times:**
- Cache TTL: 5 minutes (availability changes frequently)
- Cache key: `golf:teetimes:{date}:{time_range}`

## 6. Error Handling & Resilience

### 6.1 JSON-RPC Error Codes

```go
const (
    ErrCodeParseError     = -32700
    ErrCodeInvalidRequest = -32600
    ErrCodeMethodNotFound = -32601
    ErrCodeInvalidParams  = -32602
    ErrCodeInternalError  = -32603

    // MCP-specific errors
    ErrCodeToolNotFound   = -32001
    ErrCodeToolExecution  = -32002
    ErrCodeAsyncTimeout   = -32003
    ErrCodeAuthFailure    = -32004
)
```

### 6.2 Retry Strategy

**External API Calls:**
- Max retries: 3
- Backoff: Exponential (1s, 2s, 4s)
- Circuit breaker: Open after 5 consecutive failures

**SNS Publishing:**
- Max retries: 2
- Backoff: 500ms, 1s

## 7. Security Design

### 7.1 Authentication

**Option 1: API Key (Initial Implementation)**
- Stdio client includes API key in HTTP headers
- Lambda validates against SSM Parameter Store

**Option 2: JWT (Future)**
- OAuth2 flow for user authentication
- JWT token passed in Authorization header

### 7.2 Input Validation

- JSON schema validation for all tool inputs
- SQL injection prevention (parameterized queries)
- XSS prevention (sanitize all outputs)
- Rate limiting: 100 req/hour per client

### 7.3 Secrets Management

- All API keys in AWS Secrets Manager
- Auto-rotation where supported
- Least privilege IAM policies

## 8. Observability & Monitoring

### 8.1 CloudWatch Metrics

```
Namespace: RezAgent/MCP
Metrics:
  - ToolInvocationCount (by tool_name)
  - ToolExecutionTime (by tool_name)
  - ToolErrors (by tool_name, error_type)
  - AsyncRequestPending
  - AsyncRequestTimeout
  - ExternalAPILatency (by api_provider)
  - ExternalAPIErrors (by api_provider)
```

### 8.2 CloudWatch Logs

**Structured Logging Format:**
```json
{
  "timestamp": "2025-10-31T12:00:00Z",
  "level": "info",
  "message": "Tool executed successfully",
  "tool_name": "golf_search_tee_times",
  "request_id": "uuid-1234",
  "execution_time_ms": 1234,
  "user_id": "redacted"
}
```

### 8.3 X-Ray Tracing

Enable X-Ray for:
- End-to-end request tracing
- External API call latency
- SNS/SQS message flow
- DynamoDB operations

## 9. Scalability Considerations

### 9.1 Lambda Concurrency

- Reserved concurrency: 10 (prevent runaway costs)
- Provisioned concurrency: 1 (reduce cold starts for critical tools)

### 9.2 DynamoDB Capacity

- Billing mode: PAY_PER_REQUEST (on-demand)
- Expected throughput: <10 RCU/WCU
- Auto-scaling not needed initially

### 9.3 Rate Limiting

**Per-Client Limits:**
- 100 requests/hour (normal tools)
- 10 requests/hour (booking operations to prevent abuse)

**Implementation:**
- DynamoDB-based rate limiting table
- Key: client_id + tool_name + hour_bucket
- Increment counter on each request
- Reject if counter > limit

## 10. Testing Strategy

### 10.1 Unit Tests

- JSON-RPC message parsing/serialization
- Tool input validation
- Error handling
- Async correlation logic

### 10.2 Integration Tests

- End-to-end stdio client → Lambda → External API
- SNS/SQS async flow
- DynamoDB operations
- Secrets Manager integration

### 10.3 Load Tests

- Concurrent requests (target: 10 concurrent users)
- Async request handling (1000 pending requests)
- External API failure scenarios

## 11. Deployment Strategy

### 11.1 Phased Rollout

**Phase 1: Core Infrastructure (Week 1)**
- MCP Lambda with basic JSON-RPC server
- API Gateway routes
- SNS/SQS topics
- DynamoDB tables

**Phase 2: Tools Implementation (Week 2)**
- send_push_notification
- get_weather
- Golf course tools (search, book, reservations)

**Phase 3: Stdio Client (Week 3)**
- Local client binary
- Claude Desktop integration
- Async response handling

**Phase 4: Testing & Optimization (Week 4)**
- End-to-end testing
- Performance optimization
- Documentation

### 11.2 Makefile Updates

```makefile
build-mcp: ## Build MCP Lambda function
build-mcp-client: ## Build MCP stdio client binary
test-mcp: ## Run MCP unit tests
deploy-mcp: ## Deploy MCP infrastructure
```

## 12. Go Library Selection

### 12.1 MCP SDK

**Primary: `github.com/modelcontextprotocol/go-sdk`**
- Official SDK (maintained with Google)
- Stable release expected mid-2025
- Full protocol support

**Fallback: `github.com/mark3labs/mcp-go`**
- Community implementation
- High-level API
- Active development

### 12.2 JSON-RPC Library

**Option 1: Custom Implementation**
- Full control over request/response handling
- Lightweight (no external dependencies)
- Tailored to MCP requirements

**Option 2: `github.com/ethereum/go-ethereum/rpc`**
- Battle-tested JSON-RPC 2.0 implementation
- May be overkill for MCP use case

## 13. Success Metrics

### 13.1 Technical Metrics

- API Response Time: P95 < 500ms (sync), P95 < 30s (async)
- Error Rate: < 1%
- Availability: 99.9%
- Cold Start: < 2s

### 13.2 Business Metrics

- Tool adoption: >10 unique users in first month
- Tee time bookings: >5 successful bookings/week
- User satisfaction: NPS > 8

## 14. Open Questions & Decisions

1. **Golf Course API Provider**: Which API to integrate with?
   - Decision: Research during implementation, start with mock data

2. **Authentication Method**: API key vs JWT?
   - Decision: Start with API key, migrate to JWT later

3. **Async Response Delivery**: Polling vs SSE vs WebSocket?
   - Decision: Implement polling first, add SSE in Phase 2

4. **MCP Library**: Official SDK vs community implementation?
   - Decision: Start with official SDK if stable, fallback to mark3labs/mcp-go

## 15. References

- [MCP Specification](https://modelcontextprotocol.io/specification)
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [JSON-RPC 2.0](https://www.jsonrpc.org/specification)
- [AWS Lambda Best Practices](https://docs.aws.amazon.com/lambda/latest/dg/best-practices.html)
