package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/jrzesz33/rez_agent/internal/logging"
	"github.com/jrzesz33/rez_agent/internal/messaging"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/repository"
	appconfig "github.com/jrzesz33/rez_agent/pkg/config"
)

// WebAPIHandler handles API Gateway requests
type WebAPIHandler struct {
	config     *appconfig.Config
	repository repository.MessageRepository
	publisher  messaging.SNSPublisher
	logger     *slog.Logger
}

// NewWebAPIHandler creates a new web API handler instance
func NewWebAPIHandler(
	cfg *appconfig.Config,
	repo repository.MessageRepository,
	pub messaging.SNSPublisher,
	logger *slog.Logger,
) *WebAPIHandler {
	return &WebAPIHandler{
		config:     cfg,
		repository: repo,
		publisher:  pub,
		logger:     logger,
	}
}

// HandleRequest routes API Gateway V2 requests to appropriate handlers
func (h *WebAPIHandler) HandleRequest(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	h.logger.DebugContext(ctx, "received API request",
		slog.String("method", request.RequestContext.HTTP.Method),
		slog.String("path", request.RawPath),
	)

	// Add CORS headers
	headers := map[string]string{
		"Content-Type":                 "application/json",
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type",
	}

	// Handle OPTIONS for CORS preflight
	if request.RequestContext.HTTP.Method == "OPTIONS" {
		return events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusOK,
			Headers:    headers,
		}, nil
	}

	// Route requests
	var response events.APIGatewayV2HTTPResponse
	var err error

	path := request.RawPath
	if path == "" {
		path = request.RequestContext.HTTP.Path
	}
	method := request.RequestContext.HTTP.Method

	switch {
	case path == "/api/health" && method == "GET":
		response, err = h.handleHealth(ctx)
	case path == "/api/messages" && method == "GET":
		response, err = h.handleListMessages(ctx, request)
	case path == "/api/messages" && method == "POST":
		response, err = h.handleCreateMessage(ctx, request)
	case path == "/api/metrics" && method == "GET":
		response, err = h.handleMetrics(ctx)
	default:
		response = h.createErrorResponse(http.StatusNotFound, "endpoint not found")
	}

	if err != nil {
		h.logger.ErrorContext(ctx, "request handler error",
			slog.String("error", err.Error()),
		)
	}

	// Add CORS headers to response
	if response.Headers == nil {
		response.Headers = headers
	} else {
		for k, v := range headers {
			response.Headers[k] = v
		}
	}

	return response, err
}

// handleHealth returns the health status of the API
func (h *WebAPIHandler) handleHealth(ctx context.Context) (events.APIGatewayV2HTTPResponse, error) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"stage":     h.config.Stage.String(),
	}
	fmt.Println(ctx)
	body, err := json.Marshal(health)
	if err != nil {
		return h.createErrorResponse(http.StatusInternalServerError, "failed to marshal health response"), err
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
	}, nil
}

// handleListMessages returns a list of messages with optional filtering
func (h *WebAPIHandler) handleListMessages(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	// Parse query parameters
	var stage *models.Stage
	var status *models.Status
	limit := 100

	if stageParam, ok := request.QueryStringParameters["stage"]; ok && stageParam != "" {
		s := models.Stage(stageParam)
		if s.IsValid() {
			stage = &s
		}
	}

	if statusParam, ok := request.QueryStringParameters["status"]; ok && statusParam != "" {
		st := models.Status(statusParam)
		if st.IsValid() {
			status = &st
		}
	}

	if limitParam, ok := request.QueryStringParameters["limit"]; ok && limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	h.logger.DebugContext(ctx, "listing messages",
		slog.Any("stage", stage),
		slog.Any("status", status),
		slog.Int("limit", limit),
	)

	// Query messages from repository
	messages, err := h.repository.ListMessages(ctx, stage, status, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to list messages", slog.String("error", err.Error()))
		return h.createErrorResponse(http.StatusInternalServerError, "failed to retrieve messages"), err
	}

	response := map[string]interface{}{
		"messages": messages,
		"count":    len(messages),
	}

	body, err := json.Marshal(response)
	if err != nil {
		return h.createErrorResponse(http.StatusInternalServerError, "failed to marshal response"), err
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
	}, nil
}

