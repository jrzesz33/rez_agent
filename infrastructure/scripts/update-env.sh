cd /workspaces/rez_agent/infrastructure
touch .env
echo "STAGE=dev" >> .env
echo "DYNAMODB_TABLE_NAME=$(pulumi stack output dynamodbTableName)" >> .env
echo "AGENT_SESSION_TABLE_NAME=$(pulumi stack output agentSessionTableName)" >> .env
echo "AGENT_RESPONSE_TOPIC_ARN=$(pulumi stack output agentResponseTopicArn)" >> .env
echo "AGENT_RESPONSE_QUEUE_URL=$(pulumi stack output agentResponseQueueUrl)" >> .env
echo "MCP_SERVER_URL=$(pulumi stack output webapiUrl)/mcp" >> .env
echo "BEDROCK_MODEL_ID=amazon.nova-lite-v1:0" >> .env
echo "EVENTBRIDGE_EXECUTION_ROLE_ARN=$(pulumi stack output eventBridgeSchedulerExecutionRoleArn)" >> .env
echo "NOTIFICATIONS_TOPIC_ARN=$(pulumi stack output notificationsTopicArn)" >> .env
echo "NOTIFICATION_SQS_QUEUE_URL=$(pulumi stack output notificationsQueueUrl)" >> .env
echo "SCHEDULES_TABLE_NAME=$(pulumi stack output schedulesTableName)" >> .env
echo "SCHEDULE_CREATION_TOPIC_ARN=$(pulumi stack output scheduleCreationTopicArn)" >> .env
echo "WEB_ACTIONS_TOPIC_ARN=$(pulumi stack output webActionsTopicArn)" >> .env
echo "WEB_ACTION_SQS_QUEUE_URL=$(pulumi stack output webActionsQueueUrl)" >> .env
echo "WEBAPI_URL=$(pulumi stack output webapiUrl)" >> .env
echo "SCHEDULE_CREATION_QUEUE_URL=$(pulumi stack output scheduleCreationQueueUrl)" >> .env
mv .env ../.env

cd ..
source .env

echo "Environment variables updated in .env"