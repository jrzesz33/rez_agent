# Web Action Processor - Implementation Checklist

This checklist provides a step-by-step guide for implementing the Web Action Processor feature.

**Status Legend:**
- ⬜ Not Started
- 🔄 In Progress
- ✅ Complete
- ⚠️ Blocked

---

## Phase 1: Foundation (Week 1)

### Data Models
- [ ] ⬜ Add `MessageTypeWebAction` to `/workspaces/rez_agent/internal/models/message.go`
  - [ ] Update `IsValid()` method
  - [ ] Update tests in `message_test.go`
- [ ] ⬜ Create `/workspaces/rez_agent/internal/models/web_action.go`
  - [ ] `WebActionPayload` struct
  - [ ] `AuthConfig` struct
  - [ ] `Validate()` method
  - [ ] `NewWeatherActionPayload()` helper
  - [ ] `NewGolfReservationsPayload()` helper
  - [ ] `ParseWebActionPayload()` parser
- [ ] ⬜ Create `/workspaces/rez_agent/internal/models/web_action_result.go`
  - [ ] `WebActionResult` struct
  - [ ] `NewWebActionResult()` constructor
  - [ ] `MarkSuccess()` method
  - [ ] `MarkFailed()` method
- [ ] ⬜ Write unit tests for all models

### Infrastructure (Pulumi)
- [ ] ⬜ Update `/workspaces/rez_agent/infrastructure/main.go`
  - [ ] Create SNS topic: `rez-agent-web-actions-{stage}`
  - [ ] Create SQS queue: `rez-agent-web-actions-{stage}`
  - [ ] Create SQS DLQ: `rez-agent-web-actions-dlq-{stage}`
  - [ ] Configure SNS → SQS subscription
  - [ ] Configure SQS queue policy
  - [ ] Create DynamoDB table: `rez-agent-web-action-results-{stage}`
    - [ ] PK: `action_id` (String)
    - [ ] SK: `executed_at` (String)
    - [ ] GSI: `message-id-index`
    - [ ] GSI: `action-executed-index`
    - [ ] TTL attribute: `ttl`
  - [ ] Create IAM role: `rez-agent-webaction-role-{stage}`
  - [ ] Create IAM policy for web action processor
  - [ ] Create CloudWatch log group: `/aws/lambda/rez-agent-webaction-{stage}`
  - [ ] Create EventBridge schedule: `rez-agent-weather-scheduler-{stage}`
  - [ ] Create EventBridge schedule: `rez-agent-golf-scheduler-{stage}`
  - [ ] Create CloudWatch alarms (DLQ, errors, latency, OAuth failures)

### Repository Layer
- [ ] ⬜ Create `/workspaces/rez_agent/internal/repository/web_action_results.go`
  - [ ] `WebActionResultsRepository` interface
  - [ ] `Create()` method
  - [ ] `GetByMessageID()` method
  - [ ] `GetByAction()` method
  - [ ] Implement with DynamoDB
- [ ] ⬜ Write unit tests (mock DynamoDB)

### Configuration
- [ ] ⬜ Update `/workspaces/rez_agent/pkg/config/config.go`
  - [ ] Add `ResultsTableName` field
  - [ ] Add `WebActionsQueueURL` field
  - [ ] Add HTTP client config fields
  - [ ] Create `WebActionConfig` struct
  - [ ] Create `MustLoadWebActionConfig()` function

---

## Phase 2: Core Processor (Week 1-2)

### HTTP Client
- [ ] ⬜ Create `/workspaces/rez_agent/internal/webaction/httpclient/client.go`
  - [ ] `NewHTTPClient()` with TLS 1.2+ config
  - [ ] Timeout configuration
  - [ ] Connection pooling
  - [ ] User-Agent header
- [ ] ⬜ Write unit tests

