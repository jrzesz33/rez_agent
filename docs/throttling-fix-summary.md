# Bedrock Throttling Fix - Implementation Summary

## Problem
Agent Lambda was experiencing `ThrottlingException` errors from AWS Bedrock:
```
ThrottlingException: Too many requests, please wait before trying again.
```

## Root Cause
- No rate limiting on requests to Bedrock API
- Default boto3 retry configuration insufficient for high traffic
- No application-level retry logic for throttling errors
- Poor error handling for user experience

## Solution Implemented

### 1. **New Module: `bedrock_config.py`**
Comprehensive throttling mitigation utilities:

- **`RateLimiter` class**: Token bucket algorithm to limit requests/minute
  - Default: 30 requests/minute
  - Configurable via `BEDROCK_RATE_LIMIT` env var
  - Thread-safe with automatic token refilling

- **`create_bedrock_client()`**: Creates boto3 client with adaptive retry mode
  - Increased max retries from 4 to 10
  - Uses AWS SDK's adaptive retry mode
  - Automatically adjusts based on throttling signals

- **`@with_exponential_backoff` decorator**: Application-level retry logic
  - Specifically handles throttling exceptions
  - Exponential backoff with jitter (prevents thundering herd)
  - Configurable retry attempts and delays

### 2. **Updated: `cmd/agent/main.py`**

#### Configuration
Added environment variables for fine-tuning:
```python
BEDROCK_RATE_LIMIT = 30           # requests per minute
BEDROCK_MAX_RETRIES = 10          # boto3 SDK retries
BEDROCK_BASE_DELAY = 1.0          # SDK retry base delay
BEDROCK_MAX_DELAY = 60.0          # SDK retry max delay
BEDROCK_APP_RETRIES = 5           # Application-level retries
BEDROCK_APP_BASE_DELAY = 2.0      # App retry base delay
BEDROCK_APP_MAX_DELAY = 30.0      # App retry max delay
```

#### Rate Limiting
- Acquire token before each LLM invocation
- Block request if rate limit exceeded (with 30s timeout)
- Prevents overwhelming the API

#### Retry Logic
- Layer 1: boto3 adaptive retries (automatic)
- Layer 2: Application exponential backoff
- Combined approach provides robust error recovery

#### Error Handling
- Catches throttling exceptions gracefully
- Returns user-friendly error messages
- Appropriate HTTP status codes:
  - 429 for rate limiting (with `Retry-After` header)
  - 504 for timeouts
  - 500 for other errors

### 3. **Documentation**
Created comprehensive docs:
- `docs/bedrock-throttling-mitigation.md`: Full technical documentation
- `docs/throttling-fix-summary.md`: This summary

## Testing & Verification

### Before Deployment
```bash
# 1. Run unit tests
cd /workspaces/rez_agent
python -m pytest cmd/agent/test_bedrock_config.py -v

# 2. Verify imports
python -c "from cmd.agent.bedrock_config import RateLimiter; print('OK')"

# 3. Verify configuration loading
python -c "from cmd.agent.main import BEDROCK_RATE_LIMIT; print(f'Rate limit: {BEDROCK_RATE_LIMIT}')"
```

### After Deployment
```bash
# 1. Check Lambda logs for configuration
aws logs tail /aws/lambda/agent-function --follow | grep "throttling configuration"

# Expected output:
# Bedrock throttling configuration: rate_limit=30 req/min, max_retries=10, app_retries=5

# 2. Send test requests
curl -X POST https://your-api-url/agent \
  -H "Content-Type: application/json" \
  -d '{"message": "test"}'

# 3. Monitor for throttling errors
aws logs tail /aws/lambda/agent-function --follow | grep -i throttl

# Should see retry logs, not exceptions:
# "Throttling error on attempt 2/6, retrying in 3.5s"
```

### Load Testing
```bash
# Send concurrent requests to verify rate limiting
for i in {1..50}; do
  curl -X POST https://your-api-url/agent \
    -H "Content-Type: application/json" \
    -d '{"message": "test '$i'", "session_id": "test-'$i'"}' &
done
wait

# Expected:
# - Some requests delayed by rate limiter
# - No ThrottlingException errors in logs
# - All requests eventually succeed
```

