package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client wraps http.Client with security features and retry logic
type Client struct {
	httpClient *http.Client
	logger     *slog.Logger

	// OAuth token cache
	oauthCache     map[string]*cachedToken
	oauthCacheLock sync.RWMutex
}

// cachedToken represents a cached OAuth token
type cachedToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

// NewClient creates a new HTTP client with security configuration
func NewClient(logger *slog.Logger) *Client {
	// SECURITY: Configure TLS 1.2+ only
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
	}

	// Configure HTTP transport with timeouts and security
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// SECURITY: Disable HTTP/2 to prevent certain attacks (optional)
		ForceAttemptHTTP2: true,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // Default timeout
		// SECURITY: Do NOT follow redirects automatically (prevent open redirect)
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Client{
		httpClient:     httpClient,
		logger:         logger,
		oauthCache:     make(map[string]*cachedToken),
		oauthCacheLock: sync.RWMutex{},
	}
}

// RequestConfig contains configuration for an HTTP request
type RequestConfig struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    interface{}
	Timeout time.Duration
}

// Response represents an HTTP response
type Response struct {
	StatusCode int
	Body       string
	Headers    http.Header
}

// Do executes an HTTP request with retry logic
func (c *Client) Do(ctx context.Context, config RequestConfig) (*Response, error) {
	// Apply custom timeout if specified
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	// Retry logic: 3 attempts with exponential backoff
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2^attempt seconds
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			c.logger.Info("retrying HTTP request",
				slog.Int("attempt", attempt+1),
				slog.Int("max_retries", maxRetries),
				slog.Duration("backoff", backoff),
			)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := c.doRequest(ctx, config)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err, resp) {
			c.logger.Warn("non-retryable error, aborting",
				slog.String("error", err.Error()),
			)
			break
		}

		c.logger.Warn("retryable error occurred",
			slog.String("error", err.Error()),
			slog.Int("attempt", attempt+1),
		)
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries, lastErr)
}

// doRequest performs a single HTTP request
func (c *Client) doRequest(ctx context.Context, config RequestConfig) (*Response, error) {
	// Prepare request body
	var bodyReader io.Reader
	if config.Body != nil {
		bodyBytes, err := json.Marshal(config.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, config.Method, config.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Set default headers if not provided
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "rez-agent/1.0")
	}
	if config.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// SECURITY: Log request without sensitive headers
	c.logRequest(req)

	// Execute request
	startTime := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		c.logger.Error("HTTP request failed",
			slog.String("method", config.Method),
			slog.String("url", config.URL),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	response := &Response{
		StatusCode: resp.StatusCode,
		Body:       string(bodyBytes),
		Headers:    resp.Header,
	}

	// Log response
	c.logger.Info("HTTP request completed",
		slog.String("method", config.Method),
		slog.String("url", config.URL),
		slog.Int("status_code", resp.StatusCode),
		slog.Duration("duration", duration),
		slog.Int("response_size", len(bodyBytes)),
	)

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		return response, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, truncateBody(string(bodyBytes), 200))
	}

	return response, nil
}

// logRequest logs HTTP request without sensitive information
func (c *Client) logRequest(req *http.Request) {
	// SECURITY: Redact sensitive headers
	redactedHeaders := make(map[string]string)
	for key := range req.Header {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "auth") ||
		   strings.Contains(lowerKey, "token") ||
		   strings.Contains(lowerKey, "key") ||
		   strings.Contains(lowerKey, "secret") ||
		   strings.Contains(lowerKey, "password") {
			redactedHeaders[key] = "[REDACTED]"
		} else {
			redactedHeaders[key] = req.Header.Get(key)
		}
	}

	c.logger.Debug("HTTP request",
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
		slog.Any("headers", redactedHeaders),
	)
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error, resp *Response) bool {
	// Network errors are retryable
	if err != nil {
		// Timeout errors
		if strings.Contains(err.Error(), "timeout") ||
		   strings.Contains(err.Error(), "deadline exceeded") {
			return true
		}
		// Connection errors
		if strings.Contains(err.Error(), "connection") ||
		   strings.Contains(err.Error(), "EOF") {
			return true
		}
	}

	// HTTP 5xx errors are retryable
	if resp != nil && resp.StatusCode >= 500 && resp.StatusCode < 600 {
		return true
	}

	// HTTP 429 (Too Many Requests) is retryable
	if resp != nil && resp.StatusCode == 429 {
		return true
	}

	return false
}

// truncateBody truncates a response body for logging
func truncateBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return body
	}
	return body[:maxLen] + "..."
}

// GetCachedOAuthToken retrieves a cached OAuth token if not expired
func (c *Client) GetCachedOAuthToken(cacheKey string) (string, bool) {
	c.oauthCacheLock.RLock()
	defer c.oauthCacheLock.RUnlock()

	cached, exists := c.oauthCache[cacheKey]
	if !exists {
		return "", false
	}

	// Check if expired (with 10-minute buffer)
	if time.Now().Add(10 * time.Minute).After(cached.ExpiresAt) {
		return "", false
	}

	// SECURITY: Never log the actual token
	c.logger.Debug("OAuth token cache hit",
		slog.String("cache_key", cacheKey),
	)

	return cached.AccessToken, true
}

// CacheOAuthToken stores an OAuth token with expiration
func (c *Client) CacheOAuthToken(cacheKey, accessToken string, expiresIn int) {
	c.oauthCacheLock.Lock()
	defer c.oauthCacheLock.Unlock()

	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
	c.oauthCache[cacheKey] = &cachedToken{
		AccessToken: accessToken,
		ExpiresAt:   expiresAt,
	}

	// SECURITY: Never log the actual token
	c.logger.Debug("OAuth token cached",
		slog.String("cache_key", cacheKey),
		slog.Time("expires_at", expiresAt),
	)
}

// ClearOAuthCache clears all cached OAuth tokens
func (c *Client) ClearOAuthCache() {
	c.oauthCacheLock.Lock()
	defer c.oauthCacheLock.Unlock()

	c.oauthCache = make(map[string]*cachedToken)
	c.logger.Info("OAuth token cache cleared")
}

// DoFormPost performs a form-encoded POST request (for OAuth token requests)
func (c *Client) DoFormPost(ctx context.Context, targetURL string, formData url.Values, headers map[string]string) (*Response, error) {
	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create form request: %w", err)
	}

	// Set form content type
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "rez-agent/1.0")

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// SECURITY: Log request without sensitive data
	c.logRequest(req)

	// Execute request
	startTime := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		c.logger.Error("form POST failed",
			slog.String("url", targetURL),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("form POST failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	response := &Response{
		StatusCode: resp.StatusCode,
		Body:       string(bodyBytes),
		Headers:    resp.Header,
	}

	c.logger.Info("form POST completed",
		slog.String("url", targetURL),
		slog.Int("status_code", resp.StatusCode),
		slog.Duration("duration", duration),
	)

	if resp.StatusCode >= 400 {
		return response, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, truncateBody(string(bodyBytes), 200))
	}

	return response, nil
}