### Retry Logic
- [ ] ⬜ Create `/workspaces/rez_agent/internal/retry/retry.go`
  - [ ] `RetryConfig` struct
  - [ ] `Execute()` function with exponential backoff
  - [ ] `calculateBackoff()` with jitter
  - [ ] `IsHTTPRetryable()` error classifier
- [ ] ⬜ Write unit tests

### Secrets Manager Integration
- [ ] ⬜ Create `/workspaces/rez_agent/internal/secrets/cache.go`
  - [ ] `SecretCache` struct
  - [ ] `GetSecret()` method with caching (5 min TTL)
  - [ ] `GolfCredentials` struct
  - [ ] `GetGolfCredentials()` method
- [ ] ⬜ Write unit tests (mock Secrets Manager)

### Action Registry
- [ ] ⬜ Create `/workspaces/rez_agent/internal/webaction/actions/registry.go`
  - [ ] `ActionHandler` interface
  - [ ] `ActionRegistry` struct
  - [ ] `Register()` method
  - [ ] `Get()` method
  - [ ] `ActionResult` struct
- [ ] ⬜ Write unit tests

### Main Processor
- [ ] ⬜ Create `/workspaces/rez_agent/internal/webaction/processor.go`
  - [ ] `Processor` struct
  - [ ] `NewProcessor()` constructor
  - [ ] `ProcessWebAction()` orchestrator
  - [ ] Parse payload
  - [ ] Execute action handler
  - [ ] Store result in DynamoDB
  - [ ] Create notification message
  - [ ] Publish SNS event
  - [ ] Update message status
- [ ] ⬜ Write unit tests (mock all dependencies)

### Lambda Handler
- [ ] ⬜ Create `/workspaces/rez_agent/cmd/webaction/main.go`
  - [ ] `Handler` struct
  - [ ] `HandleEvent()` SQS event handler
  - [ ] `processRecord()` individual message processor
  - [ ] Initialize AWS clients
  - [ ] Setup structured logging
  - [ ] Bootstrap Lambda runtime
- [ ] ⬜ Add build target to Makefile
- [ ] ⬜ Test Lambda packaging

---

## Phase 3: Weather Action (Week 2)

### Weather Action Handler
- [ ] ⬜ Create `/workspaces/rez_agent/internal/webaction/actions/weather.go`
  - [ ] `WeatherHandler` struct
  - [ ] `NewWeatherHandler()` constructor
  - [ ] `Execute()` method
    - [ ] HTTP GET weather.gov API
    - [ ] Parse JSON response
    - [ ] Extract N days forecast
    - [ ] Format notification text
  - [ ] `WeatherGovResponse` struct
  - [ ] `formatWeatherNotification()` helper
- [ ] ⬜ Write unit tests (mock HTTP responses)

### Scheduler Updates for Weather
- [ ] ⬜ Update `/workspaces/rez_agent/cmd/scheduler/main.go`
  - [ ] Add EventBridge payload parsing
  - [ ] Support `action_type: "weather"` payloads
  - [ ] Create `web_action` message with weather payload
  - [ ] Set `days` argument from EventBridge payload
- [ ] ⬜ Update infrastructure to pass weather payload

### Integration Testing
- [ ] ⬜ Deploy to dev environment
- [ ] ⬜ Manually trigger weather action
- [ ] ⬜ Verify DynamoDB result record created
- [ ] ⬜ Verify TTL set correctly (3 days)
- [ ] ⬜ Verify notification sent to ntfy.sh
- [ ] ⬜ Test with different `days` arguments (1, 2, 3, 7)
- [ ] ⬜ Test error scenarios (API down, timeout)

---

## Phase 4: Golf Action (Week 3)

### OAuth Client
- [ ] ⬜ Create `/workspaces/rez_agent/internal/webaction/auth/oauth.go`
  - [ ] `OAuthClient` struct
  - [ ] `NewOAuthClient()` constructor
  - [ ] `GetToken()` method
    - [ ] Check cache first (50 min TTL)
    - [ ] Fetch credentials from Secrets Manager
    - [ ] POST to token URL
    - [ ] Parse token response
    - [ ] Cache access token
  - [ ] `OAuthTokenResponse` struct
