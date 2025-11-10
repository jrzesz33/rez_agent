# rez_agent Documentation

Welcome to the rez_agent documentation. This directory contains comprehensive guides, API references, and technical documentation for the project.

## Quick Links

### Getting Started
- [Main README](../README.md) - Project overview and quick start
- [Developer Guide](DEVELOPER_GUIDE.md) - Development workflow and best practices
- [Deployment Guide](DEPLOYMENT_GUIDE.md) - Infrastructure deployment instructions

### Technical Documentation
- [Architecture](architecture/README.md) - System architecture and design patterns
- [API Reference](api/README.md) - HTTP API endpoints and usage
- [Message Schemas](MESSAGE_SCHEMAS.md) - Message format specifications

### Additional Resources
- [Design Documents](design/) - Detailed design specifications
- [Test Examples](test/messages/) - Sample message payloads

## Documentation Structure

```
docs/
├── README.md                    # This file
├── DEVELOPER_GUIDE.md          # Development workflow and practices
├── DEPLOYMENT_GUIDE.md         # Deployment procedures
├── MESSAGE_SCHEMAS.md          # Message format specifications
├── api/
│   └── README.md              # API documentation
├── architecture/
│   └── README.md              # Architecture documentation
├── design/                     # Design documents
└── test/
    └── messages/              # Test message examples
```

## Documentation Overview

### [Developer Guide](DEVELOPER_GUIDE.md)

Comprehensive guide for developers working on rez_agent:

- **Development Environment Setup**: Configure your local environment
- **Project Structure**: Understand the codebase organization
- **Development Workflow**: Git workflow, testing, and code review
- **Code Style**: Go and Python coding standards
- **Adding Features**: Step-by-step guides for new functionality
- **Debugging**: Tools and techniques for troubleshooting
- **Performance Optimization**: Best practices for Lambda performance

**Target Audience**: Developers contributing to the project

### [Deployment Guide](DEPLOYMENT_GUIDE.md)

Complete deployment instructions for all environments:

- **Prerequisites**: Required tools and accounts
- **Initial Setup**: First-time deployment steps
- **Deployment Environments**: Dev, stage, and prod configurations
- **Deployment Process**: Step-by-step deployment instructions
- **Configuration**: Pulumi and environment variable setup
- **Secrets Management**: AWS Secrets Manager usage
- **Monitoring**: CloudWatch logs and metrics
- **Rollback Procedures**: Emergency recovery steps
- **CI/CD Pipeline**: GitHub Actions automation

**Target Audience**: DevOps engineers and deployment managers

### [Architecture Documentation](architecture/README.md)

Deep dive into system architecture:

- **System Overview**: High-level architecture
- **Architecture Patterns**: Pub/Sub, message routing, repository pattern
- **Component Diagram**: Visual representation of components
- **Data Flow**: Message flow through the system
- **Lambda Functions**: Detailed function descriptions
- **Messaging Architecture**: SNS/SQS design
- **Data Storage**: DynamoDB table schemas
- **Security Architecture**: IAM, encryption, SSRF protection
- **Scaling and Performance**: Auto-scaling and optimization

**Target Audience**: Architects and senior developers

### [API Documentation](api/README.md)

HTTP API reference:

- **Overview**: API characteristics and base URL
- **Authentication**: Current and future auth methods
- **Endpoints**: Complete endpoint reference
  - `POST /api/messages`: Create messages
  - `POST /api/schedules`: Create/manage schedules
- **Request/Response Formats**: Schema definitions
- **Error Handling**: Error codes and formats
- **Examples**: cURL, JavaScript, and Python examples
- **Security**: SSRF protection and data handling
- **MCP Server API**: Claude AI integration endpoints

**Target Audience**: API consumers and integration developers

### [Message Schemas](MESSAGE_SCHEMAS.md)

Complete message format specifications:

- **Base Message Schema**: Core message structure
- **Message Types**: Detailed schemas for all message types
  - `hello_world`: Test messages
  - `notify`: Notifications
  - `agent_response`: AI agent responses
  - `scheduled`: Scheduled tasks
  - `web_action`: HTTP API calls
  - `schedule_creation`: Dynamic schedules
- **Authentication Configuration**: Auth type schemas
- **Web Action Results**: Result record format
- **Schedule Metadata**: Schedule configuration
- **Validation Rules**: Input validation requirements

**Target Audience**: Developers and API consumers

## Additional Documentation

### Design Documents

The `design/` directory contains detailed design specifications for major features:

- System architecture decisions
- Feature implementation plans
- Performance optimization strategies
- Security considerations

### Test Examples

The `test/messages/` directory contains example message payloads:

- Web action requests
- Schedule creation requests
- Golf booking examples
- Weather forecast examples

Use these as templates for creating your own messages.

## Contributing to Documentation

When adding or modifying features, please update the relevant documentation:

1. **Code Changes**: Update Developer Guide
2. **API Changes**: Update API Documentation
3. **Infrastructure Changes**: Update Deployment Guide
4. **Architecture Changes**: Update Architecture Documentation
5. **Message Format Changes**: Update Message Schemas

### Documentation Standards

- Use clear, concise language
- Include code examples
- Add diagrams for complex concepts
- Keep examples up-to-date
- Use proper Markdown formatting
- Add links between related docs

## Getting Help

If you need help:

1. **Search Documentation**: Use Ctrl+F or search in your editor
2. **Check Examples**: Review test message examples
3. **Review Logs**: Check CloudWatch logs for errors
4. **GitHub Issues**: Report bugs or request features
5. **Code Comments**: Check inline code documentation

## Documentation Maintenance

Documentation is maintained alongside code:

- **Version Control**: All docs in Git
- **Review Process**: Docs reviewed in PRs
- **Update Triggers**: Update docs when:
  - Adding new features
  - Changing APIs
  - Modifying infrastructure
  - Updating dependencies
  - Fixing bugs with behavioral changes

## Quick Reference

### Common Commands

```bash
# Build all Lambda functions
make build

# Run tests
make test

# Deploy to dev
make deploy-dev

# View logs
make lambda-logs-webaction

# Format code
make fmt
```

### Common Endpoints

```bash
# Create a message
POST /api/messages

# Create a schedule
POST /api/schedules
```

### Environment Variables

```bash
# Required
STAGE=dev|stage|prod
DYNAMODB_TABLE_NAME=...
NTFY_URL=...

# Optional
LOG_LEVEL=DEBUG|INFO|WARN|ERROR
```

### Useful Links

- **Main README**: [../README.md](../README.md)
- **GitHub Repository**: [https://github.com/jrzesz33/rez_agent](https://github.com/jrzesz33/rez_agent)
- **AWS Console**: [https://console.aws.amazon.com/](https://console.aws.amazon.com/)
- **Pulumi Console**: [https://app.pulumi.com/](https://app.pulumi.com/)

## Documentation Roadmap

Planned documentation additions:

- [ ] MCP Server detailed guide
- [ ] AI Agent configuration guide
- [ ] Security best practices
- [ ] Performance tuning guide
- [ ] Troubleshooting cookbook
- [ ] Video tutorials
- [ ] Interactive API playground

## Feedback

Have suggestions for improving the documentation?

- Open an issue on GitHub
- Submit a pull request
- Contact the maintainers

---

**Last Updated**: 2024-01-15

**Documentation Version**: 1.0

**Maintained By**: [@jrzesz33](https://github.com/jrzesz33)
