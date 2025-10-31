# MCP Server Deployment Guide

## Prerequisites

- AWS CLI configured with appropriate credentials
- Pulumi CLI installed and configured
- Go 1.24+ installed
- Docker (for building Python Lambda)
- Access to AWS Secrets Manager for API credentials

## Quick Start

### 1. Build All Components

```bash
# Build MCP Lambda and client
make build-mcp
make build-mcp-client

# Or build everything at once
make build
```

### 2. Configure Secrets

Create AWS Secrets Manager secrets for:

**Golf Course Credentials:**
```bash
aws secretsmanager create-secret \
  --name rez-agent/golf/credentials-dev \
  --secret-string '{
    "client_id": "your-client-id",
    "client_secret": "your-client-secret",
    "username": "your-username",
    "password": "your-password"
  }'
```

**Weather API Key** (if using external weather API):
```bash
aws secretsmanager create-secret \
  --name rez-agent/weather/api-key-dev \
  --secret-string "your-api-key"
```

**MCP API Key** (for client authentication):
```bash
# Generate a random API key
API_KEY=$(openssl rand -hex 32)

# Store in environment or Secrets Manager
export MCP_API_KEY=$API_KEY
```

### 3. Deploy Infrastructure

**Important:** The MCP Lambda infrastructure updates need to be added to `infrastructure/main.go`. See the Pulumi Update section below.

```bash
cd infrastructure

# Select the appropriate stack
pulumi stack select dev

# Preview changes
pulumi preview

# Deploy
pulumi up

# Get the API Gateway URL
pulumi stack output apiGatewayEndpoint
```

### 4. Configure Claude Desktop

Create config file: `~/.config/rez-agent-mcp/config.json`

```json
{
  "mcp_server_url": "https://your-api-id.execute-api.us-east-1.amazonaws.com/mcp",
  "api_key": "your-mcp-api-key-from-step-2"
}
```

Update Claude Desktop config: `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "rez-agent": {
      "command": "/absolute/path/to/build/rez-agent-mcp-client",
      "env": {
        "MCP_SERVER_URL": "https://your-api-id.execute-api.us-east-1.amazonaws.com/mcp",
        "MCP_API_KEY": "your-mcp-api-key"
      }
    }
  }
}
```

### 5. Test the Integration

Restart Claude Desktop and try:

```
Can you send me a test notification?

What's the weather forecast for the next 2 days?
(Provide weather.gov API URL when prompted)
```

## Detailed Configuration

### Environment Variables

The MCP Lambda requires these environment variables (set by Pulumi):

| Variable | Description | Example |
|----------|-------------|---------|
| `MCP_SERVER_NAME` | Server name | `rez-agent-mcp` |
| `MCP_SERVER_VERSION` | Server version | `1.0.0` |
| `DYNAMODB_TABLE_NAME` | Messages table | `rez-agent-messages-dev` |
| `NOTIFICATIONS_TOPIC_ARN` | SNS topic for notifications | `arn:aws:sns:...` |
| `NTFY_URL` | ntfy.sh topic URL | `https://ntfy.sh/rzesz-alerts` |
| `STAGE` | Environment stage | `dev` |
| `GOLF_SECRET_NAME` | Golf credentials secret | `rez-agent/golf/credentials-dev` |
| `WEATHER_API_KEY_SECRET` | Weather API secret | `rez-agent/weather/api-key-dev` |
| `MCP_API_KEY` | Client authentication key | (generated securely) |

### API Gateway Configuration

The MCP server should be accessible via:
- **Endpoint:** `POST /mcp`
- **Authentication:** `X-API-Key` header
- **Content-Type:** `application/json`

## Testing

### Manual Testing with curl

**1. Initialize the connection:**
```bash
curl -X POST https://your-api.execute-api.us-east-1.amazonaws.com/mcp \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-03-26",
      "clientInfo": {
        "name": "test-client",
        "version": "1.0.0"
      }
    }
  }'
```

**2. List available tools:**
```bash
curl -X POST https://your-api.execute-api.us-east-1.amazonaws.com/mcp \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "jsonrpc": "2.0",
    "id": "2",
    "method": "tools/list",
    "params": {}
  }'
```

**3. Call a tool:**
```bash
curl -X POST https://your-api.execute-api.us-east-1.amazonaws.com/mcp \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "jsonrpc": "2.0",
    "id": "3",
    "method": "tools/call",
    "params": {
      "name": "send_push_notification",
      "arguments": {
        "title": "Test",
        "message": "Hello from MCP!",
        "priority": "default"
      }
    }
  }'
```

### Testing the stdio Client