- [ ] ⬜ Create `/workspaces/rez_agent/internal/webaction/auth/cache.go`
  - [ ] `TokenCache` struct with mutex
  - [ ] `Set()` method
  - [ ] `Get()` method
  - [ ] TTL expiration logic
- [ ] ⬜ Write unit tests (mock HTTP, mock Secrets Manager)

### Golf Action Handler
- [ ] ⬜ Create `/workspaces/rez_agent/internal/webaction/actions/golf.go`
  - [ ] `GolfHandler` struct
  - [ ] `NewGolfHandler()` constructor
  - [ ] `Execute()` method
    - [ ] Get OAuth token
    - [ ] HTTP GET golf API with Bearer token
    - [ ] Parse JSON response
    - [ ] Sort reservations by date
    - [ ] Take first N results
    - [ ] Format notification text
  - [ ] `GolfReservationsResponse` struct
  - [ ] `formatGolfNotification()` helper
- [ ] ⬜ Write unit tests (mock OAuth, mock HTTP)

### Secrets Manager Setup
- [ ] ⬜ Create secret: `rez-agent/golf/credentials` (dev)
  ```json
  {
    "username": "dev@example.com",
    "password": "DevPassword123!",
    "golfer_id": "12345"
  }
  ```
- [ ] ⬜ Create secret: `rez-agent/golf/credentials` (prod)
  ```json
  {
    "username": "real@email.com",
    "password": "SecurePassword!",
    "golfer_id": "91124"
  }
  ```
- [ ] ⬜ Update IAM policy to allow access

### Scheduler Updates for Golf
- [ ] ⬜ Update `/workspaces/rez_agent/cmd/scheduler/main.go`
  - [ ] Support `action_type: "golf"` payloads
  - [ ] Create `web_action` message with golf payload
  - [ ] Set `max_results` argument from EventBridge payload
- [ ] ⬜ Update infrastructure to pass golf payload

### Integration Testing
- [ ] ⬜ Deploy to dev environment
- [ ] ⬜ Manually trigger golf action
- [ ] ⬜ Verify OAuth authentication succeeds
- [ ] ⬜ Verify token caching works (check logs)
- [ ] ⬜ Verify DynamoDB result record created
- [ ] ⬜ Verify notification sent with tee times
- [ ] ⬜ Test token expiration (wait 51 minutes)
- [ ] ⬜ Test OAuth failure (invalid credentials)
- [ ] ⬜ Test API failure (invalid golfer ID)

---

## Phase 5: Observability (Week 3)

### Structured Logging
- [ ] ⬜ Ensure all handlers use structured logging
- [ ] ⬜ Add correlation IDs to all log entries
- [ ] ⬜ Redact sensitive data (passwords, tokens)
- [ ] ⬜ Log all HTTP requests (URL, status, duration)
- [ ] ⬜ Log OAuth events (cache hit/miss, auth success/failure)

### CloudWatch Metrics
- [ ] ⬜ Publish custom metrics in processor:
  - [ ] `WebActionExecuted` (dimensions: action, stage)
  - [ ] `WebActionSuccess` (dimensions: action, stage)
  - [ ] `WebActionFailed` (dimensions: action, stage)
  - [ ] `WebActionDuration` (dimensions: action, stage)
  - [ ] `HTTPRequestDuration` (dimensions: action, stage)
  - [ ] `OAuthTokenCacheHit` (dimensions: stage)
  - [ ] `OAuthTokenCacheMiss` (dimensions: stage)
  - [ ] `OAuthAuthenticationFailed` (dimensions: stage)

