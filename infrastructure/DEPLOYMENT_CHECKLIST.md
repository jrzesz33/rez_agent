# rez_agent Infrastructure Deployment Checklist

Use this checklist to ensure a successful deployment of the rez_agent infrastructure.

## Pre-Deployment Checklist

### Prerequisites

- [ ] Go 1.24+ installed (`go version`)
- [ ] Pulumi CLI installed (`pulumi version`)
- [ ] AWS CLI installed and configured (`aws --version`)
- [ ] Make installed (`make --version`)
- [ ] Docker installed (optional, for local testing)

### AWS Setup

- [ ] AWS account created
- [ ] AWS credentials configured (`aws configure` or environment variables)
- [ ] IAM user/role has required permissions:
  - [ ] DynamoDB (create tables, manage GSIs)
  - [ ] Lambda (create functions, manage permissions)
  - [ ] IAM (create roles and policies)
  - [ ] SNS/SQS (create topics and queues)
  - [ ] EventBridge (create schedules)
  - [ ] CloudWatch (create log groups and alarms)
  - [ ] EC2 (for VPC and security groups)
  - [ ] ELB (create ALBs and target groups)
  - [ ] Systems Manager (create parameters)
- [ ] AWS region selected (default: us-east-1)

### Pulumi Setup

- [ ] Pulumi account created (if using Pulumi Cloud)
- [ ] Pulumi logged in (`pulumi login`)
- [ ] Pulumi stack created (`pulumi stack init dev`)
- [ ] Stack configuration set (see Configuration section below)

## Configuration Checklist

### Required Configuration

```bash
cd infrastructure

# AWS Configuration
pulumi config set aws:region us-east-1

# rez_agent Configuration
pulumi config set stage dev
pulumi config set ntfyUrl https://ntfy.sh/rzesz-alerts
pulumi config set logRetentionDays 7
pulumi config set enableXRay true
pulumi config set schedulerCron "cron(0 12 * * ? *)"
```

### Verify Configuration

- [ ] Configuration file exists (`Pulumi.dev.yaml` or `Pulumi.prod.yaml`)
- [ ] All required config values are set (`pulumi config`)
- [ ] AWS region matches your AWS CLI region
- [ ] Stage name is correct (dev/prod)
- [ ] ntfy.sh URL is accessible
- [ ] Cron expression is valid

## Build Checklist

- [ ] Project dependencies downloaded (`make dev-env`)
- [ ] Lambda functions build successfully (`make build`)
- [ ] Build artifacts created in `build/` directory:
  - [ ] `build/scheduler.zip`
  - [ ] `build/processor.zip`
  - [ ] `build/webapi.zip`
- [ ] Build artifact sizes are reasonable (<50MB each)

## Deployment Checklist

### Pre-Deployment Verification

- [ ] Preview deployment changes (`make infra-preview`)
- [ ] Review resources to be created
- [ ] Confirm no unexpected deletions or replacements
- [ ] Estimated costs are acceptable

### Deployment

- [ ] Deploy infrastructure (`make deploy-dev` or `make deploy-prod`)
- [ ] Deployment completes without errors
- [ ] All resources created successfully
- [ ] Stack outputs are generated

### Post-Deployment Verification

- [ ] View stack outputs (`make infra-outputs`)
- [ ] Verify all expected outputs are present:
  - [ ] dynamodbTableName
  - [ ] snsTopicArn
  - [ ] sqsQueueUrl
  - [ ] schedulerLambdaArn
  - [ ] processorLambdaArn
  - [ ] webapiLambdaArn
  - [ ] albDnsName
  - [ ] webapiUrl

## Testing Checklist

### Infrastructure Testing

- [ ] DynamoDB table exists and is accessible
  ```bash
  aws dynamodb describe-table --table-name $(pulumi stack output dynamodbTableName --cwd infrastructure)
  ```

- [ ] SNS topic exists
  ```bash
  aws sns get-topic-attributes --topic-arn $(pulumi stack output snsTopicArn --cwd infrastructure)
  ```

- [ ] SQS queue exists
  ```bash
  aws sqs get-queue-attributes --queue-url $(pulumi stack output sqsQueueUrl --cwd infrastructure) --attribute-names All
  ```

- [ ] Lambda functions are active
  ```bash
  aws lambda get-function --function-name rez-agent-scheduler-dev
  aws lambda get-function --function-name rez-agent-processor-dev
  aws lambda get-function --function-name rez-agent-webapi-dev
  ```

- [ ] EventBridge schedule exists
  ```bash
  aws scheduler get-schedule --name rez-agent-daily-scheduler-dev
  ```

- [ ] ALB is active and healthy
  ```bash
  aws elbv2 describe-load-balancers --names rez-agent-alb-dev
  ```

### Functional Testing

- [ ] WebAPI health endpoint responds
  ```bash
  curl $(pulumi stack output webapiUrl --cwd infrastructure)/api/health
  ```
  Expected: `{"status":"healthy","timestamp":"..."}`

- [ ] Manually invoke Scheduler Lambda
  ```bash
  aws lambda invoke --function-name rez-agent-scheduler-dev response.json
  cat response.json
  ```
  Expected: Success response, no errors

- [ ] Verify message created in DynamoDB
  ```bash
  aws dynamodb scan --table-name $(pulumi stack output dynamodbTableName --cwd infrastructure) --limit 1
  ```

- [ ] Verify message published to SNS/SQS
  ```bash
  aws sqs receive-message --queue-url $(pulumi stack output sqsQueueUrl --cwd infrastructure)
  ```

