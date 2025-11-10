# Message Schemas

## Overview

All messages in rez_agent follow a standardized schema with metadata and type-specific payloads. Messages are stored in DynamoDB and routed through SNS/SQS.

## Base Message Schema

Every message includes these core fields:

```json
{
  "id": "string",
  "version": "string",
  "created_date": "ISO8601 timestamp",
  "created_by": "string",
  "stage": "dev|stage|prod",
  "message_type": "string",
  "status": "created|queued|processing|completed|failed",
  "payload": {},
  "arguments": {},
  "auth_config": {},
  "updated_date": "ISO8601 timestamp",
  "error_message": "string",
  "retry_count": number
}
```

### Field Definitions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | String | Yes | Unique message identifier (format: `msg_YYYYMMDDHHmmss_nnnnn`) |
| `version` | String | Yes | Message schema version (currently `1.0`) |
| `created_date` | String | Yes | ISO8601 timestamp of message creation |
| `created_by` | String | Yes | System or user that created the message |
| `stage` | String | Yes | Deployment environment (`dev`, `stage`, `prod`) |
| `message_type` | String | Yes | Type of message (see [Message Types](#message-types)) |
| `status` | String | Yes | Current processing status |
| `payload` | Object | Yes | Message-specific content |
| `arguments` | Object | No | Additional parameters for message processing |
| `auth_config` | Object | No | Authentication configuration |
| `updated_date` | String | No | ISO8601 timestamp of last update |
| `error_message` | String | No | Error details if status is `failed` |
| `retry_count` | Number | Yes | Number of processing attempts (default: 0) |

### Status Values

| Status | Description |
|--------|-------------|
| `created` | Message has been created but not yet queued |
| `queued` | Message has been published to SNS/SQS |
| `processing` | Message is currently being processed by a Lambda |
| `completed` | Message has been successfully processed |
| `failed` | Message processing has failed |

## Message Types

### 1. Hello World

Simple test message for system verification.

**Type**: `hello_world`

**Payload Schema**:
```json
{
  "message": "string"
}
```

**Example**:
```json
{
  "id": "msg_20240115120000_123456",
  "version": "1.0",
  "created_date": "2024-01-15T12:00:00Z",
  "created_by": "test-user",
  "stage": "dev",
  "message_type": "hello_world",
  "status": "created",
  "payload": {
    "message": "Hello, World!"
  },
  "retry_count": 0
}
```

### 2. Notification

Push notification to ntfy.sh.

**Type**: `notify`

**Payload Schema**:
```json
{
  "message": "string",
  "title": "string (optional)",
  "priority": "number (optional, 1-5)",
  "tags": ["string"] (optional)
}
```

**Example**:
```json
{
  "id": "msg_20240115120000_234567",
  "version": "1.0",
  "created_date": "2024-01-15T12:00:00Z",
  "created_by": "scheduler",
  "stage": "prod",
  "message_type": "notify",
  "status": "queued",
  "payload": {
    "message": "Deployment completed successfully",
    "title": "System Update",
    "priority": 4,
    "tags": ["white_check_mark"]
  },
  "retry_count": 0
}
```

### 3. Agent Response

Response from AI agent for Claude integration.

**Type**: `agent_response`

**Payload Schema**:
```json
{
  "message": "string",
  "context": "object (optional)"
}
```

**Example**:
```json
{
  "id": "msg_20240115120000_345678",
  "version": "1.0",
  "created_date": "2024-01-15T12:00:00Z",
  "created_by": "web-action-processor",
  "stage": "prod",
  "message_type": "agent_response",
  "status": "created",
  "payload": {
    "message": "Found 3 available tee times for Saturday morning",
    "context": {
      "course": "Birdsfoot Golf Course",
      "date": "2024-01-20",
      "players": 4
    }
  },
  "retry_count": 0
}
```

### 4. Scheduled Task

Message created by EventBridge Scheduler for recurring tasks.

**Type**: `scheduled`

**Payload Schema**: Varies based on the scheduled task

**Example**:
```json
{
  "id": "msg_20240115120000_456789",
  "version": "1.0",
  "created_date": "2024-01-15T12:00:00Z",
  "created_by": "eventbridge-scheduler",
  "stage": "prod",
  "message_type": "scheduled",
  "status": "created",
  "payload": {
    "schedule_name": "daily-weather-check",
    "task_type": "weather_forecast"
  },
  "retry_count": 0
}
```

### 5. Web Action

HTTP REST API call with optional OAuth authentication.

**Type**: `web_action`

**Payload Schema**:
```json
{
  "version": "1.0",
  "url": "string",
  "action": "weather|golf",

  // Golf-specific fields
  "courseID": "number (optional)",
  "numberOfPlayers": "number (optional)",
  "days": "number (optional)",
  "maxResults": "number (optional)",
  "autoBook": "boolean (optional)",
  "startSearchTime": "string (optional, HH:mm format)",
  "endSearchTime": "string (optional, HH:mm format)",
  "teeSheetID": "number (optional)",

  // Authentication configuration
  "auth_config": {
    "type": "none|oauth_password|api_key|bearer",
    "secret_name": "string (optional)",
    "token_url": "string (optional)",
    "jwks_url": "string (optional)",
    "scope": "string (optional)",
    "headers": "object (optional)"
  }
}
```

**Arguments Schema** (for golf operations):
```json
{
  "operation": "search_tee_times|book_tee_time|fetch_reservations"
}
```

#### 5.1 Weather Action Example

```json
{
  "id": "msg_20240115120000_567890",
  "version": "1.0",
  "created_date": "2024-01-15T12:00:00Z",
  "created_by": "webapi",
  "stage": "prod",
  "message_type": "web_action",
  "status": "queued",
  "payload": {
    "version": "1.0",
    "action": "weather",
    "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast"
  },
  "retry_count": 0
}
```

#### 5.2 Golf Search Example

```json
{
  "id": "msg_20240115120000_678901",
  "version": "1.0",
  "created_date": "2024-01-15T12:00:00Z",
  "created_by": "mcp-server",
  "stage": "prod",
  "message_type": "web_action",
  "status": "processing",
  "payload": {
    "version": "1.0",
    "action": "golf",
    "url": "https://birdsfoot.cps.golf/api/tee-times",
    "courseID": 1,
    "numberOfPlayers": 4,
    "days": 7,
    "maxResults": 10,
    "startSearchTime": "07:00",
    "endSearchTime": "14:00"
  },
  "arguments": {
    "operation": "search_tee_times"
  },
  "retry_count": 0
}
```

#### 5.3 Golf Booking Example

```json
{
  "id": "msg_20240115120000_789012",
  "version": "1.0",
  "created_date": "2024-01-15T12:00:00Z",
  "created_by": "ai-agent",
  "stage": "prod",
  "message_type": "web_action",
  "status": "processing",
  "payload": {
    "version": "1.0",
    "action": "golf",
    "url": "https://birdsfoot.cps.golf/api/book",
    "courseID": 1,
    "numberOfPlayers": 4,
    "teeSheetID": 123456
  },
  "arguments": {
    "operation": "book_tee_time"
  },
  "retry_count": 0
}
```

### 6. Schedule Creation

Dynamic schedule creation/management request.

**Type**: `schedule_creation`

**Arguments Schema**:
```json
{
  "action": "create|delete|update",
  "name": "string",
  "schedule_expression": "string (cron or rate expression)",
  "timezone": "string (IANA timezone)",
  "target_type": "web_action|notification",
  "message_type": "string",
  "payload": "object"
}
```

**Example - Create Schedule**:
```json
{
  "id": "msg_20240115120000_890123",
  "version": "1.0",
  "created_date": "2024-01-15T12:00:00Z",
  "created_by": "webapi",
  "stage": "prod",
  "message_type": "schedule_creation",
  "status": "created",
  "payload": {},
  "arguments": {
    "action": "create",
    "name": "daily-golf-check-7am",
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
      "days": 7
    }
  },
  "retry_count": 0
}
```

**Example - Delete Schedule**:
```json
{
  "id": "msg_20240115120000_901234",
  "version": "1.0",
  "created_date": "2024-01-15T12:00:00Z",
  "created_by": "webapi",
  "stage": "prod",
  "message_type": "schedule_creation",
  "status": "created",
  "payload": {},
  "arguments": {
    "action": "delete",
    "name": "daily-golf-check-7am"
  },
  "retry_count": 0
}
```

## Authentication Configuration

The `auth_config` object specifies how to authenticate HTTP requests.

### Auth Types

#### 1. None (Public APIs)

```json
{
  "auth_config": {
    "type": "none"
  }
}
```

#### 2. OAuth Password Grant

```json
{
  "auth_config": {
    "type": "oauth_password",
    "secret_name": "rez-agent/golf/credentials-prod",
    "token_url": "https://birdsfoot.cps.golf/identityapi/connect/token",
    "jwks_url": "https://birdsfoot.cps.golf/identityapi/.well-known/openid-configuration/jwks",
    "scope": "openid profile email",
    "headers": {
      "client-id": "onlineresweb",
      "origin": "https://birdsfoot.cps.golf"
    }
  }
}
```

**Note**: In practice, OAuth configuration is loaded from `pkg/courses/courseInfo.yaml` based on `courseID`, so `auth_config` is often omitted in web action payloads.

#### 3. API Key

```json
{
  "auth_config": {
    "type": "api_key",
    "secret_name": "rez-agent/api-keys/service-name",
    "headers": {
      "X-API-Key": "{secret_value}"
    }
  }
}
```

#### 4. Bearer Token

```json
{
  "auth_config": {
    "type": "bearer",
    "secret_name": "rez-agent/tokens/service-name"
  }
}
```

## Web Action Results

When a web action is executed, a result record is created in DynamoDB.

**Schema**:
```json
{
  "id": "result_20240115120000_123456",
  "message_id": "msg_20240115120000_567890",
  "action": "weather|golf",
  "url": "string",
  "status": "completed|failed",
  "response_code": "number (HTTP status code)",
  "response_body": "string (truncated to 50KB)",
  "error_message": "string",
  "execution_time_ms": "number",
  "created_date": "ISO8601 timestamp",
  "ttl": "number (Unix timestamp, 3 days from creation)",
  "stage": "dev|stage|prod"
}
```

**Example - Successful Weather Request**:
```json
{
  "id": "result_20240115120000_123456",
  "message_id": "msg_20240115120000_567890",
  "action": "weather",
  "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast",
  "status": "completed",
  "response_code": 200,
  "response_body": "{\"properties\":{\"periods\":[...]}}",
  "execution_time_ms": 342,
  "created_date": "2024-01-15T12:00:01Z",
  "ttl": 1705579201,
  "stage": "prod"
}
```

**Example - Failed Request**:
```json
{
  "id": "result_20240115120000_234567",
  "message_id": "msg_20240115120000_678901",
  "action": "golf",
  "url": "https://birdsfoot.cps.golf/api/tee-times",
  "status": "failed",
  "error_message": "OAuth authentication failed: invalid credentials",
  "execution_time_ms": 1523,
  "created_date": "2024-01-15T12:00:02Z",
  "ttl": 1705579202,
  "stage": "prod"
}
```

## Schedule Metadata

Dynamic schedules created via the API are stored in DynamoDB.

**Schema**:
```json
{
  "id": "schedule_20240115120000_123456",
  "name": "string (unique)",
  "schedule_arn": "string (EventBridge Schedule ARN)",
  "schedule_expression": "string (cron or rate)",
  "timezone": "string (IANA timezone)",
  "target_type": "web_action|notification",
  "message_type": "string",
  "payload": "object",
  "arguments": "object",
  "created_date": "ISO8601 timestamp",
  "updated_date": "ISO8601 timestamp",
  "status": "active|inactive",
  "stage": "dev|stage|prod"
}
```

**Example**:
```json
{
  "id": "schedule_20240115120000_123456",
  "name": "daily-golf-check-7am",
  "schedule_arn": "arn:aws:scheduler:us-east-1:123456789012:schedule/default/daily-golf-check-7am",
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
    "days": 7
  },
  "arguments": {
    "operation": "search_tee_times"
  },
  "created_date": "2024-01-15T12:00:00Z",
  "updated_date": "2024-01-15T12:00:00Z",
  "status": "active",
  "stage": "prod"
}
```

## Validation Rules

### Message Validation

All messages are validated before processing:

1. **Version**: Must be `1.0`
2. **Stage**: Must be `dev`, `stage`, or `prod`
3. **Message Type**: Must be a valid type
4. **Payload**: Must match the schema for the message type

### Web Action Validation

Web actions have additional SSRF protection:

1. **URL Format**: Must be a valid HTTPS URL
2. **Host Allowlist**: Only whitelisted domains allowed
3. **IP Address**: Direct IP addresses rejected
4. **Private IPs**: Hostnames resolving to private IPs rejected
5. **Metadata Service**: AWS metadata service blocked

**Allowed Hosts**:
- `api.weather.gov`
- `birdsfoot.cps.golf`
- `totteridge.cps.golf`

### Schedule Validation

Schedule creation requires:

1. **Name**: Unique, non-empty string
2. **Schedule Expression**: Valid cron or rate expression
3. **Timezone**: Valid IANA timezone identifier
4. **Target Type**: Valid Lambda target
5. **Message Type**: Valid message type
6. **Payload**: Valid payload for message type

## Error Messages

Common validation errors:

```json
{
  "error": "unsupported payload version: 2.0"
}

{
  "error": "invalid action type: invalid_action"
}

{
  "error": "URL validation failed: host not in allowlist: evil.com"
}

{
  "error": "payload validation failed: URL is required"
}

{
  "error": "invalid stage: invalid_stage (must be dev, stage, or prod)"
}

{
  "error": "arguments are required for schedule creation messages"
}

{
  "error": "missing required arguments for schedule creation...name, schedule_expression, target_type, and timezone are required"
}
```

## Best Practices

1. **Always Include Version**: Set `version: "1.0"` in all payloads
2. **Use Correct Stage**: Match the deployment environment
3. **Validate Before Sending**: Check schema compliance client-side
4. **Include Context**: Use `arguments` for additional metadata
5. **Handle Errors**: Check `error_message` field on failures
6. **Monitor Status**: Track message `status` transitions
7. **Set Reasonable Timeouts**: Lambda timeout is 3 minutes max
8. **Redact Sensitive Data**: Never log credentials or tokens

## Schema Versioning

Current version: `1.0`

When introducing breaking changes:
1. Increment version number
2. Update validation logic
3. Maintain backward compatibility for old versions
4. Document migration path
