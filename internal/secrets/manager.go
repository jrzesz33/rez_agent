package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// SecretValue represents a generic secret value
type SecretValue map[string]string

// OAuthCredentials represents OAuth client credentials
type OAuthCredentials struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// CachedSecret represents a cached secret with TTL
type cachedSecret struct {
	Value      SecretValue
	ExpiresAt  time.Time
}

// Manager handles AWS Secrets Manager operations with caching
type Manager struct {
	client    *secretsmanager.Client
	logger    *slog.Logger
	cache     map[string]*cachedSecret
	cacheLock sync.RWMutex
	cacheTTL  time.Duration
}

// NewManager creates a new secrets manager with caching
func NewManager(cfg aws.Config, logger *slog.Logger) *Manager {
	return &Manager{
		client:    secretsmanager.NewFromConfig(cfg),
		logger:    logger,
		cache:     make(map[string]*cachedSecret),
		cacheLock: sync.RWMutex{},
		cacheTTL:  5 * time.Minute, // Cache secrets for 5 minutes
	}
}

// GetSecret retrieves a secret from AWS Secrets Manager with caching
func (m *Manager) GetSecret(ctx context.Context, secretName string) (SecretValue, error) {
	// Check cache first
	if cached := m.getFromCache(secretName); cached != nil {
		m.logger.Debug("secret cache hit", slog.String("secret_name", "[REDACTED]"))
		return cached.Value, nil
	}

	m.logger.Debug("secret cache miss, fetching from AWS", slog.String("secret_name", "[REDACTED]"))

	// Fetch from AWS Secrets Manager
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	}

	result, err := m.client.GetSecretValue(ctx, input)
	if err != nil {
		m.logger.Error("failed to retrieve secret",
			slog.String("error", err.Error()),
			// SECURITY: Never log secret name in production
			slog.String("secret_name", "[REDACTED]"),
		)
		return nil, fmt.Errorf("failed to retrieve secret: %w", err)
	}

	if result.SecretString == nil {
		return nil, fmt.Errorf("secret has no string value")
	}

	// Parse secret JSON
	var secretValue SecretValue
	if err := json.Unmarshal([]byte(*result.SecretString), &secretValue); err != nil {
		return nil, fmt.Errorf("failed to parse secret JSON: %w", err)
	}

	// Cache the secret
	m.putInCache(secretName, secretValue)

	return secretValue, nil
}

// GetOAuthCredentials retrieves OAuth credentials from a secret
func (m *Manager) GetOAuthCredentials(ctx context.Context, secretName string) (*OAuthCredentials, error) {
	secretValue, err := m.GetSecret(ctx, secretName)
	if err != nil {
		return nil, err
	}

	// Extract OAuth fields
	creds := &OAuthCredentials{
		Username:     secretValue["username"],
		Password:     secretValue["password"],
		ClientID:     secretValue["client_id"],
		ClientSecret: secretValue["client_secret"],
	}

	// Validate required fields
	if creds.Username == "" || creds.Password == "" {
		return nil, fmt.Errorf("secret missing required OAuth fields (username, password)")
	}

	// SECURITY: Never log credentials
	m.logger.Debug("OAuth credentials retrieved",
		slog.String("secret_name", "[REDACTED]"),
	)

	return creds, nil
}

// getFromCache retrieves a secret from cache if not expired
func (m *Manager) getFromCache(secretName string) *cachedSecret {
	m.cacheLock.RLock()
	defer m.cacheLock.RUnlock()

	cached, exists := m.cache[secretName]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(cached.ExpiresAt) {
		return nil
	}

	return cached
}

// putInCache stores a secret in cache with TTL
func (m *Manager) putInCache(secretName string, value SecretValue) {
	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()

	m.cache[secretName] = &cachedSecret{
		Value:     value,
		ExpiresAt: time.Now().Add(m.cacheTTL),
	}
}

// ClearCache clears all cached secrets
func (m *Manager) ClearCache() {
	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()

	m.cache = make(map[string]*cachedSecret)
	m.logger.Debug("secret cache cleared")
}

// GetCacheSize returns the number of cached secrets
func (m *Manager) GetCacheSize() int {
	m.cacheLock.RLock()
	defer m.cacheLock.RUnlock()

	return len(m.cache)
}
