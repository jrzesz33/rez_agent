# Web Action Processor - Executive Summary

**Status:** Design Complete ✅
**Full Design:** [web-action-processor-design.md](/workspaces/rez_agent/docs/design/web-action-processor-design.md)

---

## Overview

The **Web Action Processor** is a new Lambda-based service that extends rez_agent to support HTTP REST API integration, enabling scheduled tasks that fetch data from external APIs and send notifications.

## Key Features

### 1. New Message Type: `web_action`
- Structured payload with URL, Action, Arguments
- Pluggable authentication strategies (OAuth 2.0, API keys, bearer tokens)
- Schema versioning for future extensibility

### 2. Web Action Processor Lambda
- Consumes messages from dedicated SQS queue
- Executes HTTP requests with retry logic
- Supports OAuth 2.0 password grant flow
- Stores results in DynamoDB (3-day TTL)
- Publishes completion events to trigger notifications

### 3. Scheduled Actions

#### Weather Notification (5:00 AM EST daily)
```
URL: https://api.weather.gov/gridpoints/PBZ/82,69/forecast
Action: fetch_weather
Arguments: { "days": 2 }
Output: Detailed forecast for N days
```

#### Golf Reservations (5:15 AM EST daily)
```
Authentication: OAuth 2.0 (credentials from Secrets Manager)
Step 1: POST /identityapi/connect/token
Step 2: GET /onlinereservation/UpcomingReservation
Output: Next 4 tee times (earliest first)
```

---

## Architecture Highlights

### Message Flow
```
EventBridge Scheduler
  → Scheduler Lambda (create web_action message)
  → SNS Topic (rez-agent-web-actions)
  → SQS Queue (batching, DLQ)
  → Web Action Processor Lambda
    ├─> Execute HTTP request
    ├─> Store result (DynamoDB)
    └─> Publish notification event
  → Message Processor (existing)
  → ntfy.sh notification
```

### Data Models

**WebActionPayload:**
```json
{
  "version": "1.0",
  "url": "https://api.example.com/data",
  "action": "fetch_weather",
  "arguments": {
    "days": 2
  },
  "auth_config": {
    "type": "oauth_password",
    "token_url": "https://...",
    "secret_name": "rez-agent/golf/credentials"
  }
}
```

**WebActionResult (DynamoDB):**
- PK: `action_id`
- TTL: 3 days
- Includes: HTTP response, transformed result, duration, errors

### Security

- **Secrets Manager**: OAuth credentials encrypted at rest
- **IAM Roles**: Least-privilege access per Lambda
- **TLS 1.2+**: All HTTP connections
- **Token Caching**: OAuth tokens cached 50 minutes (expire at 60)
- **Logging**: Credentials never logged, headers redacted

### Resilience

- **Retry Logic**: Exponential backoff (3 attempts)
- **Error Classification**: Transient vs. permanent failures
- **DLQ**: Failed messages after 3 SQS retries
- **Timeouts**: HTTP 30s, Lambda 5 minutes
- **Idempotency**: Duplicate-safe processing

### Observability

- **Structured Logging**: JSON with correlation IDs
- **CloudWatch Metrics**: Actions executed, duration, success/failure rates
- **X-Ray Tracing**: Distributed request tracking
- **Alarms**: DLQ messages, auth failures, high latency

---

## Implementation Plan (4 weeks)

### Week 1: Foundation
- Add `web_action` message type
- Create Pulumi infrastructure (SNS, SQS, DynamoDB results table)
- Implement Web Action Processor Lambda skeleton
- Implement HTTP client wrapper and retry logic

### Week 2: Weather Action
- Implement weather action handler
- Parse weather.gov API responses
- Format notifications
- Update Scheduler Lambda
- End-to-end testing

### Week 3: Golf Action
- Create Secrets Manager secret
- Implement OAuth 2.0 client with token caching
- Implement golf action handler
- Update Scheduler Lambda
- End-to-end testing

### Week 4: Polish
- Add CloudWatch metrics/alarms
- Complete observability (X-Ray tracing)
- Write unit and integration tests
- Documentation and runbooks

---

## New AWS Resources

