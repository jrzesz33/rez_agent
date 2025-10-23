# Web Action Processor - Architecture Diagrams

This document contains visual architecture diagrams for the Web Action Processor feature.

---

## System Architecture Overview

```mermaid
graph TB
    subgraph "Scheduled Triggers"
        EB1[EventBridge Scheduler<br/>Weather: 5:00 AM EST]
        EB2[EventBridge Scheduler<br/>Golf: 5:15 AM EST]
    end

    subgraph "Scheduler Service"
        SL[Scheduler Lambda<br/>Creates web_action messages]
    end

    subgraph "Message Persistence"
        DDB1[(DynamoDB<br/>rez-agent-messages)]
    end

    subgraph "Event Distribution"
        SNS1[SNS Topic<br/>rez-agent-web-actions]
        SQS1[SQS Queue<br/>rez-agent-web-actions]
        DLQ1[DLQ<br/>Failed Messages]
    end

    subgraph "Web Action Processing"
        WAP[Web Action Processor Lambda<br/>- Parse message<br/>- Execute HTTP<br/>- Store result<br/>- Publish completion]
    end

    subgraph "Results Storage"
        DDB2[(DynamoDB<br/>web-action-results<br/>TTL: 3 days)]
    end

    subgraph "External APIs"
        WX[Weather.gov API<br/>GET /gridpoints/...]
        GOLF[Birdsfoot Golf API<br/>OAuth + REST]
    end

    subgraph "Secrets"
        SM[Secrets Manager<br/>Golf Credentials]
    end

    subgraph "Notification Flow"
        SNS2[SNS Topic<br/>rez-agent-messages]
        SQS2[SQS Queue<br/>rez-agent-messages]
        MP[Message Processor<br/>existing]
        NS[Notification Service<br/>ntfy.sh]
    end

    EB1 --> SL
    EB2 --> SL
    SL --> DDB1
    SL --> SNS1
    SNS1 --> SQS1
    SQS1 --> WAP
    SQS1 -.retry 3x.-> DLQ1
    WAP --> DDB1
    WAP --> DDB2
    WAP --> WX
    WAP --> GOLF
    WAP --> SM
    WAP --> SNS2
    SNS2 --> SQS2
    SQS2 --> MP
    MP --> NS

    style EB1 fill:#f9f,stroke:#333,stroke-width:2px
    style EB2 fill:#f9f,stroke:#333,stroke-width:2px
    style WAP fill:#bbf,stroke:#333,stroke-width:3px
    style DDB2 fill:#bfb,stroke:#333,stroke-width:2px
    style DLQ1 fill:#fbb,stroke:#333,stroke-width:2px
```

---

## Weather Action Flow

```mermaid
sequenceDiagram
    participant EB as EventBridge<br/>(5:00 AM EST)
    participant SL as Scheduler Lambda
    participant DDB as DynamoDB<br/>messages
    participant SNS as SNS Topic<br/>web-actions
    participant SQS as SQS Queue
    participant WAP as Web Action<br/>Processor
    participant WX as Weather.gov API
    participant RDB as DynamoDB<br/>results
    participant NS as Notification<br/>Service

    EB->>SL: Trigger (cron)
    SL->>SL: Create web_action message<br/>payload: weather
    SL->>DDB: PutItem (status: created)
    SL->>SNS: Publish (message_id)
    SL->>DDB: UpdateItem (status: queued)
    SNS->>SQS: Deliver message
    SQS->>WAP: Lambda invocation
    WAP->>DDB: GetItem (fetch message)
    WAP->>DDB: UpdateItem (status: processing)
    WAP->>WX: GET /gridpoints/PBZ/82,69/forecast
    WX-->>WAP: JSON response (forecast data)
    WAP->>WAP: Parse JSON<br/>Extract N days forecast<br/>Format notification
    WAP->>RDB: PutItem (result, TTL: 3 days)
    WAP->>DDB: PutItem (notification message)
    WAP->>SNS: Publish (notification event)
    WAP->>DDB: UpdateItem (status: completed)
    SNS->>NS: Trigger notification flow
    NS-->>NS: Send to ntfy.sh
```

---

## Golf Action Flow (with OAuth)

