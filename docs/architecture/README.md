# rez_agent Architecture Documentation

## Overview

This directory contains the complete backend service architecture design for the rez_agent event-driven messaging system. The architecture is designed for a Go-based serverless application running on AWS Lambda, processing scheduled and manual messages with delivery to ntfy.sh.

## Architecture Documents

### 1. [Service Architecture](./service-architecture.md)
**Purpose**: Defines the overall system architecture, service boundaries, and component interactions.

**Contents**:
- System architecture diagram (ASCII)
- Service boundary definitions (4 Lambda functions)
- Message flow and communication patterns
- Technology decisions (DynamoDB vs RDS, Cognito vs Auth0)
- Deployment strategy and scalability considerations
- Future extensibility patterns

**Key Components**:
- Scheduler Service (EventBridge → Lambda)
- Web API Service (API Gateway → Lambda)
- Message Processor Service (SQS → Lambda)
- Notification Service (Lambda → ntfy.sh)

**Start here** to understand the overall system design and how components interact.

---

### 2. [Data Model](./data-model.md)
**Purpose**: Defines the data storage strategy, table schema, and access patterns.

**Contents**:
- DynamoDB table design (messages table)
- Primary key and GSI design (stage-created_date, status-created_date)
- Message status state machine (created → processing → completed/failed)
- Message payload schemas by message_type
- Data retention strategy (90-day TTL)
- Access patterns and query examples (Go SDK v2)
- Performance optimization (pagination, batch operations, conditional writes)

**Key Decisions**:
- DynamoDB (serverless-native, auto-scaling, no connection pooling)
- On-demand capacity mode (variable workload)
- TTL for automatic cleanup (90 days)

**Read this** after understanding service architecture, before implementing data access layer.

---

### 3. [OpenAPI Specification](../api/openapi.yaml)
**Purpose**: REST API contract for the Web API Lambda function.

**Contents**:
- Complete OpenAPI 3.0 specification
- All endpoints with request/response schemas
- Authentication flow (OAuth 2.0)
- Error response formats
- Example requests and responses

**Endpoints**:
- `GET /api/messages` - List messages with filtering/pagination
- `POST /api/messages` - Create manual message
- `GET /api/messages/{id}` - Get message by ID
- `GET /api/metrics` - Dashboard metrics
- `POST /api/auth/login` - OAuth login initiation
- `GET /api/auth/callback` - OAuth callback
- `POST /api/auth/refresh` - Token refresh
- `GET /api/health` - Health check

**Use this** to implement the Web API Lambda handler and generate API documentation.

---

### 4. [Message Schemas](./message-schemas.md)
**Purpose**: Defines SNS and SQS message formats for inter-service communication.

**Contents**:
- SNS message schema (published by Scheduler/Web API)
- SQS message schema (consumed by Message Processor)
- Go struct definitions for message parsing
- Message processing flow examples
- Idempotency and deduplication strategies
- Schema versioning approach

**Key Decisions**:
- Minimal payload (only message_id, not full data)
- Single source of truth (DynamoDB for message data)
- Correlation ID for distributed tracing

**Read this** when implementing message publishing (SNS) and consumption (SQS).

---

### 5. [Authentication & Authorization](./authentication-authorization.md)
**Purpose**: Security model for user and service-to-service authentication.

**Contents**:
- OAuth 2.0 authorization code flow (AWS Cognito)
- JWT token structure and validation
- Lambda Authorizer implementation (Go)
- Service-to-service authentication (IAM roles)
- IAM policies for least-privilege access
- Secrets management (Parameter Store)
- Future RBAC enhancement design

**Key Components**:
- AWS Cognito User Pool (user authentication)
- Lambda Authorizer (JWT validation)
- IAM roles per Lambda function
- Parameter Store for API keys

**Read this** before implementing authentication flows and API Gateway integration.

---

### 6. [Error Handling & Resilience](./error-handling-resilience.md)
**Purpose**: Strategies for handling errors and building resilience.

