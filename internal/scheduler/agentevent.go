package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
	"github.com/jrzesz33/rez_agent/internal/secrets"
	"github.com/jrzesz33/rez_agent/pkg/courses"
)

// AgentEventHandler handles scheduled agent operations using AWS Bedrock and MCP tools
type AgentEventHandler interface {
	// ExecuteScheduledEvent processes a scheduled agent event with multi-step workflow
	ExecuteScheduledEvent(ctx context.Context, event *ScheduledAgentEvent) error
}

// ScheduledAgentEvent represents an event from EventBridge Scheduler
type ScheduledAgentEvent struct {
	// ScheduleID is the unique identifier for the schedule
	ScheduleID string `json:"schedule_id"`

	// UserPrompt is the instruction from the schedule (e.g., "Book earliest tee times at 9:30 AM...")
	UserPrompt string `json:"user_prompt"`

	// CourseName is the golf course name (e.g., "Birdsfoot")
	CourseName string `json:"course_name,omitempty"`

	// NumPlayers is the number of players (default: 1)
	NumPlayers int `json:"num_players,omitempty"`

	// TriggeredAt is when the event was triggered
	TriggeredAt time.Time `json:"triggered_at"`
}

// AWSAgentEventHandler implements AgentEventHandler using AWS Bedrock
type AWSAgentEventHandler struct {
	httpClient     *httpclient.Client
	secretsManager *secrets.Manager
	mcpServerURL   string
	stage          string
	logger         *slog.Logger
	maxRetries     int
	retryDelay     time.Duration
}

// NewAWSAgentEventHandler creates a new AWS-based agent event handler
func NewAWSAgentEventHandler(
	httpClient *httpclient.Client,
	secretsManager *secrets.Manager,
	logger *slog.Logger,
) *AWSAgentEventHandler {
	mcpURL := os.Getenv("MCP_SERVER_URL")
	if mcpURL == "" {
		mcpURL = "http://localhost:8080/mcp"
	}

	stage := os.Getenv("STAGE")
	if stage == "" {
		stage = "dev"
	}

	return &AWSAgentEventHandler{
		httpClient:     httpClient,
		secretsManager: secretsManager,
		mcpServerURL:   mcpURL,
		stage:          stage,
		logger:         logger,
		maxRetries:     3,
		retryDelay:     5 * time.Second,
	}
}

// ExecuteScheduledEvent processes a scheduled agent event
func (h *AWSAgentEventHandler) ExecuteScheduledEvent(ctx context.Context, event *ScheduledAgentEvent) error {
	h.logger.InfoContext(ctx, "starting scheduled agent event execution",
		slog.String("schedule_id", event.ScheduleID),
		slog.String("user_prompt", event.UserPrompt),
	)

	// Validate event
	if err := h.validateEvent(event); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	// Execute with retry logic
	var lastErr error
	for attempt := 1; attempt <= h.maxRetries; attempt++ {
		h.logger.InfoContext(ctx, "attempting agent execution",
			slog.Int("attempt", attempt),
			slog.Int("max_retries", h.maxRetries),
		)

		err := h.executeWithContext(ctx, event)
		if err == nil {
			h.logger.InfoContext(ctx, "agent execution completed successfully",
				slog.String("schedule_id", event.ScheduleID),
			)
			return nil
		}

		lastErr = err
		h.logger.WarnContext(ctx, "agent execution attempt failed",
			slog.Int("attempt", attempt),
			slog.String("error", err.Error()),
		)

		// Don't retry on validation or configuration errors
		if isNonRetryableError(err) {
			h.logger.ErrorContext(ctx, "non-retryable error encountered",
				slog.String("error", err.Error()),
			)
			return err
		}

		// Wait before retry (except on last attempt)
		if attempt < h.maxRetries {
			h.logger.InfoContext(ctx, "waiting before retry",
				slog.Duration("delay", h.retryDelay),
			)
			time.Sleep(h.retryDelay)
		}
	}

	h.logger.ErrorContext(ctx, "agent execution failed after all retries",
		slog.Int("attempts", h.maxRetries),
		slog.String("error", lastErr.Error()),
	)

	return fmt.Errorf("failed after %d retries: %w", h.maxRetries, lastErr)
}

