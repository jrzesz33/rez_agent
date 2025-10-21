# rez_agent Service Architecture

## System Overview

The rez_agent is an event-driven messaging system built on AWS serverless infrastructure. It processes scheduled and manual messages, tracks metadata, and delivers notifications via ntfy.sh. The architecture is designed for horizontal scalability, resilience, and future extensibility.

## Service Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          rez_agent System Architecture                   │
└─────────────────────────────────────────────────────────────────────────┘

┌──────────────────┐         ┌──────────────────┐
│  EventBridge     │         │  Web Frontend    │
│  Scheduler       │         │  (React/Next.js) │
│  (24h cron)      │         └────────┬─────────┘
└────────┬─────────┘                  │
         │                            │ HTTPS
         │ Trigger                    │
         │                            ▼
         │                  ┌─────────────────────┐
         │                  │  Application Load   │
         │                  │  Balancer (ALB)     │
         │                  └──────────┬──────────┘
         │                             │
         │                             ▼
         │                  ┌─────────────────────┐
         │                  │  API Gateway        │
         │                  │  (REST API)         │
         │                  │  + Lambda Authorizer│
         │                  └──────────┬──────────┘
         │                             │
         │                             │ Invoke
         ▼                             ▼
┌────────────────────┐      ┌─────────────────────┐
│ Scheduler Lambda   │      │  Web API Lambda     │
│ - Create message   │      │  - List messages    │
│ - Publish to SNS   │      │  - Create messages  │
└────────┬───────────┘      │  - Get metrics      │
         │                  └──────────┬──────────┘
         │                             │
         │                             │
         │  Publish                    │ Publish
         │                             │
         ▼                             ▼
┌─────────────────────────────────────────────────┐
│              SNS Topic                          │
│           (rez-agent-messages)                  │
└────────────────────┬────────────────────────────┘
                     │
                     │ Subscribe
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│              SQS Queue                          │
│        (rez-agent-message-queue)                │
│  - Visibility timeout: 5 minutes                │
│  - Message retention: 14 days                   │
│  - DLQ enabled                                  │
└────────────────────┬────────────────────────────┘
                     │
                     │ Poll (Event Source Mapping)
                     │ Batch size: 10
                     ▼
┌─────────────────────────────────────────────────┐
│        Message Processor Lambda                 │
│  - Update status to "processing"                │
│  - Process message logic                        │
│  - Call Notification Service                    │
│  - Update status to "completed"/"failed"        │
└────────────────────┬────────────────────────────┘
                     │
                     │ Invoke
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│        Notification Service Lambda              │
│  - Send to ntfy.sh API                          │
│  - Retry with exponential backoff               │
│  - Circuit breaker pattern                      │
└─────────────────────────────────────────────────┘
                     │
                     │ HTTP POST
                     ▼
                ┌──────────┐
                │ ntfy.sh  │
                │ External │
                │ Service  │
                └──────────┘