**Contents**:
- Error categories (transient, permanent, validation, system)
- Retry strategy (exponential backoff with jitter)
- Circuit breaker pattern (DynamoDB-backed state)
- Timeout management (Lambda and HTTP clients)
- SQS Dead Letter Queue configuration
- Idempotency patterns (conditional DynamoDB updates)
- Graceful degradation strategies
- Partial batch failures (SQS)

**Key Patterns**:
- Exponential backoff: 1s → 2s → 4s (3 retries)
- Circuit breaker: 5 failures in 1 minute → open (30s timeout)
- DLQ: After 3 retries → manual investigation

**Read this** when implementing error handling in Lambda functions.

---

### 7. [Observability & Monitoring](./observability-monitoring.md)
**Purpose**: Logging, metrics, tracing, and alerting strategy.

**Contents**:
- Structured logging with Go `log/slog` (JSON format)
- Correlation ID propagation (across all services)
- CloudWatch Logs configuration (retention, queries)
- Custom CloudWatch metrics (messages processed, notification duration)
- Distributed tracing with AWS X-Ray (trace examples)
- CloudWatch alarms (DLQ, errors, latency, circuit breaker)
- Dashboard design (overview, service-specific)
- SLI/SLO definitions (availability, latency, success rate)

**Key Metrics**:
- Messages created, processed, failed
- Notification success rate
- API response time (p95, p99)
- Circuit breaker state

**Read this** when implementing logging, metrics, and setting up monitoring.

---

### 8. [Configuration Management](./configuration-management.md)
**Purpose**: How to manage configuration across environments.

**Contents**:
- Environment variables (Lambda configuration)
- AWS Systems Manager Parameter Store (secrets, shared config)
- DynamoDB dynamic configuration (feature flags)
- Go configuration loading patterns
- Parameter caching strategies (reduce API calls)
- Per-environment configuration (dev, stage, prod)
- Configuration validation (pre-deployment, runtime)
- Secrets rotation process

**Key Decisions**:
- Environment variables: Non-sensitive config (table names, ARNs)
- Parameter Store: Secrets (API keys) with KMS encryption
- Caching: 5-minute TTL in Lambda globals

**Read this** when setting up infrastructure and deploying Lambda functions.

---

## Implementation Roadmap

### Phase 1: Foundation (Week 1-2)
1. **Infrastructure Setup**:
   - Create DynamoDB table (messages, circuit-breaker)
   - Create SNS topic, SQS queue (with DLQ)
   - Set up AWS Cognito User Pool
   - Configure Parameter Store parameters

2. **Scheduler Lambda**:
   - Implement message creation logic
   - Publish to SNS topic
   - Configure EventBridge rule (24-hour cron)

3. **Message Processor Lambda**:
   - Implement SQS event handler
   - Status update logic (DynamoDB)
   - Partial batch failure handling

4. **Notification Service Lambda**:
   - HTTP client for ntfy.sh
   - Basic retry logic (exponential backoff)
   - Error handling

**Deliverable**: Scheduled "hello world" messages sent to ntfy.sh every 24 hours

---

### Phase 2: Web API (Week 3-4)
1. **Lambda Authorizer**:
   - JWT validation (Cognito JWKS)
   - IAM policy generation

2. **Web API Lambda**:
   - Implement all endpoints (see OpenAPI spec)
   - OAuth flow handlers (login, callback, refresh)
   - DynamoDB query logic (list messages, metrics)
   - Manual message creation

3. **API Gateway**:
   - Configure HTTP API
   - Attach Lambda Authorizer
   - CORS configuration

**Deliverable**: Web API for frontend to view messages and create manual notifications

---

### Phase 3: Resilience (Week 5)
1. **Circuit Breaker**:
   - DynamoDB-backed circuit breaker implementation
   - State management (closed → open → half-open)
   - Integration in Notification Service

2. **Enhanced Error Handling**:
   - Structured error responses
   - DLQ monitoring alarm
   - Idempotency improvements

3. **Graceful Degradation**:
   - Metrics caching (fallback for DynamoDB errors)
   - Circuit breaker fallback logic

**Deliverable**: System resilient to ntfy.sh outages and transient errors

---

### Phase 4: Observability (Week 6)
1. **Logging**:
   - Implement structured logging (slog)
   - Correlation ID propagation
   - CloudWatch Logs Insights queries

