# rez_agent Backend Architecture - Complete Design

## Executive Summary

I have designed a comprehensive backend service architecture for the **rez_agent** event-driven messaging system. The system is built on AWS serverless infrastructure using Go, processes scheduled and manual messages, and delivers notifications via ntfy.sh.

**Architecture Status**: ✅ Complete and ready for implementation

---

## Deliverables

All architecture documentation is located in `/workspaces/rez_agent/docs/`:

### 1. Service Architecture (`docs/architecture/service-architecture.md`)
**26 pages** - Complete system design with:
- Service boundary definitions (4 Lambda functions)
- System architecture diagram (ASCII visualization)
- Message flow patterns (EventBridge → SNS → SQS → Lambda → ntfy.sh)
- Technology stack decisions (DynamoDB vs RDS, Cognito vs Auth0)
- Deployment strategy (blue-green with Lambda aliases)
- Scalability and future extensibility patterns

**Key Services**:
- **Scheduler Service**: EventBridge triggers Lambda every 24h to create scheduled messages
- **Web API Service**: REST API behind API Gateway for frontend operations
- **Message Processor Service**: Consumes from SQS, orchestrates message processing
- **Notification Service**: Integrates with ntfy.sh API with circuit breaker pattern

---

### 2. Data Model (`docs/architecture/data-model.md`)
**21 pages** - Complete data layer design:
- DynamoDB table schema (messages table with PK: message_id)
- Global Secondary Indexes (stage-created_date, status-created_date)
- Message status state machine (created → processing → completed/failed)
- Access patterns with Go SDK v2 examples
- TTL configuration (90-day retention)
- Performance optimization (pagination, batch operations, conditional writes)

**Design Decision**: DynamoDB chosen over RDS for serverless-native architecture, auto-scaling, and no connection pooling issues with Lambda.

---

### 3. OpenAPI Specification (`docs/api/openapi.yaml`)
**600+ lines** - Complete REST API contract:
- 8 endpoints (messages CRUD, metrics, auth flow, health check)
- Request/response schemas with examples
- Authentication flow (OAuth 2.0 with Cognito)
- Error response formats with error codes
- Interactive documentation ready (Swagger UI, Redoc)

**Endpoints**:
- `GET/POST /api/messages` - List/create messages
- `GET /api/metrics` - Dashboard metrics
- `POST /api/auth/login`, `GET /api/auth/callback`, `POST /api/auth/refresh` - OAuth flow
- `GET /api/health` - Health check

---

### 4. Message Schemas (`docs/architecture/message-schemas.md`)
**19 pages** - SNS/SQS message formats:
- SNS message schema (published by Scheduler/Web API)
- SQS message schema (consumed by Message Processor)
- Go struct definitions for message parsing
- Message processing flow with Go code examples
- Idempotency and deduplication strategies
- Schema versioning approach (v1.0 with forward compatibility)

**Design Decision**: Minimal payload (only message_id in SNS/SQS), full data in DynamoDB as single source of truth.

---

### 5. Authentication & Authorization (`docs/architecture/authentication-authorization.md`)
**24 pages** - Complete security model:
- OAuth 2.0 authorization code flow with AWS Cognito
- JWT token structure and validation (Lambda Authorizer implementation in Go)
- Service-to-service authentication (IAM roles with least-privilege policies)
- Secrets management (Parameter Store with KMS encryption)
- Future RBAC enhancement design (admin, user, viewer roles)

**Components**:
- AWS Cognito User Pool for user authentication
- Lambda Authorizer validates JWT tokens before API access
- Dedicated IAM roles per Lambda function
- Parameter Store for API keys and configuration

---

### 6. Error Handling & Resilience (`docs/architecture/error-handling-resilience.md`)
**21 pages** - Comprehensive resilience patterns:
- Error categorization (transient, permanent, validation, system)
- Retry strategy (exponential backoff with jitter: 1s → 2s → 4s)
- Circuit breaker pattern (DynamoDB-backed state, 5 failures → OPEN)
- Timeout management (Lambda: 30s-5min, HTTP client: 10s)
- SQS Dead Letter Queue configuration (3 retries → DLQ)
- Idempotency patterns (conditional DynamoDB updates)
- Graceful degradation strategies (cached metrics, circuit breaker fallback)

**Go Implementation**: Full circuit breaker, retry logic, and error response code provided.

---

