# Web Action Processor - Design Documentation

**Version:** 1.0
**Date:** 2025-10-23
**Status:** ✅ Design Complete - Ready for Implementation

---

## Overview

This directory contains the complete technical design for the **Web Action Processor** feature, a new Lambda-based service that extends the rez_agent event-driven messaging system to support HTTP REST API integration capabilities.

### What This Feature Adds

1. **New Message Type**: `web_action` for HTTP-based actions
2. **Web Action Processor Lambda**: Executes HTTP requests, manages OAuth, stores results
3. **Scheduled Tasks**:
   - Weather notifications (5:00 AM EST)
   - Golf reservations (5:15 AM EST)

---

## Documentation Files

### 📋 Quick Start
- **[WEB_ACTION_PROCESSOR_SUMMARY.md](WEB_ACTION_PROCESSOR_SUMMARY.md)** - Executive summary (5 pages)
  - High-level overview
  - Key features and decisions
  - Cost estimates
  - Next steps

### 📖 Complete Design
- **[web-action-processor-design.md](web-action-processor-design.md)** - Full technical design (200+ pages)
  - Service architecture
  - Data models and schemas
  - API contracts and integration
  - Security architecture
  - Error handling and resilience
  - Configuration management
  - Implementation guidance with Go code examples
  - Testing strategy
  - Operational considerations

### 📊 Visual Architecture
- **[web-action-architecture-diagram.md](web-action-architecture-diagram.md)** - Mermaid diagrams
  - System architecture overview
  - Weather action flow
  - Golf action flow (with OAuth)
  - Error handling flow
  - Data model relationships
  - OAuth token caching
  - Infrastructure components
  - Action handler registry pattern
  - Observability stack
  - Deployment architecture

### ✅ Implementation Plan
- **[IMPLEMENTATION_CHECKLIST.md](IMPLEMENTATION_CHECKLIST.md)** - Task breakdown
  - Phase 1: Foundation (Week 1)
  - Phase 2: Core Processor (Week 1-2)
  - Phase 3: Weather Action (Week 2)
  - Phase 4: Golf Action (Week 3)
  - Phase 5: Observability (Week 3)
  - Phase 6: Testing & Documentation (Week 4)
  - Phase 7: Production Deployment
  - Acceptance criteria

---

## Requirements Source

This design implements the requirements specified in:
- **[NEXT_REQUIREMENT.MD](/workspaces/rez_agent/NEXT_REQUIREMENT.MD)** - Original feature requirements

---

## Quick Reference

### Key Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Message Type** | New `web_action` type | Clear separation from existing message types |
| **Result Storage** | DynamoDB with 3-day TTL | Consistent with existing architecture, auto-cleanup |
| **HTTP Client** | Go stdlib net/http | Standard library, full control over timeouts |
| **Secrets** | AWS Secrets Manager | More secure than Parameter Store for credentials |
| **Authentication** | Pluggable auth strategies | Support OAuth, API keys, bearer tokens |
| **Processor Pattern** | Action registry | Extensible for future action types |
| **OAuth Caching** | 50 minute TTL | Tokens valid 60 min, 10 min safety margin |

### Message Flow

```
EventBridge Scheduler (cron)
  ↓
Scheduler Lambda (create web_action message)
  ↓
SNS Topic: rez-agent-web-actions
  ↓
SQS Queue (batch size: 1, visibility: 5 min)
  ↓
Web Action Processor Lambda
  ├─ Parse payload
  ├─ Authenticate (if needed)
  ├─ Execute HTTP request
  ├─ Store result (DynamoDB, TTL: 3 days)
  └─ Publish notification event
  ↓
Message Processor (existing) → ntfy.sh
```

### New AWS Resources

- **SNS Topic**: `rez-agent-web-actions-{stage}`
- **SQS Queue**: `rez-agent-web-actions-{stage}`
- **SQS DLQ**: `rez-agent-web-actions-dlq-{stage}`
- **Lambda**: `rez-agent-webaction-{stage}` (512MB, 5min timeout)
- **DynamoDB**: `rez-agent-web-action-results-{stage}` (on-demand, TTL)
- **Secret**: `rez-agent/golf/credentials`
- **EventBridge Schedules**: Weather (5 AM EST), Golf (5:15 AM EST)

### Cost Estimate

**Monthly cost (prod):** ~$0.014/month
- Lambda: $0.003
- DynamoDB: $0.003
- SNS/SQS: $0.0003
- Secrets Manager: $0.003
- CloudWatch: $0.005

Negligible cost due to low invocation frequency (60 messages/month).

