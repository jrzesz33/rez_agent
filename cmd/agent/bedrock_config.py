"""
Bedrock Configuration Module

Provides optimized boto3 configuration for AWS Bedrock with:
- Exponential backoff with jitter
- Custom retry configuration
- Rate limiting
"""
import time
import logging
from typing import Optional
from functools import wraps
from threading import Lock

import boto3
from botocore.config import Config
from botocore.exceptions import ClientError

logger = logging.getLogger(__name__)


class RateLimiter:
    """
    Token bucket rate limiter for controlling Bedrock API requests.

    This prevents overwhelming the Bedrock API with too many concurrent requests.
    """

    def __init__(self, requests_per_minute: int = 30):
        """
        Initialize rate limiter.

        Args:
            requests_per_minute: Maximum number of requests allowed per minute
        """
        self.requests_per_minute = requests_per_minute
        self.tokens = requests_per_minute
        self.max_tokens = requests_per_minute
        self.last_update = time.time()
        self.lock = Lock()

    def _refill_tokens(self):
        """Refill tokens based on elapsed time"""
        now = time.time()
        elapsed = now - self.last_update

        # Add tokens based on elapsed time
        tokens_to_add = elapsed * (self.requests_per_minute / 60.0)
        self.tokens = min(self.max_tokens, self.tokens + tokens_to_add)
        self.last_update = now

    def acquire(self, tokens: int = 1, timeout: float = 30.0) -> bool:
        """
        Acquire tokens from the bucket, blocking if necessary.

        Args:
            tokens: Number of tokens to acquire (default: 1)
            timeout: Maximum time to wait for tokens (seconds)

        Returns:
            True if tokens were acquired, False if timeout occurred
        """
        deadline = time.time() + timeout

        while True:
            with self.lock:
                self._refill_tokens()

                if self.tokens >= tokens:
                    self.tokens -= tokens
                    return True

            # Check timeout
            if time.time() >= deadline:
                logger.warning(f"Rate limiter timeout after {timeout}s")
                return False

            # Wait a bit before trying again
            time.sleep(0.1)


def create_bedrock_client(
    region_name: str = "us-east-1",
    max_retries: int = 10,
    base_delay: float = 1.0,
    max_delay: float = 60.0
) -> boto3.client:
    """
    Create a boto3 Bedrock client with optimized retry configuration.

    Args:
        region_name: AWS region
        max_retries: Maximum number of retry attempts
        base_delay: Initial retry delay in seconds (exponential backoff base)
        max_delay: Maximum retry delay in seconds

    Returns:
        Configured boto3 bedrock-runtime client
    """

    # Configure retry strategy with exponential backoff
    retry_config = Config(
        region_name=region_name,
        retries={
            'max_attempts': max_retries,
            'mode': 'adaptive',  # Adaptive retry mode adjusts based on throttling
        },
        # Connection settings
        connect_timeout=10,
        read_timeout=120,
        # Use exponential backoff with full jitter
        # This helps prevent thundering herd when multiple requests retry
    )

    client = boto3.client('bedrock-runtime', config=retry_config)

    logger.info(
        f"Created Bedrock client with adaptive retry mode "
        f"(max_attempts={max_retries}, region={region_name})"
    )

    return client


def with_rate_limit(rate_limiter: RateLimiter):
    """
    Decorator to apply rate limiting to function calls.

    Args:
        rate_limiter: RateLimiter instance to use
    """
    def decorator(func):
        @wraps(func)
        def wrapper(*args, **kwargs):
            # Acquire token before making request
            if not rate_limiter.acquire(timeout=30.0):
                raise Exception("Rate limit timeout: too many concurrent requests")

            return func(*args, **kwargs)

        return wrapper
    return decorator


def with_exponential_backoff(
    max_retries: int = 5,
    base_delay: float = 1.0,
    max_delay: float = 30.0,
    exceptions: tuple = (ClientError,)
):
    """
    Decorator to add exponential backoff with jitter to function calls.

    This provides an additional layer of retry logic on top of boto3's
    built-in retries, specifically for throttling errors.

    Args:
        max_retries: Maximum number of retry attempts
        base_delay: Initial delay in seconds
        max_delay: Maximum delay in seconds
        exceptions: Tuple of exceptions to catch and retry
    """
    def decorator(func):
        @wraps(func)
        def wrapper(*args, **kwargs):
            last_exception = None

            for attempt in range(max_retries + 1):
                try:
                    return func(*args, **kwargs)

                except exceptions as e:
                    last_exception = e

                    # Check if it's a throttling error
                    is_throttling = False
                    if isinstance(e, ClientError):
                        error_code = e.response.get('Error', {}).get('Code', '')
                        is_throttling = error_code in [
                            'ThrottlingException',
                            'TooManyRequestsException',
                            'ProvisionedThroughputExceededException'
                        ]

                    # If not a throttling error, raise immediately
                    if not is_throttling:
                        raise

                    # If we've exhausted retries, raise the exception
                    if attempt >= max_retries:
                        logger.error(
                            f"Max retries ({max_retries}) reached for {func.__name__}, "
                            f"last error: {e}"
                        )
                        raise

                    # Calculate delay with exponential backoff and jitter
                    delay = min(base_delay * (2 ** attempt), max_delay)
                    # Add jitter (randomize up to 50% of delay)
                    import random
                    jitter = random.uniform(0, delay * 0.5)
                    total_delay = delay + jitter

                    logger.warning(
                        f"Throttling error on attempt {attempt + 1}/{max_retries + 1} "
                        f"for {func.__name__}, retrying in {total_delay:.2f}s: {e}"
                    )

                    time.sleep(total_delay)

            # Should not reach here, but just in case
            raise last_exception

        return wrapper
    return decorator


# Global rate limiter instance
# Default: 30 requests per minute (0.5 requests/second)
# Adjust based on your Bedrock quota
_global_rate_limiter = RateLimiter(requests_per_minute=30)


def get_rate_limiter() -> RateLimiter:
    """Get the global rate limiter instance"""
    return _global_rate_limiter


def set_rate_limit(requests_per_minute: int):
    """
    Update the global rate limiter's rate.

    Args:
        requests_per_minute: New rate limit
    """
    global _global_rate_limiter
    _global_rate_limiter = RateLimiter(requests_per_minute=requests_per_minute)
    logger.info(f"Updated global rate limit to {requests_per_minute} requests/minute")