## Configuration Recommendations

### Development Environment
```bash
# More permissive for dev/testing
BEDROCK_RATE_LIMIT=60
BEDROCK_APP_RETRIES=3
```

### Production Environment
```bash
# Conservative for reliability
BEDROCK_RATE_LIMIT=30
BEDROCK_MAX_RETRIES=10
BEDROCK_APP_RETRIES=5
BEDROCK_APP_MAX_DELAY=30.0
```

### High Traffic Production
```bash
# More aggressive throttling protection
BEDROCK_RATE_LIMIT=20
BEDROCK_MAX_RETRIES=15
BEDROCK_APP_RETRIES=7
BEDROCK_APP_MAX_DELAY=60.0
```

## Monitoring

### Key Metrics to Track

1. **Throttling Events**
   - Filter CloudWatch Logs: "Throttling error on attempt"
   - Alert if count > 10 in 5 minutes

2. **Rate Limiter Timeouts**
   - Filter: "Rate limiter timeout"
   - Indicates rate limit too restrictive

3. **HTTP 429 Responses**
   - Track user-facing rate limit errors
   - Alert if rate > 5% of requests

4. **Lambda Duration**
   - Monitor p99 latency
   - Retries increase response time
   - Alert if p99 > 30 seconds

### CloudWatch Insights Queries

```
# Count throttling retries
fields @timestamp, @message
| filter @message like /Throttling error on attempt/
| stats count() by bin(5m)

# Find sessions hitting rate limits
fields @timestamp, session_id
| filter @message like /Rate limit exceeded/
| stats count() by session_id

# Monitor retry delays
fields @timestamp, @message
| parse @message /retrying in (?<delay>\d+\.\d+)s/
| stats avg(delay), max(delay), count()
```

## Rollback Plan

If issues occur after deployment:

1. **Quick fix**: Increase rate limit
   ```bash
   aws lambda update-function-configuration \
     --function-name agent-function \
     --environment "Variables={BEDROCK_RATE_LIMIT=60,...}"
   ```

2. **Emergency rollback**: Revert to previous version
   ```bash
   # Find previous version
   aws lambda list-versions-by-function --function-name agent-function

   # Rollback
   aws lambda update-alias \
     --function-name agent-function \
     --name production \
     --function-version <previous-version>
   ```

3. **Disable rate limiting temporarily**
   ```bash
   # Set very high limit (effectively disabling)
   BEDROCK_RATE_LIMIT=10000
   ```

## Future Improvements

1. **Global Rate Limiting** (across Lambda instances)
   - Use DynamoDB atomic counters
   - Coordinate rate limiting across all instances

2. **Request Queue** (SQS-based buffering)
   - Queue requests during bursts
   - Process at steady rate
   - Better user experience during spikes

3. **Adaptive Rate Limiting**
   - Automatically adjust based on observed throttling
   - Learn optimal rate for your quota
   - Increase limit during low traffic

4. **Circuit Breaker**
   - Stop requests when error rate is high
   - Fail fast instead of wasting quota
   - Auto-recover when healthy

5. **Priority Queuing**
   - Different limits for different request types
   - Interactive requests get priority
   - Batch operations use lower priority

## Files Changed

```
cmd/agent/bedrock_config.py          [NEW]  Rate limiting & retry utilities
cmd/agent/main.py                    [MODIFIED]  Integrated throttling mitigation
docs/bedrock-throttling-mitigation.md [NEW]  Technical documentation
docs/throttling-fix-summary.md        [NEW]  This summary
```

## Deployment Checklist

- [ ] Review code changes
- [ ] Update Lambda environment variables (if needed)
- [ ] Deploy Lambda with new code
- [ ] Verify configuration in logs
- [ ] Run smoke tests
- [ ] Monitor CloudWatch for throttling errors
- [ ] Load test to verify rate limiting
- [ ] Update runbooks with new configuration options
- [ ] Train team on new environment variables
- [ ] Set up CloudWatch alarms for throttling metrics
