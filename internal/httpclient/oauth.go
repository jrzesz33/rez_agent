package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/jrzesz33/rez_agent/internal/secrets"
)

// OAuthClient handles OAuth 2.0 authentication flows
type OAuthClient struct {
	httpClient     *Client
	secretsManager *secrets.Manager
	logger         *slog.Logger
}

// NewOAuthClient creates a new OAuth client
func NewOAuthClient(httpClient *Client, secretsManager *secrets.Manager, logger *slog.Logger) *OAuthClient {
	return &OAuthClient{
		httpClient:     httpClient,
		secretsManager: secretsManager,
		logger:         logger,
	}
}

// OAuthTokenResponse represents the response from an OAuth token endpoint
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuthPasswordGrant performs OAuth 2.0 password grant flow
func (oc *OAuthClient) OAuthPasswordGrant(ctx context.Context, tokenURL, secretName, scope string, additionalHeaders map[string]string) (string, error) {
	// Generate cache key
	cacheKey := fmt.Sprintf("%s:%s", tokenURL, secretName)

	// Check cache first
	if cachedToken, found := oc.httpClient.GetCachedOAuthToken(cacheKey); found {
		oc.logger.Debug("using cached OAuth token")
		return cachedToken, nil
	}

	oc.logger.Info("fetching new OAuth token via password grant",
		slog.String("token_url", tokenURL),
		slog.String("secret_name", "[REDACTED]"),
	)

	// Fetch credentials from Secrets Manager
	creds, err := oc.secretsManager.GetOAuthCredentials(ctx, secretName)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve OAuth credentials: %w", err)
	}

	// Prepare form data
	formData := url.Values{
		"grant_type": {"password"},
		"username":   {creds.Username},
		"password":   {creds.Password},
	}

	// Add client credentials if present
	if creds.ClientID != "" {
		formData.Set("client_id", creds.ClientID)
	}
	if creds.ClientSecret != "" {
		formData.Set("client_secret", creds.ClientSecret)
	}

	// Add scope if provided
	if scope != "" {
		formData.Set("scope", scope)
	}

	// Perform token request
	resp, err := oc.httpClient.DoFormPost(ctx, tokenURL, formData, additionalHeaders)
	if err != nil {
		oc.logger.Error("OAuth token request failed",
			slog.String("error", err.Error()),
		)
		return "", fmt.Errorf("OAuth token request failed: %w", err)
	}

	// Parse token response
	var tokenResp OAuthTokenResponse
	if err := json.Unmarshal([]byte(resp.Body), &tokenResp); err != nil {
		oc.logger.Error("failed to parse OAuth token response",
			slog.String("error", err.Error()),
			slog.String("response_body", truncateBody(resp.Body, 200)),
		)
		return "", fmt.Errorf("failed to parse OAuth token response: %w", err)
	}

	// Validate response
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("OAuth response missing access_token")
	}

	// Cache the token (default to 3600 seconds if not specified)
	expiresIn := tokenResp.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600 // 1 hour default
	}
	oc.httpClient.CacheOAuthToken(cacheKey, tokenResp.AccessToken, expiresIn)

	oc.logger.Info("OAuth token acquired successfully",
		slog.String("token_type", tokenResp.TokenType),
		slog.Int("expires_in", expiresIn),
		// SECURITY: Never log the actual token
	)

	return tokenResp.AccessToken, nil
}

// AddBearerToken adds a Bearer token to request headers
func AddBearerToken(headers map[string]string, token string) map[string]string {
	if headers == nil {
		headers = make(map[string]string)
	}
	headers["Authorization"] = fmt.Sprintf("Bearer %s", token)
	return headers
}

// AddAPIKey adds an API key to request headers
func AddAPIKey(headers map[string]string, apiKey, headerName string) map[string]string {
	if headers == nil {
		headers = make(map[string]string)
	}
	if headerName == "" {
		headerName = "X-API-Key"
	}
	headers[headerName] = apiKey
	return headers
}
