package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/secrets"
	"github.com/jrzesz33/rez_agent/internal/webaction"
	"github.com/jrzesz33/rez_agent/pkg/courses"
)

// GolfReservationsTool implements the golf_get_reservations MCP tool
type GolfReservationsTool struct {
	golfHandler *webaction.GolfHandler
	logger      *slog.Logger
	stage       string
}

// NewGolfReservationsTool creates a new golf reservations tool
func NewGolfReservationsTool(httpClient *httpclient.Client, oauthClient *httpclient.OAuthClient,
	secretsManager *secrets.Manager, logger *slog.Logger) *GolfReservationsTool {
	stage := os.Getenv("STAGE")
	if stage == "" {
		stage = "dev"
	}
	return &GolfReservationsTool{
		golfHandler: webaction.NewGolfHandler(httpClient, oauthClient, secretsManager, logger),
		logger:      logger,
		stage:       stage,
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
				"course_name": {
					Type:        "string",
					Description: "Name of the golf course (e.g., 'Birdsfoot Golf Course' or 'Totteridge')",
				},
			},
			Required: []string{"course_name"},
		},
	}
}

// ValidateInput validates the tool's input arguments
func (t *GolfReservationsTool) ValidateInput(args map[string]interface{}) error {
	return ValidateInputAgainstSchema(args, t.GetDefinition().InputSchema)
}

// Execute runs the tool with the given arguments
func (t *GolfReservationsTool) Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
	courseName := GetStringArg(args, "course_name", "")

	t.logger.Info("fetching golf reservations", slog.String("course_name", courseName))

	// Load course configuration
	course, err := courses.GetCourseByName(courseName)
	if err != nil {
		return nil, fmt.Errorf("failed to find course: %w", err)
	}

	secretName := course.GetSecretName(t.stage)

	// Create web action payload
	payload := &models.WebActionPayload{
		Version:  "1.0",
		Action:   models.WebActionTypeGolf,
		CourseID: course.CourseID,
		AuthConfig: &models.AuthConfig{
			Type:       models.AuthTypeOAuthPassword,
			SecretName: secretName,
		},
	}

	argsOut := make(map[string]interface{})
	argsOut["operation"] = "fetch_reservations"

	// Execute golf handler
	results, err := t.golfHandler.Execute(ctx, argsOut, payload)
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
	stage       string
}

// NewGolfSearchTeeTimesTool creates a new golf tee time search tool
func NewGolfSearchTeeTimesTool(httpClient *httpclient.Client, oauthClient *httpclient.OAuthClient,
	secretsManager *secrets.Manager, logger *slog.Logger) *GolfSearchTeeTimesTool {
	stage := os.Getenv("STAGE")
	if stage == "" {
		stage = "dev"
	}
	return &GolfSearchTeeTimesTool{
		golfHandler: webaction.NewGolfHandler(httpClient, oauthClient, secretsManager, logger),
		logger:      logger,
		stage:       stage,
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
				"course_name": {
					Type:        "string",
					Description: "Name of the golf course (e.g., 'Birdsfoot Golf Course' or 'Totteridge')",
				},
				"start_time": {
					Type:        "string",
					Description: "Start datetime in ISO format (YYYY-MM-DDTHH:MM:SS)",
				},
				"end_time": {
					Type:        "string",
					Description: "End datetime in ISO format (YYYY-MM-DDTHH:MM:SS)",
				},
				"num_players": {
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
			Required: []string{"course_name", "start_time", "end_time", "num_players"},
		},
	}
}

// ValidateInput validates the tool's input arguments
func (t *GolfSearchTeeTimesTool) ValidateInput(args map[string]interface{}) error {
	return ValidateInputAgainstSchema(args, t.GetDefinition().InputSchema)
}

// Execute runs the tool with the given arguments
func (t *GolfSearchTeeTimesTool) Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
	courseName := GetStringArg(args, "course_name", "")
	startTime := GetStringArg(args, "start_time", "")
	endTime := GetStringArg(args, "end_time", "")
	numPlayers := GetIntArg(args, "num_players", 1)
	autoBook := GetBoolArg(args, "auto_book", false)

	t.logger.Info("searching for tee times",
		slog.String("course_name", courseName),
		slog.String("start_time", startTime),
		slog.Int("num_players", numPlayers),
		slog.Bool("auto_book", autoBook),
	)

	// Load course configuration
	course, err := courses.GetCourseByName(courseName)
	if err != nil {
		return nil, fmt.Errorf("failed to find course: %w", err)
	}

	secretName := course.GetSecretName(t.stage)

	t.logger.Info("using course configuration",
		slog.String("name", course.Name),
	)

	// Create web action payload
	payload := &models.WebActionPayload{
		Action:   models.WebActionTypeGolf,
		CourseID: course.CourseID,
		AuthConfig: &models.AuthConfig{
			Type:       models.AuthTypeOAuthPassword,
			SecretName: secretName,
		},

		StartSearchTime: startTime,
		EndSearchTime:   endTime,
		NumberOfPlayers: numPlayers,
		AutoBook:        autoBook,
	}

	_args := make(map[string]interface{})
	_args["operation"] = "search_tee_times"

	// Execute golf handler
	results, err := t.golfHandler.Execute(ctx, _args, payload)
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
	stage       string
}

// NewGolfBookTeeTimeTool creates a new golf tee time booking tool
func NewGolfBookTeeTimeTool(httpClient *httpclient.Client, oauthClient *httpclient.OAuthClient,
	secretsManager *secrets.Manager, logger *slog.Logger) *GolfBookTeeTimeTool {
	stage := os.Getenv("STAGE")
	if stage == "" {
		stage = "dev"
	}
	return &GolfBookTeeTimeTool{
		golfHandler: webaction.NewGolfHandler(httpClient, oauthClient, secretsManager, logger),
		logger:      logger,
		stage:       stage,
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
				"course_name": {
					Type:        "string",
					Description: "Name of the golf course (e.g., 'Birdsfoot Golf Course' or 'Totteridge')",
				},
				"tee_sheet_id": {
					Type:        "integer",
					Description: "The tee sheet ID from search results",
				},
			},
			Required: []string{"course_name", "tee_sheet_id"},
		},
	}
}

// ValidateInput validates the tool's input arguments
func (t *GolfBookTeeTimeTool) ValidateInput(args map[string]interface{}) error {
	return ValidateInputAgainstSchema(args, t.GetDefinition().InputSchema)
}

// Execute runs the tool with the given arguments
func (t *GolfBookTeeTimeTool) Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
	courseName := GetStringArg(args, "course_name", "")
	teeSheetID := GetIntArg(args, "tee_sheet_id", 0)

	t.logger.Info("booking tee time",
		slog.String("course_name", courseName),
		slog.Int("tee_sheet_id", teeSheetID),
	)

	// Load course configuration
	course, err := courses.GetCourseByName(courseName)
	if err != nil {
		return nil, fmt.Errorf("failed to find course: %w", err)
	}

	secretName := course.GetSecretName(t.stage)

	// Create web action payload
	payload := &models.WebActionPayload{
		Action:   models.WebActionTypeGolf,
		CourseID: course.CourseID,
		AuthConfig: &models.AuthConfig{
			Type:       models.AuthTypeOAuthPassword,
			SecretName: secretName,
		},
		TeeSheetID: teeSheetID,
	}
	_args := make(map[string]interface{})
	_args["operation"] = "book_tee_time"

	// Execute golf handler
	results, err := t.golfHandler.Execute(ctx, _args, payload)
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
