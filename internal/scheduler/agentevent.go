package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
	"github.com/jrzesz33/rez_agent/internal/models"
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

	// AuthConfig contains authentication configuration
	AuthConfig *models.AuthConfig `json:"auth_config,omitempty" dynamodbav:"auth_config,omitempty"`
}

// AWSAgentEventHandler implements AgentEventHandler using AWS Bedrock
type AWSAgentEventHandler struct {
	bedrockClient        *bedrockruntime.Client
	httpClient           *httpclient.Client
	secretsManager       *secrets.Manager
	mcpServerURL         string
	stage                string
	logger               *slog.Logger
	maxRetries           int
	retryDelay           time.Duration
	modelID              string
	defaultToolArguments map[string]interface{}
}

// NewAWSAgentEventHandler creates a new AWS-based agent event handler
func NewAWSAgentEventHandler(
	bedrockClient *bedrockruntime.Client,
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

	// Get model ID from environment variable, with fallback to latest Claude model
	modelID := os.Getenv("BEDROCK_MODEL_ID")
	if modelID == "" {
		modelID = "anthropic.claude-3-5-sonnet-20241022-v2:0"
	}

	return &AWSAgentEventHandler{
		bedrockClient:  bedrockClient,
		httpClient:     httpClient,
		secretsManager: secretsManager,
		mcpServerURL:   mcpURL,
		stage:          stage,
		logger:         logger,
		maxRetries:     3,
		retryDelay:     5 * time.Second,
		modelID:        modelID,
	}
}

