# rez_agent Architecture Quick Reference

## System Components at a Glance

### Lambda Functions (4 Total)

| Function | Trigger | Timeout | Memory | Purpose |
|----------|---------|---------|--------|---------|
| **Scheduler** | EventBridge (24h) | 30s | 256 MB | Create scheduled messages |
| **Web API** | API Gateway | 15s | 512 MB | REST API for frontend |
| **Message Processor** | SQS (batch: 10) | 5 min | 512 MB | Process messages, orchestrate delivery |
| **Notification Service** | Lambda invoke | 30s | 256 MB | Send to ntfy.sh with retries |

---

### Data Stores

| Store | Purpose | Key Config |
|-------|---------|------------|
| **DynamoDB (messages)** | Message metadata | PK: message_id, GSIs: stage-created_date, status-created_date, TTL: 90d |
| **DynamoDB (circuit-breaker)** | Circuit breaker state | PK: service_name, shared across Lambdas |
| **Parameter Store** | Secrets & config | /rez-agent/{stage}/* (ntfy-api-key, cognito-* ) |

---

### AWS Services

| Service | Resource | Purpose |
|---------|----------|---------|
| **SNS** | rez-agent-messages-{stage} | Publish message events (fanout) |
| **SQS** | rez-agent-message-queue-{stage} | Message processing queue |
| **SQS DLQ** | rez-agent-message-queue-dlq-{stage} | Failed messages (after 3 retries) |
| **Cognito** | rez-agent-users-{stage} | User authentication (OAuth 2.0) |
| **EventBridge** | schedule-rule | Trigger Scheduler every 24h |
| **API Gateway** | HTTP API | REST API endpoint (with Lambda Authorizer) |

---

## Message Flow (End-to-End)

### Scheduled Message
```
EventBridge (every 24h)
  → Scheduler Lambda
      → DynamoDB PutItem (status: created)
      → SNS Publish (message_id)
          → SQS Queue
              → Message Processor Lambda (batch: 10)
                  → DynamoDB UpdateItem (status: processing)
                  → Notification Service Lambda
                      → ntfy.sh HTTP POST
                  → DynamoDB UpdateItem (status: completed)
```

**Total Time**: ~10 seconds (p95)

---

### Manual Message (Web API)
```
Frontend (OAuth authenticated)
  → API Gateway
      → Lambda Authorizer (validate JWT)
          → Web API Lambda
              → DynamoDB PutItem (status: created)
              → SNS Publish (message_id)
              → Return 202 Accepted
  → (async) SQS → Message Processor → Notification Service → ntfy.sh
```

**API Response Time**: < 500ms (p95)

---

## Environment Variables Cheat Sheet

### Scheduler Lambda
```bash
STAGE=dev
DYNAMODB_TABLE_NAME=rez-agent-messages-dev
SNS_TOPIC_ARN=arn:aws:sns:...:rez-agent-messages-dev
LOG_LEVEL=INFO
```

### Web API Lambda
```bash
STAGE=dev
DYNAMODB_TABLE_NAME=rez-agent-messages-dev
SNS_TOPIC_ARN=arn:aws:sns:...:rez-agent-messages-dev
COGNITO_USER_POOL_ID=us-east-1_ABC123
COGNITO_CLIENT_ID=3n4u5v6w7x8y9z
LOG_LEVEL=INFO
```

### Message Processor Lambda
```bash
STAGE=dev
DYNAMODB_TABLE_NAME=rez-agent-messages-dev
NOTIFICATION_SERVICE_ARN=arn:aws:lambda:...:rez-agent-notification-service-dev
LOG_LEVEL=INFO
```

### Notification Service Lambda
```bash
STAGE=dev
NTFY_URL_PARAM=/rez-agent/dev/ntfy-url
NTFY_API_KEY_PARAM=/rez-agent/dev/ntfy-api-key
CIRCUIT_BREAKER_TABLE=rez-agent-circuit-breaker-dev
MAX_RETRIES=3
LOG_LEVEL=INFO
```

---

## API Endpoints (Quick Reference)

| Method | Endpoint | Auth | Purpose |
|--------|----------|------|---------|
| GET | /api/messages | JWT | List messages (filter: stage, status, date) |
| POST | /api/messages | JWT | Create manual message |
| GET | /api/messages/{id} | JWT | Get message by ID |
| GET | /api/metrics | JWT | Dashboard metrics (counts, success rate) |
| POST | /api/auth/login | None | Initiate OAuth login |
| GET | /api/auth/callback | None | OAuth callback (exchange code for token) |
| POST | /api/auth/refresh | None | Refresh access token |
| GET | /api/health | None | Health check |

**Base URL**: https://api-{stage}.rez-agent.example.com

---

## Error Handling Summary

### Retry Strategy
- **Max Attempts**: 3
- **Backoff**: 1s → 2s → 4s (exponential with jitter)
- **Retryable Errors**: Network timeout, 429 Rate Limit, 503 Service Unavailable
- **Non-Retryable**: 400 Bad Request, 401 Unauthorized, 404 Not Found

### Circuit Breaker
- **Threshold**: 5 failures in 1 minute → OPEN
- **Timeout**: 30 seconds (OPEN → HALF_OPEN)
- **Recovery**: 2 consecutive successes (HALF_OPEN → CLOSED)
- **Storage**: DynamoDB (shared state across Lambda instances)

### Dead Letter Queue
- **Trigger**: After 3 SQS retries
- **Alarm**: CloudWatch alarm on message count > 0
- **Resolution**: Manual investigation + redrive

---

## Observability Quick Reference

### Key Metrics
| Metric | Namespace | Dimensions | Alarm Threshold |
|--------|-----------|------------|-----------------|
| MessagesProcessed | RezAgent | Stage, Status | - |
| NotificationsSent | RezAgent | Stage, Status | Failures > 10/5min |
| ProcessingDuration | RezAgent | Stage, MessageType | p99 > 4min |
| CircuitBreakerState | RezAgent | Stage, Service | = 1 (open) |
| Lambda Errors | AWS/Lambda | FunctionName | > 5/5min |
| DLQ Message Count | AWS/SQS | QueueName | > 0 |

### CloudWatch Logs Insights Queries

**Find all logs for correlation ID**:
```sql
fields @timestamp, @message, level
| filter correlation_id = "YOUR_CORRELATION_ID"
| sort @timestamp asc
```

**Count errors by message_id**:
```sql
fields message_id, @message
| filter level = "ERROR"
| stats count() as error_count by message_id
| sort error_count desc
```

**Average API response time**:
```sql
fields duration_ms
| filter path = "/api/messages"
| stats avg(duration_ms), max(duration_ms), pct(duration_ms, 95)
```

---

## Security Checklist

- [ ] All Lambda functions have dedicated IAM roles (least-privilege)
- [ ] Secrets stored in Parameter Store (SecureString with KMS encryption)
- [ ] API Gateway enforces HTTPS only
- [ ] Lambda Authorizer validates JWT tokens (signature, expiration, issuer)
- [ ] CORS configured with allowed origins (no wildcards in prod)
- [ ] Rate limiting enabled on API Gateway (1000 req/sec prod)
- [ ] DynamoDB encryption at rest (AWS-managed KMS key)
- [ ] CloudWatch Logs retention configured (7d dev, 30d prod)
- [ ] X-Ray sampling reduced in prod (5% to minimize cost)

---

## Deployment Checklist

### Pre-Deployment
- [ ] Update Parameter Store values (if changed)
- [ ] Test CDK/Terraform synth (validate templates)
- [ ] Run unit tests (`go test ./...`)
- [ ] Review IAM policy changes (no overly permissive policies)
- [ ] Update CloudWatch alarms (if thresholds changed)

### Deploy to Dev
- [ ] Deploy infrastructure (`cdk deploy --context stage=dev`)
- [ ] Verify Lambda functions deployed (check version/alias)
- [ ] Test scheduled message flow (trigger EventBridge manually)
- [ ] Test Web API endpoints (Postman/curl)
- [ ] Check CloudWatch Logs (no errors)

### Deploy to Stage
- [ ] Deploy infrastructure (`cdk deploy --context stage=stage`)
- [ ] Run integration tests
- [ ] Monitor for 1 hour (check metrics, logs, alarms)

### Deploy to Prod
- [ ] Deploy with blue-green strategy (Lambda aliases)
- [ ] Shift traffic gradually (10% → 50% → 100%)
- [ ] Monitor CloudWatch alarms (set up on-call rotation)
- [ ] Prepare rollback plan (previous Lambda version)

### Post-Deployment
- [ ] Verify scheduled messages sent to ntfy.sh
- [ ] Test manual message creation via Web API
- [ ] Check DynamoDB item count (messages created)
- [ ] Review CloudWatch dashboard (all metrics green)
- [ ] Update documentation (if API changed)

---

## Troubleshooting Guide

### Issue: Messages not sent to ntfy.sh

**Possible Causes**:
1. Circuit breaker is OPEN (ntfy.sh unavailable)
2. Invalid API key (Parameter Store)
3. Network timeout (Lambda VPC config issue)
4. ntfy.sh rate limiting (429 responses)

**Investigation**:
1. Check CloudWatch Logs for Notification Service Lambda
2. Query circuit breaker state in DynamoDB
3. Test ntfy.sh API directly (curl)
4. Check custom metric: `CircuitBreakerState`

**Resolution**:
- If circuit open: Wait 30s for HALF_OPEN, or manually reset state
- If API key invalid: Update Parameter Store + wait 5min (cache TTL)
- If rate limited: Reduce scheduled message frequency or contact ntfy.sh

---

### Issue: Messages stuck in SQS queue

**Possible Causes**:
1. Message Processor Lambda throttled (max concurrency)
2. Lambda timeout (exceeds 5 minutes)
3. DynamoDB throttling (unlikely with on-demand)
4. All messages failing (sent to DLQ after 3 retries)

**Investigation**:
1. Check SQS metric: `ApproximateNumberOfMessagesVisible`
2. Check Lambda metric: `Throttles`, `Duration`
3. Check DLQ metric: `ApproximateNumberOfMessagesVisible`
4. Query CloudWatch Logs for Message Processor errors

**Resolution**:
- If throttled: Increase Lambda reserved concurrency
- If timeout: Optimize processing logic or increase timeout
- If DLQ has messages: Investigate error cause, redrive after fix

---

### Issue: API returns 401 Unauthorized

**Possible Causes**:
1. JWT token expired (access token: 1 hour)
2. Invalid JWT signature (wrong JWKS public key)
3. Lambda Authorizer error (JWKS fetch failed)
4. Authorization header missing/malformed

**Investigation**:
1. Check JWT token expiration (`exp` claim)
2. Verify Authorization header format: `Bearer <token>`
3. Check Lambda Authorizer CloudWatch Logs
4. Decode JWT at jwt.io to inspect claims

**Resolution**:
- If expired: Use refresh token to get new access token (POST /api/auth/refresh)
- If invalid: Re-authenticate (POST /api/auth/login)
- If JWKS issue: Check Lambda Authorizer has internet access (no VPC or NAT configured)

---

## Performance Targets (SLI/SLO)

| Metric | Target (SLO) | Current (Example) |
|--------|--------------|-------------------|
| API Availability | 99.9% (30d) | 99.95% |
| API Response Time (p95) | < 500ms | 320ms |
| API Response Time (p99) | < 1000ms | 680ms |
| Message Processing Latency (p95) | < 10s | 8.2s |
| Notification Success Rate | 99% (7d) | 99.3% |

**Error Budget** (99.9% availability): 43 minutes downtime per month

---

## Cost Estimates (Monthly)

**Assumptions**: 1,000 messages/day, 10,000 API requests/day

| Service | Usage | Cost |
|---------|-------|------|
| Lambda Invocations | 150k/month | $0.03 |
| Lambda Duration (GB-s) | 50k GB-s | $0.83 |
| DynamoDB (on-demand) | 100k writes, 300k reads | $0.37 |
| API Gateway | 300k requests | $0.30 |
| SNS | 100k publishes | $0.50 |
| SQS | 100k requests | Free (first 1M) |
| CloudWatch Logs | 5 GB | $2.50 |
| CloudWatch Metrics | 20 custom metrics | $6.00 |
| X-Ray (5% sampling) | 7.5k traces | $0.04 |
| **Total** | | **~$10.57/month** |

**At 10x scale** (10,000 messages/day): ~$50/month

---

## Key Design Decisions Recap

| Decision | Choice | Alternative | Rationale |
|----------|--------|-------------|-----------|
| Data Store | DynamoDB | RDS | Serverless-native, no connection pooling, auto-scaling |
| OAuth Provider | AWS Cognito | Auth0 | AWS-native, cost-effective, seamless integration |
| Message Queue | SNS + SQS | Kafka, RabbitMQ | Managed, serverless, low-ops overhead |
| Logging Library | log/slog | Zap | Standard library, no dependencies, good enough |
| IaC Tool | AWS CDK (Go) | Terraform | Type-safe, matches app language |
| API Type | HTTP API | REST API | Simpler, cheaper, sufficient features |
| Capacity Mode | On-Demand | Provisioned | Variable workload, no capacity planning |

---

## Go Module Structure (Recommended)

```
rez_agent/
├── cmd/
│   ├── scheduler/main.go          # Scheduler Lambda entry point
│   ├── web-api/main.go            # Web API Lambda entry point
│   ├── message-processor/main.go  # Message Processor Lambda entry point
│   ├── notification-service/main.go # Notification Service Lambda entry point
│   └── jwt-authorizer/main.go     # JWT Authorizer Lambda entry point
├── internal/
│   ├── config/config.go           # Configuration loading
│   ├── models/message.go          # Data models (Message, MessageEvent)
│   ├── dynamodb/
│   │   ├── client.go              # DynamoDB client wrapper
│   │   ├── messages.go            # Message CRUD operations
│   │   └── circuit_breaker.go     # Circuit breaker state management
│   ├── sns/publisher.go           # SNS publishing
│   ├── sqs/consumer.go            # SQS message parsing
│   ├── ssm/parameter_store.go     # Parameter Store client
│   ├── circuitbreaker/
│   │   ├── circuit_breaker.go     # Circuit breaker implementation
│   │   └── state.go               # Circuit breaker state machine
│   ├── retry/retry.go             # Exponential backoff retry logic
│   ├── correlation/context.go     # Correlation ID propagation
│   ├── ntfy/client.go             # ntfy.sh HTTP client
│   ├── auth/
│   │   ├── cognito.go             # Cognito OAuth flow
│   │   └── jwt.go                 # JWT validation
│   └── metrics/recorder.go        # CloudWatch Metrics recording
├── docs/                          # Architecture documentation (this directory)
├── infra/                         # CDK or Terraform IaC
│   ├── cdk.go                     # CDK app entry point
│   ├── stacks/                    # CDK stacks per environment
│   └── constructs/                # Reusable CDK constructs
├── go.mod
├── go.sum
└── README.md
```

---

## Additional Resources

- **AWS Lambda Go**: https://github.com/aws/aws-lambda-go
- **AWS SDK Go v2**: https://aws.github.io/aws-sdk-go-v2/docs/
- **AWS CDK Go**: https://docs.aws.amazon.com/cdk/v2/guide/work-with-cdk-go.html
- **ntfy.sh API**: https://docs.ntfy.sh/publish/
- **Go slog**: https://pkg.go.dev/log/slog
- **AWS X-Ray Go SDK**: https://github.com/aws/aws-xray-sdk-go

---

## Summary

This quick reference provides essential information for implementing and operating the rez_agent system. For detailed designs, refer to the individual architecture documents in this directory.

**Start with**: service-architecture.md → data-model.md → implementation

**Questions?** Consult the detailed architecture docs or create a GitHub issue.
