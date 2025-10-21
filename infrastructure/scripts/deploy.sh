#!/bin/bash

# rez_agent Infrastructure Deployment Script
# Usage: ./scripts/deploy.sh [dev|prod] [preview|deploy|destroy]

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
ENVIRONMENT=${1:-dev}
ACTION=${2:-preview}

# Validate environment
if [[ ! "$ENVIRONMENT" =~ ^(dev|prod)$ ]]; then
    echo -e "${RED}Error: Environment must be 'dev' or 'prod'${NC}"
    echo "Usage: $0 [dev|prod] [preview|deploy|destroy]"
    exit 1
fi

# Validate action
if [[ ! "$ACTION" =~ ^(preview|deploy|destroy)$ ]]; then
    echo -e "${RED}Error: Action must be 'preview', 'deploy', or 'destroy'${NC}"
    echo "Usage: $0 [dev|prod] [preview|deploy|destroy]"
    exit 1
fi

echo -e "${GREEN}=== rez_agent Infrastructure Deployment ===${NC}"
echo -e "Environment: ${YELLOW}$ENVIRONMENT${NC}"
echo -e "Action: ${YELLOW}$ACTION${NC}"
echo ""

# Change to project root
cd "$(dirname "$0")/../.."

# Check if AWS credentials are configured
if ! aws sts get-caller-identity &> /dev/null; then
    echo -e "${RED}Error: AWS credentials not configured${NC}"
    echo "Run 'aws configure' or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY"
    exit 1
fi

echo -e "${GREEN}AWS Account:${NC}"
aws sts get-caller-identity
echo ""

# Build Lambda functions
echo -e "${YELLOW}Building Lambda functions...${NC}"
make build
echo ""

# Change to infrastructure directory
cd infrastructure

# Select stack
echo -e "${YELLOW}Selecting Pulumi stack: $ENVIRONMENT${NC}"
pulumi stack select "$ENVIRONMENT" 2>/dev/null || {
    echo -e "${YELLOW}Stack '$ENVIRONMENT' not found. Creating...${NC}"
    pulumi stack init "$ENVIRONMENT"
}
echo ""

# Show current configuration
echo -e "${GREEN}Current configuration:${NC}"
pulumi config
echo ""

# Perform action
case $ACTION in
    preview)
        echo -e "${YELLOW}Previewing changes...${NC}"
        pulumi preview
        ;;
    deploy)
        echo -e "${YELLOW}Deploying infrastructure...${NC}"
        pulumi up --yes
        echo ""
        echo -e "${GREEN}Deployment complete!${NC}"
        echo ""
        echo -e "${GREEN}Stack outputs:${NC}"
        pulumi stack output
        ;;
    destroy)
        echo -e "${RED}WARNING: This will destroy all infrastructure!${NC}"
        read -p "Are you sure you want to destroy the $ENVIRONMENT environment? (yes/no): " confirm
        if [[ "$confirm" == "yes" ]]; then
            echo -e "${YELLOW}Destroying infrastructure...${NC}"
            pulumi destroy --yes
            echo -e "${GREEN}Destruction complete${NC}"
        else
            echo -e "${YELLOW}Destruction cancelled${NC}"
        fi
        ;;
esac

echo ""
echo -e "${GREEN}Done!${NC}"