---

## Implementation Timeline

### Week 1: Foundation
- [ ] Data models (`web_action.go`, `web_action_result.go`)
- [ ] Pulumi infrastructure (SNS, SQS, DynamoDB, IAM, EventBridge)
- [ ] Repository layer (results table)
- [ ] Core processor skeleton

### Week 2: Weather Action
- [ ] HTTP client wrapper
- [ ] Retry logic with exponential backoff
- [ ] Weather action handler
- [ ] Scheduler updates
- [ ] End-to-end testing

### Week 3: Golf Action
- [ ] Secrets Manager integration
- [ ] OAuth 2.0 client with token caching
- [ ] Golf action handler
- [ ] Scheduler updates
- [ ] End-to-end testing

### Week 4: Observability & Testing
- [ ] CloudWatch metrics, alarms, dashboard
- [ ] X-Ray tracing
- [ ] Unit tests (>80% coverage)
- [ ] Integration tests
- [ ] Load testing
- [ ] Documentation and runbooks

---

## Getting Started

### For Reviewers

1. **Start here**: [WEB_ACTION_PROCESSOR_SUMMARY.md](WEB_ACTION_PROCESSOR_SUMMARY.md)
2. **Review diagrams**: [web-action-architecture-diagram.md](web-action-architecture-diagram.md)
3. **Deep dive**: [web-action-processor-design.md](web-action-processor-design.md)

### For Implementers

1. **Review**: [IMPLEMENTATION_CHECKLIST.md](IMPLEMENTATION_CHECKLIST.md)
2. **Reference**: [web-action-processor-design.md](web-action-processor-design.md) (Section 10: Implementation Guidance)
3. **Track progress**: Update checklist as you complete tasks

### For Operators

1. **Architecture**: [web-action-architecture-diagram.md](web-action-architecture-diagram.md)
2. **Runbooks**: [web-action-processor-design.md](web-action-processor-design.md) (Section 12.4: Runbook)
3. **Monitoring**: [web-action-processor-design.md](web-action-processor-design.md) (Section 12.1-12.3: Metrics, Alarms, Dashboard)

---

## Key Features

### Weather Notification Action

**Schedule:** 5:00 AM EST daily

```json
{
  "version": "1.0",
  "url": "https://api.weather.gov/gridpoints/PBZ/82,69/forecast",
  "action": "fetch_weather",
  "arguments": {
    "days": 2
  },
  "auth_config": {
    "type": "none"
  }
}
```
```curl

  curl -X POST $WEBAPI_URL/api/messages \
    -H "Content-Type: application/json" \
    -d '{
      "version": "1.0",
      "url": "https://api.weather.gov/gridpoints/TOP/31,80/forecast",
      "action": "weather",
      "arguments": {
        "days": 2
      },
      "auth_config": {
        "type": "none"
      },
      "stage": "dev",
      "message_type":"web_action"
    }'

  curl -X POST $WEBAPI_URL/api/messages \
    -H "Content-Type: application/json" \
    -d @docs/test/messages/web_api_get_reservations.json
```

**Output:** Detailed forecast for N days sent to ntfy.sh

### Golf Reservations Action

**Schedule:** 5:15 AM EST daily

```json
    curl -X POST $WEBAPI_URL/api/messages \
      -H "Content-Type: application/json" \
      -d '{
    "version": "1.0",
    "url": "https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/UpcomingReservation",
    "action": "golf",
    "arguments": {
      "max_results": 4
    },
    "auth_config": {
      "type": "oauth_password",
      "token_url": "https://birdsfoot.cps.golf/identityapi/connect/token",
      "secret_name": "HIDE"
    },
    "stage": "dev",
    "message_type":"web_action"
  }'



```

**Authentication:** OAuth 2.0 password grant flow
**Output:** Next 4 tee times (earliest first) sent to ntfy.sh

---

## Security Highlights

- ✅ **Secrets Manager**: OAuth credentials encrypted at rest with KMS
- ✅ **IAM Roles**: Least-privilege access per Lambda function
- ✅ **TLS 1.2+**: All HTTP connections use modern TLS
- ✅ **Token Caching**: OAuth tokens cached 50 min (expire at 60)
- ✅ **Logging**: Credentials and tokens never logged, headers redacted

---

## Resilience Patterns

- ✅ **Retry Logic**: Exponential backoff (3 attempts: 0s, 2s, 4s)
- ✅ **Error Classification**: Transient (5xx, timeout) vs. permanent (4xx, auth)
- ✅ **Dead Letter Queue**: Failed messages after 3 SQS retries
- ✅ **Timeouts**: HTTP 30s, Lambda 5 min, SQS visibility 5 min
- ✅ **Idempotency**: Safe to retry message processing