```bash
# Run the client manually
./build/rez-agent-mcp-client

# Send a JSON-RPC request via stdin
echo '{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"test","version":"1.0.0"}}}' | ./build/rez-agent-mcp-client
```

## Pulumi Infrastructure Updates

Add the following to `infrastructure/main.go` (after the existing Lambda functions):

```go
// ========================================
// MCP Lambda Function
// ========================================

// MCP Lambda Role
mcpRole, err := iam.NewRole(ctx, fmt.Sprintf("rez-agent-mcp-role-%s", stage), &iam.RoleArgs{
	Name: pulumi.String(fmt.Sprintf("rez-agent-mcp-role-%s", stage)),
	AssumeRolePolicy: pulumi.String(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": {"Service": "lambda.amazonaws.com"},
			"Action": "sts:AssumeRole"
		}]
	}`),
	Tags: commonTags,
})
if err != nil {
	return err
}

// MCP Lambda Policy
_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("rez-agent-mcp-policy-%s", stage), &iam.RolePolicyArgs{
	Role: mcpRole.Name,
	Policy: pulumi.All(messagesTable.Arn, notificationsTopic.Arn).ApplyT(func(args []interface{}) string {
		tableArn := args[0].(string)
		topicArn := args[1].(string)
		return fmt.Sprintf(`{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": [
						"dynamodb:GetItem",
						"dynamodb:PutItem",
						"dynamodb:UpdateItem"
					],
					"Resource": ["%s", "%s/*"]
				},
				{
					"Effect": "Allow",
					"Action": ["sns:Publish"],
					"Resource": "%s"
				},
				{
					"Effect": "Allow",
					"Action": ["secretsmanager:GetSecretValue"],
					"Resource": "arn:aws:secretsmanager:*:*:secret:rez-agent/*"
				},
				{
					"Effect": "Allow",
					"Action": [
						"logs:CreateLogGroup",
						"logs:CreateLogStream",
						"logs:PutLogEvents"
					],
					"Resource": "arn:aws:logs:*:*:*"
				},
				{
					"Effect": "Allow",
					"Action": [
						"xray:PutTraceSegments",
						"xray:PutTelemetryRecords"
					],
					"Resource": "*"
				}
			]
		}`, tableArn, tableArn, topicArn)
	}).(pulumi.StringOutput),
})
if err != nil {
	return err
}

// MCP Lambda Log Group
mcpLogGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("rez-agent-mcp-logs-%s", stage), &cloudwatch.LogGroupArgs{
	Name:            pulumi.String(fmt.Sprintf("/aws/lambda/rez-agent-mcp-%s", stage)),
	RetentionInDays: pulumi.Int(logRetentionDays),
	Tags:            commonTags,
})
if err != nil {
	return err
}

// MCP Lambda Function
mcpLambda, err := lambda.NewFunction(ctx, fmt.Sprintf("rez-agent-mcp-%s", stage), &lambda.FunctionArgs{
	Name:    pulumi.String(fmt.Sprintf("rez-agent-mcp-%s", stage)),
	Runtime: pulumi.String("provided.al2"),
	Role:    mcpRole.Arn,
	Handler: pulumi.String("bootstrap"),
	Code:    pulumi.NewFileArchive("../build/mcp.zip"),
	Environment: &lambda.FunctionEnvironmentArgs{
		Variables: pulumi.StringMap{
			"MCP_SERVER_NAME":        pulumi.String("rez-agent-mcp"),
			"MCP_SERVER_VERSION":     pulumi.String("1.0.0"),
			"DYNAMODB_TABLE_NAME":    messagesTable.Name,
			"NOTIFICATIONS_TOPIC_ARN": notificationsTopic.Arn,
			"NTFY_URL":               pulumi.String(ntfyUrl),
			"STAGE":                  pulumi.String(stage),
			"GOLF_SECRET_NAME":       pulumi.String(fmt.Sprintf("rez-agent/golf/credentials-%s", stage)),
			"WEATHER_API_KEY_SECRET": pulumi.String(fmt.Sprintf("rez-agent/weather/api-key-%s", stage)),
			"AWS_REGION":             pulumi.String("us-east-1"), // or your region
		},
	},
	MemorySize: pulumi.Int(512),
	Timeout:    pulumi.Int(30),
	TracingConfig: &lambda.FunctionTracingConfigArgs{
		Mode: pulumi.String(map[bool]string{true: "Active", false: "PassThrough"}[enableXRay]),
	},
	Tags: commonTags,
}, pulumi.DependsOn([]pulumi.Resource{mcpLogGroup}))
if err != nil {
	return err
}