// CreateMessageRequest represents a request to create a new message
// For simple notification messages, use the "payload" field.
// For web action messages, embed the WebActionPayload fields directly at the root level.
type CreateMessageRequest struct {
	// Simple message fields
	Payload     string             `json:"payload,omitempty"`
	MessageType models.MessageType `json:"message_type,omitempty"`
	Stage       models.Stage       `json:"stage,omitempty"`

	// Web action fields (embedded from WebActionPayload)
	Version    string                 `json:"version,omitempty"`
	URL        string                 `json:"url,omitempty"`
	Action     models.WebActionType   `json:"action,omitempty"`
	Arguments  map[string]interface{} `json:"arguments,omitempty"`
	AuthConfig *models.AuthConfig     `json:"auth_config,omitempty"`
}

// handleCreateMessage creates a new message manually
func (h *WebAPIHandler) handleCreateMessage(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	var req CreateMessageRequest
	err := json.Unmarshal([]byte(request.Body), &req)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to parse request body", slog.String("error", err.Error()))
		return h.createErrorResponse(http.StatusBadRequest, "invalid request body"), err
	}

	// Detect if this is a web action request (has version, url, action fields)
	isWebAction := req.Version != "" || req.URL != "" || req.Action != ""

	// Validate request - either payload or web action fields must be provided
	if req.Payload == "" && !isWebAction {
		return h.createErrorResponse(http.StatusBadRequest, "either payload or web action fields (version, url, action) are required"), nil
	}

	if req.Payload != "" && isWebAction {
		return h.createErrorResponse(http.StatusBadRequest, "cannot specify both payload and web action fields"), nil
	}

	// Use config stage if not provided
	stage := req.Stage
	if stage == "" {
		stage = h.config.Stage
	}

	if !stage.IsValid() {
		return h.createErrorResponse(http.StatusBadRequest, "invalid stage value"), nil
	}

	// Determine message type
	messageType := req.MessageType
	if messageType == "" {
		// Auto-detect message type based on request content
		if isWebAction {
			messageType = models.MessageTypeWebAction
		} else {
			messageType = models.MessageTypeNotification
		}
	}

	if !messageType.IsValid() {
		return h.createErrorResponse(http.StatusBadRequest, "invalid message_type value"), nil
	}

	// Prepare payload string
	var payloadStr string
	if isWebAction {
		// Construct WebActionPayload from request fields
		webActionPayload := &models.WebActionPayload{
			Version:    req.Version,
			URL:        req.URL,
			Action:     req.Action,
			Arguments:  req.Arguments,
			AuthConfig: req.AuthConfig,
		}

		// Validate web action if message type is web_action
		if messageType == models.MessageTypeWebAction {
			if err := webActionPayload.Validate(); err != nil {
				h.logger.ErrorContext(ctx, "invalid web action payload", slog.String("error", err.Error()))
				return h.createErrorResponse(http.StatusBadRequest, fmt.Sprintf("invalid web action: %s", err.Error())), nil
			}
		}

		// Serialize web action to JSON
		webActionJSON, err := json.Marshal(webActionPayload)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to serialize web action", slog.String("error", err.Error()))
			return h.createErrorResponse(http.StatusInternalServerError, "failed to serialize web action"), err
		}
		payloadStr = string(webActionJSON)
	} else {
		payloadStr = req.Payload
	}

	// Create message
	message := models.NewMessage("webapi", stage, messageType, payloadStr)

	h.logger.DebugContext(ctx, "creating message",
		slog.String("message_id", message.ID),
		slog.String("type", message.MessageType.String()),
	)

	// Save to repository
	err = h.repository.SaveMessage(ctx, message)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to save message", slog.String("error", err.Error()))
		return h.createErrorResponse(http.StatusInternalServerError, "failed to save message"), err
	}

	// Mark as queued
	message.MarkQueued()
	err = h.repository.UpdateStatus(ctx, message.ID, message.Status, "")
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to update message status", slog.String("error", err.Error()))
	}

	// Publish to SNS
	err = h.publisher.PublishMessage(ctx, message)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to publish message", slog.String("error", err.Error()))
		return h.createErrorResponse(http.StatusInternalServerError, "failed to publish message"), err
	}

	body, err := json.Marshal(message)
	if err != nil {
		return h.createErrorResponse(http.StatusInternalServerError, "failed to marshal response"), err
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusCreated,
		Body:       string(body),
	}, nil
}

