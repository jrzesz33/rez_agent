# Sequence Diagrams

This document contains Mermaid sequence diagrams that illustrate the key workflows and interactions in the rez_agent system.

## Table of Contents

- [Message Processing Flow](#message-processing-flow)
- [Web Action Execution](#web-action-execution)
- [Golf Tee Time Booking](#golf-tee-time-booking)
- [AI Agent Interaction (Legacy)](#ai-agent-interaction-legacy)
- [AI Agent with MCP Integration](#ai-agent-with-mcp-integration)
- [Schedule Creation](#schedule-creation)
- [EventBridge Scheduler Workflow](#eventbridge-scheduler-workflow)
- [Authentication Flow](#authentication-flow)
- [Error Handling and DLQ](#error-handling-and-dlq)

---

## Message Processing Flow

Basic message flow from API Gateway through to notification delivery.

```mermaid
sequenceDiagram
    participant Client
    participant API as API Gateway
    participant WebAPI as WebAPI Lambda
    participant SNS as SNS Topic
    participant SQS as SQS Queue
    participant Processor as Processor Lambda
    participant DDB as DynamoDB
    participant ntfy as ntfy.sh

    Client->>API: POST /api/messages
    API->>WebAPI: Invoke Lambda
    WebAPI->>DDB: Save message
    DDB-->>WebAPI: Message saved
    WebAPI->>SNS: Publish to topic
    SNS-->>WebAPI: Published
    WebAPI-->>API: 201 Created
    API-->>Client: Response

    SNS->>SQS: Deliver message
    SQS->>Processor: Trigger Lambda
    Processor->>DDB: Update message status
    Processor->>ntfy: Send notification
    ntfy-->>Processor: Notification sent
    Processor->>DDB: Update to 'completed'
    Processor->>SQS: Delete message
```

---

## Web Action Execution

Flow for executing web actions (weather, golf API calls).

```mermaid
sequenceDiagram
    participant Client
    participant API as API Gateway
    participant WebAPI as WebAPI Lambda
    participant SNS as Web Actions Topic
    participant SQS as Web Actions Queue
    participant WebAction as WebAction Lambda
    participant Secrets as Secrets Manager
    participant External as External API
    participant DDB as DynamoDB
    participant Results as Results Table

    Client->>API: POST /api/messages<br/>(type: web_action)
    API->>WebAPI: Invoke
    WebAPI->>DDB: Save message
    WebAPI->>SNS: Publish web_action
    SNS->>SQS: Deliver
    SQS->>WebAction: Trigger Lambda

    alt OAuth Required
        WebAction->>Secrets: Get credentials
        Secrets-->>WebAction: Return credentials
        WebAction->>External: POST /token
        External-->>WebAction: Access token
        WebAction->>WebAction: Validate JWT
    end

    WebAction->>External: API request
    External-->>WebAction: Response
    WebAction->>Results: Store result
    WebAction->>DDB: Update status
    WebAction->>SQS: Delete message
```

---

## Golf Tee Time Booking

Complete flow for searching and booking golf tee times.

```mermaid
sequenceDiagram
    participant User
    participant Agent as AI Agent
    participant MCP as MCP Server
    participant WebAction as WebAction Lambda
    participant Secrets as Secrets Manager
    participant Golf as Golf Course API
    participant DDB as Results Table

    User->>Agent: "Search tee times for<br/>tomorrow at 2pm"
    Agent->>Agent: Parse request
    Agent->>MCP: golf_search_tee_times<br/>(course, date, time)

    MCP->>Secrets: Get golf credentials
    Secrets-->>MCP: username/password
    MCP->>Golf: POST /connect/token<br/>(OAuth password grant)
    Golf-->>MCP: Access token + JWT
    MCP->>MCP: Validate JWT with JWKS

    MCP->>Golf: GET /TeeTimes<br/>(Authorization: Bearer token)
    Golf-->>MCP: Available tee times
    MCP->>DDB: Store results
    MCP-->>Agent: Formatted tee times

    Agent-->>User: "Found 3 tee times:<br/>- 2:00pm (4 spots)<br/>- 2:15pm (2 spots)<br/>- 2:30pm (4 spots)"

    User->>Agent: "Book the 2:00pm slot"
    Agent->>MCP: golf_book_tee_time<br/>(teeSheetId: 12345)

    MCP->>Golf: POST /ReserveTeeTimes<br/>(Authorization: Bearer token)
    Golf-->>MCP: Booking confirmation
    MCP->>DDB: Store confirmation
    MCP-->>Agent: Booking successful

    Agent-->>User: "✓ Booked! Confirmation #67890"
```

## AI Agent with MCP Integration

New flow using LangChain MCP Adapter (post-refactor).

```mermaid
sequenceDiagram
    participant User
    participant API as API Gateway
    participant Agent as Agent Lambda
    participant MCP as MCP Server
    participant Bedrock as AWS Bedrock
    participant Golf as External APIs
    participant DDB as Session Table

    User->>API: POST /agent<br/>{"message": "Book tee time"}
    API->>Agent: Invoke

    Agent->>DDB: Load session
    Agent->>MCP: Initialize connection
    MCP-->>Agent: Available tools

    Agent->>Bedrock: Invoke with MCP tools
    Note over Agent,Bedrock: LangChain binds<br/>MCP tools to LLM

    Bedrock-->>Agent: Tool call:<br/>golf_search_tee_times
    Agent->>MCP: Execute tool<br/>(via LangChain adapter)
    MCP->>Golf: API request
    Golf-->>MCP: Response
    MCP-->>Agent: Tool result

    Agent->>Bedrock: Re-invoke with results
    Bedrock-->>Agent: "Found 3 times..."
    Agent->>DDB: Save session
    Agent-->>API: Response
    API-->>User: AI response

    Note over Agent,MCP: All tool execution<br/>is synchronous via<br/>HTTP transport
```

---

## Schedule Creation

Dynamic EventBridge schedule creation workflow.

```mermaid
sequenceDiagram
    participant Client
    participant API as API Gateway
    participant WebAPI as WebAPI Lambda
    participant SNS as Schedule Creation Topic
    participant Scheduler as Scheduler Lambda
    participant EventBridge as EventBridge Scheduler
    participant DDB as Schedules Table
    participant IAM as IAM Role

    Client->>API: POST /api/schedules<br/>{"action": "create"}
    API->>WebAPI: Invoke
    WebAPI->>DDB: Validate & save schedule
    WebAPI->>SNS: Publish schedule_creation
    WebAPI-->>Client: Schedule created (pending)

    SNS->>Scheduler: Trigger Lambda
    Scheduler->>DDB: Load schedule details
    Scheduler->>IAM: Verify execution role

    Scheduler->>EventBridge: CreateSchedule
    Note over Scheduler,EventBridge: Pass execution role ARN<br/>for Lambda invocation
    EventBridge-->>Scheduler: Schedule created

    Scheduler->>DDB: Update status: 'active'
    Scheduler-->>SNS: Acknowledge

    Note over EventBridge: Schedule triggers<br/>at cron expression
    EventBridge->>Scheduler: Invoke at schedule time
    Scheduler->>SNS: Publish scheduled message
```

---

## EventBridge Scheduler Workflow

Recurring schedule execution and message publishing.

```mermaid
sequenceDiagram
    participant EventBridge as EventBridge Scheduler
    participant Scheduler as Scheduler Lambda
    participant DDB as DynamoDB
    participant SNS as SNS Topics
    participant SQS as SQS Queues
    participant WebAction as WebAction Lambda

    Note over EventBridge: Cron: 0 12 * * ? *<br/>(Daily at noon UTC)

    EventBridge->>Scheduler: Trigger scheduled event
    Scheduler->>DDB: Query active schedules
    DDB-->>Scheduler: List of schedules

    loop For each active schedule
        Scheduler->>Scheduler: Evaluate conditions
        alt Schedule is active
            Scheduler->>DDB: Create message record

            alt Message type: web_action
                Scheduler->>SNS: Publish to Web Actions Topic
                SNS->>SQS: Deliver to Web Actions Queue
                SQS->>WebAction: Trigger Lambda
            else Message type: notify
                Scheduler->>SNS: Publish to Notifications Topic
                SNS->>SQS: Deliver to Notifications Queue
            end

            Scheduler->>DDB: Update last_run timestamp
        end
    end

    Scheduler-->>EventBridge: Execution complete
```

---

## Authentication Flow

OAuth 2.0 Password Grant with JWT validation.

```mermaid
sequenceDiagram
    participant Lambda as WebAction Lambda
    participant Secrets as Secrets Manager
    participant OAuth as OAuth Server
    participant JWKS as JWKS Endpoint
    participant API as Protected API

    Lambda->>Secrets: GetSecretValue<br/>(rez-agent/golf/credentials)
    Secrets-->>Lambda: {username, password,<br/>client_id, client_secret}

    Lambda->>OAuth: POST /connect/token
    Note over Lambda,OAuth: grant_type=password<br/>username, password<br/>client_id, client_secret<br/>scope=onlinereservation

    OAuth-->>Lambda: {access_token,<br/>refresh_token,<br/>expires_in}

    Lambda->>Lambda: Decode JWT header
    Lambda->>JWKS: GET /.well-known/.../jwks
    JWKS-->>Lambda: Public keys (JWK Set)

    Lambda->>Lambda: Verify JWT signature<br/>using public key
    Lambda->>Lambda: Validate claims<br/>(exp, aud, iss)

    alt JWT Valid
        Lambda->>API: GET /resource<br/>Authorization: Bearer {token}
        API-->>Lambda: Protected resource data
    else JWT Invalid
        Lambda->>Lambda: Log error
        Lambda-->>Lambda: Throw auth error
    end
```

---

## Error Handling and DLQ

Dead letter queue processing and retry logic.

```mermaid
sequenceDiagram
    participant SNS as SNS Topic
    participant SQS as Main Queue
    participant Lambda as Processor Lambda
    participant DLQ as Dead Letter Queue
    participant CloudWatch as CloudWatch
    participant Alert as Monitoring Alert

    SNS->>SQS: Deliver message
    SQS->>Lambda: Invoke (attempt 1)
    Lambda->>Lambda: Process message
    Lambda--xLambda: Error occurs
    Lambda-->>SQS: Processing failed

    Note over SQS: Visibility timeout expires

    SQS->>Lambda: Invoke (attempt 2)
    Lambda--xLambda: Error occurs again
    Lambda-->>SQS: Processing failed

    SQS->>Lambda: Invoke (attempt 3)
    Lambda--xLambda: Error occurs again
    Lambda-->>SQS: Max retries reached

    SQS->>DLQ: Move to DLQ<br/>(maxReceiveCount: 3)
    DLQ->>CloudWatch: Log DLQ message

    CloudWatch->>Alert: Trigger alarm<br/>(DLQ not empty)
    Alert-->>Alert: Send notification<br/>to operations team

    Note over DLQ: Manual investigation<br/>and reprocessing required
```

---

## MCP Server Tool Execution

Detailed MCP protocol interaction.

```mermaid
sequenceDiagram
    participant Agent as Agent Lambda
    participant MCP as MCP Server
    participant Secrets as Secrets Manager
    participant External as External API
    participant DDB as Results Table

    Agent->>MCP: POST /mcp<br/>{"method": "initialize"}
    MCP-->>Agent: {protocolVersion,<br/>capabilities,<br/>serverInfo}

    Agent->>MCP: POST /mcp<br/>{"method": "tools/list"}
    MCP-->>Agent: {tools: [<br/> golf_search_tee_times,<br/> golf_book_tee_time,<br/> get_weather,<br/> send_notification<br/>]}

    Agent->>MCP: POST /mcp<br/>{"method": "tools/call",<br/> "params": {<br/>  "name": "golf_search_tee_times",<br/>  "arguments": {...}<br/>}}

    MCP->>Secrets: GetSecretValue
    Secrets-->>MCP: Credentials
    MCP->>External: OAuth + API call
    External-->>MCP: API response
    MCP->>DDB: Store result

    MCP-->>Agent: {content: [{<br/> type: "text",<br/> text: "Found 3 tee times..."<br/>}]}

    Agent->>Agent: LangChain processes<br/>tool result
    Agent->>Agent: Continue agent loop
```

---

## API Gateway Request Flow

HTTP API routing to Lambda functions.

```mermaid
sequenceDiagram
    participant Client
    participant API as API Gateway<br/>HTTP API
    participant Auth as Custom Authorizer<br/>(Future)
    participant Route as Route Handler
    participant Lambda as Target Lambda
    participant Logs as CloudWatch Logs

    Client->>API: HTTP Request
    API->>Logs: Log request details

    alt Authentication Required
        API->>Auth: Validate token
        Auth-->>API: Allow/Deny
    end

    API->>Route: Match route<br/>(e.g., POST /api/messages)
    Route->>Lambda: Invoke integration<br/>(AWS_PROXY)

    Lambda->>Lambda: Process request
    alt Success
        Lambda-->>Route: {statusCode: 200, body: {...}}
    else Error
        Lambda-->>Route: {statusCode: 500, body: {error}}
    end

    Route-->>API: Response
    API->>Logs: Log response
    API-->>Client: HTTP Response
```

---

## Session Management

AI Agent session persistence and retrieval.

```mermaid
sequenceDiagram
    participant Client
    participant Agent as Agent Lambda
    participant DDB as Sessions Table
    participant TTL as DynamoDB TTL

    Client->>Agent: POST /agent<br/>{session_id: "abc123"}

    alt Session Exists
        Agent->>DDB: GetItem(session_id)
        DDB-->>Agent: {session_id,<br/>messages: [...],<br/>created_at, ttl}
        Agent->>Agent: Restore conversation
    else New Session
        Agent->>Agent: Generate session_id
        Agent->>DDB: PutItem<br/>{session_id,<br/>created_at: now(),<br/>ttl: now() + 24h}
        DDB-->>Agent: Session created
    end

    Agent->>Agent: Process message<br/>with Bedrock
    Agent->>DDB: UpdateItem<br/>(append message,<br/>update ttl)

    Agent-->>Client: Response with session_id

    Note over DDB,TTL: After 24 hours
    TTL->>DDB: Delete expired session
```

---

## Cost Tracking and Rate Limiting

Bedrock usage tracking and daily budget enforcement.

```mermaid
sequenceDiagram
    participant User
    participant Agent as Agent Lambda
    participant Limiter as Cost Limiter
    participant DDB as Cost Tracking Table
    participant Bedrock as AWS Bedrock

    User->>Agent: Chat request
    Agent->>Limiter: Check budget
    Limiter->>DDB: Query today's usage
    DDB-->>Limiter: {total_cost: $2.50,<br/>daily_cap: $10.00}

    alt Budget Available
        Limiter->>Limiter: Estimate request cost<br/>(tokens * price)
        Limiter-->>Agent: Allowed (estimate: $0.15)

        Agent->>Bedrock: Invoke model
        Bedrock-->>Agent: Response + token counts

        Agent->>Limiter: Update actual cost
        Limiter->>DDB: UpdateItem<br/>(add cost, tokens)

        Agent-->>User: Response
    else Budget Exceeded
        Limiter-->>Agent: Denied (cap reached)
        Agent-->>User: 429 Too Many Requests<br/>"Daily spending limit reached"
    end

    Note over Limiter,DDB: Resets at midnight UTC
```

---

## Pulumi Deployment Flow

Infrastructure deployment and updates.

```mermaid
sequenceDiagram
    participant Dev as Developer
    participant CI as GitHub Actions
    participant Pulumi as Pulumi Service
    participant AWS as AWS

    Dev->>Dev: make build
    Dev->>CI: git push

    CI->>CI: Run tests
    CI->>CI: Build Lambda packages

    alt PR to main
        CI->>CI: pulumi preview
        CI-->>Dev: Show changes in PR
    else Push to main
        CI->>Pulumi: pulumi up --yes

        Pulumi->>AWS: Create/Update DynamoDB
        AWS-->>Pulumi: Table ARN

        Pulumi->>AWS: Create/Update Lambda
        AWS-->>Pulumi: Function ARN

        Pulumi->>AWS: Create/Update API Gateway
        AWS-->>Pulumi: API Endpoint

        Pulumi->>AWS: Create/Update IAM Roles
        Pulumi->>AWS: Configure EventBridge

        Pulumi-->>CI: Deployment complete
        CI-->>Dev: Success notification
    end
```

---

## Notes

### Diagram Conventions

- **Solid lines (→)**: Synchronous calls
- **Dashed lines (-->>)**: Return values/responses
- **Cross marks (--x)**: Failed operations
- **Notes**: Additional context or important information
- **Alt blocks**: Conditional logic paths
- **Loop blocks**: Repeated operations

### Viewing Diagrams

These diagrams use Mermaid syntax and can be viewed in:
- GitHub (native rendering)
- VS Code (with Mermaid extension)
- Documentation sites (MkDocs, Docusaurus, etc.)
- Mermaid Live Editor: https://mermaid.live/

### Related Documentation

- [Architecture Overview](architecture/README.md)
- [API Documentation](api/README.md)
- [Message Schemas](MESSAGE_SCHEMAS.md)
- [Deployment Guide](DEPLOYMENT_GUIDE.md)