// ExecuteScheduledEvent processes a scheduled agent event
func (h *AWSAgentEventHandler) ExecuteScheduledEvent(ctx context.Context, event *ScheduledAgentEvent) error {

	// Set default tool arguments
	defToolArgs := make(map[string]interface{})
	defToolArgs["course_name"] = event.CourseName
	h.defaultToolArguments = defToolArgs

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

	// Step 3: Load MCP tools
	h.logger.InfoContext(ctx, "loading MCP tools")
	tools, err := h.getMCPTools(ctx)
	if err != nil {
		return fmt.Errorf("failed to load MCP tools: %w", err)
	}

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
	fmt.Println(result)
	/*/ Step 6: Send notification with results
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
	*/
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

	// Get weather URL from course configuration
	weatherURL, err := course.GetActionURL("get-weather")
	if err != nil {
		return "", fmt.Errorf("failed to get weather URL for course %s: %w", courseName, err)
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
		"jsonrpc": "2.0",
		"method":  "initialize",
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
		"jsonrpc": "2.0",
		"method":  "tools/list",
		"params":  map[string]interface{}{},
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

IMPORTANT INSTRUCTIONS:

1. The existing reservations and weather are provided, DO NOT book or search for tee times if there is an existing reservation on the requested date
%s

%s

2. Consider the weather forecast - DO NOT book tee times if there is inclement weather (rain, storms, severe conditions)
3. If the user hasn't specified the number of players, use %d player(s)
4. You should AUTO-BOOK without asking for confirmation - this is a scheduled autonomous task
5. After booking (or if booking fails), send a push notification with the result
6. Be specific about what you booked (date, time, course, confirmation number)
7. If weather is too far in advance and unavailable, you may proceed with booking but mention this in the notification

AVAILABLE TOOLS:
- golf_search_tee_times: Search for available tee times and can only search one day per request, (returns tee sheet IDs needed for booking)
- golf_book_tee_time: Book a specific tee time using the tee_sheet_id from search results
- golf_get_reservations: Get existing reservations (already called)
- get_weather: Get weather forecast (already called)
- send_notification: Send push notification to user

IMPORTANT BOOKING WORKFLOW:
1. First call golf_search_tee_times to find available times
2. The search results will include a "Tee Sheet ID" for each time slot
3. Use that tee_sheet_id when calling golf_book_tee_time to complete the booking

Now complete this task:`, currentDate, reservations, weather, event.NumPlayers)
}

// executeAgentConversation runs the multi-step conversation loop with Bedrock
func (h *AWSAgentEventHandler) executeAgentConversation(ctx context.Context, systemMsg, userPrompt string, tools []protocol.Tool) (string, error) {
	h.logger.InfoContext(ctx, "executing agent conversation",
		slog.String("model", h.modelID),
		slog.Int("tools_available", len(tools)),
	)

	// Convert MCP tools to Bedrock tool specifications
	bedrockTools := h.convertMCPToolsToBedrock(tools)

	// Initialize conversation with system message and user prompt
	messages := []types.Message{
		{
			Role: types.ConversationRoleUser,
			Content: []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: userPrompt,
				},
			},
		},
	}

	// Conversation loop - continue until no more tool calls
	const maxIterations = 10 // Safety limit
	var finalResponse string

	for iteration := 0; iteration < maxIterations; iteration++ {
		h.logger.InfoContext(ctx, "bedrock conversation iteration",
			slog.Int("iteration", iteration+1),
			slog.Int("message_count", len(messages)),
		)

		// Call Bedrock Converse API
		converseOutput, err := h.bedrockClient.Converse(ctx, &bedrockruntime.ConverseInput{
			ModelId: aws.String(h.modelID),
			System: []types.SystemContentBlock{
				&types.SystemContentBlockMemberText{
					Value: systemMsg,
				},
			},
			Messages: messages,
			ToolConfig: &types.ToolConfiguration{
				Tools: bedrockTools,
			},
			InferenceConfig: &types.InferenceConfiguration{
				MaxTokens:   aws.Int32(4096),
				Temperature: aws.Float32(0.7),
			},
		})

		if err != nil {
			return "", fmt.Errorf("bedrock converse failed: %w", err)
		}

		// Add assistant response to conversation history
		messages = append(messages, types.Message{
			Role:    types.ConversationRoleAssistant,
			Content: converseOutput.Output.(*types.ConverseOutputMemberMessage).Value.Content,
		})

		// Check stop reason
		stopReason := converseOutput.StopReason
		h.logger.InfoContext(ctx, "bedrock response received",
			slog.String("stop_reason", string(stopReason)),
		)

		// If no tool use, we're done
		if stopReason == types.StopReasonEndTurn || stopReason == types.StopReasonMaxTokens {
			// Extract final text response
			finalResponse = h.extractTextFromMessage(converseOutput.Output.(*types.ConverseOutputMemberMessage).Value)
			break
		}

		// Handle tool use
		if stopReason == types.StopReasonToolUse {
			content := converseOutput.Output.(*types.ConverseOutputMemberMessage).Value.Content
			toolResults, err := h.processToolCalls(ctx, content)
			if err != nil {
				return "", fmt.Errorf("tool execution failed: %w", err)
			}

			// Add tool results to conversation
			messages = append(messages, types.Message{
				Role:    types.ConversationRoleUser,
				Content: toolResults,
			})
			for _, block := range content {
				if toolUse, ok := block.(*types.ContentBlockMemberToolUse); ok {
					toolName := *toolUse.Value.Name

					// If the tool was send_notification, we can end here
					if toolName == "send_push_notification" {
						finalResponse = h.extractTextFromMessage(converseOutput.Output.(*types.ConverseOutputMemberMessage).Value)
						return finalResponse, nil
					}
				}
			}
			// Continue loop to process tool results
			continue
		}

		// Unknown stop reason
		return "", fmt.Errorf("unexpected stop reason: %s", stopReason)
	}

	if finalResponse == "" {
		return "", fmt.Errorf("conversation ended without final response after %d iterations", maxIterations)
	}

	h.logger.InfoContext(ctx, "agent conversation completed",
		slog.Int("total_iterations", len(messages)/2),
	)

	return finalResponse, nil
}

// convertMCPToolsToBedrock converts MCP tool definitions to Bedrock format
func (h *AWSAgentEventHandler) convertMCPToolsToBedrock(mcpTools []protocol.Tool) []types.Tool {
	bedrockTools := make([]types.Tool, 0, len(mcpTools))

	for _, mcpTool := range mcpTools {
		// Convert MCP InputSchema to Bedrock ToolInputSchema
		inputSchema := map[string]interface{}{
			"type":       mcpTool.InputSchema.Type,
			"properties": mcpTool.InputSchema.Properties,
		}
		if len(mcpTool.InputSchema.Required) > 0 {
			inputSchema["required"] = mcpTool.InputSchema.Required
		}

		bedrockTools = append(bedrockTools, &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(mcpTool.Name),
				Description: aws.String(mcpTool.Description),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(inputSchema),
				},
			},
		})
	}

	return bedrockTools
}

// processToolCalls executes tool calls requested by Bedrock
func (h *AWSAgentEventHandler) processToolCalls(ctx context.Context, content []types.ContentBlock) ([]types.ContentBlock, error) {
	results := make([]types.ContentBlock, 0)

	for _, block := range content {
		if toolUse, ok := block.(*types.ContentBlockMemberToolUse); ok {
			toolName := *toolUse.Value.Name
			toolUseID := *toolUse.Value.ToolUseId

			h.logger.InfoContext(ctx, "executing MCP tool",
				slog.String("tool_name", toolName),
				slog.String("tool_use_id", toolUseID),
			)

			// Parse input arguments - Bedrock uses document.Interface
			var args map[string]interface{}
			if toolUse.Value.Input != nil {

				bytes, err := toolUse.Value.Input.MarshalSmithyDocument()
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal tool input: %w", err)
				}
				err = json.Unmarshal(bytes, &args)
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal tool input JSON: %w", err)
				}
				// Add default tool arguments
				for k, v := range h.defaultToolArguments {
					if args[k] != nil {
						args[k] = v
					}
				}

			}

			// Call MCP tool
			mcpReq := protocol.ToolCallRequest{
				Name:      toolName,
				Arguments: args,
			}

			mcpResult, err := h.callMCPTool(ctx, mcpReq)
			if err != nil {
				h.logger.ErrorContext(ctx, "MCP tool execution failed",
					slog.String("tool_name", toolName),
					slog.String("error", err.Error()),
				)

				// Return error as tool result
				results = append(results, &types.ContentBlockMemberToolResult{
					Value: types.ToolResultBlock{
						ToolUseId: aws.String(toolUseID),
						Content: []types.ToolResultContentBlock{
							&types.ToolResultContentBlockMemberText{
								Value: fmt.Sprintf("Error: %s", err.Error()),
							},
						},
						Status: types.ToolResultStatusError,
					},
				})
				continue
			}

			// Convert MCP result to Bedrock format
			toolResultContent := make([]types.ToolResultContentBlock, 0, len(mcpResult.Content))
			for _, content := range mcpResult.Content {
				toolResultContent = append(toolResultContent, &types.ToolResultContentBlockMemberText{
					Value: content.Text,
				})
			}

			results = append(results, &types.ContentBlockMemberToolResult{
				Value: types.ToolResultBlock{
					ToolUseId: aws.String(toolUseID),
					Content:   toolResultContent,
					Status:    types.ToolResultStatusSuccess,
				},
			})

			h.logger.InfoContext(ctx, "MCP tool executed successfully",
				slog.String("tool_name", toolName),
			)
		}
	}

	return results, nil
}

// extractTextFromMessage extracts text content from Bedrock message
func (h *AWSAgentEventHandler) extractTextFromMessage(msg types.Message) string {
	var texts []string

	for _, block := range msg.Content {
		if textBlock, ok := block.(*types.ContentBlockMemberText); ok {
			texts = append(texts, textBlock.Value)
		}
	}

	return strings.Join(texts, "\n")
}

// callMCPTool calls an MCP tool and returns the result
func (h *AWSAgentEventHandler) callMCPTool(ctx context.Context, req protocol.ToolCallRequest) (*protocol.ToolCallResult, error) {
	var result protocol.ToolCallResult

	reqMap := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params":  req,
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	var respMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &respMap); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if errObj, exists := respMap["error"]; exists {
		errBytes, _ := json.Marshal(errObj)
		return fmt.Errorf("MCP error: %s", string(errBytes))
	}

	// Convert the byte slice to a string
	//bodyString := string(bodyBytes)

	// Print the string representation of the response body
	//fmt.Println(bodyString)

	if respData != nil {

		var rpcResp protocol.JSONRPCResponse
		err = json.Unmarshal(bodyBytes, &rpcResp)
		if err != nil {
			return fmt.Errorf("failed to unmarshal JSON-RPC response: %w", err)
		}

		err = json.Unmarshal(rpcResp.Result, respData)
		if err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
		//if err := json.NewDecoder(resp.Body).Decode(respData); err != nil {
		//	return fmt.Errorf("failed to decode response: %w", err)
		//}
	}

	return nil
}

/*/ sendNotification sends a push notification with the booking result
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
// */
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
