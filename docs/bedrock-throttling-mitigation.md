# Bedrock Throttling Mitigation

This document describes the throttling mitigation strategies implemented in the rez_agent to handle AWS Bedrock API rate limits.

## Problem

The agent was experiencing `ThrottlingException` errors when calling AWS Bedrock's Converse API:

```
botocore.errorfactory.ThrottlingException: An error occurred (ThrottlingException)
when calling the Converse operation (reached max retries: 4):
Too many requests, please wait before trying again.
```

This occurs when:
1. Multiple concurrent requests hit the API
2. Requests are made too rapidly in succession
3. Bedrock's rate limits are exceeded

## Solution Components

### 1. Adaptive Retry Configuration (`bedrock_config.py`)

**Purpose**: Configure boto3's built-in retry mechanism with adaptive mode

**Implementation**:
```python
retry_config = Config(
    retries={
        'max_attempts': 10,  # Increased from default 4
        'mode': 'adaptive',  # Adjusts retry behavior based on throttling signals
    }
)
```

**Benefits**:
- Automatically adjusts retry timing based on server responses
- Uses exponential backoff with jitter
- Prevents "thundering herd" when multiple requests retry simultaneously

### 2. Token Bucket Rate Limiter

**Purpose**: Limit the number of requests per minute to stay under Bedrock quotas

**Implementation**:
```python
rate_limiter = RateLimiter(requests_per_minute=30)
```

**How it works**:
- Maintains a "bucket" of tokens (30 by default)
- Each request consumes 1 token
- Tokens refill at a constant rate (30 per minute = 0.5 per second)
- Requests block until a token is available

**Configuration**:
- Default: 30 requests/minute (conservative)
- Configurable via environment variable: `BEDROCK_RATE_LIMIT`
- Adjust based on your Bedrock quota

### 3. Application-Level Exponential Backoff

**Purpose**: Add an additional retry layer specifically for throttling errors

**Implementation**:
```python
@with_exponential_backoff(max_retries=5, base_delay=2.0, max_delay=30.0)
def invoke_llm():
    return llm_with_tools.invoke(messages)
```

**Backoff strategy**:
- Attempt 1: Immediate
- Attempt 2: Wait 2.0s + jitter (0-1s)
- Attempt 3: Wait 4.0s + jitter (0-2s)
- Attempt 4: Wait 8.0s + jitter (0-4s)
- Attempt 5: Wait 16.0s + jitter (0-8s)
- Attempt 6: Wait 30.0s + jitter (0-15s, capped at max_delay)

**Jitter**: Randomizes delays by up to 50% to prevent synchronized retries

### 4. Graceful Error Handling

**Purpose**: Provide user-friendly error messages and appropriate HTTP status codes

**Implementation**:
- Throttling errors → HTTP 429 with `Retry-After` header
- Generic errors → HTTP 500 with error details
- Timeout errors → HTTP 504
- User-facing messages avoid technical jargon

## Configuration

### Environment Variables

Add these to your Lambda environment:

```bash
# Rate limiting (requests per minute)
BEDROCK_RATE_LIMIT=30  # Default: 30 req/min

# Boto3 retry configuration
BEDROCK_MAX_RETRIES=10  # Default: 10
BEDROCK_BASE_DELAY=1.0  # Default: 1.0 seconds
BEDROCK_MAX_DELAY=60.0  # Default: 60.0 seconds

# Application-level retry
BEDROCK_APP_RETRIES=5  # Default: 5
BEDROCK_APP_BASE_DELAY=2.0  # Default: 2.0 seconds
BEDROCK_APP_MAX_DELAY=30.0  # Default: 30.0 seconds
```

### Recommended Settings by Use Case

#### Low Traffic (< 100 requests/day)
```bash
BEDROCK_RATE_LIMIT=60  # Higher limit OK
BEDROCK_MAX_RETRIES=5  # Fewer retries needed
BEDROCK_APP_RETRIES=3
```

#### Medium Traffic (100-1000 requests/day)
```bash
BEDROCK_RATE_LIMIT=30  # Default (recommended)
BEDROCK_MAX_RETRIES=10  # Default
BEDROCK_APP_RETRIES=5
```

#### High Traffic (> 1000 requests/day)
```bash
BEDROCK_RATE_LIMIT=20  # More conservative
BEDROCK_MAX_RETRIES=15  # More retries
BEDROCK_APP_RETRIES=7
BEDROCK_APP_MAX_DELAY=60.0  # Longer max delay
```

