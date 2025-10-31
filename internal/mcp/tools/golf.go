package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/secrets"
	"github.com/jrzesz33/rez_agent/internal/webaction"
)

// GolfReservationsTool implements the golf_get_reservations MCP tool
type GolfReservationsTool struct {
	golfHandler *webaction.GolfHandler
	logger      *slog.Logger
}

// NewGolfReservationsTool creates a new golf reservations tool
func NewGolfReservationsTool(httpClient *httpclient.Client, oauthClient *httpclient.OAuthClient,
	secretsManager *secrets.Manager, logger *slog.Logger) *GolfReservationsTool {
	return &GolfReservationsTool{
		golfHandler: webaction.NewGolfHandler(httpClient, oauthClient, secretsManager, logger),
		logger:      logger,
	}
}

// GetDefinition returns the tool's MCP definition
func (t *GolfReservationsTool) GetDefinition() protocol.Tool {
	return protocol.Tool{
		Name:        "golf_get_reservations",
		Description: "Get current golf course reservations for the user",
		InputSchema: protocol.InputSchema{
			Type: "object",
			Properties: map[string]protocol.Property{
				"api_url": {
					Type:        "string",
					Description: "Golf course API URL for fetching reservations",
				},
				"token_url": {
					Type:        "string",
					Description: "OAuth token endpoint URL",
				},
				"jwks_url": {
					Type:        "string",
					Description: "JWKS URL for JWT verification",
				},
				"secret_name": {
					Type:        "string",
					Description: "AWS Secrets Manager secret name for golf credentials",
				},
			},
			Required: []string{"api_url", "token_url", "secret_name"},
		},
	}
}

// ValidateInput validates the tool's input arguments
func (t *GolfReservationsTool) ValidateInput(args map[string]interface{}) error {
	return ValidateInputAgainstSchema(args, t.GetDefinition().InputSchema)
}

// Execute runs the tool with the given arguments
func (t *GolfReservationsTool) Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
	apiURL := GetStringArg(args, "api_url", "")
	tokenURL := GetStringArg(args, "token_url", "")
	jwksURL := GetStringArg(args, "jwks_url", "")
	secretName := GetStringArg(args, "secret_name", "")

	t.logger.Info("fetching golf reservations",
		slog.String("api_url", apiURL),
	)

	// Create web action payload
	payload := &models.WebActionPayload{
		Action: models.WebActionTypeGolf,
		URL:    apiURL,
		AuthConfig: &models.AuthConfig{
			Type:       models.AuthTypeOAuthPassword,
			TokenURL:   tokenURL,
			SecretName: secretName,
			Scope:      "openid profile email",
			JWKSURL:    jwksURL,
		},
		Arguments: map[string]interface{}{
			"operation": "fetch_reservations",
		},
	}

	// Execute golf handler
	results, err := t.golfHandler.Execute(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reservations: %w", err)
	}

	// Convert results to content
	var content []protocol.Content
	for _, result := range results {
		content = append(content, protocol.NewTextContent(result))
	}

	return content, nil
}

// GolfSearchTeeTimesTool implements the golf_search_tee_times MCP tool
type GolfSearchTeeTimesTool struct {
	golfHandler *webaction.GolfHandler
	logger      *slog.Logger
}

// NewGolfSearchTeeTimesTool creates a new golf tee time search tool
func NewGolfSearchTeeTimesTool(httpClient *httpclient.Client, oauthClient *httpclient.OAuthClient,
	secretsManager *secrets.Manager, logger *slog.Logger) *GolfSearchTeeTimesTool {
	return &GolfSearchTeeTimesTool{
		golfHandler: webaction.NewGolfHandler(httpClient, oauthClient, secretsManager, logger),
		logger:      logger,
	}
}

// GetDefinition returns the tool's MCP definition
func (t *GolfSearchTeeTimesTool) GetDefinition() protocol.Tool {
	return protocol.Tool{
		Name:        "golf_search_tee_times",
		Description: "Search for available tee times and optionally book the earliest one",
		InputSchema: protocol.InputSchema{
			Type: "object",
			Properties: map[string]protocol.Property{
				"api_url": {
					Type:        "string",
					Description: "Golf course API URL for searching tee times",
				},
				"token_url": {
					Type:        "string",
					Description: "OAuth token endpoint URL",
				},
				"jwks_url": {
					Type:        "string",
					Description: "JWKS URL for JWT verification",
				},
				"secret_name": {
					Type:        "string",
					Description: "AWS Secrets Manager secret name for golf credentials",
				},
				"date": {
					Type:        "string",
					Format:      "date",
					Description: "Date to search (YYYY-MM-DD)",
				},
				"time_range_start": {
					Type:        "string",
					Description: "Earliest time (HH:MM, 24-hour format)",
				},
				"time_range_end": {
					Type:        "string",
					Description: "Latest time (HH:MM, 24-hour format)",
				},
				"players": {
					Type:        "integer",
					Minimum:     intPtr(1),
					Maximum:     intPtr(4),
					Description: "Number of players",
				},
				"auto_book": {
					Type:        "boolean",
					Default:     false,
					Description: "Automatically book the earliest available time",
				},
			},
			Required: []string{"api_url", "token_url", "secret_name", "date", "players"},
		},
	}
}

// ValidateInput validates the tool's input arguments
func (t *GolfSearchTeeTimesTool) ValidateInput(args map[string]interface{}) error {
	return ValidateInputAgainstSchema(args, t.GetDefinition().InputSchema)
}