| Resource | Name | Purpose |
|----------|------|---------|
| SNS Topic | `rez-agent-web-actions-{stage}` | Publish web action events |
| SQS Queue | `rez-agent-web-actions-{stage}` | Buffer messages for processor |
| SQS DLQ | `rez-agent-web-actions-dlq-{stage}` | Failed message storage |
| Lambda | `rez-agent-webaction-{stage}` | Web action processor |
| DynamoDB | `rez-agent-web-action-results-{stage}` | Store results (3-day TTL) |
| Secret | `rez-agent/golf/credentials` | Golf API OAuth credentials |
| EventBridge | `rez-agent-weather-scheduler-{stage}` | 5 AM EST daily trigger |
| EventBridge | `rez-agent-golf-scheduler-{stage}` | 5:15 AM EST daily trigger |

---

## Cost Estimate

**Monthly Cost (prod):**
- Lambda: ~$0.003 (60 invocations/month)
- DynamoDB: ~$0.003 (reads/writes)
- SNS/SQS: ~$0.0003
- Secrets Manager: ~$0.003 (60 API calls)
- CloudWatch Logs: ~$0.005

**Total: ~$0.014/month** (negligible due to low invocation frequency)

---

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Message Type | New `web_action` type | Clear separation from existing types |
| Result Storage | DynamoDB with TTL | Consistent with existing architecture |
| HTTP Client | Go stdlib net/http | Standard library, full control |
| Secrets | AWS Secrets Manager | More secure than Parameter Store |
| Authentication | Pluggable strategies | Support multiple auth types |
| Processor Pattern | Action registry | Extensible for future actions |
| OAuth Caching | 50 minute TTL | Tokens valid 60 min, safe margin |

---

## Go Module Structure

```
internal/
├── models/
│   ├── web_action.go           # NEW: Payload model
│   └── web_action_result.go    # NEW: Result model
├── webaction/                   # NEW: Web action processing
│   ├── processor.go             # Main orchestrator
│   ├── actions/
│   │   ├── registry.go          # Action handler registry
│   │   ├── weather.go           # Weather handler
│   │   └── golf.go              # Golf handler
│   ├── auth/
│   │   ├── oauth.go             # OAuth 2.0 client
│   │   └── cache.go             # Token caching
│   └── httpclient/
│       └── client.go            # HTTP wrapper
├── secrets/                     # NEW: Secrets Manager
│   └── cache.go
└── retry/                       # NEW: Retry logic
    └── retry.go

cmd/
└── webaction/                   # NEW: Lambda entrypoint
    └── main.go
```

---

## Testing Strategy

### Unit Tests
- Weather action handler (mock HTTP responses)
- Golf action handler (mock OAuth + API)
- OAuth client (token caching)
- Retry logic (exponential backoff)

### Integration Tests
- End-to-end flow in dev environment
- Manual message creation → SQS → processing → result verification

### Load Tests
- Simulate 60 messages (30 days worth)
- Verify no throttling or timeouts

---

## Operational Runbooks

### Incident: Golf Authentication Failing
1. Check CloudWatch alarm: `OAuthAuthenticationFailures`
2. Verify Secrets Manager secret: `rez-agent/golf/credentials`
3. Test credentials manually via curl
4. Update secret if expired or rotate credentials

### Incident: Weather API Timeout
1. Check CloudWatch alarm: `WebActionHighLatency`
2. Verify weather.gov API availability
3. Review HTTP timeout configuration
4. SQS auto-retry will handle transient failures

---

## Next Steps

1. ✅ Review and approve design document
2. ⬜ Implement Pulumi infrastructure changes
3. ⬜ Create Go packages and models
4. ⬜ Implement Weather action (Week 2)
5. ⬜ Implement Golf action (Week 3)
6. ⬜ Add observability and testing (Week 4)
7. ⬜ Deploy to dev for testing
8. ⬜ Deploy to prod after validation

---

## Design Approval Checklist

- [ ] Architecture aligns with existing rez_agent patterns
- [ ] Security requirements met (Secrets Manager, IAM, TLS 1.2+)
- [ ] Observability complete (logs, metrics, alarms, X-Ray)
- [ ] Cost estimate acceptable (~$0.014/month)
- [ ] Implementation timeline acceptable (4 weeks)
- [ ] All requirements from NEXT_REQUIREMENT.MD addressed
- [ ] Code examples provided for critical components
- [ ] Testing strategy defined
- [ ] Operational runbooks prepared

---

**Full Technical Design:** [web-action-processor-design.md](/workspaces/rez_agent/docs/design/web-action-processor-design.md) (200+ pages)

**Author:** Backend System Architect
**Date:** 2025-10-23
**Status:** ✅ Complete and ready for implementation
