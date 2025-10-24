#!/bin/bash

# rez_agent Infrastructure Quick Start Script
# This script helps you get started with deploying the infrastructure

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}"
cat << "EOF"
╔═══════════════════════════════════════════════════════════════╗
║                                                               ║
║              rez_agent Infrastructure Quick Start            ║
║                                                               ║
╚═══════════════════════════════════════════════════════════════╝
EOF
echo -e "${NC}"

# Step 1: Check prerequisites
echo -e "${YELLOW}Step 1/6: Checking prerequisites...${NC}"
echo ""

check_command() {
    if command -v "$1" &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} $1 is installed"
        return 0
    else
        echo -e "  ${RED}✗${NC} $1 is not installed"
        return 1
    fi
}

MISSING_DEPS=0
check_command "go" || MISSING_DEPS=1
check_command "pulumi" || MISSING_DEPS=1
check_command "aws" || MISSING_DEPS=1
check_command "make" || MISSING_DEPS=1

if [ $MISSING_DEPS -eq 1 ]; then
    echo ""
    echo -e "${RED}Missing required dependencies. Please install them and try again.${NC}"
    echo ""
    echo "Installation instructions:"
    echo "  - Go: https://golang.org/doc/install"
    echo "  - Pulumi: curl -fsSL https://get.pulumi.com | sh"
    echo "  - AWS CLI: https://aws.amazon.com/cli/"
    echo "  - Make: sudo apt-get install build-essential (Linux) or use Xcode (macOS)"
    exit 1
fi

echo ""

# Step 2: Check AWS credentials
echo -e "${YELLOW}Step 2/6: Checking AWS credentials...${NC}"
echo ""

if aws sts get-caller-identity &> /dev/null; then
    echo -e "${GREEN}AWS credentials are configured${NC}"
    aws sts get-caller-identity
else
    echo -e "${RED}AWS credentials are not configured${NC}"
    echo ""
    echo "Please configure AWS credentials:"
    echo "  aws configure"
    echo ""
    echo "Or set environment variables:"
    echo "  export AWS_ACCESS_KEY_ID=your-access-key"
    echo "  export AWS_SECRET_ACCESS_KEY=your-secret-key"
    echo "  export AWS_REGION=us-east-1"
    exit 1
fi

echo ""

# Step 3: Initialize Pulumi
echo -e "${YELLOW}Step 3/6: Initializing Pulumi...${NC}"
echo ""

cd "$(dirname "$0")/../.."

if [ ! -d "infrastructure/.pulumi" ]; then
    echo "Please login to Pulumi:"
    echo ""
    read -p "Do you want to use Pulumi Cloud (recommended)? (yes/no): " use_cloud

    cd infrastructure
    if [[ "$use_cloud" == "yes" ]]; then
        pulumi login
    else
        echo "Using local backend..."
        pulumi login --local
    fi
    cd ..
else
    echo -e "${GREEN}Pulumi is already initialized${NC}"
fi

echo ""

# Step 4: Select environment
echo -e "${YELLOW}Step 4/6: Selecting environment...${NC}"
echo ""
echo "Available environments:"
echo "  1) dev (development)"
echo "  2) prod (production)"
echo ""
read -p "Select environment (1 or 2): " env_choice

case $env_choice in
    1)
        ENVIRONMENT="dev"
        ;;
    2)
        ENVIRONMENT="prod"
        ;;
    *)
        echo -e "${RED}Invalid choice. Using 'dev' as default.${NC}"
        ENVIRONMENT="dev"
        ;;
esac

echo -e "${GREEN}Selected environment: $ENVIRONMENT${NC}"
echo ""

# Step 5: Configure stack
echo -e "${YELLOW}Step 5/6: Configuring Pulumi stack...${NC}"
echo ""

cd infrastructure

# Initialize stack if it doesn't exist
if ! pulumi stack ls | grep -q "$ENVIRONMENT"; then
    echo "Creating new stack: $ENVIRONMENT"
    pulumi stack init "$ENVIRONMENT"
fi

pulumi stack select "$ENVIRONMENT"