### X-Ray Tracing
- [ ] ⬜ Enable X-Ray tracing in Lambda config
- [ ] ⬜ Add X-Ray SDK to HTTP client
- [ ] ⬜ Add X-Ray subsegments for:
  - [ ] DynamoDB operations
  - [ ] HTTP requests
  - [ ] Secrets Manager calls
  - [ ] SNS publish
- [ ] ⬜ Test tracing in X-Ray console

### CloudWatch Alarms
- [ ] ⬜ Verify alarms created in infrastructure:
  - [ ] DLQ messages > 0
  - [ ] Lambda errors > 3
  - [ ] OAuth failures > 2
  - [ ] High latency (p95 > 60s)
- [ ] ⬜ Configure SNS topic for alarm notifications
- [ ] ⬜ Subscribe ops team email/Slack

### CloudWatch Dashboard
- [ ] ⬜ Create dashboard: `rez-agent-web-actions-{stage}`
  - [ ] Widget: Web actions executed (24h)
  - [ ] Widget: Success vs. failed rate
  - [ ] Widget: Action duration (p50, p95, p99)
  - [ ] Widget: HTTP request duration
  - [ ] Widget: OAuth cache hit rate
  - [ ] Widget: DLQ message count
  - [ ] Widget: Lambda errors
  - [ ] Widget: Lambda concurrent executions
  - [ ] Widget: Recent error logs
  - [ ] Widget: Top 10 slowest actions

---

## Phase 6: Testing & Documentation (Week 4)

### Unit Tests
- [ ] ⬜ Achieve >80% code coverage
- [ ] ⬜ Test all error paths
- [ ] ⬜ Test retry logic with mocked failures
- [ ] ⬜ Test OAuth token caching
- [ ] ⬜ Test action handlers with various API responses

### Integration Tests
- [ ] ⬜ Create integration test suite
- [ ] ⬜ Test weather action end-to-end (dev)
- [ ] ⬜ Test golf action end-to-end (dev)
- [ ] ⬜ Test message flow: EventBridge → Lambda → DynamoDB → SNS → Notification
- [ ] ⬜ Test error scenarios:
  - [ ] HTTP timeout
  - [ ] API 500 error
  - [ ] OAuth failure
  - [ ] DynamoDB throttling
  - [ ] Secrets Manager error

### Load Testing
- [ ] ⬜ Simulate 30 days of messages (60 total)
- [ ] ⬜ Verify no Lambda throttling
- [ ] ⬜ Verify no DynamoDB throttling
- [ ] ⬜ Verify SQS handles backlog
- [ ] ⬜ Verify all messages processed successfully

### Documentation
- [ ] ⬜ Write API documentation for web action payload schema
- [ ] ⬜ Write runbook for common incidents
  - [ ] Golf authentication failing
  - [ ] Weather API timeout
  - [ ] DLQ messages appearing
  - [ ] High Lambda duration
- [ ] ⬜ Write deployment guide
- [ ] ⬜ Write troubleshooting guide
- [ ] ⬜ Update ARCHITECTURE_SUMMARY.md

---

## Phase 7: Production Deployment

### Pre-Deployment Checklist
- [ ] ⬜ All tests passing in dev
- [ ] ⬜ Code review completed
- [ ] ⬜ Security review completed
- [ ] ⬜ Secrets created in prod Secrets Manager
- [ ] ⬜ CloudWatch alarms configured
- [ ] ⬜ Runbooks reviewed by ops team
- [ ] ⬜ Rollback plan documented

### Deployment Steps
- [ ] ⬜ Deploy infrastructure to prod
- [ ] ⬜ Deploy Lambda code to prod
- [ ] ⬜ Verify EventBridge schedules created
- [ ] ⬜ Manually trigger weather action (test)
- [ ] ⬜ Manually trigger golf action (test)
- [ ] ⬜ Verify notifications sent to ntfy.sh
- [ ] ⬜ Monitor CloudWatch dashboard for 24 hours
- [ ] ⬜ Verify scheduled runs at 5:00 AM and 5:15 AM EST

