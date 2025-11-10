package models

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// WebActionType represents the type of web action to perform
type WebActionType string

const (
	// WebActionTypeWeather fetches weather forecast data
	WebActionTypeWeather WebActionType = "weather"
	// WebActionTypeGolf fetches golf reservation data
	WebActionTypeGolf WebActionType = "golf"
)

// IsValid checks if the web action type value is valid
func (wat WebActionType) IsValid() bool {
	switch wat {
	case WebActionTypeWeather, WebActionTypeGolf:
		return true
	default:
		return false
	}
}

// String returns the string representation of the web action type
func (wat WebActionType) String() string {
	return string(wat)
}

// AuthType represents the authentication method for HTTP requests
type AuthType string

const (
	// AuthTypeNone indicates no authentication required
	AuthTypeNone AuthType = "none"
	// AuthTypeOAuthPassword indicates OAuth 2.0 password grant
	AuthTypeOAuthPassword AuthType = "oauth_password"
	// AuthTypeAPIKey indicates API key authentication
	AuthTypeAPIKey AuthType = "api_key"
	// AuthTypeBearer indicates bearer token authentication
	AuthTypeBearer AuthType = "bearer"
)

// IsValid checks if the auth type value is valid
func (at AuthType) IsValid() bool {
	switch at {
	case AuthTypeNone, AuthTypeOAuthPassword, AuthTypeAPIKey, AuthTypeBearer:
		return true
	default:
		return false
	}
}

// String returns the string representation of the auth type
func (at AuthType) String() string {
	return string(at)
}

// AuthConfig contains authentication configuration for web actions
type AuthConfig struct {
	// Type is the authentication method to use
	Type AuthType `json:"type" dynamodbav:"type"`

	// SecretName is the AWS Secrets Manager secret name (if applicable)
	SecretName string `json:"secret_name,omitempty" dynamodbav:"secret_name,omitempty"`

	// TokenURL is the OAuth 2.0 token endpoint (for OAuth flows)
	TokenURL string `json:"token_url,omitempty" dynamodbav:"token_url,omitempty"`

	// JWKSURL is the JWKS endpoint for JWT verification (for OAuth flows)
	JWKSURL string `json:"jwks_url,omitempty" dynamodbav:"jwks_url,omitempty"`

	// Scope is the OAuth 2.0 scope (for OAuth flows)
	Scope string `json:"scope,omitempty" dynamodbav:"scope,omitempty"`

	// Headers contains additional HTTP headers for authentication
	Headers map[string]string `json:"headers,omitempty" dynamodbav:"headers,omitempty"`
}

// WebActionPayload represents the configuration for a web action request
type WebActionPayload struct {
	// Version is the payload schema version
	Version string `json:"version" dynamodbav:"version"`

	// URL is the target API endpoint
	URL string `json:"url" dynamodbav:"url"`

	// Action is the action type identifier
	Action WebActionType `json:"action" dynamodbav:"action"`

	//Start Search Time for golf tee time search
	StartSearchTime string `json:"startSearchTime,omitempty" dynamodbav:"startSearchTime,omitempty"`

	//End Search Time for golf tee time search
	EndSearchTime string `json:"endSearchTime,omitempty" dynamodbav:"endSearchTime,omitempty"`
	// AutoBook indicates whether to auto-book available tee times
	AutoBook bool `json:"autoBook,omitempty" dynamodbav:"autoBook,omitempty"`

	// CourseID is the identifier for the golf course
	CourseID int `json:"courseID,omitempty" dynamodbav:"courseID,omitempty"`

	//MaxResults limits the number of results returned
	MaxResults int `json:"maxResults,omitempty" dynamodbav:"maxResults,omitempty"`

	//Days number of days for search
	Days int `json:"days,omitempty" dynamodbav:"days,omitempty"`

	// Number of Players for golf tee time search
	NumberOfPlayers int `json:"numberOfPlayers,omitempty" dynamodbav:"numberOfPlayers,omitempty"`

	// teeSheetId is the identifier for the golf tee sheet
	TeeSheetID int `json:"teeSheetID,omitempty" dynamodbav:"teeSheetID,omitempty"`

	// AuthConfig contains authentication configuration
	AuthConfig *AuthConfig `json:"auth_config,omitempty" dynamodbav:"auth_config,omitempty"`
}

// AllowedHosts defines the whitelist of allowed hostnames for SSRF prevention
var AllowedHosts = map[string]bool{
	"api.weather.gov":    true,
	"birdsfoot.cps.golf": true,
}

func (p *WebActionPayload) ToJSONString() (string, error) {
	// Serialize web action to JSON
	webActionJSON, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("failed to serialize web action... %v", err)
	}
	return string(webActionJSON), nil

}