2. **Metrics**:
   - Custom CloudWatch metrics (messages, notifications, duration)
   - Metrics recording in all Lambdas

3. **Tracing**:
   - X-Ray SDK integration
   - Trace annotations (correlation_id, message_id)

4. **Monitoring**:
   - CloudWatch alarms (DLQ, errors, latency)
   - Dashboard creation
   - Runbook documentation

**Deliverable**: Full observability stack with proactive alerting

---

### Phase 5: Frontend & Polish (Week 7-8)
1. **Frontend**:
   - React/Next.js dashboard
   - OAuth integration
   - Message list, metrics display
   - Manual message creation form

2. **Documentation**:
   - API documentation (from OpenAPI spec)
   - Deployment guide
   - Runbooks for common issues

3. **Testing**:
   - Unit tests (Go)
   - Integration tests (LocalStack or AWS)
   - E2E tests (frontend + backend)

**Deliverable**: Production-ready system with web interface

---

## Technology Stack Summary

### Core Infrastructure
- **Compute**: AWS Lambda (Go 1.24)
- **API Gateway**: AWS HTTP API
- **Messaging**: SNS (pub/sub), SQS (queue with DLQ)
- **Database**: DynamoDB (on-demand, TTL enabled)
- **Scheduling**: EventBridge (cron rules)
- **Authentication**: AWS Cognito (OAuth 2.0, JWT)

### Configuration & Secrets
- **Configuration**: Environment variables, Parameter Store
- **Secrets**: Parameter Store (SecureString, KMS encrypted)

### Observability
- **Logging**: CloudWatch Logs, Go `log/slog` (JSON)
- **Metrics**: CloudWatch Metrics (built-in + custom)
- **Tracing**: AWS X-Ray
- **Alerting**: CloudWatch Alarms → SNS

### Development & Deployment
- **IaC**: AWS CDK (Go) or Terraform
- **CI/CD**: GitHub Actions (or AWS CodePipeline)
- **Deployment**: Blue-green with Lambda aliases

### External Services
- **Notifications**: ntfy.sh (push notification API)

---

## Design Principles Recap

1. **Event-driven**: SNS/SQS decouples services, allows async processing
2. **Serverless**: No infrastructure management, auto-scaling, pay-per-use
3. **Resilient**: Retries, circuit breakers, DLQ, graceful degradation
4. **Observable**: Structured logs, metrics, distributed tracing, correlation IDs
5. **Secure**: OAuth 2.0, JWT, IAM roles, encrypted secrets, least-privilege
6. **Extensible**: Generic message schema, pluggable notification services, feature flags
7. **Cost-optimized**: On-demand capacity, log retention policies, X-Ray sampling
8. **Testable**: Clear service boundaries, dependency injection, unit/integration tests

---

## Next Steps

1. **Review all architecture documents** in this directory (start with service-architecture.md)
2. **Set up AWS account** and configure IAM roles/policies
3. **Create GitHub repository** for code (separate from docs)
4. **Initialize Go project** with module structure:
   ```
   rez_agent/
   ├── cmd/
   │   ├── scheduler/
   │   ├── web-api/
   │   ├── message-processor/
   │   ├── notification-service/
   │   └── jwt-authorizer/
   ├── internal/
   │   ├── config/
   │   ├── models/
   │   ├── dynamodb/
   │   ├── sns/
   │   ├── ssm/
   │   ├── circuitbreaker/
   │   ├── retry/
   │   └── correlation/
   ├── docs/ (this directory)
   ├── infra/ (CDK or Terraform)
   └── go.mod
   ```
5. **Implement Phase 1** (Foundation) following the roadmap above
6. **Deploy to dev environment** and test scheduled message flow
7. **Iterate** through remaining phases

---

## Questions or Clarifications?

If any part of the architecture needs clarification or adjustment during implementation, refer back to the detailed design documents. Each document includes:
- **Rationale**: Why decisions were made
- **Trade-offs**: Alternatives considered
- **Go code examples**: Implementation guidance
- **AWS configuration**: Service settings and IAM policies

Good luck with the implementation!
