cd /workspaces/rez_agent
# make build-webapi
make build-webaction
cd infrastructure
# go build -o pulumi-rez-agent .
pulumi up -y
export WEBAPI_URL="$(pulumi stack output webapiUrl)"
#echo "export WEBAPI_URL=$WEBAPI_URL"
curl -X POST \
  -H "Content-Type: application/json" \
  -d @/workspaces/rez_agent/docs/test/messages/web_api_get_tee_times.json \
  $WEBAPI_URL/api/messages