// Execute runs the tool with the given arguments
func (t *GolfSearchTeeTimesTool) Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
	apiURL := GetStringArg(args, "api_url", "")
	tokenURL := GetStringArg(args, "token_url", "")
	jwksURL := GetStringArg(args, "jwks_url", "")
	secretName := GetStringArg(args, "secret_name", "")
	date := GetStringArg(args, "date", "")
	timeStart := GetStringArg(args, "time_range_start", "")
	timeEnd := GetStringArg(args, "time_range_end", "")
	players := GetIntArg(args, "players", 1)
	autoBook := GetBoolArg(args, "auto_book", false)

	t.logger.Info("searching for tee times",
		slog.String("date", date),
		slog.Int("players", players),
		slog.Bool("auto_book", autoBook),
	)

	// Create web action payload
	payload := &models.WebActionPayload{
		Action: models.WebActionTypeGolf,
		URL:    apiURL,
		AuthConfig: &models.AuthConfig{
			Type:       models.AuthTypeOAuthPassword,
			TokenURL:   tokenURL,
			SecretName: secretName,
			Scope:      "openid profile email",
			JWKSURL:    jwksURL,
		},
		Arguments: map[string]interface{}{
			"operation":        "search_tee_times",
			"date":             date,
			"time_range_start": timeStart,
			"time_range_end":   timeEnd,
			"players":          players,
			"auto_book":        autoBook,
		},
	}

	// Execute golf handler
	results, err := t.golfHandler.Execute(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to search tee times: %w", err)
	}

	// Convert results to content
	var content []protocol.Content
	for _, result := range results {
		content = append(content, protocol.NewTextContent(result))
	}

	return content, nil
}

// GolfBookTeeTimeTool implements the golf_book_tee_time MCP tool
type GolfBookTeeTimeTool struct {
	golfHandler *webaction.GolfHandler
	logger      *slog.Logger
}

// NewGolfBookTeeTimeTool creates a new golf tee time booking tool
func NewGolfBookTeeTimeTool(httpClient *httpclient.Client, oauthClient *httpclient.OAuthClient,
	secretsManager *secrets.Manager, logger *slog.Logger) *GolfBookTeeTimeTool {
	return &GolfBookTeeTimeTool{
		golfHandler: webaction.NewGolfHandler(httpClient, oauthClient, secretsManager, logger),
		logger:      logger,
	}
}

// GetDefinition returns the tool's MCP definition
func (t *GolfBookTeeTimeTool) GetDefinition() protocol.Tool {
	return protocol.Tool{
		Name:        "golf_book_tee_time",
		Description: "Book a specific tee time at a golf course",
		InputSchema: protocol.InputSchema{
			Type: "object",
			Properties: map[string]protocol.Property{
				"api_url": {
					Type:        "string",
					Description: "Golf course API URL for booking",
				},
				"token_url": {
					Type:        "string",
					Description: "OAuth token endpoint URL",
				},
				"jwks_url": {
					Type:        "string",
					Description: "JWKS URL for JWT verification (required for booking)",
				},
				"secret_name": {
					Type:        "string",
					Description: "AWS Secrets Manager secret name for golf credentials",
				},
				"tee_time_id": {
					Type:        "string",
					Description: "Tee time identifier from search results",
				},
				"date": {
					Type:        "string",
					Format:      "date",
					Description: "Date of the tee time (YYYY-MM-DD)",
				},
				"time": {
					Type:        "string",
					Description: "Time of the tee time (HH:MM, 24-hour format)",
				},
				"players": {
					Type:        "integer",
					Minimum:     intPtr(1),
					Maximum:     intPtr(4),
					Description: "Number of players",
				},
			},
			Required: []string{"api_url", "token_url", "jwks_url", "secret_name", "tee_time_id", "date", "time", "players"},
		},
	}
}

// ValidateInput validates the tool's input arguments
func (t *GolfBookTeeTimeTool) ValidateInput(args map[string]interface{}) error {
	return ValidateInputAgainstSchema(args, t.GetDefinition().InputSchema)
}

// Execute runs the tool with the given arguments
func (t *GolfBookTeeTimeTool) Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
	apiURL := GetStringArg(args, "api_url", "")
	tokenURL := GetStringArg(args, "token_url", "")
	jwksURL := GetStringArg(args, "jwks_url", "")
	secretName := GetStringArg(args, "secret_name", "")
	teeTimeID := GetStringArg(args, "tee_time_id", "")
	date := GetStringArg(args, "date", "")
	time := GetStringArg(args, "time", "")
	players := GetIntArg(args, "players", 1)

	t.logger.Info("booking tee time",
		slog.String("tee_time_id", teeTimeID),
		slog.String("date", date),
		slog.String("time", time),
		slog.Int("players", players),
	)

	// Create web action payload
	payload := &models.WebActionPayload{
		Action: models.WebActionTypeGolf,
		URL:    apiURL,
		AuthConfig: &models.AuthConfig{
			Type:       models.AuthTypeOAuthPassword,
			TokenURL:   tokenURL,
			SecretName: secretName,
			Scope:      "openid profile email",
			JWKSURL:    jwksURL,
		},
		Arguments: map[string]interface{}{
			"operation":   "book_tee_time",
			"tee_time_id": teeTimeID,
			"date":        date,
			"time":        time,
			"players":     players,
		},
	}

	// Execute golf handler
	results, err := t.golfHandler.Execute(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to book tee time: %w", err)
	}

	// Convert results to content
	var content []protocol.Content
	for _, result := range results {
		content = append(content, protocol.NewTextContent(result))
	}

	return content, nil
}
