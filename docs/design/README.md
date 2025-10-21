# Rez Agent Design

The rez-agent application is an event driven system that processes messages and either completes a task or produces more messages. 

## Initial Implementation
1. The system shall have a scheduled event every 24 hours to send a "hello world" message to the ntfy.sh
2. The system shall have a concept of keeping metadata with messages including:
    - created date: date message was created
    - created by: system that created the message
    - stage : references the target environment the message is in (dev, stage, prod)
3. A front end application shall also be available for administration purposes including:
    - Display metrics on messages 
    - Create a New Message Manually
    - Authenticate Users with OAuth2

## Integrations
- ntfy.sh
    Push notifications should be sent to https://ntfy.sh/rzesz-alerts when referencing notifications for the system
- Amazon EventBridge Scheduler
    To schedule a job to run every 24 hours initially but designed for more jobs and services to run in the future.
- Amazon SQS Queue and SNS Topic
    To queue and process messages, these services will provide the messaging layer.

## Infrastructure Specifications

- Pulumi shall be used as the Infrastructure Automation Layer to Deploy, Configure, and Maintain:
    - Amazon SQL/SNS
    - Amazon EventBridge Scheduler
    - LAMBDA for the services tier
    - LAMBDA for the Front End Application
    - Load Balancer for Front End Application to be accessed


## Development Specifications
- Go Lang should be utilized as the Primary Language unless its not available.
- Unit Tests should Be Developed

## Deployment
- Github Actions and Pulumi should be used for Deployment