---

## Observability

### Structured Logging
- JSON format with correlation IDs
- Action type, URL, duration, HTTP status
- Credentials redacted

### CloudWatch Metrics
- `WebActionExecuted` (count)
- `WebActionSuccess` / `WebActionFailed` (count)
- `WebActionDuration` (milliseconds)
- `OAuthTokenCacheHit` / `OAuthTokenCacheMiss` (count)
- `OAuthAuthenticationFailed` (count)

### CloudWatch Alarms
- DLQ messages > 0
- Lambda errors > 3
- OAuth failures > 2
- High latency (p95 > 60s)

### X-Ray Tracing
- End-to-end request tracing
- HTTP request spans
- DynamoDB operation spans
- Secrets Manager call spans

---

## Testing Strategy

### Unit Tests
- All models, handlers, utilities
- Mock HTTP responses
- Mock OAuth flows
- Mock DynamoDB operations
- Target: >80% code coverage

### Integration Tests
- End-to-end message flow
- Weather action in dev environment
- Golf action in dev environment
- Error scenarios (timeouts, auth failures, API errors)

### Load Tests
- Simulate 60 messages (30 days worth)
- Verify no throttling
- Verify SQS handles backlog

---

## Acceptance Criteria

### ✅ Functional Requirements
- [ ] Weather action scheduled at 5:00 AM EST daily
- [ ] Golf action scheduled at 5:15 AM EST daily
- [ ] HTTP requests to external APIs succeed
- [ ] OAuth authentication works with cached tokens
- [ ] Results stored in DynamoDB with 3-day TTL
- [ ] Notifications sent to ntfy.sh

### ✅ Non-Functional Requirements
- [ ] HTTP timeout: 30 seconds
- [ ] Lambda timeout: 5 minutes
- [ ] Retry logic: 3 attempts with exponential backoff
- [ ] Token caching: 50 minutes
- [ ] Result retrieval: <500ms (p95)

### ✅ Security Requirements
- [ ] Secrets encrypted at rest (Secrets Manager + KMS)
- [ ] IAM roles with least-privilege
- [ ] TLS 1.2+ for all connections
- [ ] Credentials never logged

### ✅ Observability Requirements
- [ ] Structured logging with correlation IDs
- [ ] CloudWatch metrics published
- [ ] CloudWatch alarms configured
- [ ] X-Ray tracing enabled
- [ ] CloudWatch dashboard created

---

## Next Steps

1. ✅ **Design Review**
   - Review this README and summary
   - Review architecture diagrams
   - Approve design document

2. ⬜ **Infrastructure Setup**
   - Implement Pulumi changes
   - Deploy to dev environment
   - Verify resources created

3. ⬜ **Implementation**
   - Follow [IMPLEMENTATION_CHECKLIST.md](IMPLEMENTATION_CHECKLIST.md)
   - Complete Phase 1 (Foundation)
   - Complete Phase 2 (Weather Action)
   - Complete Phase 3 (Golf Action)
   - Complete Phase 4 (Observability & Testing)

4. ⬜ **Testing & Validation**
   - Run unit tests
   - Run integration tests
   - Run load tests
   - Manual testing in dev

5. ⬜ **Production Deployment**
   - Deploy to prod
   - Monitor for 24 hours
   - Verify scheduled runs
   - Collect feedback

---

## Support & Questions

For questions or clarifications about this design:

1. **Architecture Questions**: See [web-action-processor-design.md](web-action-processor-design.md)
2. **Implementation Questions**: See [IMPLEMENTATION_CHECKLIST.md](IMPLEMENTATION_CHECKLIST.md)
3. **Visual Reference**: See [web-action-architecture-diagram.md](web-action-architecture-diagram.md)
4. **Quick Reference**: See [WEB_ACTION_PROCESSOR_SUMMARY.md](WEB_ACTION_PROCESSOR_SUMMARY.md)

---

## Document History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2025-10-23 | 1.0 | Backend System Architect | Initial design complete |

---

**Status:** ✅ Design Complete - Ready for Implementation

**Total Documentation:** ~250 pages across 4 documents
- Technical design: 200+ pages
- Summary: 10 pages
- Diagrams: 20+ visualizations
- Checklist: 200+ tasks

**Estimated Implementation Time:** 4 weeks (1 engineer)

**Estimated Monthly Cost:** $0.014 (negligible)
