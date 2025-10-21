package notification

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewNtfyClient(t *testing.T) {
	config := NtfyClientConfig{
		BaseURL:    "https://ntfy.sh/test",
		Timeout:    5 * time.Second,
		MaxRetries: 2,
	}

	client := NewNtfyClient(config)

	if client == nil {
		t.Fatal("NewNtfyClient() returned nil")
	}
	if client.baseURL != config.BaseURL {
		t.Errorf("baseURL = %v, want %v", client.baseURL, config.BaseURL)
	}
	if client.httpClient.Timeout != config.Timeout {
		t.Errorf("timeout = %v, want %v", client.httpClient.Timeout, config.Timeout)
	}
	if client.maxRetries != config.MaxRetries {
		t.Errorf("maxRetries = %v, want %v", client.maxRetries, config.MaxRetries)
	}
}

func TestNewNtfyClient_Defaults(t *testing.T) {
	config := NtfyClientConfig{
		BaseURL: "https://ntfy.sh/test",
	}

	client := NewNtfyClient(config)

	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("timeout = %v, want default %v", client.httpClient.Timeout, 10*time.Second)
	}
	if client.maxRetries != 3 {
		t.Errorf("maxRetries = %v, want default %v", client.maxRetries, 3)
	}
	if client.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestNtfyClient_Send_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient(NtfyClientConfig{
		BaseURL:    server.URL,
		MaxRetries: 1,
		Logger:     slog.Default(),
	})

	ctx := context.Background()
	err := client.Send(ctx, "test message")
	if err != nil {
		t.Errorf("Send() error = %v, want nil", err)
	}
}

func TestNtfyClient_Send_Retry(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient(NtfyClientConfig{
		BaseURL:    server.URL,
		MaxRetries: 3,
		Logger:     slog.Default(),
	})

	ctx := context.Background()
	err := client.Send(ctx, "test message")
	if err != nil {
		t.Errorf("Send() error = %v, want nil", err)
	}
	if attemptCount != 2 {
		t.Errorf("attemptCount = %d, want 2", attemptCount)
	}
}

func TestNtfyClient_Send_MaxRetriesExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewNtfyClient(NtfyClientConfig{
		BaseURL:    server.URL,
		MaxRetries: 2,
		Logger:     slog.Default(),
	})

	ctx := context.Background()
	err := client.Send(ctx, "test message")
	if err == nil {
		t.Error("Send() error = nil, want error")
	}
}

func TestNtfyClient_Send_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient(NtfyClientConfig{
		BaseURL:    server.URL,
		MaxRetries: 3,
		Logger:     slog.Default(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := client.Send(ctx, "test message")
	if err == nil {
		t.Error("Send() error = nil, want context deadline exceeded")
	}
}

func TestNtfyClient_SendWithTitle_Success(t *testing.T) {
	var receivedTitle string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTitle = r.Header.Get("Title")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient(NtfyClientConfig{
		BaseURL:    server.URL,
		MaxRetries: 1,
		Logger:     slog.Default(),
	})

	ctx := context.Background()
	expectedTitle := "Test Title"
	err := client.SendWithTitle(ctx, expectedTitle, "test message")
	if err != nil {
		t.Errorf("SendWithTitle() error = %v, want nil", err)
	}
	if receivedTitle != expectedTitle {
		t.Errorf("received title = %v, want %v", receivedTitle, expectedTitle)
	}
}

func TestNtfyClient_Interface(t *testing.T) {
	// Verify that NtfyClient implements Client interface
	var _ Client = (*NtfyClient)(nil)
}