```mermaid
sequenceDiagram
    participant EB as EventBridge<br/>(5:15 AM EST)
    participant SL as Scheduler Lambda
    participant DDB as DynamoDB<br/>messages
    participant SNS as SNS Topic<br/>web-actions
    participant SQS as SQS Queue
    participant WAP as Web Action<br/>Processor
    participant SM as Secrets<br/>Manager
    participant AUTH as Golf API<br/>/connect/token
    participant API as Golf API<br/>/UpcomingReservation
    participant RDB as DynamoDB<br/>results
    participant NS as Notification<br/>Service

    EB->>SL: Trigger (cron)
    SL->>SL: Create web_action message<br/>payload: golf
    SL->>DDB: PutItem (status: created)
    SL->>SNS: Publish (message_id)
    SL->>DDB: UpdateItem (status: queued)
    SNS->>SQS: Deliver message
    SQS->>WAP: Lambda invocation
    WAP->>DDB: GetItem (fetch message)
    WAP->>DDB: UpdateItem (status: processing)

    Note over WAP,SM: OAuth Authentication
    WAP->>SM: GetSecretValue<br/>(credentials)
    SM-->>WAP: username, password
    WAP->>AUTH: POST /connect/token<br/>(OAuth password grant)
    AUTH-->>WAP: access_token (expires: 60m)
    WAP->>WAP: Cache token (50m TTL)

    Note over WAP,API: Fetch Reservations
    WAP->>API: GET /UpcomingReservation<br/>Authorization: Bearer {token}
    API-->>WAP: JSON response (reservations)

    WAP->>WAP: Parse JSON<br/>Sort by date<br/>Take first 4<br/>Format notification
    WAP->>RDB: PutItem (result, TTL: 3 days)
    WAP->>DDB: PutItem (notification message)
    WAP->>SNS: Publish (notification event)
    WAP->>DDB: UpdateItem (status: completed)
    SNS->>NS: Trigger notification flow
    NS-->>NS: Send to ntfy.sh
```

---

## Error Handling Flow

```mermaid
graph TB
    START[Web Action Processor<br/>Receives SQS Message]
    FETCH[Fetch Message from DynamoDB]
    PARSE[Parse WebActionPayload]
    EXEC[Execute Action Handler]
    HTTP[HTTP Request]
    SUCCESS{HTTP<br/>Success?}
    RETRY{Retry<br/>Attempts<br/>< 3?}
    STORE_SUCCESS[Store Success Result<br/>DynamoDB]
    STORE_FAIL[Store Failed Result<br/>DynamoDB]
    PUBLISH[Publish Notification Event]
    UPDATE_COMPLETE[Update Message<br/>status: completed]
    UPDATE_FAILED[Update Message<br/>status: failed]
    DELETE_MSG[Delete SQS Message<br/>Success]
    RETURN_ERROR[Return Error to SQS]
    SQS_RETRY{SQS<br/>Retries<br/>< 3?}
    DLQ[Send to DLQ]
    ALARM[Trigger CloudWatch Alarm]

    START --> FETCH
    FETCH --> PARSE
    PARSE --> EXEC
    EXEC --> HTTP
    HTTP --> SUCCESS

    SUCCESS -->|Yes| STORE_SUCCESS
    SUCCESS -->|No 5xx/timeout| RETRY
    SUCCESS -->|No 4xx| STORE_FAIL

    RETRY -->|Yes| HTTP
    RETRY -->|No| STORE_FAIL

    STORE_SUCCESS --> PUBLISH
    PUBLISH --> UPDATE_COMPLETE
    UPDATE_COMPLETE --> DELETE_MSG

    STORE_FAIL --> UPDATE_FAILED
    UPDATE_FAILED --> RETURN_ERROR
    RETURN_ERROR --> SQS_RETRY
    SQS_RETRY -->|Yes| START
    SQS_RETRY -->|No| DLQ
    DLQ --> ALARM

    style START fill:#bfb,stroke:#333,stroke-width:2px
    style DELETE_MSG fill:#bfb,stroke:#333,stroke-width:2px
    style DLQ fill:#fbb,stroke:#333,stroke-width:3px
    style ALARM fill:#fbb,stroke:#333,stroke-width:2px
    style STORE_SUCCESS fill:#9f9,stroke:#333,stroke-width:2px
    style STORE_FAIL fill:#f99,stroke:#333,stroke-width:2px
```

---

## Data Model Relationships