- [ ] Check CloudWatch Logs
  ```bash
  make lambda-logs-scheduler
  make lambda-logs-processor
  make lambda-logs-webapi
  ```

### Monitoring Testing

- [ ] CloudWatch Log Groups exist
  ```bash
  aws logs describe-log-groups --log-group-name-prefix /aws/lambda/rez-agent
  ```

- [ ] CloudWatch Alarms are configured
  ```bash
  aws cloudwatch describe-alarms --alarm-name-prefix rez-agent
  ```

- [ ] X-Ray tracing is working (if enabled)
  ```bash
  aws xray get-trace-summaries --start-time $(date -u -d '1 hour ago' +%s) --end-time $(date -u +%s)
  ```

## Production Deployment Checklist

### Additional Production Considerations

- [ ] Change stack to production (`pulumi stack select prod`)
- [ ] Update configuration for production:
  - [ ] `pulumi config set stage prod`
  - [ ] `pulumi config set logRetentionDays 30`
  - [ ] `pulumi config set ntfyUrl https://ntfy.sh/rzesz-alerts` (production URL)
- [ ] Review IAM permissions for least privilege
- [ ] Configure SNS topic for alarm notifications
- [ ] Set up CloudWatch Dashboard
- [ ] Enable CloudWatch detailed monitoring
- [ ] Consider Reserved Concurrency for Lambda functions
- [ ] Set up backup strategy for DynamoDB
- [ ] Enable DynamoDB Point-in-Time Recovery (PITR)
- [ ] Configure VPC endpoints (if using VPC)
- [ ] Set up HTTPS with ACM certificate for ALB
- [ ] Configure custom domain name
- [ ] Set up CI/CD pipeline
- [ ] Document runbooks for common operations
- [ ] Perform load testing
- [ ] Set up monitoring alerts (PagerDuty, Slack, etc.)
- [ ] Review and optimize costs
- [ ] Plan for disaster recovery

## Rollback Checklist

If deployment fails or issues are discovered:

- [ ] Check Pulumi deployment logs
- [ ] Review CloudWatch logs for errors
- [ ] Identify failed resources
- [ ] Rollback to previous version
  ```bash
  cd infrastructure
  pulumi stack history
  pulumi stack rollback
  ```
- [ ] Or destroy and redeploy
  ```bash
  make infra-destroy
  make deploy-dev
  ```

## Cleanup Checklist

To destroy all infrastructure:

- [ ] Backup any important data from DynamoDB
- [ ] Confirm you want to destroy everything
- [ ] Run destroy command
  ```bash
  make infra-destroy
  ```
- [ ] Verify all resources are deleted
  ```bash
  aws dynamodb list-tables | grep rez-agent
  aws lambda list-functions | grep rez-agent
  aws sns list-topics | grep rez-agent
  aws sqs list-queues | grep rez-agent
  ```
- [ ] Check for any orphaned resources in AWS Console
- [ ] Remove Pulumi stack
  ```bash
  cd infrastructure
  pulumi stack rm dev  # or prod
  ```

## Troubleshooting Reference

### Common Issues

| Issue | Solution |
|-------|----------|
| Build fails | `go mod tidy && make clean && make build` |
| Pulumi fails | Check AWS credentials: `aws sts get-caller-identity` |
| Lambda deploy fails | Verify ZIP file size < 50MB |
| ALB health check fails | Check `/api/health` endpoint in Lambda logs |
| EventBridge not triggering | Verify IAM role permissions |
| SQS messages stuck | Check processor Lambda logs and DLQ |
| No logs in CloudWatch | Wait 1-2 minutes for log propagation |
| High costs | Review CloudWatch Logs retention and ALB usage |

### Getting Help

1. Review documentation:
   - `/workspaces/rez_agent/infrastructure/README.md`
   - `/workspaces/rez_agent/INFRASTRUCTURE.md`
   - `/workspaces/rez_agent/docs/architecture/README.md`

2. Check AWS Console:
   - CloudWatch Logs for error messages
   - Lambda function configuration
   - DynamoDB table status
   - IAM role permissions

3. Use Pulumi diagnostics:
   ```bash
   pulumi stack --show-ids
   pulumi stack export > stack-backup.json
   pulumi preview -v=3
   ```

4. Community resources:
   - Pulumi Community: https://www.pulumi.com/community/
   - AWS Forums: https://forums.aws.amazon.com/

## Deployment Success Criteria

Your deployment is successful when:

- [ ] All infrastructure resources are created without errors
- [ ] WebAPI health endpoint returns 200 OK
- [ ] Scheduler Lambda can be invoked manually
- [ ] Messages appear in DynamoDB after scheduler runs
- [ ] Processor Lambda processes messages from SQS
- [ ] CloudWatch Logs show activity from all Lambdas
- [ ] No alarms are triggered
- [ ] All tests pass
- [ ] Costs are within expected range
- [ ] Documentation is updated with any customizations

## Next Steps After Deployment

- [ ] Set up monitoring dashboard
- [ ] Configure alerting to Slack/PagerDuty
- [ ] Document custom configurations
- [ ] Train team on operations
- [ ] Set up CI/CD pipeline
- [ ] Plan for scaling and optimization
- [ ] Schedule regular reviews and updates

---

**Deployment Date**: _______________
**Deployed By**: _______________
**Environment**: [ ] Dev [ ] Staging [ ] Production
**Pulumi Stack**: _______________
**AWS Region**: _______________
**Notes**:
_______________________________________________________________
_______________________________________________________________
_______________________________________________________________