### 7. Observability & Monitoring (`docs/architecture/observability-monitoring.md`)
**25 pages** - Full observability stack:
- Structured logging with Go `log/slog` (JSON format, correlation IDs)
- CloudWatch Logs configuration (retention: 7d dev, 30d prod)
- Custom CloudWatch metrics (messages processed, notification duration, circuit breaker state)
- Distributed tracing with AWS X-Ray (trace examples, sampling strategy)
- CloudWatch alarms (DLQ, errors, latency, circuit breaker open)
- Dashboard design (overview with 12 widgets)
- SLI/SLO definitions (99.9% availability, p95 < 500ms API response time)

**Key Metrics**:
- MessagesCreated, MessagesProcessed, NotificationsSent
- ProcessingDuration, NotificationDuration, CircuitBreakerState
- API response time, success rate

---

### 8. Configuration Management (`docs/architecture/configuration-management.md`)
**22 pages** - Configuration strategy:
- Environment variables (Lambda configuration for non-sensitive values)
- AWS Systems Manager Parameter Store (secrets with KMS encryption)
- DynamoDB dynamic configuration (feature flags, circuit breaker state)
- Go configuration loading patterns with validation
- Parameter caching (5-minute TTL in Lambda globals)
- Per-environment configuration (dev, stage, prod)
- Secrets rotation process (manual with 5-minute downtime)

**Parameter Hierarchy**: `/rez-agent/{stage}/{parameter-name}`

---

### 9. Quick Reference Guide (`docs/architecture/quick-reference.md`)
**15 pages** - Implementation cheat sheet:
- Component summary table (all Lambdas with timeout/memory)
- Message flow diagrams (scheduled and manual)
- Environment variables cheat sheet per Lambda
- API endpoints quick reference
- Error handling summary (retry, circuit breaker, DLQ)
- Observability quick reference (metrics, CloudWatch Logs Insights queries)
- Troubleshooting guide (common issues and resolutions)
- Performance targets (SLI/SLO)
- Cost estimates (monthly: ~$10.57 for 1k messages/day)

---

### 10. Architecture README (`docs/architecture/README.md`)
**25 pages** - Implementation roadmap:
- All architecture documents overview
- 8-week implementation roadmap (Phase 1-5)
- Technology stack summary
- Design principles recap
- Go module structure recommendation
- Next steps for implementation

**Phases**:
1. Foundation (Week 1-2): Infrastructure + Scheduler + Processor + Notification
2. Web API (Week 3-4): Lambda Authorizer + Web API + API Gateway
3. Resilience (Week 5): Circuit breaker + enhanced error handling
4. Observability (Week 6): Logging + metrics + tracing + alarms
5. Frontend & Polish (Week 7-8): React dashboard + documentation + testing

---

## Architecture Highlights

### Event-Driven Flow
```
EventBridge (24h schedule) → Scheduler Lambda
                             ↓
                          DynamoDB (create message)
                             ↓
                          SNS Topic (publish event)
                             ↓
                          SQS Queue (buffer messages)
                             ↓
                     Message Processor Lambda (batch: 10)
                             ↓
                     Notification Service Lambda
                             ↓
                          ntfy.sh API (HTTP POST)
```

**Async Processing**: SNS/SQS decouples services, allows parallel processing, handles backpressure.

---

### Service Boundaries

| Service | Responsibility | Input | Output |
|---------|---------------|-------|--------|
| Scheduler | Generate scheduled messages | EventBridge cron | SNS event |
| Web API | REST API for frontend | API Gateway requests | JSON responses |
| Message Processor | Orchestrate message processing | SQS events | Status updates |
| Notification Service | Deliver to ntfy.sh | Lambda invoke | Success/failure |

**Separation of Concerns**: Each Lambda has single responsibility, clear inputs/outputs.

---

### Resilience Patterns

1. **Exponential Backoff Retries**: 3 attempts with increasing delay (1s → 2s → 4s)
2. **Circuit Breaker**: Fail fast when ntfy.sh unavailable (5 failures → OPEN for 30s)
3. **Dead Letter Queue**: Capture permanently failed messages for investigation
4. **Idempotency**: Conditional DynamoDB updates prevent duplicate processing
5. **Timeout Management**: Lambda timeouts match SQS visibility timeout (5 minutes)

**Result**: System resilient to transient failures, recovers gracefully, prevents cascading failures.

---

### Security Model