// handleMetrics returns metrics about messages
func (h *WebAPIHandler) handleMetrics(ctx context.Context) (events.APIGatewayV2HTTPResponse, error) {
	h.logger.DebugContext(ctx, "retrieving metrics")

	// Get messages by status
	allMessages, err := h.repository.ListMessages(ctx, nil, nil, 1000)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to retrieve messages for metrics", slog.String("error", err.Error()))
		return h.createErrorResponse(http.StatusInternalServerError, "failed to retrieve metrics"), err
	}

	// Calculate metrics
	metrics := map[string]interface{}{
		"total":     len(allMessages),
		"by_status": make(map[string]int),
		"by_stage":  make(map[string]int),
		"by_type":   make(map[string]int),
	}

	byStatus := make(map[string]int)
	byStage := make(map[string]int)
	byType := make(map[string]int)

	for _, msg := range allMessages {
		byStatus[msg.Status.String()]++
		byStage[msg.Stage.String()]++
		byType[msg.MessageType.String()]++
	}

	metrics["by_status"] = byStatus
	metrics["by_stage"] = byStage
	metrics["by_type"] = byType

	body, err := json.Marshal(metrics)
	if err != nil {
		return h.createErrorResponse(http.StatusInternalServerError, "failed to marshal metrics"), err
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
	}, nil
}

// createErrorResponse creates a standardized error response
func (h *WebAPIHandler) createErrorResponse(statusCode int, message string) events.APIGatewayV2HTTPResponse {
	errorBody := map[string]string{
		"error":  message,
		"status": strconv.Itoa(statusCode),
	}
	body, _ := json.Marshal(errorBody)

	return events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Body:       string(body),
	}
}

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logging.GetLogLevel(),
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg := appconfig.MustLoad()

	logger.Info("web api lambda starting",
		slog.String("stage", cfg.Stage.String()),
		slog.String("region", cfg.AWSRegion),
	)

	// Initialize AWS SDK
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("failed to load AWS config", slog.String("error", err.Error()))
		panic(fmt.Sprintf("failed to load AWS config: %v", err))
	}

	// Create AWS clients
	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	snsClient := sns.NewFromConfig(awsCfg)

	// Create repository and publisher
	repo := repository.NewDynamoDBRepository(dynamoClient, cfg.DynamoDBTableName)

	// Use topic routing if both topics are configured, otherwise fall back to legacy single topic
	publisher := messaging.NewTopicRoutingSNSClient(
		snsClient,
		cfg.WebActionsSNSTopicArn,
		cfg.NotificationsSNSTopicArn,
		cfg.AgentResponseTopicArn,
		logger,
	)
	logger.Info("using topic-routing SNS client",
		slog.String("web_actions_topic", cfg.WebActionsSNSTopicArn),
		slog.String("notifications_topic", cfg.NotificationsSNSTopicArn),
	)

	// Create handler
	handler := NewWebAPIHandler(cfg, repo, publisher, logger)

	// Start Lambda handler
	lambda.Start(handler.HandleRequest)
}
