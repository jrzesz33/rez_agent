package main

// This file contains an alternative implementation using API Gateway instead of ALB
// To use this, replace the ALB section in main.go with this code

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CreateAPIGateway creates an HTTP API Gateway for the WebAPI Lambda
// This is a lower-cost alternative to Application Load Balancer
// Cost: ~$3.50/month for 10K requests vs ~$16/month for ALB
func CreateAPIGateway(
	ctx *pulumi.Context,
	stage string,
	webapiLambda *lambda.Function,
	commonTags pulumi.StringMap,
) (*apigatewayv2.Api, *apigatewayv2.Stage, error) {

	// Create HTTP API
	api, err := apigatewayv2.NewApi(ctx, fmt.Sprintf("rez-agent-api-%s", stage), &apigatewayv2.ApiArgs{
		Name:         pulumi.String(fmt.Sprintf("rez-agent-api-%s", stage)),
		ProtocolType: pulumi.String("HTTP"),
		Description:  pulumi.String("rez_agent REST API"),
		CorsConfiguration: &apigatewayv2.ApiCorsConfigurationArgs{
			AllowOrigins: pulumi.StringArray{pulumi.String("*")},
			AllowMethods: pulumi.StringArray{
				pulumi.String("GET"),
				pulumi.String("POST"),
				pulumi.String("PUT"),
				pulumi.String("DELETE"),
				pulumi.String("OPTIONS"),
			},
			AllowHeaders: pulumi.StringArray{
				pulumi.String("Content-Type"),
				pulumi.String("Authorization"),
				pulumi.String("X-Correlation-ID"),
			},
			MaxAge: pulumi.Int(300),
		},
		Tags: commonTags,
	})
	if err != nil {
		return nil, nil, err
	}

	// Lambda permission for API Gateway
	_, err = lambda.NewPermission(ctx, fmt.Sprintf("rez-agent-webapi-apigw-permission-%s", stage), &lambda.PermissionArgs{
		Action:    pulumi.String("lambda:InvokeFunction"),
		Function:  webapiLambda.Name,
		Principal: pulumi.String("apigateway.amazonaws.com"),
		SourceArn: api.ExecutionArn.ApplyT(func(arn string) string {
			return fmt.Sprintf("%s/*/*", arn)
		}).(pulumi.StringOutput),
	})
	if err != nil {
		return nil, nil, err
	}

	// API Gateway Integration with Lambda
	integration, err := apigatewayv2.NewIntegration(ctx, fmt.Sprintf("rez-agent-api-integration-%s", stage), &apigatewayv2.IntegrationArgs{
		ApiId:             api.ID(),
		IntegrationType:   pulumi.String("AWS_PROXY"),
		IntegrationUri:    webapiLambda.Arn,
		IntegrationMethod: pulumi.String("POST"),
		PayloadFormatVersion: pulumi.String("2.0"),
		TimeoutMilliseconds: pulumi.Int(30000),
	})
	if err != nil {
		return nil, nil, err
	}

	// Default route (catch-all)
	_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("rez-agent-api-default-route-%s", stage), &apigatewayv2.RouteArgs{
		ApiId:    api.ID(),
		RouteKey: pulumi.String("$default"),
		Target: integration.ID().ApplyT(func(id string) string {
			return fmt.Sprintf("integrations/%s", id)
		}).(pulumi.StringOutput),
	})
	if err != nil {
		return nil, nil, err
	}

	// Specific routes (optional - for better API Gateway metrics)
	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/health"},
		{"GET", "/api/messages"},
		{"POST", "/api/messages"},
		{"GET", "/api/metrics"},
		{"POST", "/api/auth/login"},
		{"GET", "/api/auth/callback"},
		{"POST", "/api/auth/refresh"},
	}

	for _, route := range routes {
		routeKey := fmt.Sprintf("%s %s", route.method, route.path)
		_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("rez-agent-api-route-%s-%s-%s", stage, route.method, route.path), &apigatewayv2.RouteArgs{
			ApiId:    api.ID(),
			RouteKey: pulumi.String(routeKey),
			Target: integration.ID().ApplyT(func(id string) string {
				return fmt.Sprintf("integrations/%s", id)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return nil, nil, err
		}
	}

	// Auto-deploy stage
	apiStage, err := apigatewayv2.NewStage(ctx, fmt.Sprintf("rez-agent-api-stage-%s", stage), &apigatewayv2.StageArgs{
		ApiId:      api.ID(),
		Name:       pulumi.String("$default"),
		AutoDeploy: pulumi.Bool(true),
		AccessLogSettings: &apigatewayv2.StageAccessLogSettingsArgs{
			DestinationArn: pulumi.String(fmt.Sprintf("arn:aws:logs:*:*:log-group:/aws/apigateway/rez-agent-%s", stage)),
			Format: pulumi.String(`{
				"requestId":"$context.requestId",
				"ip":"$context.identity.sourceIp",
				"requestTime":"$context.requestTime",
				"httpMethod":"$context.httpMethod",
				"routeKey":"$context.routeKey",
				"status":"$context.status",
				"protocol":"$context.protocol",
				"responseLength":"$context.responseLength",
				"integrationLatency":"$context.integrationLatency",
				"correlationId":"$context.requestHeader.X-Correlation-ID"
			}`),
		},
		DefaultRouteSettings: &apigatewayv2.StageDefaultRouteSettingsArgs{
			ThrottlingBurstLimit: pulumi.Int(5000),
			ThrottlingRateLimit:  pulumi.Float64(1000),
		},
		Tags: commonTags,
	})
	if err != nil {
		return nil, nil, err
	}

	return api, apiStage, nil
}

// Example usage in main.go:
//
// Replace the ALB section with:
//
// api, apiStage, err := CreateAPIGateway(ctx, stage, webapiLambda, commonTags)
// if err != nil {
//     return err
// }
//
// ctx.Export("apiGatewayUrl", apiStage.InvokeUrl)
// ctx.Export("apiGatewayId", api.ID())