┌─────────────────────────────────────────────────┐
│           Data Layer (DynamoDB)                 │
│  - messages table (PK: message_id)              │
│  - GSI: stage-created_date-index                │
│  - GSI: status-created_date-index               │
│  - TTL enabled on old messages                  │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│          Observability & Security               │
│  - CloudWatch Logs (all Lambdas)                │
│  - CloudWatch Metrics (custom metrics)          │
│  - X-Ray Tracing (distributed tracing)          │
│  - AWS Cognito (user authentication)            │
│  - Systems Manager Parameter Store (secrets)    │
│  - KMS (encryption at rest)                     │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│          Dead Letter Queue (DLQ)                │
│  - Captures failed messages after max retries   │
│  - CloudWatch alarm on message count            │
│  - Manual replay or investigation               │
└─────────────────────────────────────────────────┘
```

## Service Boundaries

### 1. Scheduler Service (Lambda)
**Responsibility**: Generate scheduled messages and publish to SNS

**Trigger**: EventBridge rule (cron: `rate(24 hours)`)

**Functions**:
- Create message metadata (created_date, created_by="scheduler", stage, message_type)
- Generate unique message_id (UUID)
- Publish message event to SNS topic
- Record message in DynamoDB with status="created"

**Environment Variables**:
- `SNS_TOPIC_ARN`: Target SNS topic
- `DYNAMODB_TABLE_NAME`: Message metadata table
- `STAGE`: Environment (dev/stage/prod)

**Timeout**: 30 seconds
**Memory**: 256 MB
**Concurrency**: Reserved (1) to prevent duplicate scheduling

---

### 2. Web API Service (Lambda)
**Responsibility**: REST API for web frontend operations

**Trigger**: API Gateway HTTP requests

**Endpoints** (see OpenAPI spec):
- `GET /api/messages` - List and filter messages
- `POST /api/messages` - Create manual message
- `GET /api/metrics` - Dashboard metrics
- `POST /api/auth/login` - OAuth initiation
- `GET /api/auth/callback` - OAuth callback
- `GET /api/health` - Health check

**Functions**:
- Validate JWT tokens (via Lambda Authorizer)
- Query DynamoDB with filters and pagination
- Publish manual messages to SNS
- Aggregate metrics from DynamoDB
- Handle OAuth flow with Cognito

**Environment Variables**:
- `SNS_TOPIC_ARN`: Message publishing topic
- `DYNAMODB_TABLE_NAME`: Message metadata table
- `COGNITO_USER_POOL_ID`: User pool for authentication
- `COGNITO_CLIENT_ID`: OAuth client ID
- `JWT_SECRET_PARAM`: SSM parameter for JWT validation
- `STAGE`: Environment

**Timeout**: 15 seconds
**Memory**: 512 MB
**Concurrency**: Auto-scaling (min: 1, max: 100)

---

### 3. Message Processor Service (Lambda)
**Responsibility**: Process messages from SQS and coordinate delivery

**Trigger**: SQS queue event source mapping (batch size: 10)

**Functions**:
- Update message status to "processing" in DynamoDB
- Extract message payload and metadata
- Invoke Notification Service with retry logic
- Update message status to "completed" or "failed"
- Record processing metrics (duration, success/failure)
- Handle partial batch failures (return failed message IDs)

**Environment Variables**:
- `DYNAMODB_TABLE_NAME`: Message metadata table
- `NOTIFICATION_SERVICE_ARN`: Notification Lambda ARN
- `STAGE`: Environment

**Timeout**: 5 minutes (matches SQS visibility timeout)
**Memory**: 512 MB
**Concurrency**: Auto-scaling (max: 50)
**Reserved Concurrency**: Prevents throttling downstream services

**Error Handling**:
- Partial batch failure response (failed messages return to queue)
- Max retries: 3 (configured in SQS)
- After max retries → DLQ

---

### 4. Notification Service (Lambda)
**Responsibility**: Deliver notifications to ntfy.sh with resilience patterns

**Trigger**: Synchronous invocation from Message Processor

**Functions**:
- Send HTTP POST to ntfy.sh API
- Implement circuit breaker pattern (using in-memory state or DynamoDB)
- Exponential backoff retry (3 attempts: 1s, 2s, 4s)
- Log success/failure metrics
- Return delivery status to caller

**Circuit Breaker States**:
- **Closed**: Normal operation, requests flow through
- **Open**: Fail fast after threshold failures (5 failures in 1 minute)
- **Half-Open**: Test if service recovered (1 request)

**Environment Variables**:
- `NTFY_URL`: ntfy.sh base URL (e.g., `https://ntfy.sh/rez-agent-dev`)
- `NTFY_API_KEY_PARAM`: SSM parameter for ntfy.sh authentication (if required)
- `MAX_RETRIES`: 3
- `CIRCUIT_BREAKER_TABLE`: DynamoDB table for circuit state (optional)

**Timeout**: 30 seconds
**Memory**: 256 MB
**Retry Strategy**: Built-in (not Lambda retry, but application-level)

---

## Service Communication Patterns

### Asynchronous Message Flow (Primary)
```
Scheduler/Web API → SNS → SQS → Message Processor → Notification Service
```

**Benefits**:
- Decoupling: Publishers don't know about consumers
- Buffering: SQS handles load spikes
- Reliability: Message persistence and retries
- Scalability: Parallel processing with Lambda concurrency