```mermaid
erDiagram
    MESSAGE ||--o{ WEB_ACTION_RESULT : creates
    MESSAGE {
        string id PK
        timestamp created_date SK
        string message_type
        string payload
        string status
        string stage
    }

    WEB_ACTION_RESULT {
        string action_id PK
        timestamp executed_at SK
        string message_id FK
        string action
        string url
        string status
        int http_status_code
        string response_body
        string transformed_result
        string error_message
        int ttl
        int duration_ms
        string stage
    }

    WEB_ACTION_PAYLOAD {
        string version
        string url
        string action
        json arguments
        json auth_config
    }

    AUTH_CONFIG {
        string type
        string secret_name
        string token_url
        string scope
        json headers
    }

    MESSAGE ||--|| WEB_ACTION_PAYLOAD : contains
    WEB_ACTION_PAYLOAD ||--o| AUTH_CONFIG : may_have
```

---

## OAuth Token Caching

```mermaid
stateDiagram-v2
    [*] --> CheckCache: GetToken() called
    CheckCache --> CacheHit: Token exists & valid
    CheckCache --> CacheMiss: No token or expired

    CacheMiss --> FetchSecret: Retrieve credentials
    FetchSecret --> AuthRequest: POST /connect/token
    AuthRequest --> ValidateResponse: Check HTTP 200
    ValidateResponse --> CacheToken: Store token (50m TTL)
    CacheToken --> ReturnToken

    CacheHit --> ReturnToken
    ReturnToken --> [*]

    AuthRequest --> AuthFailed: HTTP 401/403
    AuthFailed --> [*]: Return error

    note right of CacheToken
        Token TTL: 50 minutes
        API token expiry: 60 minutes
        Safe 10-minute margin
    end note
```

---

## Infrastructure Components

```mermaid
graph LR
    subgraph "EventBridge Schedulers"
        ES1[Weather Scheduler<br/>cron: 0 10 * * ? *]
        ES2[Golf Scheduler<br/>cron: 15 10 * * ? *]
    end

    subgraph "Lambda Functions"
        L1[Scheduler Lambda<br/>256MB, 60s]
        L2[Web Action Processor<br/>512MB, 300s]
    end

    subgraph "DynamoDB Tables"
        T1[messages<br/>On-Demand<br/>TTL: 90 days]
        T2[web-action-results<br/>On-Demand<br/>TTL: 3 days]
    end

    subgraph "SNS Topics"
        S1[web-actions]
        S2[messages]
    end

    subgraph "SQS Queues"
        Q1[web-actions<br/>Visibility: 5m]
        Q2[web-actions-dlq<br/>Retention: 14d]
    end

    subgraph "Secrets Manager"
        SEC1[golf/credentials<br/>KMS encrypted]
    end

    subgraph "CloudWatch"
        CW1[Log Groups]
        CW2[Metrics]
        CW3[Alarms]
        CW4[Dashboard]
    end

    ES1 --> L1
    ES2 --> L1
    L1 --> T1
    L1 --> S1
    S1 --> Q1
    Q1 --> L2
    Q1 -.failed.-> Q2
    L2 --> T1
    L2 --> T2
    L2 --> S2
    L2 --> SEC1
    L2 --> CW1
    L2 --> CW2
    Q2 --> CW3
    CW2 --> CW4

    style L2 fill:#bbf,stroke:#333,stroke-width:3px
    style T2 fill:#bfb,stroke:#333,stroke-width:2px
    style Q2 fill:#fbb,stroke:#333,stroke-width:2px
    style SEC1 fill:#ffb,stroke:#333,stroke-width:2px
```

---

## Action Handler Registry Pattern

```mermaid
classDiagram
    class Processor {
        -registry ActionRegistry
        -messageRepo MessageRepository
        -resultsRepo ResultsRepository
        +ProcessWebAction(msg Message) error
    }

    class ActionRegistry {
        -handlers map[string]ActionHandler
        +Register(name string, handler ActionHandler)
        +Get(name string) ActionHandler
    }

    class ActionHandler {
        <<interface>>
        +Execute(ctx, payload) ActionResult
    }

    class WeatherHandler {
        -httpClient HTTPClient
        -logger Logger
        +Execute(ctx, payload) ActionResult
    }

    class GolfHandler {
        -httpClient HTTPClient
        -oauthClient OAuthClient
        -logger Logger
        +Execute(ctx, payload) ActionResult
    }

    class ActionResult {
        +HTTPStatusCode int
        +ResponseBody string
        +TransformedResult string
        +Error error
        +Duration time.Duration
    }

    Processor --> ActionRegistry
    ActionRegistry --> ActionHandler
    ActionHandler <|.. WeatherHandler
    ActionHandler <|.. GolfHandler
    ActionHandler --> ActionResult
```