#### Burst Traffic (periodic spikes)
```bash
BEDROCK_RATE_LIMIT=40  # Allow bursts
BEDROCK_MAX_RETRIES=10
BEDROCK_APP_RETRIES=5
# Consider using SQS for request buffering
```

## Monitoring

### CloudWatch Metrics to Monitor

1. **Lambda Errors**
   - Filter: `ThrottlingException`
   - Alert when count > 5 in 5 minutes

2. **Lambda Duration**
   - Monitor for increased latency due to retries
   - Alert when p99 > 30 seconds

3. **HTTP Status Codes**
   - Track 429 responses
   - Alert when rate > 10% of requests

4. **Bedrock Quotas** (via Service Quotas)
   - Monitor current usage vs. quota
   - Request quota increase if consistently near limit

### Logging

The implementation logs important events:

```python
# Rate limiter timeout
logger.warning(f"Rate limiter timeout after {timeout}s")

# Throttling retry
logger.warning(
    f"Throttling error on attempt {attempt + 1}/{max_retries + 1}, "
    f"retrying in {total_delay:.2f}s: {e}"
)

# Exhausted retries
logger.error(
    f"Max retries ({max_retries}) reached for {func.__name__}, "
    f"last error: {e}"
)
```

Search CloudWatch Logs for these patterns to identify throttling issues.

## Testing

### Verify Rate Limiting

```bash
# Send rapid requests to test rate limiter
for i in {1..50}; do
  curl -X POST https://your-api.amazonaws.com/agent \
    -H "Content-Type: application/json" \
    -d '{"message": "test '$i'"}' &
done
wait

# Expected: Some requests blocked/delayed by rate limiter
# No ThrottlingException errors
```

### Verify Retry Logic

```python
# Temporarily lower rate limit to trigger throttling
import os
os.environ['BEDROCK_RATE_LIMIT'] = '1'

# Make multiple concurrent requests
# Expected: Automatic retries with exponential backoff
# User sees eventual success (not errors)
```

## Troubleshooting

### Still seeing ThrottlingException?

1. **Check your Bedrock quota**
   ```bash
   aws service-quotas get-service-quota \
     --service-code bedrock \
     --quota-code <quota-code>
   ```

2. **Lower the rate limit**
   ```bash
   # Set more conservative limit
   BEDROCK_RATE_LIMIT=15
   ```

3. **Increase retry delays**
   ```bash
   BEDROCK_APP_BASE_DELAY=5.0
   BEDROCK_APP_MAX_DELAY=120.0
   ```

4. **Check for concurrent Lambda instances**
   - Each Lambda instance has its own rate limiter
   - With N instances, effective rate = N × BEDROCK_RATE_LIMIT
   - Consider using DynamoDB for global rate limiting

### Rate limiter blocking legitimate traffic?

1. **Increase rate limit**
   ```bash
   BEDROCK_RATE_LIMIT=60  # or higher
   ```

2. **Reduce timeout**
   ```python
   # In bedrock_config.py, adjust:
   if not rate_limiter.acquire(timeout=10.0):  # Fail faster
   ```

3. **Implement request queuing**
   - Use SQS to buffer requests during bursts
   - Process requests at a steady rate from queue

### High latency?

- Retries increase response time
- Monitor `Lambda Duration` metric
- Consider async processing for non-interactive requests
- Use streaming responses for long-running requests

## Future Improvements

1. **Global Rate Limiting**
   - Use DynamoDB to coordinate rate limiting across Lambda instances
   - Prevents total rate from exceeding quota with concurrent functions

2. **Request Queuing**
   - Add SQS queue in front of agent Lambda
   - Process requests at controlled rate
   - Provides buffering for burst traffic

3. **Circuit Breaker**
   - Stop making requests when error rate is high
   - Prevents wasting quota on failing requests
   - Automatically recover when service is healthy

4. **Priority Queue**
   - Different rate limits for different request types
   - Interactive requests get higher priority
   - Batch operations use lower priority

5. **Adaptive Rate Limiting**
   - Automatically adjust rate based on observed throttling
   - Increase rate when successful, decrease when throttled
   - Learn optimal rate for your quota

## References

- [AWS Bedrock Quotas](https://docs.aws.amazon.com/bedrock/latest/userguide/quotas.html)
- [Boto3 Retry Configuration](https://boto3.amazonaws.com/v1/documentation/api/latest/guide/retries.html)
- [Exponential Backoff and Jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
- [Token Bucket Algorithm](https://en.wikipedia.org/wiki/Token_bucket)
