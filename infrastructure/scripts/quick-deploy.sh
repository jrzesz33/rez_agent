cd /workspaces/rez_agent
make build
cd infrastructure
go build -o pulumi-rez-agent .
pulumi up -y