---

## Observability Stack

```mermaid
graph TB
    subgraph "Lambda Execution"
        L[Web Action Processor]
    end

    subgraph "Structured Logging"
        LOG1[JSON Logs<br/>- Correlation ID<br/>- Action type<br/>- Duration<br/>- HTTP status]
    end

    subgraph "CloudWatch Logs"
        CWL[Log Groups<br/>/aws/lambda/rez-agent-webaction]
    end

    subgraph "CloudWatch Metrics"
        M1[WebActionExecuted]
        M2[WebActionSuccess]
        M3[WebActionFailed]
        M4[WebActionDuration]
        M5[OAuthTokenCacheHit]
    end

    subgraph "X-Ray Tracing"
        XRAY[Distributed Traces<br/>- HTTP requests<br/>- DynamoDB calls<br/>- Secrets Manager<br/>- SNS publish]
    end

    subgraph "CloudWatch Alarms"
        A1[DLQ Messages > 0]
        A2[Lambda Errors > 3]
        A3[OAuth Failures > 2]
        A4[High Latency p95]
    end

    subgraph "CloudWatch Dashboard"
        DASH[Web Actions Dashboard<br/>- Execution count<br/>- Success rate<br/>- Duration trends<br/>- Error logs]
    end

    subgraph "Alerts"
        SNS[SNS Topic<br/>Ops Team]
    end

    L --> LOG1
    LOG1 --> CWL
    L --> M1
    L --> M2
    L --> M3
    L --> M4
    L --> M5
    L --> XRAY
    M3 --> A1
    M1 --> A2
    M5 --> A3
    M4 --> A4
    A1 --> SNS
    A2 --> SNS
    A3 --> SNS
    A4 --> SNS
    M1 --> DASH
    M2 --> DASH
    M3 --> DASH
    M4 --> DASH
    CWL --> DASH

    style L fill:#bbf,stroke:#333,stroke-width:3px
    style DASH fill:#bfb,stroke:#333,stroke-width:2px
    style SNS fill:#fbb,stroke:#333,stroke-width:2px
```

---

## Deployment Architecture

```mermaid
graph TB
    subgraph "Development"
        DEV_EB[EventBridge Dev]
        DEV_L[Lambda Dev]
        DEV_DDB[DynamoDB Dev]
        DEV_SNS[SNS Dev]
        DEV_SQS[SQS Dev]
    end

    subgraph "Production"
        PROD_EB[EventBridge Prod]
        PROD_L[Lambda Prod]
        PROD_DDB[DynamoDB Prod]
        PROD_SNS[SNS Prod]
        PROD_SQS[SQS Prod]
    end

    subgraph "Shared Resources"
        SM[Secrets Manager<br/>Multi-environment]
        CW[CloudWatch<br/>Separate log groups]
    end

    subgraph "Deployment Pipeline"
        GH[GitHub Actions]
        PULUMI[Pulumi<br/>Infrastructure as Code]
        BUILD[Go Build<br/>GOOS=linux]
    end

    DEV_EB --> DEV_L
    DEV_L --> DEV_DDB
    DEV_L --> DEV_SNS
    DEV_SNS --> DEV_SQS

    PROD_EB --> PROD_L
    PROD_L --> PROD_DDB
    PROD_L --> PROD_SNS
    PROD_SNS --> PROD_SQS

    DEV_L --> SM
    PROD_L --> SM
    DEV_L --> CW
    PROD_L --> CW

    GH --> BUILD
    BUILD --> PULUMI
    PULUMI --> DEV_EB
    PULUMI --> PROD_EB

    style PULUMI fill:#f9f,stroke:#333,stroke-width:2px
    style SM fill:#ffb,stroke:#333,stroke-width:2px
```

---

These diagrams provide a comprehensive visual representation of the Web Action Processor architecture, covering:

1. **System Overview**: High-level component interaction
2. **Message Flows**: Weather and Golf action sequences
3. **Error Handling**: Retry logic and DLQ flow
4. **Data Models**: DynamoDB table relationships
5. **OAuth Caching**: Token lifecycle state machine
6. **Infrastructure**: AWS resource topology
7. **Action Registry**: Code architecture pattern
8. **Observability**: Logging, metrics, tracing, and alerting
9. **Deployment**: Multi-environment architecture

These diagrams can be rendered in any Markdown viewer that supports Mermaid (GitHub, VS Code, etc.).