**Message Flow**:
1. Scheduler or Web API creates message metadata in DynamoDB
2. Publishes event to SNS topic with message_id
3. SNS fans out to SQS queue (allows future additional subscribers)
4. SQS triggers Message Processor Lambda (batch processing)
5. Processor updates status, invokes Notification Service
6. Notification Service delivers to ntfy.sh
7. Processor updates final status in DynamoDB

### Synchronous Invocation (Internal)
```
Message Processor → Notification Service (Lambda invoke)
```

**Benefits**:
- Immediate response for success/failure
- Simpler error handling
- Direct control over retries

---

## Data Flow Sequence

### Scheduled Message Flow
```
1. EventBridge triggers Scheduler Lambda (every 24 hours)
2. Scheduler creates message record in DynamoDB:
   {
     message_id: "uuid",
     created_date: "2025-10-21T10:00:00Z",
     created_by: "scheduler",
     stage: "dev",
     message_type: "scheduled_hello",
     status: "created",
     payload: { text: "hello world" }
   }
3. Scheduler publishes to SNS topic:
   {
     message_id: "uuid",
     event_type: "message_created",
     timestamp: "2025-10-21T10:00:00Z"
   }
4. SNS delivers to SQS queue
5. Message Processor Lambda triggered:
   - Updates status to "processing"
   - Invokes Notification Service
6. Notification Service sends to ntfy.sh
7. Processor updates status to "completed"
```

### Manual Message Flow (Web API)
```
1. User authenticates via OAuth (Cognito)
2. Frontend sends POST /api/messages with JWT
3. API Gateway validates JWT (Lambda Authorizer)
4. Web API Lambda creates message in DynamoDB
5. Web API publishes to SNS (same flow as scheduled)
6. Returns 202 Accepted with message_id to frontend
7. Async processing continues via SQS → Processor → Notification
```

---

## Technology Decisions

### DynamoDB vs RDS
**Recommendation: DynamoDB**

**Rationale**:
- **Serverless-native**: No connection pooling issues with Lambda
- **Auto-scaling**: Handles variable load without capacity planning
- **Performance**: Single-digit millisecond latency for key-value access
- **Cost**: Pay-per-request pricing aligns with event-driven workload
- **No cold starts**: No connection overhead like RDS

**Trade-offs**:
- Limited query flexibility (use GSIs strategically)
- Eventual consistency for GSI queries (acceptable for this use case)
- No ACID transactions across multiple items (not required here)

**When to reconsider**:
- Complex relational queries needed
- ACID transactions critical
- Strong consistency always required
- Existing RDS infrastructure

---

### OAuth Provider
**Recommendation: AWS Cognito**

**Rationale**:
- **AWS-native**: Seamless integration with API Gateway, Lambda
- **Built-in features**: User pools, OAuth 2.0, JWT tokens, MFA
- **Managed service**: No infrastructure to maintain
- **Cost-effective**: Free tier (50k MAU), then $0.0055 per MAU
- **Identity federation**: Can integrate Google, Facebook, SAML

**Alternative: Auth0**
- **Pros**: Better developer experience, extensive docs, more features
- **Cons**: Higher cost ($240+/month), external dependency, vendor lock-in

**For rez_agent**: Cognito is recommended for simplicity and AWS integration.

---

### Logging Library
**Recommendation: Go `log/slog` (standard library)**

**Rationale**:
- **Standard library**: No external dependencies (Go 1.21+)
- **Structured logging**: Native JSON output
- **Performance**: Minimal overhead
- **Contextual logging**: Context propagation built-in

**Alternative: Zap**
- **Pros**: Faster, more features, field validation
- **Cons**: External dependency, more complex

**For rez_agent**: Start with `slog`, migrate to Zap if performance critical.

---

## Deployment Strategy

### Stages
- **dev**: Development environment, isolated resources
- **stage**: Pre-production, mirrors production config
- **prod**: Production environment, stricter alarms and quotas

### Infrastructure as Code
**Recommendation**: AWS CDK (Go) or Terraform

**CDK Benefits**:
- Type-safe (Go)
- AWS best practices built-in
- Constructs for common patterns (Lambda, API Gateway, etc.)