// executeWithContext performs the actual agent execution
func (h *AWSAgentEventHandler) executeWithContext(ctx context.Context, event *ScheduledAgentEvent) error {
	// Step 1: Fetch existing reservations
	h.logger.InfoContext(ctx, "fetching existing reservations")
	reservations, err := h.fetchReservations(ctx, event.CourseName)
	if err != nil {
		return fmt.Errorf("failed to fetch reservations: %w", err)
	}

	// Step 2: Get weather forecast
	h.logger.InfoContext(ctx, "fetching weather forecast")
	weather, err := h.getWeather(ctx, event.CourseName)
	if err != nil {
		// Weather might not be available far in advance, log but don't fail
		h.logger.WarnContext(ctx, "weather forecast not available",
			slog.String("error", err.Error()),
		)
		weather = "Weather forecast not available for this date range."
	}

	// Step 3: Load MCP tools
	h.logger.InfoContext(ctx, "loading MCP tools")
	tools, err := h.getMCPTools(ctx)
	if err != nil {
		return fmt.Errorf("failed to load MCP tools: %w", err)
	}

	// Step 4: Construct system message with context
	systemMessage := h.constructSystemMessage(event, reservations, weather)

	h.logger.InfoContext(ctx, "system message constructed",
		slog.Int("system_message_length", len(systemMessage)),
		slog.Int("available_tools", len(tools)),
	)

	// Step 5: Execute multi-step conversation with Bedrock
	result, err := h.executeAgentConversation(ctx, systemMessage, event.UserPrompt, tools)
	if err != nil {
		return fmt.Errorf("agent conversation failed: %w", err)
	}

	// Step 6: Send notification with results
	h.logger.InfoContext(ctx, "sending notification with results")
	if err := h.sendNotification(ctx, result); err != nil {
		// Log but don't fail the entire operation if notification fails
		h.logger.WarnContext(ctx, "failed to send notification",
			slog.String("error", err.Error()),
		)
	}

	// Step 7: Check for inclement weather on existing reservations
	if err := h.checkWeatherForReservations(ctx, reservations, event.CourseName); err != nil {
		h.logger.WarnContext(ctx, "failed to check weather for existing reservations",
			slog.String("error", err.Error()),
		)
	}

	h.logger.InfoContext(ctx, "agent event execution completed successfully")
	return nil
}

// validateEvent validates the scheduled agent event
func (h *AWSAgentEventHandler) validateEvent(event *ScheduledAgentEvent) error {
	if event.ScheduleID == "" {
		return fmt.Errorf("schedule_id is required")
	}
	if event.UserPrompt == "" {
		return fmt.Errorf("user_prompt is required")
	}
	if event.CourseName == "" {
		// Try to extract from user prompt
		if strings.Contains(strings.ToLower(event.UserPrompt), "birdsfoot") {
			event.CourseName = "Birdsfoot Golf Course"
		} else if strings.Contains(strings.ToLower(event.UserPrompt), "totteridge") {
			event.CourseName = "Totteridge"
		} else {
			return fmt.Errorf("course_name is required or must be in user_prompt")
		}
	}
	if event.NumPlayers <= 0 {
		event.NumPlayers = 1 // Default to 1 player
	}
	if event.NumPlayers > 4 {
		return fmt.Errorf("num_players must be between 1 and 4")
	}
	return nil
}

// fetchReservations fetches existing golf reservations via MCP
func (h *AWSAgentEventHandler) fetchReservations(ctx context.Context, courseName string) (string, error) {
	// Call MCP tool golf_get_reservations
	req := protocol.ToolCallRequest{
		Name: "golf_get_reservations",
		Arguments: map[string]interface{}{
			"course_name": courseName,
		},
	}

	result, err := h.callMCPTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("MCP tool call failed: %w", err)
	}

	// Extract text content from result
	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}

	return "No reservations found.", nil
}

