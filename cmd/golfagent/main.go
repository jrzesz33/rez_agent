package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/jrzesz33/rez_agent/internal/golfagent"
	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/logging"
	"github.com/jrzesz33/rez_agent/internal/secrets"
)

// agentExecutor implements [a2asrv.AgentExecutor], which is a required [a2asrv.RequestHandler] dependency.
// It is responsible for invoking an agent, translating its outputs to a2a.Event object and writing them to the provided [eventqueue.Queue].
type agentExecutor struct{}

var mux = http.NewServeMux()

var agentHandler *golfagent.AWSAgentEventHandler

func (*agentExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, q eventqueue.Queue) error {
	var event golfagent.ScheduledAgentEvent
	event.CourseName = "Birdsfoot Golf Course"
	event.UserPrompt = reqCtx.Message.Parts[0].Meta()["Text"].(string)

	agentHandler.ExecuteScheduledEvent(ctx)
	response := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: "Hello, world!"})
	return q.Write(ctx, response)
}

func (*agentExecutor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, q eventqueue.Queue) error {
	return nil
}

func init() {

	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logging.GetLogLevel(),
	}))
	slog.SetDefault(logger)

	// Initialize AWS SDK
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("failed to load AWS config", slog.String("error", err.Error()))
		panic(fmt.Sprintf("failed to load AWS config: %v", err))
	}
	bedrockClient := bedrockruntime.NewFromConfig(awsCfg)

	// Create HTTP client and secrets manager for agent event handler
	httpClient := httpclient.NewClient(logger)
	secretsManager := secrets.NewManager(awsCfg, logger)

	// Create agent event handler
	agentHandler = golfagent.NewAWSAgentEventHandler(
		bedrockClient,
		httpClient,
		secretsManager,
		logger,
	)

	agentCard := &a2a.AgentCard{
		Name:               "Hello World Agent",
		Description:        "Just a hello world agent",
		URL:                "TODO",
		PreferredTransport: a2a.TransportProtocolJSONRPC,
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		Skills: []a2a.AgentSkill{
			{
				ID:          "hello_world",
				Name:        "Hello, world!",
				Description: "Returns a 'Hello, world!'",
				Tags:        []string{"hello world"},
				Examples:    []string{"hi", "hello"},
			},
		},
	}
	// A transport-agnostic implementation of A2A protocol methods.
	// The behavior is configurable using option-arguments of form a2asrv.With*(), for example:
	// a2asrv.NewHandler(executor, a2asrv.WithTaskStore(customStore))
	requestHandler := a2asrv.NewHandler(&agentExecutor{})

	mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(requestHandler))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(agentCard))

}

func main() {
	lambda.Start(httpadapter.New(mux).ProxyWithContext)
}