// Validate performs comprehensive validation of the payload including SSRF prevention
func (p *WebActionPayload) Validate() error {
	// Validate version
	if p.Version != "1.0" {
		return fmt.Errorf("unsupported payload version: %s", p.Version)
	}

	// Validate action type
	if !p.Action.IsValid() {
		return fmt.Errorf("invalid action type: %s", p.Action)
	}

	// SECURITY: SSRF Prevention - Comprehensive URL validation
	if err := p.validateURL(); err != nil {
		return fmt.Errorf("URL validation failed: %w", err)
	}

	// Validate authentication configuration
	if p.AuthConfig != nil {
		if err := p.AuthConfig.Validate(); err != nil {
			return fmt.Errorf("auth config validation failed: %w", err)
		}
	}

	return nil
}

// validateURL performs comprehensive URL validation to prevent SSRF attacks
func (p *WebActionPayload) validateURL() error {
	if p.URL == "" {
		return fmt.Errorf("URL is required")
	}

	// Parse URL
	parsedURL, err := url.Parse(p.URL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// SECURITY: Enforce HTTPS only (except for localhost in dev)
	if parsedURL.Scheme != "https" && parsedURL.Host != "localhost" {
		return fmt.Errorf("only HTTPS URLs are allowed, got: %s", parsedURL.Scheme)
	}

	// SECURITY: Check host against allowlist
	host := parsedURL.Hostname()
	if !AllowedHosts[host] {
		return fmt.Errorf("host not in allowlist: %s", host)
	}

	// SECURITY: Prevent IP address usage (force DNS)
	if isIPAddress(host) {
		return fmt.Errorf("IP addresses are not allowed, use hostname instead: %s", host)
	}

	// SECURITY: Resolve hostname and check for private IPs
	if err := validateHostname(host); err != nil {
		return fmt.Errorf("hostname validation failed: %w", err)
	}

	return nil
}

// isIPAddress checks if the string is an IP address
func isIPAddress(host string) bool {
	return net.ParseIP(host) != nil
}

// validateHostname checks if a hostname resolves to private IP ranges (SSRF protection)
func validateHostname(hostname string) error {
	// Resolve hostname to IP addresses
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("hostname resolution failed: %w", err)
	}

	// Check all resolved IPs
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("hostname resolves to private IP: %s -> %s", hostname, ip.String())
		}
		if isAWSMetadataIP(ip) {
			return fmt.Errorf("hostname resolves to AWS metadata service IP: %s -> %s", hostname, ip.String())
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is in a private range
func isPrivateIP(ip net.IP) bool {
	// RFC 1918 private ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",    // Loopback
		"169.254.0.0/16", // Link-local
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
		"::1/128",        // IPv6 loopback
	}

	for _, rangeStr := range privateRanges {
		_, subnet, err := net.ParseCIDR(rangeStr)
		if err != nil {
			continue
		}
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// isAWSMetadataIP checks if an IP is the AWS metadata service
func isAWSMetadataIP(ip net.IP) bool {
	// AWS metadata service IPs
	metadataIPs := []string{
		"169.254.169.254",
		"fd00:ec2::254",
	}

	ipStr := ip.String()
	for _, metaIP := range metadataIPs {
		if ipStr == metaIP {
			return true
		}
	}

	return false
}

// Validate validates the authentication configuration
func (ac *AuthConfig) Validate() error {
	if !ac.Type.IsValid() {
		return fmt.Errorf("invalid auth type: %s", ac.Type)
	}

	// Validate OAuth configuration
	if ac.Type == AuthTypeOAuthPassword {
		if ac.SecretName == "" {
			return fmt.Errorf("secret_name is required for OAuth authentication")
		}
		if ac.TokenURL == "" {
			return fmt.Errorf("token_url is required for OAuth authentication")
		}
	}

	// Validate token URL if present
	if ac.TokenURL != "" {
		parsedURL, err := url.Parse(ac.TokenURL)
		if err != nil {
			return fmt.Errorf("invalid token_url format: %w", err)
		}
		if parsedURL.Scheme != "https" {
			return fmt.Errorf("token_url must use HTTPS: %s", ac.TokenURL)
		}
	}

	return nil
}

// WebActionResult represents the outcome of a web action execution
type WebActionResult struct {
	// ID is the unique identifier for the result (same as message ID)
	ID string `json:"id" dynamodbav:"id"`

	// MessageID is the ID of the message that triggered this action
	MessageID string `json:"message_id" dynamodbav:"message_id"`

	// Action is the action type that was executed
	Action WebActionType `json:"action" dynamodbav:"action"`

	// URL is the URL that was accessed
	URL string `json:"url" dynamodbav:"url"`

	// Status indicates success or failure
	Status Status `json:"status" dynamodbav:"status"`

	// ResponseCode is the HTTP response code
	ResponseCode int `json:"response_code,omitempty" dynamodbav:"response_code,omitempty"`

	// ResponseBody is the HTTP response body (truncated for storage)
	ResponseBody string `json:"response_body,omitempty" dynamodbav:"response_body,omitempty"`

	// ErrorMessage contains error details if Status is Failed
	ErrorMessage string `json:"error_message,omitempty" dynamodbav:"error_message,omitempty"`

	// ExecutionTime is the duration of the request in milliseconds
	ExecutionTimeMs int64 `json:"execution_time_ms" dynamodbav:"execution_time_ms"`

	// CreatedDate is when the result was created
	CreatedDate time.Time `json:"created_date" dynamodbav:"created_date"`

	// TTL is the Unix timestamp when this record should be deleted (3 days)
	TTL int64 `json:"ttl" dynamodbav:"ttl"`

	// Stage is the environment
	Stage Stage `json:"stage" dynamodbav:"stage"`
}

// NewWebActionResult creates a new web action result with TTL set to 3 days
func NewWebActionResult(messageID string, action WebActionType, url string, stage Stage) *WebActionResult {
	now := time.Now().UTC()
	threeDaysLater := now.Add(72 * time.Hour)

	return &WebActionResult{
		ID:          generateResultID(now),
		MessageID:   messageID,
		Action:      action,
		URL:         url,
		Status:      StatusProcessing,
		CreatedDate: now,
		TTL:         threeDaysLater.Unix(),
		Stage:       stage,
	}
}

// generateResultID generates a unique result ID
func generateResultID(t time.Time) string {
	return "result_" + t.Format("20060102150405") + "_" + fmt.Sprintf("%d", t.Nanosecond()%1000000)
}

// MarkSuccess marks the result as successful
func (r *WebActionResult) MarkSuccess(responseCode int, responseBody string, executionMs int64) {
	r.Status = StatusCompleted
	r.ResponseCode = responseCode
	r.ResponseBody = truncateResponseBody(responseBody)
	r.ExecutionTimeMs = executionMs
}

// MarkFailure marks the result as failed
func (r *WebActionResult) MarkFailure(errorMessage string, executionMs int64) {
	r.Status = StatusFailed
	r.ErrorMessage = errorMessage
	r.ExecutionTimeMs = executionMs
}

// truncateResponseBody limits response body size for storage (max 50KB)
func truncateResponseBody(body string) string {
	const maxSize = 50 * 1024 // 50KB
	if len(body) <= maxSize {
		return body
	}
	return body[:maxSize] + "... [TRUNCATED]"
}

// ParseWebActionPayload parses a JSON string into a WebActionPayload
func ParseWebActionPayload(payloadJSON map[string]interface{}) (*WebActionPayload, error) {
	jsonBytes, err := json.Marshal(payloadJSON)
	if err != nil {
		fmt.Println("Error marshaling map:", err)
		return nil, fmt.Errorf("failed to parse web action payload: %w", err)
	}
	var payload WebActionPayload
	if err := json.Unmarshal(jsonBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse web action payload: %w", err)
	}

	// Validate the parsed payload
	if err := payload.Validate(); err != nil {
		return nil, fmt.Errorf("payload validation failed: %w", err)
	}

	return &payload, nil
}

// ToJSON converts the payload to a JSON string
func (p *WebActionPayload) ToJSON() (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("failed to marshal web action payload: %w", err)
	}
	return string(data), nil
}

// RedactSensitiveData returns a copy of the payload with sensitive data redacted for logging
func (p *WebActionPayload) RedactSensitiveData() *WebActionPayload {
	redacted := *p

	// Redact auth config
	if redacted.AuthConfig != nil {
		redactedAuth := *redacted.AuthConfig
		if redactedAuth.SecretName != "" {
			redactedAuth.SecretName = "[REDACTED]"
		}
		if redactedAuth.Headers != nil {
			redactedHeaders := make(map[string]string)
			for k, v := range redactedAuth.Headers {
				lowerKey := strings.ToLower(k)
				if strings.Contains(lowerKey, "auth") ||
					strings.Contains(lowerKey, "token") ||
					strings.Contains(lowerKey, "key") ||
					strings.Contains(lowerKey, "secret") {
					redactedHeaders[k] = "[REDACTED]"
				} else {
					redactedHeaders[k] = v
				}
			}
			redactedAuth.Headers = redactedHeaders
		}
		redacted.AuthConfig = &redactedAuth
	}

	return &redacted
}