// getWeather fetches weather forecast for the golf course location
func (h *AWSAgentEventHandler) getWeather(ctx context.Context, courseName string) (string, error) {
	// Get course configuration to find weather URL
	course, err := courses.GetCourseByName(courseName)
	if err != nil {
		return "", fmt.Errorf("course not found: %w", err)
	}

	// Hard-code weather URL for Birdsfoot (weather.gov API)
	weatherURL := "https://api.weather.gov/gridpoints/TOP/31,80/forecast"
	if course.CourseID == 2 { // Totteridge
		weatherURL = "https://api.weather.gov/gridpoints/TOP/31,80/forecast" // TODO: Update with correct coordinates
	}

	// Call MCP tool get_weather
	req := protocol.ToolCallRequest{
		Name: "get_weather",
		Arguments: map[string]interface{}{
			"location": weatherURL,
			"days":     7, // Get 7-day forecast for advance booking
		},
	}

	result, err := h.callMCPTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("MCP tool call failed: %w", err)
	}

	// Extract text content from result
	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}

	return "", fmt.Errorf("no weather data in response")
}

// getMCPTools loads available tools from the MCP server
func (h *AWSAgentEventHandler) getMCPTools(ctx context.Context) ([]protocol.Tool, error) {
	// First initialize the MCP connection
	initReq := map[string]interface{}{
		"method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "rez-agent-scheduler",
				"version": "1.0.0",
			},
		},
	}

	if err := h.callMCPMethod(ctx, initReq, nil); err != nil {
		return nil, fmt.Errorf("MCP initialize failed: %w", err)
	}

	// List available tools
	listReq := map[string]interface{}{
		"method": "tools/list",
		"params": map[string]interface{}{},
	}

	var listResp protocol.ToolsListResult
	if err := h.callMCPMethod(ctx, listReq, &listResp); err != nil {
		return nil, fmt.Errorf("MCP tools/list failed: %w", err)
	}

	h.logger.InfoContext(ctx, "MCP tools loaded",
		slog.Int("tool_count", len(listResp.Tools)),
	)

	return listResp.Tools, nil
}

// constructSystemMessage builds the system prompt with context
func (h *AWSAgentEventHandler) constructSystemMessage(event *ScheduledAgentEvent, reservations, weather string) string {
	currentDate := time.Now().Format("Monday, January 2, 2006")

	return fmt.Sprintf(`You are an AI assistant that helps with golf tee time bookings. You have been given a scheduled task to complete autonomously.

CURRENT DATE: %s

EXISTING RESERVATIONS:
%s

WEATHER FORECAST:
%s

IMPORTANT INSTRUCTIONS:
1. Check the existing reservations above to avoid booking conflicts
2. Consider the weather forecast - DO NOT book tee times if there is inclement weather (rain, storms, severe conditions)
3. If the user hasn't specified the number of players, use %d player(s)
4. You should AUTO-BOOK without asking for confirmation - this is a scheduled autonomous task
5. After booking (or if booking fails), send a push notification with the result
6. Be specific about what you booked (date, time, course, confirmation number)
7. If weather is too far in advance and unavailable, you may proceed with booking but mention this in the notification

AVAILABLE TOOLS:
- golf_search_tee_times: Search for available tee times
- golf_book_tee_time: Book a specific tee time
- golf_get_reservations: Get existing reservations (already called)
- get_weather: Get weather forecast (already called)
- send_notification: Send push notification to user

Now complete this task:`, currentDate, reservations, weather, event.NumPlayers)
}

