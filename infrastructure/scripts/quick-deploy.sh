cd /workspaces/rez_agent
make build-agent
cd infrastructure
go build -o pulumi-rez-agent .
pulumi up -y
source ./scripts/set_env.sh
echo "Set Environment Variable WEBAPI_URL=$WEBAPI_URL"