- **User Authentication**: OAuth 2.0 with AWS Cognito (authorization code grant)
- **API Authorization**: Lambda Authorizer validates JWT tokens (signature, expiration, issuer)
- **Service-to-Service**: IAM roles with least-privilege policies per Lambda
- **Secrets**: Parameter Store (SecureString) with KMS encryption
- **Network**: HTTPS only, CORS configured, rate limiting (1000 req/sec prod)

**Zero Trust**: Every request authenticated/authorized, no implicit trust between services.

---

### Observability

**Three Pillars**:
1. **Logging**: Structured JSON logs with correlation IDs (search across all services)
2. **Metrics**: CloudWatch custom metrics (messages, notifications, duration, circuit breaker)
3. **Tracing**: AWS X-Ray distributed tracing (visualize request flow, identify bottlenecks)

**Proactive Monitoring**: CloudWatch alarms on DLQ, errors, latency, circuit breaker state → SNS → Slack/PagerDuty.

---

## Technology Stack

### AWS Services
- **Compute**: Lambda (Go 1.24)
- **API**: API Gateway (HTTP API)
- **Messaging**: SNS (pub/sub), SQS (queue + DLQ)
- **Database**: DynamoDB (on-demand, TTL)
- **Scheduling**: EventBridge (cron)
- **Auth**: Cognito (OAuth 2.0, JWT)
- **Secrets**: Systems Manager Parameter Store
- **Observability**: CloudWatch Logs/Metrics, X-Ray

### Go Libraries
- `log/slog`: Structured logging
- `github.com/aws/aws-lambda-go`: Lambda runtime
- `github.com/aws/aws-sdk-go-v2`: AWS SDK
- `github.com/golang-jwt/jwt/v5`: JWT validation
- `github.com/google/uuid`: UUID generation

### Infrastructure as Code
- **AWS CDK (Go)** or **Terraform** for infrastructure deployment

---

## Cost Estimate

**Assumptions**: 1,000 messages/day, 10,000 API requests/day

| Category | Monthly Cost |
|----------|--------------|
| Lambda (compute) | $0.86 |
| DynamoDB (storage + I/O) | $0.37 |
| API Gateway | $0.30 |
| SNS/SQS | $0.50 |
| CloudWatch Logs/Metrics | $8.50 |
| X-Ray (5% sampling) | $0.04 |
| **Total** | **~$10.57/month** |

**At 10x scale** (10,000 messages/day): ~$50/month

**Cost Optimizations**: On-demand DynamoDB, X-Ray sampling (5%), log retention policies, reserved Lambda concurrency.

---

## Performance Targets (SLI/SLO)

| Metric | Target |
|--------|--------|
| API Availability | 99.9% (30-day rolling) |
| API Response Time (p95) | < 500ms |
| API Response Time (p99) | < 1000ms |
| Message Processing Latency (p95) | < 10 seconds (end-to-end) |
| Notification Delivery Success Rate | 99% (7-day rolling) |

**Error Budget** (99.9% availability): 43 minutes downtime per month

---

## Implementation Roadmap

### Phase 1: Foundation (Week 1-2)
- Set up DynamoDB tables, SNS/SQS queues, Cognito User Pool
- Implement Scheduler Lambda (EventBridge → create message → SNS)
- Implement Message Processor Lambda (SQS → process → update status)
- Implement Notification Service Lambda (send to ntfy.sh)
- **Deliverable**: Scheduled "hello world" messages every 24 hours

### Phase 2: Web API (Week 3-4)
- Implement Lambda Authorizer (JWT validation)
- Implement Web API Lambda (all endpoints from OpenAPI spec)
- Configure API Gateway with CORS and rate limiting
- **Deliverable**: REST API for frontend (view messages, create manual messages)

### Phase 3: Resilience (Week 5)
- Implement circuit breaker (DynamoDB-backed state)
- Enhance error handling (structured errors, graceful degradation)
- Configure DLQ alarms
- **Deliverable**: System resilient to ntfy.sh outages

### Phase 4: Observability (Week 6)
- Implement structured logging (slog with correlation IDs)
- Add custom CloudWatch metrics
- Integrate AWS X-Ray tracing
- Configure CloudWatch alarms and dashboard
- **Deliverable**: Full observability stack with proactive alerting

### Phase 5: Frontend & Polish (Week 7-8)
- Build React/Next.js dashboard (OAuth integration, message list, metrics)
- Write runbooks for common issues
- Write unit and integration tests
- **Deliverable**: Production-ready system with web interface

---

## Next Steps

