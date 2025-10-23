# Web Action Processor - Technical Design Document

**Version:** 1.0
**Date:** 2025-10-23
**Status:** Design Complete
**Author:** Backend System Architect

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Requirements Analysis](#requirements-analysis)
3. [Service Architecture](#service-architecture)
4. [Data Models & Schemas](#data-models--schemas)
5. [Message Flow Architecture](#message-flow-architecture)
6. [API Contracts & Integration](#api-contracts--integration)
7. [Security Architecture](#security-architecture)
8. [Error Handling & Resilience](#error-handling--resilience)
9. [Configuration Management](#configuration-management)
10. [Implementation Guidance](#implementation-guidance)
11. [Testing Strategy](#testing-strategy)
12. [Operational Considerations](#operational-considerations)

---

## 1. Executive Summary

### 1.1 Overview

This document provides the complete technical architecture for the **Web Action Processor**, a new Lambda-based service that extends the rez_agent event-driven messaging system to support HTTP REST API integration capabilities.

### 1.2 Key Capabilities

- **HTTP REST API Integration**: Fetch data from external REST APIs
- **OAuth 2.0 Authentication**: Support for password grant flow with secure credential management
- **Result Persistence**: Store API responses in DynamoDB with 3-day TTL
- **Event Publishing**: Emit completion events after successful API calls
- **Scheduled Actions**: EventBridge-triggered tasks for weather and golf reservations

### 1.3 Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Message Type** | New "web_action" type | Clear separation from existing message types |
| **Result Storage** | DynamoDB with TTL | Consistent with existing architecture, automatic cleanup |
| **HTTP Client** | Go net/http with custom wrapper | Standard library, full control over timeouts and retries |
| **Secrets Management** | AWS Secrets Manager | More secure than Parameter Store for credentials |
| **Authentication** | Pluggable auth strategies | Support OAuth, API keys, bearer tokens |
| **Processor Pattern** | Action registry with handlers | Extensible for future action types |

---

## 2. Requirements Analysis

### 2.1 Functional Requirements

#### FR-1: Web Action Message Type
- New message type to represent web-based actions
- Payload must include: URL, Action type, optional Arguments
- Support versioning for payload schema evolution

#### FR-2: Web Action Processor Lambda
- Consume messages from SQS queue (new dedicated queue)
- Execute HTTP GET/POST requests to external APIs
- Handle OAuth 2.0 authentication flows
- Persist results to DynamoDB with 3-day TTL
- Publish completion events to SNS

#### FR-3: Weather Notification Action
- **Schedule**: 5:00 AM EST daily
- **URL**: `https://api.weather.gov/gridpoints/PBZ/82,69/forecast`
- **Action**: Fetch and parse weather forecast
- **Argument**: Number of days (default: 2)
- **Output**: Notification with detailed forecast

#### FR-4: Golf Reservations Action
- **Schedule**: 5:15 AM EST daily
- **Authentication**: OAuth 2.0 password grant flow
- **Credentials**: Stored in AWS Secrets Manager
- **Steps**:
  1. Authenticate: POST to `/identityapi/connect/token`
  2. Fetch reservations: GET `/onlineapi/api/v1/onlinereservation/UpcomingReservation`
- **Output**: Notification with next 4 tee times (earliest first)

### 2.2 Non-Functional Requirements

#### NFR-1: Performance
- HTTP request timeout: 30 seconds (configurable)
- OAuth token caching: 50 minutes (tokens valid for 60 min)
- Lambda timeout: 5 minutes (matches processor)
- Result retrieval: < 500ms (p95)

#### NFR-2: Reliability
- Retry failed HTTP requests (3 attempts with exponential backoff)
- Handle transient network failures gracefully
- Dead Letter Queue for permanent failures
- Idempotent action execution

#### NFR-3: Security
- Secrets stored in AWS Secrets Manager (encrypted at rest)
- TLS 1.2+ for all HTTP connections
- OAuth tokens never logged or persisted
- IAM roles with least-privilege access

#### NFR-4: Observability
- Structured logging with correlation IDs
- CloudWatch metrics for HTTP requests, auth failures, action duration
- X-Ray distributed tracing
- Alarms for failed actions and DLQ messages

---

## 3. Service Architecture

### 3.1 System Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         EventBridge Scheduler                            │
│  ┌──────────────────────────┐    ┌──────────────────────────┐          │
│  │ Weather Schedule         │    │ Golf Schedule            │          │
│  │ cron(0 10 * * ? *)      │    │ cron(15 10 * * ? *)     │          │
│  │ (5 AM EST = 10 AM UTC)  │    │ (5:15 AM EST)           │          │
│  └────────────┬─────────────┘    └────────────┬─────────────┘          │
└───────────────┼──────────────────────────────┼────────────────────────┘
                │                              │
                │    Invoke Scheduler Lambda   │
                └──────────────┬───────────────┘
                               ▼
                ┌──────────────────────────────┐
                │  Scheduler Lambda            │
                │  - Create web_action message │
                │  - Populate URL, action,     │
                │    arguments                 │
                │  - Store in DynamoDB         │
                └──────────────┬───────────────┘
                               │
                               │ Publish SNS event
                               ▼
                ┌──────────────────────────────┐
                │  SNS Topic                   │
                │  rez-agent-web-actions-{env} │
                └──────────────┬───────────────┘
                               │
                               │ Fan-out to subscribers
                               ▼
                ┌──────────────────────────────┐
                │  SQS Queue                   │
                │  rez-agent-web-actions-{env} │
                │  - Visibility: 5 minutes     │
                │  - Batch: 1 message          │
                │  - DLQ after 3 retries       │
                └──────────────┬───────────────┘
                               │
                               │ Event source mapping
                               ▼
                ┌──────────────────────────────┐
                │  Web Action Processor Lambda │
                │  ┌────────────────────────┐  │
                │  │ 1. Parse web action    │  │
                │  │ 2. Get credentials     │  │
                │  │ 3. Execute HTTP request│  │
                │  │ 4. Store result        │  │
                │  │ 5. Publish completion  │  │
                │  └────────────────────────┘  │
                └──────────────┬───────────────┘
                               │
                    ┌──────────┴──────────┐
                    │                     │
                    ▼                     ▼
    ┌───────────────────────┐  ┌─────────────────────────┐
    │  DynamoDB             │  │  SNS Topic              │
    │  web-action-results   │  │  rez-agent-messages     │
    │  - TTL: 3 days        │  │  (existing)             │
    │  - PK: action_id      │  └──────────┬──────────────┘
    │  - Result payload     │             │
    └───────────────────────┘             │ Triggers notification
                                          ▼
                              ┌─────────────────────────┐
                              │  Message Processor      │
                              │  (existing)             │
                              │  → Notification Service │
                              │  → ntfy.sh              │
                              └─────────────────────────┘

External Integrations:
┌────────────────────────────┐  ┌─────────────────────────────┐
│  Weather.gov API           │  │  Birdsfoot Golf API         │
│  GET /gridpoints/.../...   │  │  POST /connect/token        │
│  (No auth required)        │  │  GET /UpcomingReservation   │
└────────────────────────────┘  └─────────────────────────────┘
```

### 3.2 Service Boundaries

#### 3.2.1 Web Action Processor Lambda

**Responsibilities:**
- Process web action messages from SQS
- Execute HTTP requests to external APIs
- Manage authentication flows (OAuth, API keys)
- Transform API responses into notifications
- Persist results with TTL
- Emit completion events

**Inputs:**
- SQS events containing web action messages
- AWS Secrets Manager credentials
- DynamoDB message records

**Outputs:**
- DynamoDB result records
- SNS completion events (triggers notification)
- CloudWatch logs and metrics

**Does NOT:**
- Perform message scheduling (Scheduler Lambda's responsibility)
- Send notifications directly (Message Processor's responsibility)
- Manage long-lived state (stateless processing)

#### 3.2.2 Integration with Existing Services

**Scheduler Lambda** (MODIFIED):
- Add support for creating `web_action` message types
- Schedule weather action at 5:00 AM EST
- Schedule golf action at 5:15 AM EST
- Payload includes: `url`, `action`, `arguments`

**Message Processor** (UNCHANGED):
- Continues to process notification requests
- Receives completion events from Web Action Processor
- Sends formatted notifications to ntfy.sh

**Notification Service** (UNCHANGED):
- No changes required

### 3.3 Technology Stack

| Component | Technology | Version |
|-----------|-----------|---------|
| Runtime | Go | 1.24 |
| Lambda Runtime | provided.al2 | - |
| HTTP Client | net/http | stdlib |
| AWS SDK | aws-sdk-go-v2 | latest |
| Logging | log/slog | stdlib |
| JSON Parsing | encoding/json | stdlib |
| Time Zones | time/tzdata | stdlib |

---

## 4. Data Models & Schemas

### 4.1 Web Action Message Type

#### 4.1.1 Message Model Extension

Add new message type to `/workspaces/rez_agent/internal/models/message.go`:

```go
const (
    // Existing types
    MessageTypeHelloWorld MessageType = "hello_world"
    MessageTypeManual     MessageType = "manual"
    MessageTypeScheduled  MessageType = "scheduled"

    // NEW: Web action message type
    MessageTypeWebAction  MessageType = "web_action"
)

// Update IsValid() method to include MessageTypeWebAction
```

#### 4.1.2 Web Action Payload Schema

Create `/workspaces/rez_agent/internal/models/web_action.go`:

```go
package models

import (
    "encoding/json"
    "fmt"
    "time"
)

// WebActionPayload represents the structured payload for web_action messages
type WebActionPayload struct {
    // Version for schema evolution (current: "1.0")
    Version string `json:"version"`

    // URL is the target endpoint (required)
    URL string `json:"url"`

    // Action identifies the action handler (required)
    // Examples: "fetch_weather", "fetch_golf_reservations"
    Action string `json:"action"`

    // Arguments is an optional key-value map of action-specific parameters
    Arguments map[string]interface{} `json:"arguments,omitempty"`

    // AuthConfig specifies authentication requirements (optional)
    AuthConfig *AuthConfig `json:"auth_config,omitempty"`
}

// AuthConfig defines authentication strategy
type AuthConfig struct {
    // Type: "none", "oauth_password", "bearer_token", "api_key"
    Type string `json:"type"`

    // SecretName references AWS Secrets Manager secret (for oauth, api_key)
    SecretName string `json:"secret_name,omitempty"`

    // TokenURL for OAuth flows
    TokenURL string `json:"token_url,omitempty"`

    // Scope for OAuth flows
    Scope string `json:"scope,omitempty"`

    // Additional headers required for auth
    Headers map[string]string `json:"headers,omitempty"`
}

// Validate checks if the payload is valid
func (p *WebActionPayload) Validate() error {
    if p.Version == "" {
        return fmt.Errorf("version is required")
    }
    if p.URL == "" {
        return fmt.Errorf("url is required")
    }
    if p.Action == "" {
        return fmt.Errorf("action is required")
    }

    // Validate auth config if present
    if p.AuthConfig != nil {
        validTypes := map[string]bool{
            "none":           true,
            "oauth_password": true,
            "bearer_token":   true,
            "api_key":        true,
        }
        if !validTypes[p.AuthConfig.Type] {
            return fmt.Errorf("invalid auth type: %s", p.AuthConfig.Type)
        }

        if p.AuthConfig.Type == "oauth_password" && p.AuthConfig.TokenURL == "" {
            return fmt.Errorf("token_url required for oauth_password auth")
        }
    }

    return nil
}

// ToJSON converts payload to JSON string
func (p *WebActionPayload) ToJSON() (string, error) {
    data, err := json.Marshal(p)
    if err != nil {
        return "", fmt.Errorf("failed to marshal payload: %w", err)
    }
    return string(data), nil
}

// ParseWebActionPayload parses JSON payload into WebActionPayload
func ParseWebActionPayload(payload string) (*WebActionPayload, error) {
    var p WebActionPayload
    if err := json.Unmarshal([]byte(payload), &p); err != nil {
        return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
    }

    if err := p.Validate(); err != nil {
        return nil, fmt.Errorf("invalid payload: %w", err)
    }

    return &p, nil
}

// NewWeatherActionPayload creates payload for weather action
func NewWeatherActionPayload(days int) *WebActionPayload {
    if days <= 0 {
        days = 2 // default
    }

    return &WebActionPayload{
        Version: "1.0",
        URL:     "https://api.weather.gov/gridpoints/PBZ/82,69/forecast",
        Action:  "fetch_weather",
        Arguments: map[string]interface{}{
            "days": days,
        },
        AuthConfig: &AuthConfig{
            Type: "none",
        },
    }
}

// NewGolfReservationsPayload creates payload for golf reservations action
func NewGolfReservationsPayload(golferID string, maxResults int) *WebActionPayload {
    if maxResults <= 0 {
        maxResults = 4 // default
    }

    return &WebActionPayload{
        Version: "1.0",
        URL:     "https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/UpcomingReservation",
        Action:  "fetch_golf_reservations",
        Arguments: map[string]interface{}{
            "golfer_id":   golferID,
            "page_size":   14,
            "current_page": 1,
            "max_results": maxResults,
        },
        AuthConfig: &AuthConfig{
            Type:       "oauth_password",
            TokenURL:   "https://birdsfoot.cps.golf/identityapi/connect/token",
            SecretName: "rez-agent/golf/credentials",
            Scope:      "openid profile onlinereservation sale inventory sh customer email recommend references",
            Headers: map[string]string{
                "client-id":    "onlineresweb",
                "content-type": "application/x-www-form-urlencoded",
            },
        },
    }
}
```

### 4.2 Web Action Result Schema

Create `/workspaces/rez_agent/internal/models/web_action_result.go`:

```go
package models

import (
    "time"
)

// WebActionResult represents the result of a web action stored in DynamoDB
type WebActionResult struct {
    // ActionID is the unique identifier (PK)
    ActionID string `json:"action_id" dynamodbav:"action_id"`

    // MessageID references the original message
    MessageID string `json:"message_id" dynamodbav:"message_id"`

    // Action is the action type executed
    Action string `json:"action" dynamodbav:"action"`

    // URL is the target endpoint called
    URL string `json:"url" dynamodbav:"url"`

    // Status: "success", "failed"
    Status string `json:"status" dynamodbav:"status"`

    // HTTPStatusCode from the API response
    HTTPStatusCode int `json:"http_status_code" dynamodbav:"http_status_code"`

    // ResponseBody is the raw API response (max 400KB)
    ResponseBody string `json:"response_body" dynamodbav:"response_body"`

    // TransformedResult is the processed/formatted result
    TransformedResult string `json:"transformed_result" dynamodbav:"transformed_result"`

    // ErrorMessage if status is "failed"
    ErrorMessage string `json:"error_message,omitempty" dynamodbav:"error_message,omitempty"`

    // ExecutedAt is when the action was executed
    ExecutedAt time.Time `json:"executed_at" dynamodbav:"executed_at"`

    // TTL for automatic deletion (Unix timestamp, 3 days from execution)
    TTL int64 `json:"ttl" dynamodbav:"ttl"`

    // Duration in milliseconds
    DurationMs int64 `json:"duration_ms" dynamodbav:"duration_ms"`

    // Stage (dev, stage, prod)
    Stage Stage `json:"stage" dynamodbav:"stage"`
}

// NewWebActionResult creates a new result record
func NewWebActionResult(messageID, action, url string, stage Stage) *WebActionResult {
    now := time.Now().UTC()
    ttl := now.Add(3 * 24 * time.Hour).Unix() // 3 days

    return &WebActionResult{
        ActionID:   generateActionID(now),
        MessageID:  messageID,
        Action:     action,
        URL:        url,
        ExecutedAt: now,
        TTL:        ttl,
        Stage:      stage,
    }
}

// generateActionID generates unique action ID
func generateActionID(t time.Time) string {
    return "action_" + t.Format("20060102150405") + "_" + fmt.Sprintf("%d", t.Nanosecond()%1000000)
}

// MarkSuccess updates result with success data
func (r *WebActionResult) MarkSuccess(statusCode int, responseBody, transformedResult string, duration time.Duration) {
    r.Status = "success"
    r.HTTPStatusCode = statusCode
    r.ResponseBody = responseBody
    r.TransformedResult = transformedResult
    r.DurationMs = duration.Milliseconds()
}

// MarkFailed updates result with failure data
func (r *WebActionResult) MarkFailed(errorMessage string, duration time.Duration) {
    r.Status = "failed"
    r.ErrorMessage = errorMessage
    r.DurationMs = duration.Milliseconds()
}
```

### 4.3 DynamoDB Table Schema

#### 4.3.1 Table: `rez-agent-web-action-results-{stage}`

```
Primary Key:
  - Partition Key (PK): action_id (String)
  - Sort Key (SK): executed_at (String - ISO 8601 format)

Attributes:
  - action_id: String (PK)
  - executed_at: String (SK)
  - message_id: String
  - action: String
  - url: String
  - status: String ("success" | "failed")
  - http_status_code: Number
  - response_body: String (max 400KB)
  - transformed_result: String
  - error_message: String (optional)
  - ttl: Number (Unix timestamp)
  - duration_ms: Number
  - stage: String

Global Secondary Indexes:
  1. message-id-index
     - PK: message_id
     - SK: executed_at
     - Projection: ALL
     - Use case: Look up results by original message

  2. action-executed-index
     - PK: action
     - SK: executed_at
     - Projection: ALL
     - Use case: Query results by action type

Time To Live (TTL):
  - Attribute: ttl
  - Enabled: true
  - Automatic deletion: 3 days after execution

Billing Mode: PAY_PER_REQUEST (on-demand)
```

### 4.4 SNS/SQS Message Formats

#### 4.4.1 SNS Published Message (from Scheduler)

```json
{
  "message_id": "msg_20251023050000_123456",
  "event_type": "message_created",
  "timestamp": "2025-10-23T10:00:00Z",
  "stage": "prod"
}
```

#### 4.4.2 SQS Message (consumed by Web Action Processor)

```json
{
  "Type": "Notification",
  "MessageId": "sns-msg-id-123",
  "TopicArn": "arn:aws:sns:us-east-1:123456789012:rez-agent-web-actions-prod",
  "Message": "{\"message_id\":\"msg_20251023050000_123456\",\"event_type\":\"message_created\",\"timestamp\":\"2025-10-23T10:00:00Z\",\"stage\":\"prod\"}",
  "Timestamp": "2025-10-23T10:00:00.000Z",
  "SignatureVersion": "1",
  "Signature": "...",
  "SigningCertURL": "..."
}
```

#### 4.4.3 Completion Event (published to existing SNS topic)

```json
{
  "message_id": "msg_20251023050015_789012",
  "event_type": "message_created",
  "timestamp": "2025-10-23T10:00:15Z",
  "stage": "prod"
}
```

This triggers the existing Message Processor → Notification flow.

---

## 5. Message Flow Architecture

### 5.1 End-to-End Flow

```
1. EventBridge Scheduler (5:00 AM EST)
   │
   ├─> Scheduler Lambda invoked
   │   ├─> Create Message record in DynamoDB
   │   │   - ID: msg_20251023100000_123
   │   │   - Type: web_action
   │   │   - Payload: {"version":"1.0","url":"...","action":"fetch_weather",...}
   │   │   - Status: created
   │   │
   │   ├─> Publish SNS event to web-actions topic
   │   │   - message_id: msg_20251023100000_123
   │   │
   │   └─> Update Message status to queued
   │
2. SNS Topic: rez-agent-web-actions-prod
   │
   ├─> Fan-out to SQS Queue
   │
3. SQS Queue: rez-agent-web-actions-prod
   │
   ├─> Lambda Event Source Mapping (batch size: 1)
   │
4. Web Action Processor Lambda
   │
   ├─> Parse SQS event → extract message_id
   │
   ├─> Fetch Message from DynamoDB (message_id)
   │
   ├─> Parse WebActionPayload from Message.Payload
   │
   ├─> Update Message status: processing
   │
   ├─> Execute Action Handler based on payload.Action
   │   │
   │   ├─> [fetch_weather]
   │   │   ├─> HTTP GET weather.gov/gridpoints/...
   │   │   ├─> Parse JSON response
   │   │   ├─> Extract N days forecast
   │   │   └─> Format notification text
   │   │
   │   ├─> [fetch_golf_reservations]
   │   │   ├─> Get credentials from Secrets Manager
   │   │   ├─> HTTP POST /connect/token (OAuth)
   │   │   ├─> Cache access token (50 min TTL)
   │   │   ├─> HTTP GET /UpcomingReservation (with Bearer token)
   │   │   ├─> Parse JSON response
   │   │   ├─> Sort by datetime, take first 4
   │   │   └─> Format notification text
   │   │
   │   └─> [future actions...]
   │
   ├─> Store WebActionResult in DynamoDB
   │   - action_id: action_20251023100002_456
   │   - message_id: msg_20251023100000_123
   │   - status: success
   │   - response_body: {...}
   │   - transformed_result: "Weather forecast for 2 days..."
   │   - ttl: 1729854000 (3 days)
   │
   ├─> Create notification Message in DynamoDB
   │   - ID: msg_20251023100002_789
   │   - Type: manual
   │   - Payload: "Weather forecast for 2 days..."
   │   - Status: created
   │
   ├─> Publish SNS event to messages topic
   │   - message_id: msg_20251023100002_789
   │
   ├─> Update original Message status: completed
   │
   └─> Delete SQS message (success)

5. Existing Flow (unchanged)
   │
   ├─> SQS Queue: rez-agent-messages-prod
   │
   ├─> Message Processor Lambda
   │
   ├─> Notification Service
   │
   └─> ntfy.sh notification sent
```

### 5.2 Failure Scenarios

#### 5.2.1 Transient HTTP Failure

```
Web Action Processor
│
├─> HTTP request fails (timeout, 5xx)
│
├─> Retry with exponential backoff (3 attempts)
│   - Attempt 1: immediate
│   - Attempt 2: +2s
│   - Attempt 3: +4s
│
├─> If all retries fail:
│   ├─> Store WebActionResult (status: failed)
│   ├─> Update Message status: failed
│   ├─> Return error → SQS retries message (up to 3 times)
│   └─> After 3 SQS retries → Dead Letter Queue
```

#### 5.2.2 OAuth Authentication Failure

```
Web Action Processor
│
├─> POST /connect/token fails (401, 403)
│
├─> Log auth failure
│
├─> Store WebActionResult (status: failed, error: "auth failed")
│
├─> Update Message status: failed
│
├─> Delete SQS message (permanent failure, no retry)
│
└─> CloudWatch alarm triggered
```

#### 5.2.3 DynamoDB Failure

```
Web Action Processor
│
├─> DynamoDB PutItem fails (throttling, service error)
│
├─> Log error with correlation ID
│
├─> Return error → SQS retries message
│
└─> After 3 SQS retries → Dead Letter Queue
```

---

## 6. API Contracts & Integration

### 6.1 Weather.gov API Integration

#### 6.1.1 Request

```
GET https://api.weather.gov/gridpoints/PBZ/82,69/forecast
Headers:
  User-Agent: rez-agent/1.0 (contact@example.com)
  Accept: application/geo+json
```

#### 6.1.2 Response (Abbreviated)

```json
{
  "type": "Feature",
  "properties": {
    "updated": "2025-10-23T09:00:00+00:00",
    "periods": [
      {
        "number": 1,
        "name": "Today",
        "startTime": "2025-10-23T06:00:00-04:00",
        "temperature": 72,
        "temperatureUnit": "F",
        "windSpeed": "5 to 10 mph",
        "shortForecast": "Partly Cloudy",
        "detailedForecast": "Partly cloudy, with a high near 72."
      },
      {
        "number": 2,
        "name": "Tonight",
        "startTime": "2025-10-23T18:00:00-04:00",
        "temperature": 55,
        "temperatureUnit": "F",
        "windSpeed": "5 mph",
        "shortForecast": "Clear",
        "detailedForecast": "Clear, with a low around 55."
      }
    ]
  }
}
```

#### 6.1.3 Transformed Notification Output

```
Weather Forecast (2 days):

Today: Partly cloudy, high 72°F
Partly cloudy, with a high near 72. Winds 5-10 mph.

Tonight: Clear, low 55°F
Clear, with a low around 55. Winds 5 mph.

Tomorrow: Sunny, high 75°F
Sunny skies. Winds 5-10 mph.

Tomorrow Night: Mostly Clear, low 58°F
Mostly clear. Winds light.
```

### 6.2 Birdsfoot Golf API Integration

#### 6.2.1 Authentication Request

```
POST https://birdsfoot.cps.golf/identityapi/connect/token
Headers:
  Content-Type: application/x-www-form-urlencoded
  Accept: application/json
  client-id: onlineresweb
  User-Agent: rez-agent/1.0

Body (form-encoded):
  grant_type=password
  scope=openid profile onlinereservation sale inventory sh customer email recommend references
  username={from_secrets}
  password={from_secrets}
  client_id=js1
  client_secret=v4secret
```

#### 6.2.2 Authentication Response

```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 3600,
  "token_type": "Bearer",
  "refresh_token": "...",
  "scope": "openid profile onlinereservation..."
}
```

#### 6.2.3 Reservations Request

```
GET https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/UpcomingReservation?golferId=91124&pageSize=14&currentPage=1
Headers:
  Authorization: Bearer {access_token}
  Accept: application/json
  client-id: onlineresweb
  User-Agent: rez-agent/1.0
```

#### 6.2.4 Reservations Response (Abbreviated)

```json
{
  "success": true,
  "data": {
    "items": [
      {
        "reservationId": 123456,
        "dateTime": "2025-10-25T08:00:00",
        "courseName": "Birdsfoot Golf Club",
        "numberOfPlayers": 4,
        "confirmationNumber": "ABC123"
      },
      {
        "reservationId": 123457,
        "dateTime": "2025-10-27T09:30:00",
        "courseName": "Birdsfoot Golf Club",
        "numberOfPlayers": 2,
        "confirmationNumber": "ABC124"
      }
    ],
    "totalCount": 2
  }
}
```

#### 6.2.5 Transformed Notification Output

```
Golf Reservations (Next 4 Tee Times):

1. Friday, Oct 25 @ 8:00 AM
   Birdsfoot Golf Club
   4 players | Confirmation: ABC123

2. Sunday, Oct 27 @ 9:30 AM
   Birdsfoot Golf Club
   2 players | Confirmation: ABC124

No additional reservations found.
```

### 6.3 AWS Secrets Manager Secret Schema

#### 6.3.1 Secret: `rez-agent/golf/credentials`

```json
{
  "username": "user@example.com",
  "password": "SecurePassword123!",
  "golfer_id": "91124"
}
```

**IAM Policy Requirements:**
- Action: `secretsmanager:GetSecretValue`
- Resource: `arn:aws:secretsmanager:*:*:secret:rez-agent/golf/credentials-*`

---

## 7. Security Architecture

### 7.1 Authentication & Authorization

#### 7.1.1 IAM Role for Web Action Processor Lambda

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "dynamodb:GetItem",
        "dynamodb:PutItem",
        "dynamodb:UpdateItem",
        "dynamodb:Query"
      ],
      "Resource": [
        "arn:aws:dynamodb:*:*:table/rez-agent-messages-{stage}",
        "arn:aws:dynamodb:*:*:table/rez-agent-messages-{stage}/index/*",
        "arn:aws:dynamodb:*:*:table/rez-agent-web-action-results-{stage}",
        "arn:aws:dynamodb:*:*:table/rez-agent-web-action-results-{stage}/index/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": [
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueAttributes"
      ],
      "Resource": "arn:aws:sqs:*:*:rez-agent-web-actions-{stage}"
    },
    {
      "Effect": "Allow",
      "Action": "sns:Publish",
      "Resource": "arn:aws:sns:*:*:rez-agent-messages-{stage}"
    },
    {
      "Effect": "Allow",
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "arn:aws:secretsmanager:*:*:secret:rez-agent/*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:*:*:*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "xray:PutTraceSegments",
        "xray:PutTelemetryRecords"
      ],
      "Resource": "*"
    }
  ]
}
```

### 7.2 Secrets Management

#### 7.2.1 Golf API Credentials

**Storage:** AWS Secrets Manager
**Secret Name:** `rez-agent/golf/credentials`
**Encryption:** KMS default key
**Rotation:** Manual (initially), can enable automatic rotation later
**Access Pattern:**
1. Lambda retrieves secret on first OAuth request
2. Cache secret in Lambda global scope (duration: Lambda lifecycle)
3. Access token cached for 50 minutes (expires in 60)

#### 7.2.2 Secret Retrieval Code

```go
package secrets

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"

    "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// SecretCache provides thread-safe caching of secrets
type SecretCache struct {
    client *secretsmanager.Client
    cache  map[string]*cachedSecret
    mu     sync.RWMutex
}

type cachedSecret struct {
    value     string
    expiresAt time.Time
}

// NewSecretCache creates a new secret cache
func NewSecretCache(client *secretsmanager.Client) *SecretCache {
    return &SecretCache{
        client: client,
        cache:  make(map[string]*cachedSecret),
    }
}

// GetSecret retrieves a secret with caching (5 min TTL)
func (c *SecretCache) GetSecret(ctx context.Context, secretName string) (string, error) {
    // Check cache first
    c.mu.RLock()
    cached, exists := c.cache[secretName]
    c.mu.RUnlock()

    if exists && time.Now().Before(cached.expiresAt) {
        return cached.value, nil
    }

    // Fetch from Secrets Manager
    result, err := c.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
        SecretId: &secretName,
    })
    if err != nil {
        return "", fmt.Errorf("failed to get secret %s: %w", secretName, err)
    }

    secretValue := *result.SecretString

    // Cache for 5 minutes
    c.mu.Lock()
    c.cache[secretName] = &cachedSecret{
        value:     secretValue,
        expiresAt: time.Now().Add(5 * time.Minute),
    }
    c.mu.Unlock()

    return secretValue, nil
}

// GolfCredentials represents parsed golf API credentials
type GolfCredentials struct {
    Username string `json:"username"`
    Password string `json:"password"`
    GolferID string `json:"golfer_id"`
}

// GetGolfCredentials retrieves and parses golf credentials
func (c *SecretCache) GetGolfCredentials(ctx context.Context) (*GolfCredentials, error) {
    secretValue, err := c.GetSecret(ctx, "rez-agent/golf/credentials")
    if err != nil {
        return nil, err
    }

    var creds GolfCredentials
    if err := json.Unmarshal([]byte(secretValue), &creds); err != nil {
        return nil, fmt.Errorf("failed to parse credentials: %w", err)
    }

    return &creds, nil
}
```

### 7.3 Network Security

#### 7.3.1 TLS Configuration

```go
package httpclient

import (
    "crypto/tls"
    "net/http"
    "time"
)

// NewHTTPClient creates a secure HTTP client
func NewHTTPClient(timeout time.Duration) *http.Client {
    return &http.Client{
        Timeout: timeout,
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                MinVersion: tls.VersionTLS12,
                MaxVersion: tls.VersionTLS13,
            },
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 10,
            IdleConnTimeout:     90 * time.Second,
        },
    }
}
```

#### 7.3.2 Sensitive Data Handling

**Logging Policy:**
- NEVER log OAuth tokens, passwords, or API keys
- Redact authorization headers in logs
- Mask PII in error messages

**Example:**
```go
// BAD
logger.Info("auth request", slog.String("password", password))

// GOOD
logger.Info("auth request", slog.String("username", username), slog.String("password", "[REDACTED]"))
```

---

## 8. Error Handling & Resilience

### 8.1 Error Classification

| Error Type | Retry Strategy | Action |
|------------|----------------|--------|
| **Transient Network** (timeout, 5xx) | Exponential backoff (3 attempts) | Retry, then fail to DLQ |
| **Auth Failure** (401, 403) | No retry | Fail immediately, alarm |
| **Client Error** (400, 404) | No retry | Fail immediately, log |
| **Rate Limit** (429) | Exponential backoff with jitter | Retry with longer delays |
| **DynamoDB Throttle** | SDK auto-retry | Exponential backoff (SDK handles) |
| **Secrets Manager Error** | Retry (3 attempts) | Fail to DLQ if all fail |

### 8.2 Retry Logic Implementation

```go
package retry

import (
    "context"
    "fmt"
    "log/slog"
    "math"
    "math/rand"
    "time"
)

// RetryConfig defines retry behavior
type RetryConfig struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
    Jitter      bool
}

// DefaultRetryConfig for HTTP requests
var DefaultRetryConfig = RetryConfig{
    MaxAttempts: 3,
    BaseDelay:   1 * time.Second,
    MaxDelay:    10 * time.Second,
    Jitter:      true,
}

// RetryableFunc is a function that can be retried
type RetryableFunc func(ctx context.Context) error

// IsRetryable determines if an error should be retried
type IsRetryable func(error) bool

// Execute retries a function with exponential backoff
func Execute(ctx context.Context, config RetryConfig, fn RetryableFunc, isRetryable IsRetryable, logger *slog.Logger) error {
    var lastErr error

    for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
        err := fn(ctx)
        if err == nil {
            return nil
        }

        lastErr = err

        if !isRetryable(err) {
            logger.WarnContext(ctx, "non-retryable error",
                slog.String("error", err.Error()),
                slog.Int("attempt", attempt),
            )
            return err
        }

        if attempt < config.MaxAttempts {
            delay := calculateBackoff(attempt, config)
            logger.InfoContext(ctx, "retrying after error",
                slog.String("error", err.Error()),
                slog.Int("attempt", attempt),
                slog.Duration("delay", delay),
            )

            select {
            case <-time.After(delay):
                // Continue to next attempt
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }

    return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// calculateBackoff calculates delay with exponential backoff and optional jitter
func calculateBackoff(attempt int, config RetryConfig) time.Duration {
    delay := config.BaseDelay * time.Duration(math.Pow(2, float64(attempt-1)))

    if delay > config.MaxDelay {
        delay = config.MaxDelay
    }

    if config.Jitter {
        jitter := time.Duration(rand.Int63n(int64(delay / 4)))
        delay += jitter
    }

    return delay
}

// IsHTTPRetryable checks if HTTP error is retryable
func IsHTTPRetryable(err error, statusCode int) bool {
    // Network errors are retryable
    if err != nil {
        return true
    }

    // 5xx server errors are retryable
    if statusCode >= 500 && statusCode < 600 {
        return true
    }

    // 429 rate limit is retryable
    if statusCode == 429 {
        return true
    }

    // Everything else is not retryable
    return false
}
```

### 8.3 Circuit Breaker Pattern

**Decision:** NOT implementing circuit breaker for external APIs initially.

**Rationale:**
- Scheduled tasks run once daily (low request volume)
- Weather.gov and Golf API are external services we don't control
- SQS DLQ provides adequate failure isolation
- Circuit breaker adds complexity for minimal benefit

**Future Enhancement:**
If we add high-frequency actions, consider implementing circuit breaker similar to existing notification service.

### 8.4 Timeout Management

```go
// Lambda Configuration
Timeout: 5 minutes (300 seconds)

// HTTP Client Timeouts
HTTP Request Timeout: 30 seconds
OAuth Request Timeout: 10 seconds

// Context Timeouts
Action Execution Context: Lambda timeout - 30s (270s)
HTTP Request Context: 30s

// SQS Visibility Timeout
Visibility Timeout: 5 minutes (matches Lambda timeout)
```

---

## 9. Configuration Management

### 9.1 Environment Variables

```bash
# Web Action Processor Lambda
DYNAMODB_TABLE_NAME=rez-agent-messages-{stage}
DYNAMODB_RESULTS_TABLE_NAME=rez-agent-web-action-results-{stage}
SNS_TOPIC_ARN=arn:aws:sns:us-east-1:123456789012:rez-agent-messages-{stage}
SQS_QUEUE_URL=https://sqs.us-east-1.amazonaws.com/123456789012/rez-agent-web-actions-{stage}
STAGE=dev|stage|prod
AWS_REGION=us-east-1

# HTTP Client Configuration
HTTP_TIMEOUT_SECONDS=30
HTTP_MAX_IDLE_CONNS=100
HTTP_IDLE_CONN_TIMEOUT_SECONDS=90

# OAuth Configuration
OAUTH_TOKEN_CACHE_DURATION_MINUTES=50
```

### 9.2 Scheduler Lambda Configuration Updates

Add new EventBridge schedules for weather and golf actions:

```yaml
Weather Schedule:
  Name: rez-agent-weather-scheduler-{stage}
  Cron: cron(0 10 * * ? *)  # 5 AM EST = 10 AM UTC
  Payload:
    action_type: "weather"
    days: 2

Golf Schedule:
  Name: rez-agent-golf-scheduler-{stage}
  Cron: cron(15 10 * * ? *)  # 5:15 AM EST = 10:15 AM UTC
  Payload:
    action_type: "golf"
    max_results: 4
```

### 9.3 Feature Flags (Future)

Store in DynamoDB config table:

```json
{
  "feature": "web_action_processor",
  "enabled": true,
  "config": {
    "max_response_size_kb": 400,
    "enable_weather": true,
    "enable_golf": true,
    "weather_days_default": 2,
    "golf_max_results": 4
  }
}
```

---

## 10. Implementation Guidance

### 10.1 Go Module Structure

```
/workspaces/rez_agent/
├── cmd/
│   ├── scheduler/            # MODIFY: Add web action scheduling
│   ├── processor/            # UNCHANGED
│   ├── webapi/              # UNCHANGED
│   └── webaction/           # NEW: Web Action Processor Lambda
│       └── main.go
├── internal/
│   ├── models/
│   │   ├── message.go       # MODIFY: Add MessageTypeWebAction
│   │   ├── web_action.go    # NEW: WebActionPayload model
│   │   └── web_action_result.go  # NEW: WebActionResult model
│   ├── repository/
│   │   ├── dynamodb.go      # MODIFY: Add results table methods
│   │   └── web_action_results.go  # NEW: Results repository
│   ├── webaction/           # NEW: Web action processing
│   │   ├── processor.go     # Main processor orchestrator
│   │   ├── actions/
│   │   │   ├── registry.go  # Action handler registry
│   │   │   ├── weather.go   # Weather action handler
│   │   │   └── golf.go      # Golf action handler
│   │   ├── auth/
│   │   │   ├── oauth.go     # OAuth 2.0 client
│   │   │   └── cache.go     # Token caching
│   │   └── httpclient/
│   │       └── client.go    # HTTP client wrapper
│   ├── secrets/             # NEW: Secrets Manager integration
│   │   └── cache.go
│   └── retry/               # NEW: Retry logic
│       └── retry.go
└── infrastructure/
    └── main.go              # MODIFY: Add new resources
```

### 10.2 Implementation Phases

#### Phase 1: Foundation (Week 1)
- [ ] Add `MessageTypeWebAction` to models
- [ ] Create `WebActionPayload` model with validation
- [ ] Create `WebActionResult` model
- [ ] Create DynamoDB results table in Pulumi
- [ ] Create SNS topic and SQS queue for web actions
- [ ] Update IAM roles

#### Phase 2: Core Processor (Week 1-2)
- [ ] Implement Web Action Processor Lambda skeleton
- [ ] Implement SQS message parsing
- [ ] Implement DynamoDB message fetching
- [ ] Implement action handler registry pattern
- [ ] Implement results repository (DynamoDB)
- [ ] Add structured logging with correlation IDs

#### Phase 3: HTTP & Auth (Week 2)
- [ ] Implement secure HTTP client wrapper
- [ ] Implement Secrets Manager cache
- [ ] Implement OAuth 2.0 password grant client
- [ ] Implement token caching (50 min TTL)
- [ ] Implement retry logic with exponential backoff

#### Phase 4: Weather Action (Week 2)
- [ ] Implement weather action handler
- [ ] Parse weather.gov API response
- [ ] Format weather notification
- [ ] Update Scheduler Lambda for weather schedule
- [ ] End-to-end testing

#### Phase 5: Golf Action (Week 3)
- [ ] Create Secrets Manager secret for golf credentials
- [ ] Implement golf action handler
- [ ] Implement OAuth authentication flow
- [ ] Parse golf API response
- [ ] Format golf notification
- [ ] Update Scheduler Lambda for golf schedule
- [ ] End-to-end testing

#### Phase 6: Observability (Week 3)
- [ ] Add CloudWatch custom metrics
- [ ] Add X-Ray tracing
- [ ] Create CloudWatch alarms (DLQ, failures, latency)
- [ ] Create CloudWatch dashboard
- [ ] Add detailed logging

#### Phase 7: Testing & Documentation (Week 4)
- [ ] Unit tests for all components
- [ ] Integration tests
- [ ] Load testing (simulate daily runs)
- [ ] Runbook documentation
- [ ] API documentation

### 10.3 Critical Code Examples

#### 10.3.1 Web Action Processor Main Handler

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"
    "time"

    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
    "github.com/aws/aws-sdk-go-v2/service/sns"

    "github.com/yourusername/rez_agent/internal/models"
    "github.com/yourusername/rez_agent/internal/repository"
    "github.com/yourusername/rez_agent/internal/retry"
    "github.com/yourusername/rez_agent/internal/secrets"
    "github.com/yourusername/rez_agent/internal/webaction"
    "github.com/yourusername/rez_agent/internal/webaction/actions"
    "github.com/yourusername/rez_agent/internal/webaction/auth"
    "github.com/yourusername/rez_agent/internal/webaction/httpclient"
    appconfig "github.com/yourusername/rez_agent/pkg/config"
)

type Handler struct {
    config           *appconfig.WebActionConfig
    messageRepo      repository.MessageRepository
    resultsRepo      repository.WebActionResultsRepository
    processor        *webaction.Processor
    logger           *slog.Logger
}

func NewHandler(
    cfg *appconfig.WebActionConfig,
    messageRepo repository.MessageRepository,
    resultsRepo repository.WebActionResultsRepository,
    processor *webaction.Processor,
    logger *slog.Logger,
) *Handler {
    return &Handler{
        config:      cfg,
        messageRepo: messageRepo,
        resultsRepo: resultsRepo,
        processor:   processor,
        logger:      logger,
    }
}

func (h *Handler) HandleEvent(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
    h.logger.InfoContext(ctx, "processing SQS batch",
        slog.Int("record_count", len(event.Records)),
    )

    var failures []events.SQSBatchItemFailure

    for _, record := range event.Records {
        if err := h.processRecord(ctx, record); err != nil {
            h.logger.ErrorContext(ctx, "failed to process record",
                slog.String("message_id", record.MessageId),
                slog.String("error", err.Error()),
            )
            failures = append(failures, events.SQSBatchItemFailure{
                ItemIdentifier: record.MessageId,
            })
        }
    }

    return events.SQSEventResponse{
        BatchItemFailures: failures,
    }, nil
}

func (h *Handler) processRecord(ctx context.Context, record events.SQSMessage) error {
    // Parse SNS message from SQS
    var snsMessage struct {
        MessageID string `json:"message_id"`
        EventType string `json:"event_type"`
        Stage     string `json:"stage"`
    }

    if err := json.Unmarshal([]byte(record.Body), &snsMessage); err != nil {
        return fmt.Errorf("failed to parse SNS message: %w", err)
    }

    // Fetch message from DynamoDB
    message, err := h.messageRepo.GetByID(ctx, snsMessage.MessageID)
    if err != nil {
        return fmt.Errorf("failed to fetch message: %w", err)
    }

    // Validate message type
    if message.MessageType != models.MessageTypeWebAction {
        h.logger.WarnContext(ctx, "skipping non-web-action message",
            slog.String("message_id", message.ID),
            slog.String("type", message.MessageType.String()),
        )
        return nil
    }

    // Process web action
    return h.processor.ProcessWebAction(ctx, message)
}

func main() {
    // Setup structured logging
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
    slog.SetDefault(logger)

    // Load configuration
    cfg := appconfig.MustLoadWebActionConfig()

    logger.Info("web action processor starting",
        slog.String("stage", cfg.Stage.String()),
        slog.String("region", cfg.AWSRegion),
    )

    // Initialize AWS SDK
    awsCfg, err := config.LoadDefaultConfig(context.Background(),
        config.WithRegion(cfg.AWSRegion),
    )
    if err != nil {
        logger.Error("failed to load AWS config", slog.String("error", err.Error()))
        panic(err)
    }

    // Create AWS clients
    dynamoClient := dynamodb.NewFromConfig(awsCfg)
    snsClient := sns.NewFromConfig(awsCfg)
    secretsClient := secretsmanager.NewFromConfig(awsCfg)

    // Create repositories
    messageRepo := repository.NewDynamoDBRepository(dynamoClient, cfg.DynamoDBTableName)
    resultsRepo := repository.NewWebActionResultsRepository(dynamoClient, cfg.ResultsTableName)

    // Create HTTP client
    httpClient := httpclient.NewHTTPClient(30 * time.Second)

    // Create secrets cache
    secretsCache := secrets.NewSecretCache(secretsClient)

    // Create OAuth client
    oauthClient := auth.NewOAuthClient(httpClient, secretsCache, logger)

    // Create action registry
    registry := actions.NewRegistry()
    registry.Register("fetch_weather", actions.NewWeatherHandler(httpClient, logger))
    registry.Register("fetch_golf_reservations", actions.NewGolfHandler(httpClient, oauthClient, logger))

    // Create processor
    processor := webaction.NewProcessor(
        cfg,
        messageRepo,
        resultsRepo,
        snsClient,
        registry,
        logger,
    )

    // Create handler
    handler := NewHandler(cfg, messageRepo, resultsRepo, processor, logger)

    // Start Lambda
    lambda.Start(handler.HandleEvent)
}
```

#### 10.3.2 Weather Action Handler

```go
package actions

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
    "strings"
    "time"
)

type WeatherHandler struct {
    httpClient *http.Client
    logger     *slog.Logger
}

func NewWeatherHandler(httpClient *http.Client, logger *slog.Logger) *WeatherHandler {
    return &WeatherHandler{
        httpClient: httpClient,
        logger:     logger,
    }
}

func (h *WeatherHandler) Execute(ctx context.Context, payload *models.WebActionPayload) (*ActionResult, error) {
    h.logger.InfoContext(ctx, "executing weather action",
        slog.String("url", payload.URL),
    )

    // Get number of days from arguments
    days := 2
    if daysArg, ok := payload.Arguments["days"].(float64); ok {
        days = int(daysArg)
    }

    // Create HTTP request
    req, err := http.NewRequestWithContext(ctx, "GET", payload.URL, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("User-Agent", "rez-agent/1.0")
    req.Header.Set("Accept", "application/geo+json")

    // Execute request
    startTime := time.Now()
    resp, err := h.httpClient.Do(req)
    duration := time.Since(startTime)

    if err != nil {
        return nil, fmt.Errorf("HTTP request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return &ActionResult{
            HTTPStatusCode: resp.StatusCode,
            Error:          fmt.Errorf("unexpected status code: %d", resp.StatusCode),
            Duration:       duration,
        }, nil
    }

    // Parse response
    var weatherData WeatherGovResponse
    if err := json.NewDecoder(resp.Body).Decode(&weatherData); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }

    // Transform to notification
    notification := h.formatWeatherNotification(weatherData, days)

    return &ActionResult{
        HTTPStatusCode:    resp.StatusCode,
        ResponseBody:      mustMarshal(weatherData),
        TransformedResult: notification,
        Duration:          duration,
    }, nil
}

func (h *WeatherHandler) formatWeatherNotification(data WeatherGovResponse, days int) string {
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("Weather Forecast (%d days):\n\n", days))

    periodCount := days * 2 // day + night
    if len(data.Properties.Periods) < periodCount {
        periodCount = len(data.Properties.Periods)
    }

    for i := 0; i < periodCount; i++ {
        period := data.Properties.Periods[i]
        sb.WriteString(fmt.Sprintf("%s: %s, %s %d°F\n",
            period.Name,
            period.ShortForecast,
            tempLabel(period.IsDaytime),
            period.Temperature,
        ))
        sb.WriteString(fmt.Sprintf("%s\n\n", period.DetailedForecast))
    }

    return sb.String()
}

func tempLabel(isDaytime bool) string {
    if isDaytime {
        return "high"
    }
    return "low"
}

type WeatherGovResponse struct {
    Properties struct {
        Updated time.Time `json:"updated"`
        Periods []struct {
            Number           int       `json:"number"`
            Name             string    `json:"name"`
            StartTime        time.Time `json:"startTime"`
            Temperature      int       `json:"temperature"`
            TemperatureUnit  string    `json:"temperatureUnit"`
            WindSpeed        string    `json:"windSpeed"`
            ShortForecast    string    `json:"shortForecast"`
            DetailedForecast string    `json:"detailedForecast"`
            IsDaytime        bool      `json:"isDaytime"`
        } `json:"periods"`
    } `json:"properties"`
}
```

#### 10.3.3 Golf Action Handler with OAuth

```go
package actions

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
    "sort"
    "strings"
    "time"
)

type GolfHandler struct {
    httpClient  *http.Client
    oauthClient *auth.OAuthClient
    logger      *slog.Logger
}

func NewGolfHandler(httpClient *http.Client, oauthClient *auth.OAuthClient, logger *slog.Logger) *GolfHandler {
    return &GolfHandler{
        httpClient:  httpClient,
        oauthClient: oauthClient,
        logger:      logger,
    }
}

func (h *GolfHandler) Execute(ctx context.Context, payload *models.WebActionPayload) (*ActionResult, error) {
    h.logger.InfoContext(ctx, "executing golf action",
        slog.String("url", payload.URL),
    )

    // Get max results from arguments
    maxResults := 4
    if maxArg, ok := payload.Arguments["max_results"].(float64); ok {
        maxResults = int(maxArg)
    }

    // Authenticate using OAuth
    token, err := h.oauthClient.GetToken(ctx, payload.AuthConfig)
    if err != nil {
        return nil, fmt.Errorf("OAuth authentication failed: %w", err)
    }

    // Build URL with query parameters
    golferID, _ := payload.Arguments["golfer_id"].(string)
    pageSize, _ := payload.Arguments["page_size"].(float64)
    currentPage, _ := payload.Arguments["current_page"].(float64)

    url := fmt.Sprintf("%s?golferId=%s&pageSize=%d&currentPage=%d",
        payload.URL, golferID, int(pageSize), int(currentPage))

    // Create HTTP request
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Accept", "application/json")
    req.Header.Set("client-id", "onlineresweb")
    req.Header.Set("User-Agent", "rez-agent/1.0")

    // Execute request
    startTime := time.Now()
    resp, err := h.httpClient.Do(req)
    duration := time.Since(startTime)

    if err != nil {
        return nil, fmt.Errorf("HTTP request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return &ActionResult{
            HTTPStatusCode: resp.StatusCode,
            Error:          fmt.Errorf("unexpected status code: %d", resp.StatusCode),
            Duration:       duration,
        }, nil
    }

    // Parse response
    var golfData GolfReservationsResponse
    if err := json.NewDecoder(resp.Body).Decode(&golfData); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }

    // Transform to notification
    notification := h.formatGolfNotification(golfData, maxResults)

    return &ActionResult{
        HTTPStatusCode:    resp.StatusCode,
        ResponseBody:      mustMarshal(golfData),
        TransformedResult: notification,
        Duration:          duration,
    }, nil
}

func (h *GolfHandler) formatGolfNotification(data GolfReservationsResponse, maxResults int) string {
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("Golf Reservations (Next %d Tee Times):\n\n", maxResults))

    if !data.Success || len(data.Data.Items) == 0 {
        sb.WriteString("No upcoming reservations found.")
        return sb.String()
    }

    // Sort by date/time (earliest first)
    sort.Slice(data.Data.Items, func(i, j int) bool {
        return data.Data.Items[i].DateTime.Before(data.Data.Items[j].DateTime)
    })

    // Take first N results
    count := maxResults
    if len(data.Data.Items) < count {
        count = len(data.Data.Items)
    }

    for i := 0; i < count; i++ {
        res := data.Data.Items[i]
        sb.WriteString(fmt.Sprintf("%d. %s @ %s\n",
            i+1,
            res.DateTime.Format("Monday, Jan 2"),
            res.DateTime.Format("3:04 PM"),
        ))
        sb.WriteString(fmt.Sprintf("   %s\n", res.CourseName))
        sb.WriteString(fmt.Sprintf("   %d players | Confirmation: %s\n\n",
            res.NumberOfPlayers,
            res.ConfirmationNumber,
        ))
    }

    if len(data.Data.Items) == 0 {
        sb.WriteString("No additional reservations found.")
    }

    return sb.String()
}

type GolfReservationsResponse struct {
    Success bool `json:"success"`
    Data    struct {
        Items []struct {
            ReservationID      int       `json:"reservationId"`
            DateTime           time.Time `json:"dateTime"`
            CourseName         string    `json:"courseName"`
            NumberOfPlayers    int       `json:"numberOfPlayers"`
            ConfirmationNumber string    `json:"confirmationNumber"`
        } `json:"items"`
        TotalCount int `json:"totalCount"`
    } `json:"data"`
}
```

---

## 11. Testing Strategy

### 11.1 Unit Tests

```go
// internal/webaction/actions/weather_test.go
func TestWeatherHandler_Execute(t *testing.T) {
    tests := []struct {
        name           string
        payload        *models.WebActionPayload
        mockResponse   string
        mockStatusCode int
        expectedError  bool
    }{
        {
            name: "successful weather fetch",
            payload: models.NewWeatherActionPayload(2),
            mockResponse: `{"properties":{"periods":[...]}}`,
            mockStatusCode: 200,
            expectedError: false,
        },
        {
            name: "API returns 500",
            payload: models.NewWeatherActionPayload(2),
            mockStatusCode: 500,
            expectedError: false, // Returns ActionResult with error
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### 11.2 Integration Tests

```bash
# Test end-to-end flow in dev environment

# 1. Create web action message
aws dynamodb put-item \
  --table-name rez-agent-messages-dev \
  --item '{
    "id": {"S": "test_msg_123"},
    "created_date": {"S": "2025-10-23T10:00:00Z"},
    "message_type": {"S": "web_action"},
    "payload": {"S": "{\"version\":\"1.0\",\"url\":\"...\",\"action\":\"fetch_weather\",...}"},
    "status": {"S": "created"},
    "stage": {"S": "dev"}
  }'

# 2. Publish SNS event
aws sns publish \
  --topic-arn arn:aws:sns:us-east-1:123456789012:rez-agent-web-actions-dev \
  --message '{"message_id":"test_msg_123","event_type":"message_created","stage":"dev"}'

# 3. Wait for processing (check CloudWatch Logs)
aws logs tail /aws/lambda/rez-agent-webaction-dev --follow

# 4. Verify result in DynamoDB
aws dynamodb scan \
  --table-name rez-agent-web-action-results-dev \
  --filter-expression "message_id = :mid" \
  --expression-attribute-values '{":mid":{"S":"test_msg_123"}}'
```

### 11.3 Load Testing

```bash
# Simulate 30 days of scheduled tasks (60 messages)
for i in {1..60}; do
  aws sns publish \
    --topic-arn arn:aws:sns:us-east-1:123456789012:rez-agent-web-actions-dev \
    --message "{\"message_id\":\"load_test_$i\",\"event_type\":\"message_created\",\"stage\":\"dev\"}"
  sleep 2
done
```

---

## 12. Operational Considerations

### 12.1 CloudWatch Metrics

```go
// Custom metrics to publish
metrics := []string{
    "WebActionExecuted",           // Count of actions executed
    "WebActionSuccess",            // Count of successful actions
    "WebActionFailed",             // Count of failed actions
    "WebActionDuration",           // Duration in milliseconds
    "HTTPRequestDuration",         // HTTP request duration
    "OAuthTokenCacheHit",          // OAuth token cache hits
    "OAuthTokenCacheMiss",         // OAuth token cache misses
    "OAuthAuthenticationFailed",   // OAuth auth failures
    "SecretsManagerCalls",         // Secrets Manager API calls
}

// Dimensions
dimensions := []struct{
    Name  string
    Value string
}{
    {"Action", "fetch_weather"},
    {"Stage", "prod"},
}
```

### 12.2 CloudWatch Alarms

```yaml
Alarms:
  - Name: WebActionDLQMessages
    Metric: ApproximateNumberOfMessagesVisible
    Namespace: AWS/SQS
    Threshold: 1
    ComparisonOperator: GreaterThanThreshold
    Action: SNS notification to ops team

  - Name: WebActionProcessorErrors
    Metric: Errors
    Namespace: AWS/Lambda
    Threshold: 3
    EvaluationPeriods: 2
    Action: SNS notification

  - Name: WebActionHighLatency
    Metric: Duration
    Namespace: AWS/Lambda
    Statistic: p95
    Threshold: 60000  # 60 seconds
    Action: SNS notification

  - Name: OAuthAuthenticationFailures
    Metric: OAuthAuthenticationFailed
    Namespace: RezAgent
    Threshold: 2
    EvaluationPeriods: 1
    Action: SNS notification (critical)
```

### 12.3 Monitoring Dashboard

```
CloudWatch Dashboard: rez-agent-web-actions-{stage}

Widgets:
  1. Web Actions Executed (24h)
  2. Success vs. Failed Rate
  3. Action Duration (p50, p95, p99)
  4. HTTP Request Duration
  5. OAuth Token Cache Hit Rate
  6. DLQ Message Count
  7. Lambda Errors
  8. Lambda Concurrent Executions
  9. Recent Error Logs
  10. Top 10 Slowest Actions
```

### 12.4 Runbook

#### Incident: Golf Authentication Failing

**Symptoms:**
- CloudWatch alarm: `OAuthAuthenticationFailures`
- Lambda logs show 401 errors from `/connect/token`

**Investigation:**
1. Check Secrets Manager secret: `aws secretsmanager get-secret-value --secret-id rez-agent/golf/credentials`
2. Verify credentials manually via curl (from runbook)
3. Check golf API status page (if available)

**Resolution:**
- If credentials expired: Update secret, Lambda will pick up on next invocation
- If API is down: Disable EventBridge schedule temporarily
- If API changed: Update OAuth config in code, redeploy

#### Incident: Weather API Timeout

**Symptoms:**
- CloudWatch alarm: `WebActionHighLatency`
- Lambda logs show HTTP timeout errors

**Investigation:**
1. Check weather.gov API status: curl test from EC2
2. Review HTTP timeout configuration
3. Check network connectivity (VPC if applicable)

**Resolution:**
- If transient: SQS will auto-retry
- If persistent API issue: Increase HTTP timeout or disable schedule

### 12.5 Cost Estimate

```
Monthly Cost (dev):
  - Lambda Invocations: 60/month (2 actions * 30 days) * $0.20/1M = $0.000012
  - Lambda Duration: 60 * 5s * $0.0000166667/GB-second (512MB) = $0.0025
  - DynamoDB: 60 writes + 180 reads = $0.003
  - SNS: 60 publishes = $0.00003
  - SQS: 60 messages = $0.000024
  - Secrets Manager: 60 API calls = $0.003
  - CloudWatch Logs: ~10 MB = $0.005
  Total: ~$0.014/month

Monthly Cost (prod):
  - Same as dev
  Total: ~$0.014/month

Note: Negligible cost due to low invocation frequency (2x/day)
```

---

## Summary

This technical design document provides a complete architecture for the Web Action Processor feature, including:

1. **Service Architecture**: New Lambda processor integrated with existing event-driven flow
2. **Data Models**: Extensible `WebActionPayload` and `WebActionResult` schemas
3. **Message Flow**: EventBridge → Scheduler → SNS → SQS → Processor → Results + Notification
4. **API Integration**: Weather.gov (no auth) and Golf API (OAuth 2.0 password grant)
5. **Security**: Secrets Manager, IAM least-privilege, TLS 1.2+, credential caching
6. **Resilience**: Retry logic, SQS DLQ, timeout management, error classification
7. **Implementation**: Go 1.24 code examples, module structure, 4-week roadmap
8. **Testing**: Unit, integration, and load testing strategies
9. **Operations**: CloudWatch metrics/alarms, runbooks, cost estimates

**Next Steps:**
1. Review and approve this design
2. Create Pulumi infrastructure code
3. Begin Phase 1 implementation (Foundation)
4. Iterate through remaining phases

**Design Approval Checklist:**
- [ ] Architecture aligns with existing patterns
- [ ] Security requirements met (secrets, IAM, TLS)
- [ ] Observability requirements met (logging, metrics, alarms)
- [ ] Cost estimate acceptable
- [ ] Implementation timeline acceptable (4 weeks)
- [ ] All requirements from NEXT_REQUIREMENT.MD addressed

**Files to Create:**
- `/workspaces/rez_agent/internal/models/web_action.go`
- `/workspaces/rez_agent/internal/models/web_action_result.go`
- `/workspaces/rez_agent/internal/webaction/` (entire package)
- `/workspaces/rez_agent/cmd/webaction/main.go`
- `/workspaces/rez_agent/infrastructure/main.go` (updates)

---

**Document Status:** ✅ Complete and ready for implementation
