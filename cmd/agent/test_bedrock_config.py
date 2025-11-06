"""
Unit tests for bedrock_config module

These tests verify the rate limiting and retry logic without requiring boto3.
"""
import time
import unittest
from unittest.mock import Mock, patch
from threading import Thread


# Mock boto3 to allow imports
import sys

# Create a proper mock for ClientError that inherits from Exception
class MockClientError(Exception):
    def __init__(self, error_response, operation_name):
        self.response = error_response
        self.operation_name = operation_name
        super().__init__(f"{error_response.get('Error', {}).get('Code', 'Unknown')}: {operation_name}")

# Mock modules
sys.modules['boto3'] = Mock()
sys.modules['botocore'] = Mock()
sys.modules['botocore.config'] = Mock()

# Mock botocore.exceptions with ClientError
mock_exceptions = Mock()
mock_exceptions.ClientError = MockClientError
sys.modules['botocore.exceptions'] = mock_exceptions

from bedrock_config import RateLimiter, with_exponential_backoff


class TestRateLimiter(unittest.TestCase):
    """Test cases for RateLimiter class"""

    def test_rate_limiter_initialization(self):
        """Test that rate limiter initializes correctly"""
        limiter = RateLimiter(requests_per_minute=60)
        self.assertEqual(limiter.requests_per_minute, 60)
        self.assertEqual(limiter.max_tokens, 60)
        self.assertEqual(limiter.tokens, 60)

    def test_acquire_single_token(self):
        """Test acquiring a single token"""
        limiter = RateLimiter(requests_per_minute=60)
        result = limiter.acquire(tokens=1, timeout=1.0)
        self.assertTrue(result)
        self.assertEqual(limiter.tokens, 59)

    def test_acquire_multiple_tokens(self):
        """Test acquiring multiple tokens at once"""
        limiter = RateLimiter(requests_per_minute=60)
        result = limiter.acquire(tokens=5, timeout=1.0)
        self.assertTrue(result)
        self.assertEqual(limiter.tokens, 55)

    def test_acquire_timeout(self):
        """Test that acquire times out when no tokens available"""
        limiter = RateLimiter(requests_per_minute=60)
        # Consume all tokens
        limiter.tokens = 0

        start_time = time.time()
        result = limiter.acquire(tokens=1, timeout=0.5)
        elapsed = time.time() - start_time

        self.assertFalse(result)
        self.assertGreaterEqual(elapsed, 0.5)
        self.assertLess(elapsed, 1.0)

    def test_token_refill(self):
        """Test that tokens refill over time"""
        limiter = RateLimiter(requests_per_minute=60)  # 1 token/second
        limiter.tokens = 0

        # Wait 0.5 seconds - should get ~0.5 tokens
        time.sleep(0.5)
        limiter._refill_tokens()
        self.assertGreater(limiter.tokens, 0)
        self.assertLess(limiter.tokens, 1)

        # Wait another 0.5 seconds - should have ~1 token total
        time.sleep(0.5)
        limiter._refill_tokens()
        self.assertGreater(limiter.tokens, 0.5)
        self.assertLess(limiter.tokens, 2)

    def test_max_tokens_cap(self):
        """Test that tokens don't exceed max_tokens"""
        limiter = RateLimiter(requests_per_minute=10)
        limiter.tokens = 0

        # Wait long enough to generate more than max tokens
        time.sleep(2.0)
        limiter._refill_tokens()

        # Should be capped at max_tokens
        self.assertLessEqual(limiter.tokens, limiter.max_tokens)

    def test_concurrent_access(self):
        """Test thread-safe concurrent token acquisition"""
        limiter = RateLimiter(requests_per_minute=100)
        successful_acquisitions = []

        def acquire_token():
            result = limiter.acquire(tokens=1, timeout=2.0)
            successful_acquisitions.append(result)

        # Launch 120 threads (more than available tokens)
        threads = [Thread(target=acquire_token) for _ in range(120)]
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join()

        # All acquisitions should eventually succeed (with token refill)
        # but some may time out if too many concurrent requests
        success_count = sum(successful_acquisitions)
        self.assertGreater(success_count, 80)  # Most should succeed


class TestExponentialBackoff(unittest.TestCase):
    """Test cases for exponential backoff decorator"""

    def test_successful_call_no_retry(self):
        """Test that successful calls don't trigger retries"""
        call_count = [0]

        @with_exponential_backoff(max_retries=3, base_delay=0.1)
        def successful_function():
            call_count[0] += 1
            return "success"

        result = successful_function()
        self.assertEqual(result, "success")
        self.assertEqual(call_count[0], 1)

    def test_non_throttling_error_immediate_raise(self):
        """Test that non-throttling errors are raised immediately"""
        call_count = [0]

        @with_exponential_backoff(max_retries=3, base_delay=0.1)
        def failing_function():
            call_count[0] += 1
            # Non-throttling error
            error_response = {'Error': {'Code': 'ValidationException'}}
            raise MockClientError(error_response, 'test_operation')

        with self.assertRaises(MockClientError):
            failing_function()

        # Should fail immediately, no retries
        self.assertEqual(call_count[0], 1)

    def test_throttling_error_with_retries(self):
        """Test that throttling errors trigger retries"""
        call_count = [0]

        @with_exponential_backoff(max_retries=3, base_delay=0.1, max_delay=1.0)
        def throttling_function():
            call_count[0] += 1
            if call_count[0] < 3:
                # Throttling error on first 2 attempts
                error_response = {'Error': {'Code': 'ThrottlingException'}}
                raise MockClientError(error_response, 'test_operation')
            return "success after retries"

        start_time = time.time()
        result = throttling_function()
        elapsed = time.time() - start_time

        self.assertEqual(result, "success after retries")
        self.assertEqual(call_count[0], 3)  # Initial + 2 retries
        # Should have some delay from retries
        self.assertGreater(elapsed, 0.1)

    def test_max_retries_exhausted(self):
        """Test that function raises after max retries"""
        call_count = [0]

        @with_exponential_backoff(max_retries=2, base_delay=0.1, max_delay=1.0)
        def always_throttling():
            call_count[0] += 1
            error_response = {'Error': {'Code': 'ThrottlingException'}}
            raise MockClientError(error_response, 'test_operation')

        with self.assertRaises(MockClientError):
            always_throttling()

        # Should try: initial + 2 retries = 3 attempts
        self.assertEqual(call_count[0], 3)

    def test_exponential_backoff_timing(self):
        """Test that delays follow exponential backoff pattern"""
        delays = []
        call_count = [0]

        @with_exponential_backoff(max_retries=3, base_delay=0.1, max_delay=10.0)
        def timing_function():
            current_time = time.time()
            if call_count[0] > 0:
                delays.append(current_time - timing_function.last_time)
            timing_function.last_time = current_time
            call_count[0] += 1

            if call_count[0] <= 3:
                error_response = {'Error': {'Code': 'ThrottlingException'}}
                raise MockClientError(error_response, 'test_operation')
            return "success"

        timing_function.last_time = time.time()

        result = timing_function()
        self.assertEqual(result, "success")

        # Verify delays increase (roughly exponentially)
        # Delay 1: ~0.1s + jitter
        # Delay 2: ~0.2s + jitter
        # Delay 3: ~0.4s + jitter
        self.assertEqual(len(delays), 3)
        # Each delay should be longer than previous (with some tolerance for jitter)
        # Just verify they're increasing in general
        self.assertGreater(delays[1], delays[0] * 0.8)
        self.assertGreater(delays[2], delays[1] * 0.8)


if __name__ == '__main__':
    unittest.main()