1. **Review Architecture Documents**: Read `/workspaces/rez_agent/docs/architecture/README.md` first, then review individual documents as needed.

2. **Set Up AWS Environment**:
   - Create AWS account and configure IAM users
   - Set up development tools (AWS CLI, AWS CDK or Terraform)

3. **Initialize Go Project**:
   ```bash
   cd /workspaces/rez_agent
   go mod init github.com/your-org/rez_agent
   mkdir -p cmd/{scheduler,web-api,message-processor,notification-service,jwt-authorizer}
   mkdir -p internal/{config,models,dynamodb,sns,sqs,ssm,circuitbreaker,retry,correlation}
   ```

4. **Implement Phase 1** (Foundation):
   - Deploy infrastructure (DynamoDB, SNS, SQS, EventBridge)
   - Implement Scheduler Lambda
   - Implement Message Processor Lambda
   - Implement Notification Service Lambda
   - Test scheduled message flow

5. **Iterate Through Remaining Phases** following the roadmap.

---

## Key Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Data Store** | DynamoDB | Serverless-native, auto-scaling, no connection pooling issues |
| **Message Queue** | SNS + SQS | Managed, serverless, decouples services, handles backpressure |
| **Auth Provider** | AWS Cognito | AWS-native, cost-effective, seamless Lambda integration |
| **Logging** | log/slog | Standard library, structured JSON, no external dependencies |
| **API Type** | HTTP API | Simpler, cheaper than REST API, sufficient features |
| **Capacity Mode** | On-Demand | Variable workload, no capacity planning required |
| **IaC Tool** | AWS CDK (Go) | Type-safe, matches application language, AWS best practices |

---

## Documentation Files

All architecture documentation is in `/workspaces/rez_agent/docs/`:

1. **`architecture/service-architecture.md`** - System design and service boundaries
2. **`architecture/data-model.md`** - DynamoDB schema and access patterns
3. **`api/openapi.yaml`** - REST API specification
4. **`architecture/message-schemas.md`** - SNS/SQS message formats
5. **`architecture/authentication-authorization.md`** - Security model
6. **`architecture/error-handling-resilience.md`** - Resilience patterns
7. **`architecture/observability-monitoring.md`** - Logging, metrics, tracing
8. **`architecture/configuration-management.md`** - Configuration strategy
9. **`architecture/quick-reference.md`** - Implementation cheat sheet
10. **`architecture/README.md`** - Documentation index and roadmap

**Total**: 200+ pages of comprehensive architecture documentation with Go code examples, AWS configuration, and implementation guidance.

---

## Architecture Completeness Checklist

- ✅ Service boundaries defined (4 Lambda functions)
- ✅ Data model designed (DynamoDB tables, GSIs, TTL)
- ✅ REST API contract defined (OpenAPI 3.0 specification)
- ✅ Message schemas defined (SNS/SQS JSON schemas)
- ✅ Authentication strategy defined (OAuth 2.0 with Cognito)
- ✅ Error handling patterns defined (retry, circuit breaker, DLQ)
- ✅ Observability strategy defined (logging, metrics, tracing, alarms)
- ✅ Configuration management defined (env vars, Parameter Store, feature flags)
- ✅ Technology recommendations provided (DynamoDB, Cognito, CDK)
- ✅ Implementation roadmap created (8-week plan)
- ✅ Go code examples provided (all critical patterns)
- ✅ AWS configuration documented (IAM policies, service settings)
- ✅ Cost estimates provided (~$10.57/month for 1k messages/day)
- ✅ Performance targets defined (SLI/SLO)
- ✅ Deployment strategy defined (blue-green with Lambda aliases)

---

## Summary

The rez_agent backend architecture is **complete and ready for implementation**. The design provides:

1. **Comprehensive documentation**: 200+ pages covering all aspects of the system
2. **Go code examples**: Implementation guidance for all critical patterns
3. **AWS configuration**: Detailed service settings, IAM policies, alarms
4. **Implementation roadmap**: 8-week plan from foundation to production
5. **Extensibility**: Designed for future expansion (multiple job types, notification channels)
6. **Production-ready patterns**: Resilience, observability, security, cost optimization

The architecture supports the initial "hello world" use case while providing a foundation for complex multi-job-type workflows without requiring redesign.

**All architecture documents are located in** `/workspaces/rez_agent/docs/`

**Start implementation** by reviewing `/workspaces/rez_agent/docs/architecture/README.md` and following the Phase 1 roadmap.