// executeAgentConversation runs the multi-step conversation loop with Bedrock
func (h *AWSAgentEventHandler) executeAgentConversation(ctx context.Context, systemMsg, userPrompt string, tools []protocol.Tool) (string, error) {
	// TODO: Implement Bedrock conversation loop
	// This will use AWS Bedrock's Converse API with tool support:
	// 1. Create BedrockRuntimeClient from aws-sdk-go-v2/service/bedrockruntime
	// 2. Convert MCP tools to Bedrock tool definitions
	// 3. Call Converse API with system message, user prompt, and tools
	// 4. Loop through tool calls:
	//    - When Bedrock requests a tool, call it via MCP
	//    - Feed tool results back to Bedrock
	//    - Continue until Bedrock stops requesting tools
	// 5. Return final assistant message
	//
	// Model: anthropic.claude-3-5-sonnet-20241022-v2:0
	//
	// For now, return a placeholder to allow compilation

	h.logger.InfoContext(ctx, "executing agent conversation",
		slog.String("model", "anthropic.claude-3-5-sonnet-20241022-v2:0"),
		slog.Int("tools_available", len(tools)),
	)

	// Placeholder for actual Bedrock implementation
	return fmt.Sprintf("Agent conversation completed for: %s", userPrompt), nil
}

// callMCPTool calls an MCP tool and returns the result
func (h *AWSAgentEventHandler) callMCPTool(ctx context.Context, req protocol.ToolCallRequest) (*protocol.ToolCallResult, error) {
	var result protocol.ToolCallResult

	reqMap := map[string]interface{}{
		"method": "tools/call",
		"params": req,
	}

	if err := h.callMCPMethod(ctx, reqMap, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// callMCPMethod makes an HTTP call to the MCP server
func (h *AWSAgentEventHandler) callMCPMethod(ctx context.Context, reqData map[string]interface{}, respData interface{}) error {
	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.mcpServerURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("MCP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MCP server returned status %d", resp.StatusCode)
	}

	if respData != nil {
		if err := json.NewDecoder(resp.Body).Decode(respData); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// sendNotification sends a push notification with the booking result
func (h *AWSAgentEventHandler) sendNotification(ctx context.Context, message string) error {
	// Call MCP tool send_notification
	req := protocol.ToolCallRequest{
		Name: "send_notification",
		Arguments: map[string]interface{}{
			"title":    "Golf Booking Result",
			"message":  message,
			"priority": 4,
		},
	}

	_, err := h.callMCPTool(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}

	h.logger.InfoContext(ctx, "notification sent successfully")
	return nil
}

// checkWeatherForReservations checks weather for existing reservations and warns if bad
func (h *AWSAgentEventHandler) checkWeatherForReservations(ctx context.Context, reservations, courseName string) error {
	// Parse reservations to find dates
	// For each date, check weather
	// If bad weather found, send warning notification

	// This is a simplified implementation - would need proper date parsing
	if strings.Contains(strings.ToLower(reservations), "no reservations") {
		return nil // No reservations to check
	}

	// Get weather again (already have it from earlier, but API is fast)
	weather, err := h.getWeather(ctx, courseName)
	if err != nil {
		return err
	}

	// Check for bad weather keywords
	hasInclementWeather := strings.Contains(strings.ToLower(weather), "rain") ||
		strings.Contains(strings.ToLower(weather), "storm") ||
		strings.Contains(strings.ToLower(weather), "thunder")

	if hasInclementWeather {
		warning := fmt.Sprintf("⚠️ Weather Alert: Your existing golf reservations may have inclement weather. Please review:\n\n%s", weather)
		h.logger.InfoContext(ctx, "sending weather warning for existing reservations")
		return h.sendNotification(ctx, warning)
	}

	return nil
}

// isNonRetryableError checks if an error should not be retried
func isNonRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// Configuration errors
	if strings.Contains(errMsg, "invalid") ||
		strings.Contains(errMsg, "not found") ||
		strings.Contains(errMsg, "required") {
		return true
	}

	// Authentication errors
	if strings.Contains(errMsg, "unauthorized") ||
		strings.Contains(errMsg, "forbidden") ||
		strings.Contains(errMsg, "authentication") {
		return true
	}

	return false
}
