package notification

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client defines the interface for notification operations
type Client interface {
	Send(ctx context.Context, message string) error
}

// NtfyClient is an HTTP client for sending notifications to ntfy.sh
type NtfyClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
	maxRetries int
}

// NtfyClientConfig holds configuration for the Ntfy client
type NtfyClientConfig struct {
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
	Logger     *slog.Logger
}

// NewNtfyClient creates a new ntfy.sh notification client
func NewNtfyClient(config NtfyClientConfig) *NtfyClient {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	return &NtfyClient{
		baseURL: config.BaseURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:     config.Logger,
		maxRetries: config.MaxRetries,
	}
}

// Send sends a notification message to ntfy.sh with retry logic
func (c *NtfyClient) Send(ctx context.Context, message string) error {
	var lastErr error

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, etc.
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			c.logger.DebugContext(ctx, "retrying notification send",
				slog.Int("attempt", attempt+1),
				slog.Int("max_retries", c.maxRetries),
				slog.Duration("backoff", backoff),
			)

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		err := c.sendOnce(ctx, message)
		if err == nil {
			c.logger.DebugContext(ctx, "notification sent successfully",
				slog.Int("attempt", attempt+1),
			)
			return nil
		}

		lastErr = err
		c.logger.WarnContext(ctx, "failed to send notification",
			slog.Int("attempt", attempt+1),
			slog.String("error", err.Error()),
		)
	}

	return fmt.Errorf("failed to send notification after %d attempts: %w", c.maxRetries, lastErr)
}

// sendOnce attempts to send a notification once without retries
func (c *NtfyClient) sendOnce(ctx context.Context, message string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewBufferString(message))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for error details
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy.sh returned non-success status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SendWithTitle sends a notification with a custom title
func (c *NtfyClient) SendWithTitle(ctx context.Context, title, message string) error {
	var lastErr error

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			c.logger.DebugContext(ctx, "retrying notification send with title",
				slog.Int("attempt", attempt+1),
				slog.Int("max_retries", c.maxRetries),
				slog.Duration("backoff", backoff),
			)

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		err := c.sendOnceWithTitle(ctx, title, message)
		if err == nil {
			c.logger.DebugContext(ctx, "notification with title sent successfully",
				slog.Int("attempt", attempt+1),
			)
			return nil
		}

		lastErr = err
		c.logger.WarnContext(ctx, "failed to send notification with title",
			slog.Int("attempt", attempt+1),
			slog.String("error", err.Error()),
		)
	}

	return fmt.Errorf("failed to send notification with title after %d attempts: %w", c.maxRetries, lastErr)
}

// sendOnceWithTitle attempts to send a notification with title once without retries
func (c *NtfyClient) sendOnceWithTitle(ctx context.Context, title, message string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewBufferString(message))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Title", title)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy.sh returned non-success status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
