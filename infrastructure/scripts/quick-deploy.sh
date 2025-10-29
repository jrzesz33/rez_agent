cd /workspaces/rez_agent
make build
cd infrastructure
go build -o pulumi-rez-agent .
pulumi up -y
export WEBAPI_URL="$(pulumi stack output webapiUrl)"
echo "Set Environment Variable WEBAPI_URL=$WEBAPI_URL"