**Terraform Benefits**:
- Multi-cloud (if future expansion needed)
- Mature ecosystem, HCL language

**For rez_agent**: AWS CDK in Go aligns with application language.

### Blue-Green Deployment
**For Lambdas**:
- Use Lambda aliases (`live`, `blue`, `green`)
- Weighted traffic shifting (10% → 50% → 100%)
- CloudWatch alarms trigger automatic rollback

**For API Gateway**:
- Stage variables point to Lambda aliases
- Canary deployments (10% traffic to new version)

---

## Scalability Considerations

### Horizontal Scaling
- **Lambdas**: Auto-scale based on event rate (max concurrency limits)
- **SQS**: Unlimited throughput (standard queue)
- **DynamoDB**: On-demand mode (auto-scales read/write capacity)
- **API Gateway**: 10,000 requests/second default (can increase)

### Throttling and Backpressure
- **SQS visibility timeout**: 5 minutes (matches Lambda timeout)
- **Lambda reserved concurrency**: Prevent overwhelming ntfy.sh
- **Circuit breaker**: Fail fast when ntfy.sh down
- **API rate limiting**: API Gateway throttling (1000 req/sec per stage)

### Cost Optimization
- **Lambda memory**: Right-size based on profiling (start 256 MB)
- **DynamoDB on-demand**: Pay only for actual usage
- **CloudWatch Logs retention**: 7 days for dev, 30 days for prod
- **X-Ray sampling**: 5% sampling in prod (reduce cost)

---

## Security Boundaries

### Network Security
- **VPC**: Not required for Lambdas (public AWS services only)
- **If future RDS/ElastiCache**: Place Lambdas in VPC with NAT Gateway

### Data Security
- **Encryption at rest**: DynamoDB (KMS), SQS, SNS, Parameter Store
- **Encryption in transit**: HTTPS/TLS for all API calls
- **Secrets management**: AWS Systems Manager Parameter Store (SecureString)
- **IAM roles**: Least-privilege per Lambda function

### Authentication
- **Frontend users**: Cognito User Pool with OAuth 2.0
- **Service-to-service**: IAM roles (Lambda execution roles)
- **API Gateway**: Lambda Authorizer validates JWT tokens

---

## Future Extensibility

### Adding New Message Types
1. Add new `message_type` enum value
2. Implement type-specific processing in Message Processor
3. No changes to SNS/SQS schema (generic message envelope)

### Additional Notification Channels
1. Add new Notification Service Lambda (e.g., SendGrid, Twilio)
2. Subscribe to same SNS topic with message filtering
3. Message Processor routes based on message metadata

### Multi-Region Deployment
1. Replicate DynamoDB with Global Tables
2. Deploy Lambdas in multiple regions
3. Route53 for global traffic management
4. SNS/SQS per region (eventual consistency acceptable)

---

## Non-Functional Requirements

### Performance Targets
- **API response time**: p95 < 500ms, p99 < 1000ms
- **Message processing latency**: p95 < 10 seconds (end-to-end)
- **Notification delivery**: p95 < 5 seconds (ntfy.sh call)

### Availability Targets
- **API availability**: 99.9% (managed by AWS services)
- **Message delivery**: 99.95% (with retries and DLQ)
- **Data durability**: 99.999999999% (DynamoDB guarantee)

### Compliance
- **Data retention**: 90 days default, configurable via DynamoDB TTL
- **Audit logging**: CloudWatch Logs with retention policy
- **GDPR considerations**: User data in Cognito, message metadata in DynamoDB

---

## Summary

The rez_agent architecture leverages AWS serverless services for a highly scalable, resilient, and cost-effective event-driven messaging system. Key design principles:

1. **Event-driven**: SNS/SQS decouples services
2. **Serverless**: No infrastructure management, auto-scaling
3. **Resilient**: Retries, DLQ, circuit breakers, timeouts
4. **Observable**: Structured logging, metrics, distributed tracing
5. **Secure**: Cognito, IAM, encryption, least-privilege
6. **Extensible**: Generic message schema, pluggable notification services

This architecture supports the initial "hello world" use case while providing a foundation for complex multi-job-type workflows.
