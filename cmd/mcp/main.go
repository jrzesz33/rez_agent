package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/logging"
	"github.com/jrzesz33/rez_agent/internal/mcp/server"
	"github.com/jrzesz33/rez_agent/internal/mcp/tools"
	"github.com/jrzesz33/rez_agent/internal/secrets"
	"github.com/jrzesz33/rez_agent/pkg/config"
)

type Handler struct {
	mcpServer *server.MCPServer
	logger    *slog.Logger
	apiKey    string
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logging.GetLogLevel(),
	}))

	logger.Info("MCP Lambda Function Starting...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", slog.String("error", err.Error()))
		panic(err)
	}

	// Initialize AWS SDK
	awsCfg, err := awsConfig.LoadDefaultConfig(context.Background(),
		awsConfig.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("failed to load AWS config", slog.String("error", err.Error()))
		panic(err)
	}

	logger.Info("MCP Lambda initialized configuration")

	// Create MCP server
	serverName := os.Getenv("MCP_SERVER_NAME")
	if serverName == "" {
		serverName = "rez-agent-mcp"
	}

	serverVersion := os.Getenv("MCP_SERVER_VERSION")
	if serverVersion == "" {
		serverVersion = "1.0.0"
	}

	mcpServer := server.NewMCPServer(serverName, serverVersion, logger)

	// Initialize dependencies
	httpClient := httpclient.NewClient(logger)
	secretsManager := secrets.NewManager(awsCfg, logger)
	oauthClient := httpclient.NewOAuthClient(httpClient, secretsManager, logger)

	// Register MCP tools
	logger.Info("registering MCP tools...")

	// 1. Notification tool
	notificationTool := tools.NewNotificationTool(cfg.NtfyURL, logger)
	if err := mcpServer.RegisterTool(notificationTool); err != nil {
		logger.Error("failed to register notification tool", slog.String("error", err.Error()))
		panic(err)
	}

	// 2. Weather tool
	weatherTool := tools.NewWeatherTool(httpClient, logger)
	if err := mcpServer.RegisterTool(weatherTool); err != nil {
		logger.Error("failed to register weather tool", slog.String("error", err.Error()))
		panic(err)
	}

	// 3. Golf reservations tool
	golfReservationsTool := tools.NewGolfReservationsTool(httpClient, oauthClient, secretsManager, logger)
	if err := mcpServer.RegisterTool(golfReservationsTool); err != nil {
		logger.Error("failed to register golf reservations tool", slog.String("error", err.Error()))
		panic(err)
	}

	// 4. Golf search tee times tool
	golfSearchTool := tools.NewGolfSearchTeeTimesTool(httpClient, oauthClient, secretsManager, logger)
	if err := mcpServer.RegisterTool(golfSearchTool); err != nil {
		logger.Error("failed to register golf search tool", slog.String("error", err.Error()))
		panic(err)
	}

	// 5. Golf book tee time tool
	golfBookTool := tools.NewGolfBookTeeTimeTool(httpClient, oauthClient, secretsManager, logger)
	if err := mcpServer.RegisterTool(golfBookTool); err != nil {
		logger.Error("failed to register golf book tool", slog.String("error", err.Error()))
		panic(err)
	}

	logger.Info("MCP server initialized successfully",
		slog.Int("tool_count", 5),
	)

	// Get API key from environment (for authentication)
	apiKey := os.Getenv("MCP_API_KEY")
	if apiKey == "" {
		logger.Warn("MCP_API_KEY not set, authentication disabled")
	}

	handler := &Handler{
		mcpServer: mcpServer,
		logger:    logger,
		apiKey:    apiKey,
	}

	lambda.Start(handler.HandleAPIGatewayRequest)
}

// HandleAPIGatewayRequest processes API Gateway HTTP API requests
func (h *Handler) HandleAPIGatewayRequest(ctx context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	h.logger.Info("received MCP request",
		slog.String("path", event.RawPath),
		slog.String("method", event.RequestContext.HTTP.Method),
		slog.String("request_id", event.RequestContext.RequestID),
	)

	// Validate API key if configured
	if h.apiKey != "" {
		providedKey := event.Headers["x-api-key"]
		if providedKey != h.apiKey {
			h.logger.Warn("invalid API key provided",
				slog.String("remote_addr", event.RequestContext.HTTP.SourceIP),
			)
			return events.APIGatewayV2HTTPResponse{
				StatusCode: 401,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{"jsonrpc":"2.0","error":{"code":-32004,"message":"Invalid API key"},"id":null}`,
			}, nil
		}
	}

	// Handle JSON-RPC request
	responseBody, err := h.mcpServer.HandleRequest(ctx, []byte(event.Body))
	if err != nil {
		h.logger.Error("failed to handle MCP request",
			slog.String("error", err.Error()),
			slog.String("request_id", event.RequestContext.RequestID),
		)

		// Return JSON-RPC error
		errorResp := map[string]interface{}{
			"jsonrpc": "2.0",
			"error": map[string]interface{}{
				"code":    -32603,
				"message": "Internal error",
				"data":    err.Error(),
			},
			"id": nil,
		}

		errorBody, _ := json.Marshal(errorResp)
		return events.APIGatewayV2HTTPResponse{
			StatusCode: 200, // JSON-RPC errors still return 200 OK
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(errorBody),
		}, nil
	}

	h.logger.Info("MCP request completed successfully",
		slog.String("request_id", event.RequestContext.RequestID),
	)

	return events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(responseBody),
	}, nil
}