### Post-Deployment
- [ ] ⬜ Monitor DLQ (should be empty)
- [ ] ⬜ Monitor Lambda errors (should be zero)
- [ ] ⬜ Monitor OAuth success rate (should be 100%)
- [ ] ⬜ Review CloudWatch Logs for first week
- [ ] ⬜ Verify TTL cleanup working (check after 3 days)
- [ ] ⬜ Collect feedback from notifications

---

## Optional Enhancements (Future)

### Additional Actions
- [ ] ⬜ Design action: Fetch stock prices
- [ ] ⬜ Design action: Check website uptime
- [ ] ⬜ Design action: Fetch GitHub notifications
- [ ] ⬜ Design action: Check calendar events

### Performance Optimization
- [ ] ⬜ Implement connection pooling for HTTP
- [ ] ⬜ Implement DynamoDB batch writes
- [ ] ⬜ Optimize Lambda cold starts
- [ ] ⬜ Add response compression

### Advanced Features
- [ ] ⬜ Implement circuit breaker for external APIs
- [ ] ⬜ Add feature flags (DynamoDB config table)
- [ ] ⬜ Support webhook-based actions
- [ ] ⬜ Add action result webhooks (callbacks)
- [ ] ⬜ Implement action chaining (pipeline)

### Monitoring Enhancements
- [ ] ⬜ Add Slack notifications for alarms
- [ ] ⬜ Add PagerDuty integration
- [ ] ⬜ Create Grafana dashboard
- [ ] ⬜ Add synthetic monitoring (canary tests)

---

## Acceptance Criteria

### Weather Action
- ✅ Scheduled at 5:00 AM EST daily
- ✅ Fetches forecast from weather.gov API
- ✅ Supports configurable number of days (default: 2)
- ✅ Sends notification with detailed forecast
- ✅ Result stored in DynamoDB with 3-day TTL

### Golf Action
- ✅ Scheduled at 5:15 AM EST daily
- ✅ Authenticates via OAuth 2.0 password grant
- ✅ Credentials stored in Secrets Manager (encrypted)
- ✅ Fetches upcoming reservations
- ✅ Sends notification with next 4 tee times
- ✅ Result stored in DynamoDB with 3-day TTL

### Resilience
- ✅ HTTP failures retried 3 times with exponential backoff
- ✅ Failed messages sent to DLQ after 3 SQS retries
- ✅ OAuth tokens cached for 50 minutes
- ✅ All errors logged with correlation IDs

### Observability
- ✅ CloudWatch metrics published for all actions
- ✅ CloudWatch alarms configured for failures
- ✅ X-Ray tracing enabled
- ✅ CloudWatch dashboard created
- ✅ Runbooks documented

### Security
- ✅ Secrets encrypted at rest (KMS)
- ✅ IAM roles with least-privilege access
- ✅ TLS 1.2+ for all HTTP connections
- ✅ Tokens never logged
- ✅ Credentials never logged

---

## Progress Tracking

**Overall Progress:** 0% (0/XX tasks complete)

**Phase 1 (Foundation):** 0% (0/XX tasks)
**Phase 2 (Core Processor):** 0% (0/XX tasks)
**Phase 3 (Weather Action):** 0% (0/XX tasks)
**Phase 4 (Golf Action):** 0% (0/XX tasks)
**Phase 5 (Observability):** 0% (0/XX tasks)
**Phase 6 (Testing):** 0% (0/XX tasks)
**Phase 7 (Deployment):** 0% (0/XX tasks)

---

## Notes

- Update this checklist as tasks are completed
- Mark tasks as ✅ when complete
- Mark tasks as ⚠️ if blocked (add blocker notes)
- Mark tasks as 🔄 when in progress
- Add new tasks as discovered during implementation
- Link to PRs/commits for completed tasks

**Last Updated:** 2025-10-23
