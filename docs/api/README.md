# API Documentation

## Table of Contents

1. [Overview](#overview)
2. [Authentication](#authentication)
3. [Base URL](#base-url)
4. [Endpoints](#endpoints)
5. [Request/Response Formats](#requestresponse-formats)
6. [Error Handling](#error-handling)
7. [Rate Limiting](#rate-limiting)
8. [Examples](#examples)

## Overview

The rez_agent Web API provides HTTP endpoints for creating messages and managing schedules. The API is hosted on AWS Lambda and exposed via API Gateway HTTP API.

### API Characteristics

- **Protocol**: HTTPS only
- **Format**: JSON
- **Style**: RESTful
- **Authentication**: None (currently public, behind API Gateway)
- **Region**: us-east-1

## Authentication

Currently, the API does not require authentication. In production environments, consider implementing:

- API Keys via API Gateway
- AWS IAM authentication
- OAuth 2.0
- Cognito User Pools

## Base URL

The base URL is output by Pulumi after deployment:

```bash
# Get the Web API URL
cd infrastructure
pulumi stack output WebApiUrl
```

**Format**: `https://{api-id}.execute-api.us-east-1.amazonaws.com`

## Endpoints

### 1. Create Message

Create a new message for processing.

**Endpoint**: `POST /api/messages`

**Request Body**:

```json
{
  "message_type": "web_action",
  "stage": "dev",
  "payload": {
    "version": "1.0",
    "action": "weather",
    "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast"
  },
  "arguments": {}
}
```

**Required Fields**:
- `message_type`: Type of message (see [Message Types](#message-types))
- `stage`: Deployment stage (`dev`, `stage`, `prod`)
- `payload`: Message-specific payload

**Optional Fields**:
- `arguments`: Additional parameters for message processing

**Response** (201 Created):

```json
{
  "success": true,
  "message_id": "msg_20240115120000_123456",
  "message": {
    "id": "msg_20240115120000_123456",
    "version": "1.0",
    "created_date": "2024-01-15T12:00:00Z",
    "created_by": "webapi",
    "stage": "dev",
    "message_type": "web_action",
    "status": "created",
    "payload": { ... },
    "retry_count": 0
  }
}
```

**Error Response** (400 Bad Request):

```json
{
  "success": false,
  "error": "invalid message type: invalid_type"
}
```

**cURL Example**:

```bash
curl -X POST https://your-api-url.execute-api.us-east-1.amazonaws.com/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "web_action",
    "stage": "dev",
    "payload": {
      "version": "1.0",
      "action": "weather",
      "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast"
    }
  }'
```

### 2. Create Schedule

Create or manage EventBridge schedules.

**Endpoint**: `POST /api/schedules`

**Request Body** (Create Schedule):

```json
{
  "action": "create",
  "name": "daily-weather-check",
  "schedule_expression": "cron(0 12 * * ? *)",
  "timezone": "America/New_York",
  "target_type": "web_action",
  "message_type": "web_action",
  "payload": {
    "version": "1.0",
    "action": "weather",
    "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast"
  }
}
```

**Required Fields**:
- `action`: Action to perform (`create`, `delete`, `update`)
- `name`: Unique schedule name
- `schedule_expression`: Cron or rate expression
- `timezone`: IANA timezone identifier
- `target_type`: Target Lambda function type
- `message_type`: Type of message to create
- `payload`: Message payload template

**Schedule Expression Formats**:

```
# Cron format: cron(Minutes Hours Day-of-month Month Day-of-week Year)
cron(0 12 * * ? *)        # Daily at noon UTC
cron(0 9 ? * MON-FRI *)   # Weekdays at 9 AM
cron(0 */6 * * ? *)       # Every 6 hours

# Rate format: rate(Value Unit)
rate(1 hour)              # Every hour
rate(30 minutes)          # Every 30 minutes
rate(1 day)               # Every day
```

**Timezone Examples**:
- `America/New_York`
- `America/Los_Angeles`
- `Europe/London`
- `UTC`

**Response** (201 Created):

```json
{
  "success": true,
  "message_id": "msg_20240115120000_789012",
  "schedule_name": "daily-weather-check"
}
```

**Error Response** (400 Bad Request):

```json
{
  "success": false,
  "error": "invalid schedule expression: cron(invalid)"
}
```

**cURL Example**:

```bash
curl -X POST https://your-api-url.execute-api.us-east-1.amazonaws.com/api/schedules \
  -H "Content-Type: application/json" \
  -d '{
    "action": "create",
    "name": "daily-golf-check",
    "schedule_expression": "cron(0 8 * * ? *)",
    "timezone": "America/New_York",
    "target_type": "web_action",
    "message_type": "web_action",
    "payload": {
      "version": "1.0",
      "action": "golf",
      "url": "https://birdsfoot.cps.golf/api/reservations",
      "courseID": 1,
      "numberOfPlayers": 4,
      "days": 7
    }
  }'
```

## Request/Response Formats

### Message Types

| Type | Description | Payload Schema |
|------|-------------|----------------|
| `hello_world` | Simple test message | `{ "message": "string" }` |
| `notify` | Notification message | `{ "message": "string", "title": "string" }` |
| `agent_response` | AI agent response | Any |
| `scheduled` | Scheduled task | Any |
| `web_action` | Web action request | [WebActionPayload](#webactionpayload) |
| `schedule_creation` | Schedule creation | See [Create Schedule](#2-create-schedule) |

### WebActionPayload

```json
{
  "version": "1.0",
  "action": "weather|golf",
  "url": "https://...",

  // Golf-specific fields
  "courseID": 1,
  "numberOfPlayers": 4,
  "days": 7,
  "maxResults": 10,
  "autoBook": false,
  "startSearchTime": "07:00",
  "endSearchTime": "14:00",

  // Authentication (optional, loaded from course config)
  "auth_config": {
    "type": "oauth_password",
    "secret_name": "rez-agent/golf/credentials-prod",
    "token_url": "https://...",
    "jwks_url": "https://...",
    "scope": "openid profile email"
  }
}
```

### Weather Action Payload

```json
{
  "version": "1.0",
  "action": "weather",
  "url": "https://api.weather.gov/gridpoints/{office}/{gridX},{gridY}/forecast"
}
```

**Example**:
```json
{
  "version": "1.0",
  "action": "weather",
  "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast"
}
```

### Golf Action Payload

```json
{
  "version": "1.0",
  "action": "golf",
  "url": "https://birdsfoot.cps.golf/api/...",
  "courseID": 1,
  "numberOfPlayers": 4,
  "days": 7,
  "maxResults": 10,
  "autoBook": false,
  "startSearchTime": "07:00",
  "endSearchTime": "14:00"
}
```

**Golf Operations** (set via `arguments.operation`):
- `search_tee_times`: Search for available tee times
- `book_tee_time`: Book a specific tee time
- `fetch_reservations`: Get upcoming reservations

**Example - Search Tee Times**:
```json
{
  "message_type": "web_action",
  "stage": "dev",
  "payload": {
    "version": "1.0",
    "action": "golf",
    "url": "https://birdsfoot.cps.golf/api/tee-times",
    "courseID": 1,
    "numberOfPlayers": 4,
    "days": 7,
    "maxResults": 10
  },
  "arguments": {
    "operation": "search_tee_times"
  }
}
```

**Example - Book Tee Time**:
```json
{
  "message_type": "web_action",
  "stage": "dev",
  "payload": {
    "version": "1.0",
    "action": "golf",
    "url": "https://birdsfoot.cps.golf/api/book",
    "courseID": 1,
    "numberOfPlayers": 4,
    "teeSheetID": 12345
  },
  "arguments": {
    "operation": "book_tee_time"
  }
}
```

## Error Handling

### HTTP Status Codes

| Status Code | Description |
|-------------|-------------|
| 200 | Success (for GET requests) |
| 201 | Created (for POST requests) |
| 400 | Bad Request (invalid input) |
| 401 | Unauthorized (if auth is enabled) |
| 404 | Not Found |
| 500 | Internal Server Error |
| 502 | Bad Gateway (Lambda error) |
| 503 | Service Unavailable |

### Error Response Format

```json
{
  "success": false,
  "error": "error message describing what went wrong",
  "details": {
    "field": "specific_field_name",
    "reason": "validation failure reason"
  }
}
```

### Common Errors

**Invalid Message Type**:
```json
{
  "success": false,
  "error": "invalid message type: unknown_type (must be hello_world, notify, agent_response, scheduled, web_action, or schedule_creation)"
}
```

**Invalid Web Action Payload**:
```json
{
  "success": false,
  "error": "payload validation failed: URL validation failed: host not in allowlist: evil.com"
}
```

**Missing Required Field**:
```json
{
  "success": false,
  "error": "missing required field: schedule_expression"
}
```

**Invalid Schedule Expression**:
```json
{
  "success": false,
  "error": "invalid cron expression: cron(invalid)"
}
```

## Rate Limiting

Currently, no rate limiting is enforced. For production use, consider:

- **API Gateway Throttling**: 10,000 requests/second
- **Usage Plans**: Tiered rate limits per API key
- **Lambda Concurrency Limits**: 1000 concurrent executions (default)

## Examples

### Example 1: Send Weather Notification

```bash
# Create a weather check message
curl -X POST https://your-api-url.execute-api.us-east-1.amazonaws.com/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "web_action",
    "stage": "prod",
    "payload": {
      "version": "1.0",
      "action": "weather",
      "url": "https://api.weather.gov/gridpoints/PBZ/95,64/forecast"
    }
  }'

# Response
{
  "success": true,
  "message_id": "msg_20240115120000_123456"
}
```

### Example 2: Schedule Daily Golf Check

```bash
# Create a daily schedule for golf tee times
curl -X POST https://your-api-url.execute-api.us-east-1.amazonaws.com/api/schedules \
  -H "Content-Type: application/json" \
  -d '{
    "action": "create",
    "name": "daily-golf-7am",
    "schedule_expression": "cron(0 7 * * ? *)",
    "timezone": "America/New_York",
    "target_type": "web_action",
    "message_type": "web_action",
    "payload": {
      "version": "1.0",
      "action": "golf",
      "url": "https://birdsfoot.cps.golf/api/tee-times",
      "courseID": 1,
      "numberOfPlayers": 4,
      "days": 7,
      "maxResults": 5,
      "startSearchTime": "07:00",
      "endSearchTime": "14:00"
    },
    "arguments": {
      "operation": "search_tee_times"
    }
  }'

# Response
{
  "success": true,
  "message_id": "msg_20240115070000_789012",
  "schedule_name": "daily-golf-7am"
}
```

### Example 3: Send Simple Notification

```bash
# Create a simple notification
curl -X POST https://your-api-url.execute-api.us-east-1.amazonaws.com/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "notify",
    "stage": "prod",
    "payload": {
      "message": "System deployment completed successfully",
      "title": "Deployment Status"
    }
  }'

# Response
{
  "success": true,
  "message_id": "msg_20240115120000_456789"
}
```

### Example 4: Using JavaScript/Node.js

```javascript
const axios = require('axios');

const API_URL = 'https://your-api-url.execute-api.us-east-1.amazonaws.com';

async function createWeatherCheck() {
  try {
    const response = await axios.post(`${API_URL}/api/messages`, {
      message_type: 'web_action',
      stage: 'prod',
      payload: {
        version: '1.0',
        action: 'weather',
        url: 'https://api.weather.gov/gridpoints/TOP/31,80/forecast'
      }
    });

    console.log('Message created:', response.data.message_id);
    return response.data;
  } catch (error) {
    console.error('Error:', error.response.data);
    throw error;
  }
}

async function createDailySchedule() {
  try {
    const response = await axios.post(`${API_URL}/api/schedules`, {
      action: 'create',
      name: 'daily-weather-noon',
      schedule_expression: 'cron(0 12 * * ? *)',
      timezone: 'America/New_York',
      target_type: 'web_action',
      message_type: 'web_action',
      payload: {
        version: '1.0',
        action: 'weather',
        url: 'https://api.weather.gov/gridpoints/TOP/31,80/forecast'
      }
    });

    console.log('Schedule created:', response.data.schedule_name);
    return response.data;
  } catch (error) {
    console.error('Error:', error.response.data);
    throw error;
  }
}
```

### Example 5: Using Python

```python
import requests
import json

API_URL = "https://your-api-url.execute-api.us-east-1.amazonaws.com"

def create_weather_check():
    """Create a weather check message."""
    response = requests.post(
        f"{API_URL}/api/messages",
        json={
            "message_type": "web_action",
            "stage": "prod",
            "payload": {
                "version": "1.0",
                "action": "weather",
                "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast"
            }
        }
    )

    if response.status_code == 201:
        data = response.json()
        print(f"Message created: {data['message_id']}")
        return data
    else:
        print(f"Error: {response.json()}")
        raise Exception(response.json()['error'])

def create_golf_schedule():
    """Create a daily golf tee time check schedule."""
    response = requests.post(
        f"{API_URL}/api/schedules",
        json={
            "action": "create",
            "name": "daily-golf-check",
            "schedule_expression": "cron(0 8 * * ? *)",
            "timezone": "America/New_York",
            "target_type": "web_action",
            "message_type": "web_action",
            "payload": {
                "version": "1.0",
                "action": "golf",
                "url": "https://birdsfoot.cps.golf/api/tee-times",
                "courseID": 1,
                "numberOfPlayers": 4,
                "days": 7
            },
            "arguments": {
                "operation": "search_tee_times"
            }
        }
    )

    if response.status_code == 201:
        data = response.json()
        print(f"Schedule created: {data['schedule_name']}")
        return data
    else:
        print(f"Error: {response.json()}")
        raise Exception(response.json()['error'])

if __name__ == "__main__":
    # Create a weather check
    create_weather_check()

    # Create a golf schedule
    create_golf_schedule()
```

## Security Considerations

### SSRF Protection

The Web Action Lambda includes comprehensive SSRF (Server-Side Request Forgery) protection:

1. **URL Allowlist**: Only whitelisted domains are accessible
2. **HTTPS Only**: HTTP requests are rejected
3. **IP Address Blocking**: Direct IP addresses are rejected
4. **Private IP Protection**: Hostnames resolving to private IPs are blocked
5. **Metadata Service Protection**: AWS metadata service is blocked

**Allowed Hosts** (see `internal/models/webaction.go`):
- `api.weather.gov`
- `birdsfoot.cps.golf`
- `totteridge.cps.golf`

### Sensitive Data Handling

- **Credentials**: Stored in AWS Secrets Manager, never in logs
- **OAuth Tokens**: Short-lived, cached for performance
- **JWT Verification**: All tokens verified with JWKS
- **Logging**: Sensitive fields redacted in logs

## MCP Server API

The MCP Server Lambda exposes a separate Lambda Function URL for Claude AI integration.

**Endpoint**: `https://{lambda-url}.lambda-url.us-east-1.on.aws/`

**Protocol**: [Model Context Protocol](https://github.com/anthropics/mcp)

**Available Tools**:
- `golf_search_tee_times`
- `golf_book_tee_time`
- `golf_fetch_reservations`
- `get_weather_forecast`
- `send_notification`

See [MCP Documentation](../mcp/README.md) for detailed MCP tool schemas.
