cd /workspaces/rez_agent
# make build-webapi
# make build-mcp
# make build-webaction
cd infrastructure
go build -o pulumi-rez-agent .
pulumi up -y

#source ./scripts/set_env.sh

#echo "export WEBAPI_URL=$WEBAPI_URL"
#curl -X POST \
#  -H "Content-Type: application/json" \
#  -d @/workspaces/rez_agent/docs/test/messages/web_api_weather.json \
#  $WEBAPI_URL/api/messages
