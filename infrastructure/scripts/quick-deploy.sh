cd /workspaces/rez_agent
make build
cd infrastructure
go build -o pulumi-rez-agent .
pulumi up -y
echo "Setting Environment Variable $ WEBAPI_URL= $(pulumi stack output webapiUrl)"
export WEBAPI_URL="$(pulumi stack output webapiUrl)"