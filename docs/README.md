# rez_agent Documentation

## Overview

This directory contains all design and architecture documentation for the rez_agent event-driven messaging system.

## Quick Links

- **[Architecture Summary](/workspaces/rez_agent/ARCHITECTURE_SUMMARY.md)** - Executive summary and deliverables overview
- **[Architecture Documentation](./architecture/README.md)** - Complete architecture design documents
- **[Quick Reference](./architecture/quick-reference.md)** - Implementation cheat sheet
- **[OpenAPI Specification](./api/openapi.yaml)** - REST API contract

## Documentation Structure

```
docs/
├── README.md (this file)
├── architecture/
│   ├── README.md                         # Architecture index and roadmap
│   ├── service-architecture.md           # System design (26 pages)
│   ├── data-model.md                     # DynamoDB schema (21 pages)
│   ├── message-schemas.md                # SNS/SQS messages (19 pages)
│   ├── authentication-authorization.md   # Security model (24 pages)
│   ├── error-handling-resilience.md      # Resilience patterns (21 pages)
│   ├── observability-monitoring.md       # Logging/metrics/tracing (25 pages)
│   ├── configuration-management.md       # Configuration strategy (22 pages)
│   └── quick-reference.md                # Cheat sheet (15 pages)
├── api/
│   └── openapi.yaml                      # REST API specification (600+ lines)
└── design/
    └── README.md                         # Initial design notes
```

## Getting Started

### For Architects/Reviewers

1. Read [Architecture Summary](/workspaces/rez_agent/ARCHITECTURE_SUMMARY.md)
2. Review [Service Architecture](./architecture/service-architecture.md)
3. Examine [Data Model](./architecture/data-model.md)
4. Review [OpenAPI Specification](./api/openapi.yaml)

### For Implementers

1. Read [Architecture README](./architecture/README.md) for implementation roadmap
2. Use [Quick Reference](./architecture/quick-reference.md) as cheat sheet
3. Refer to individual architecture documents for detailed designs
4. Follow the 8-week implementation roadmap (Phase 1-5)

### For Operators

1. Read [Observability & Monitoring](./architecture/observability-monitoring.md)
2. Review [Error Handling & Resilience](./architecture/error-handling-resilience.md)
3. Check [Configuration Management](./architecture/configuration-management.md)
4. Reference [Quick Reference](./architecture/quick-reference.md) for troubleshooting

## Architecture Highlights

### System Components

- **4 Lambda Functions**: Scheduler, Web API, Message Processor, Notification Service
- **Event-Driven**: EventBridge → SNS → SQS → Lambda → ntfy.sh
- **Serverless**: DynamoDB, API Gateway, Cognito, CloudWatch, X-Ray
- **Resilient**: Retries, circuit breaker, DLQ, graceful degradation
- **Observable**: Structured logs, custom metrics, distributed tracing, alarms

### Key Decisions

| Decision | Choice | Documentation |
|----------|--------|---------------|
| Data Store | DynamoDB | [Data Model](./architecture/data-model.md) |
| Message Queue | SNS + SQS | [Service Architecture](./architecture/service-architecture.md) |
| Auth Provider | AWS Cognito | [Authentication](./architecture/authentication-authorization.md) |
| API Contract | OpenAPI 3.0 | [OpenAPI Spec](./api/openapi.yaml) |
| Resilience | Circuit Breaker | [Error Handling](./architecture/error-handling-resilience.md) |
| Logging | Go log/slog | [Observability](./architecture/observability-monitoring.md) |

### Performance Targets (SLO)

- **API Availability**: 99.9% (30-day rolling)
- **API Response Time**: p95 < 500ms, p99 < 1000ms
- **Message Processing Latency**: p95 < 10 seconds
- **Notification Success Rate**: 99% (7-day rolling)

### Cost Estimate

- **~$10.57/month** for 1,000 messages/day, 10,000 API requests/day
- **~$50/month** at 10x scale (10,000 messages/day)

## Implementation Roadmap

### Phase 1: Foundation (Week 1-2)
- Infrastructure setup (DynamoDB, SNS/SQS, Cognito)
- Scheduler Lambda (EventBridge → create message → SNS)
- Message Processor Lambda (SQS → process → update status)
- Notification Service Lambda (send to ntfy.sh)

**Deliverable**: Scheduled "hello world" messages every 24 hours

### Phase 2: Web API (Week 3-4)
- Lambda Authorizer (JWT validation)
- Web API Lambda (all REST endpoints)
- API Gateway configuration (CORS, rate limiting)

**Deliverable**: REST API for frontend

### Phase 3: Resilience (Week 5)
- Circuit breaker implementation (DynamoDB-backed)
- Enhanced error handling
- DLQ alarms and monitoring

**Deliverable**: System resilient to failures

### Phase 4: Observability (Week 6)
- Structured logging with correlation IDs
- Custom CloudWatch metrics
- AWS X-Ray tracing integration
- CloudWatch alarms and dashboard

**Deliverable**: Full observability stack

### Phase 5: Frontend & Polish (Week 7-8)
- React/Next.js dashboard
- OAuth integration
- Unit and integration tests
- Runbooks and documentation

**Deliverable**: Production-ready system

## Technology Stack

### AWS Services
- Lambda (Go 1.24), API Gateway (HTTP API), DynamoDB (on-demand)
- SNS (pub/sub), SQS (queue + DLQ), EventBridge (scheduling)
- Cognito (OAuth 2.0), Systems Manager Parameter Store (secrets)
- CloudWatch Logs/Metrics, X-Ray (tracing)

### Go Libraries
- `log/slog` (logging), `aws/aws-lambda-go` (runtime)
- `aws/aws-sdk-go-v2` (AWS SDK), `golang-jwt/jwt/v5` (JWT)
- `google/uuid` (UUIDs)

### Infrastructure as Code
- AWS CDK (Go) or Terraform

## Documentation Quality

- ✅ **200+ pages** of comprehensive architecture documentation
- ✅ **Go code examples** for all critical patterns
- ✅ **AWS configuration** (IAM policies, service settings, alarms)
- ✅ **Implementation guidance** (8-week roadmap, troubleshooting)
- ✅ **Production-ready patterns** (resilience, observability, security)

## Next Steps

1. Review [Architecture Summary](/workspaces/rez_agent/ARCHITECTURE_SUMMARY.md)
2. Read [Architecture README](./architecture/README.md)
3. Set up AWS account and development environment
4. Initialize Go project structure
5. Begin Phase 1 implementation (Foundation)

## Questions?

Refer to the detailed architecture documents or create a GitHub issue for clarifications during implementation.

---

**Documentation Version**: 1.0 (2025-10-21)

**Status**: Complete and ready for implementation