// Lambda permission for API Gateway to invoke MCP
_, err = lambda.NewPermission(ctx, fmt.Sprintf("rez-agent-mcp-apigw-permission-%s", stage), &lambda.PermissionArgs{
	Action:    pulumi.String("lambda:InvokeFunction"),
	Function:  mcpLambda.Name,
	Principal: pulumi.String("apigateway.amazonaws.com"),
	SourceArn: httpApi.ExecutionArn.ApplyT(func(arn string) string {
		return fmt.Sprintf("%s/*/*", arn)
	}).(pulumi.StringOutput),
})
if err != nil {
	return err
}

// API Gateway Integration for MCP
mcpApiIntegration, err := apigatewayv2.NewIntegration(ctx, fmt.Sprintf("rez-agent-mcp-api-integration-%s", stage), &apigatewayv2.IntegrationArgs{
	ApiId:                httpApi.ID(),
	IntegrationType:      pulumi.String("AWS_PROXY"),
	IntegrationUri:       mcpLambda.Arn,
	IntegrationMethod:    pulumi.String("POST"),
	PayloadFormatVersion: pulumi.String("2.0"),
})
if err != nil {
	return err
}

// API Gateway Route for MCP
_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("rez-agent-mcp-route-%s", stage), &apigatewayv2.RouteArgs{
	ApiId:    httpApi.ID(),
	RouteKey: pulumi.String("POST /mcp"),
	Target: mcpApiIntegration.ID().ApplyT(func(id string) string {
		return fmt.Sprintf("integrations/%s", id)
	}).(pulumi.StringOutput),
})
if err != nil {
	return err
}

// Export MCP Lambda ARN
ctx.Export("mcpLambdaArn", mcpLambda.Arn)
```

## Troubleshooting

### Common Issues

**1. "Invalid API key" error:**
- Verify the `MCP_API_KEY` environment variable matches in Lambda and client
- Check the `X-API-Key` header is being sent correctly

**2. "Tool not found" error:**
- Run `tools/list` to verify the tool is registered
- Check Lambda logs for tool registration errors

**3. "Authentication failed" for golf tools:**
- Verify golf credentials in Secrets Manager
- Check token URL and JWKS URL are correct
- Ensure OAuth2 flow is working (check Lambda logs)

**4. Weather API not working:**
- Verify the weather.gov API URL is correct
- Check the API is accessible from Lambda (VPC settings)
- Review Lambda logs for HTTP errors

**5. Notification not received:**
- Verify ntfy.sh topic URL is correct
- Check ntfy.sh is accessible from Lambda
- Test notification manually: `curl -d "test" https://ntfy.sh/rzesz-alerts`

### Viewing Logs

```bash
# MCP Lambda logs
aws logs tail /aws/lambda/rez-agent-mcp-dev --follow

# Or with Pulumi
cd infrastructure
pulumi stack output | grep mcpLambdaArn
aws logs tail /aws/lambda/$(pulumi stack output mcpLambdaArn | cut -d: -f7) --follow
```

### Testing Individual Components

**Test ntfy.sh:**
```bash
curl -d "Test message" https://ntfy.sh/rzesz-alerts
```

**Test weather.gov API:**
```bash
curl "https://api.weather.gov/gridpoints/TOP/31,80/forecast" \
  -H "User-Agent: test"
```

**Test OAuth2 flow:**
(Requires valid credentials in Secrets Manager)

## Monitoring

### CloudWatch Metrics

Monitor these key metrics:
- Lambda invocation count
- Lambda error rate
- Lambda duration (P50, P95, P99)
- API Gateway 4xx/5xx errors

### Alarms

Set up CloudWatch alarms for:
- High error rate (>5%)
- High latency (P95 > 1000ms)
- Failed authentications (>10/min)

### X-Ray Tracing

If X-Ray is enabled, view traces at:
https://console.aws.amazon.com/xray/home

## Security Best Practices

1. **Rotate API Keys:** Change `MCP_API_KEY` every 90 days
2. **Limit IAM Permissions:** Use least privilege for Lambda role
3. **Secure Secrets:** Never commit secrets to git
4. **Monitor Access:** Review CloudWatch logs regularly
5. **Rate Limiting:** Consider API Gateway throttling for production

## Rollback Procedure

If issues occur after deployment:

```bash
cd infrastructure

# Rollback to previous version
pulumi stack history
pulumi stack select <previous-update-id>
pulumi refresh
pulumi up --yes
```

## Support

For issues or questions:
1. Check CloudWatch logs first
2. Review design documents in `docs/design/`
3. Consult this deployment guide
4. Check MCP specification: https://modelcontextprotocol.io

---

**Version:** 1.0.0
**Last Updated:** 2025-10-31