# Check if configuration exists
if ! pulumi config get stage &> /dev/null; then
    echo "Configuring stack..."

    # Ask for AWS region
    read -p "Enter AWS region [us-east-1]: " aws_region
    aws_region=${aws_region:-us-east-1}
    pulumi config set aws:region "$aws_region"

    # Set stage
    pulumi config set stage "$ENVIRONMENT"

    # Ask for ntfy.sh URL
    read -p "Enter ntfy.sh URL [https://ntfy.sh/rzesz-alerts]: " ntfy_url
    ntfy_url=${ntfy_url:-https://ntfy.sh/rzesz-alerts}
    pulumi config set ntfyUrl "$ntfy_url"

    # Set log retention based on environment
    if [ "$ENVIRONMENT" == "prod" ]; then
        pulumi config set logRetentionDays 30
    else
        pulumi config set logRetentionDays 7
    fi

    # Enable X-Ray
    pulumi config set enableXRay true

    # Set scheduler cron
    pulumi config set schedulerCron "cron(0 12 * * ? *)"

    echo -e "${GREEN}Stack configured successfully${NC}"
else
    echo -e "${GREEN}Stack is already configured${NC}"
fi

echo ""
echo "Current configuration:"
pulumi config

echo ""

cd ..

# Step 6: Build and Deploy
echo -e "${YELLOW}Step 6/6: Building and deploying...${NC}"
echo ""
echo "What would you like to do?"
echo "  1) Preview changes (recommended first step)"
echo "  2) Deploy infrastructure"
echo "  3) Skip for now"
echo ""
read -p "Select option (1-3): " deploy_choice

case $deploy_choice in
    1)
        echo ""
        echo -e "${YELLOW}Building Lambda functions...${NC}"
        make build
        echo ""
        echo -e "${YELLOW}Previewing infrastructure changes...${NC}"
        cd infrastructure
        pulumi preview
        cd ..
        echo ""
        echo -e "${GREEN}Preview complete!${NC}"
        echo ""
        echo "To deploy, run:"
        echo "  make deploy-$ENVIRONMENT"
        echo ""
        echo "Or manually:"
        echo "  cd infrastructure"
        echo "  pulumi up"
        ;;
    2)
        echo ""
        echo -e "${YELLOW}Building Lambda functions...${NC}"
        make build
        echo ""
        echo -e "${YELLOW}Deploying infrastructure...${NC}"
        cd infrastructure
        pulumi up --yes
        echo ""
        echo -e "${GREEN}Deployment complete!${NC}"
        echo ""
        echo -e "${GREEN}Stack outputs:${NC}"
        pulumi stack output
        echo ""
        echo "WebAPI URL: $(pulumi stack output webapiUrl)"
        echo ""
        echo "Test the deployment:"
        echo "  curl \$(pulumi stack output webapiUrl --cwd infrastructure)/api/health"
        cd ..
        ;;
    3)
        echo ""
        echo -e "${YELLOW}Skipping deployment${NC}"
        echo ""
        echo "To preview changes later, run:"
        echo "  make infra-preview"
        echo ""
        echo "To deploy, run:"
        echo "  make deploy-$ENVIRONMENT"
        ;;
    *)
        echo -e "${RED}Invalid choice${NC}"
        ;;
esac

echo ""
echo -e "${BLUE}╔═══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                                                               ║${NC}"
echo -e "${BLUE}║                     Setup Complete!                           ║${NC}"
echo -e "${BLUE}║                                                               ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}Next steps:${NC}"
echo ""
echo "1. View infrastructure outputs:"
echo "   make infra-outputs"
echo ""
echo "2. Monitor Lambda logs:"
echo "   make lambda-logs-scheduler"
echo "   make lambda-logs-processor"
echo "   make lambda-logs-webaction"
echo "   make lambda-logs-webapi"
echo ""
echo "3. Test the WebAPI:"
echo "   curl \$(pulumi stack output webapiUrl --cwd infrastructure)/api/health"
echo ""
echo "4. Read the documentation:"
echo "   cat infrastructure/README.md"
echo ""
echo -e "${YELLOW}Useful commands:${NC}"
echo "   make help                 # Show all available commands"
echo "   make deploy-dev           # Deploy to dev environment"
echo "   make deploy-prod          # Deploy to prod environment"
echo "   make infra-preview        # Preview infrastructure changes"
echo "   make infra-destroy        # Destroy infrastructure"
echo ""
echo -e "${GREEN}Happy building! 🚀${NC}"
echo ""
