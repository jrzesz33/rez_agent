package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/dynamodb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/scheduler"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/sns"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/sqs"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Load configuration
		cfg := config.New(ctx, "")
		stage := cfg.Require("stage")
		ntfyUrl := cfg.Require("ntfyUrl")
		logRetentionDays := cfg.RequireInt("logRetentionDays")
		enableXRay := cfg.RequireBool("enableXRay")
		schedulerCron := cfg.Require("schedulerCron")

		// Common tags
		commonTags := pulumi.StringMap{
			"Project":     pulumi.String("rez-agent"),
			"Stage":       pulumi.String(stage),
			"ManagedBy":   pulumi.String("pulumi"),
			"Environment": pulumi.String(stage),
		}

		// ========================================
		// DynamoDB Table
		// ========================================
		messagesTable, err := dynamodb.NewTable(ctx, fmt.Sprintf("rez-agent-messages-%s", stage), &dynamodb.TableArgs{
			Name:        pulumi.String(fmt.Sprintf("rez-agent-messages-%s", stage)),
			BillingMode: pulumi.String("PAY_PER_REQUEST"),
			HashKey:     pulumi.String("id"),
			RangeKey:    pulumi.String("created_date"),
			Attributes: dynamodb.TableAttributeArray{
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("id"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("created_date"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("status"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("stage"),
					Type: pulumi.String("S"),
				},
			},
			GlobalSecondaryIndexes: dynamodb.TableGlobalSecondaryIndexArray{
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("stage-created_date-index"),
					HashKey:        pulumi.String("stage"),
					RangeKey:       pulumi.String("created_date"),
					ProjectionType: pulumi.String("ALL"),
				},
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("status-created_date-index"),
					HashKey:        pulumi.String("status"),
					RangeKey:       pulumi.String("created_date"),
					ProjectionType: pulumi.String("ALL"),
				},
			},
			Ttl: &dynamodb.TableTtlArgs{
				AttributeName: pulumi.String("ttl"),
				Enabled:       pulumi.Bool(true),
			},
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// ========================================
		// SNS Topic
		// ========================================
		messagesTopic, err := sns.NewTopic(ctx, fmt.Sprintf("rez-agent-messages-%s", stage), &sns.TopicArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-messages-%s", stage)),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// ========================================
		// SQS Queues (Main Queue + DLQ)
		// ========================================
		// Dead Letter Queue
		dlq, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-messages-dlq-%s", stage), &sqs.QueueArgs{
			Name:                    pulumi.String(fmt.Sprintf("rez-agent-messages-dlq-%s", stage)),
			MessageRetentionSeconds: pulumi.Int(1209600), // 14 days
			Tags:                    commonTags,
		})
		if err != nil {
			return err
		}

		// Main Queue
		messagesQueue, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-messages-%s", stage), &sqs.QueueArgs{
			Name:                     pulumi.String(fmt.Sprintf("rez-agent-messages-%s", stage)),
			VisibilityTimeoutSeconds: pulumi.Int(300),     // 5 minutes
			MessageRetentionSeconds:  pulumi.Int(1209600), // 14 days
			RedrivePolicy: dlq.Arn.ApplyT(func(arn string) string {
				return fmt.Sprintf(`{"deadLetterTargetArn":"%s","maxReceiveCount":3}`, arn)
			}).(pulumi.StringOutput),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// SNS to SQS Subscription
		_, err = sns.NewTopicSubscription(ctx, fmt.Sprintf("rez-agent-messages-subscription-%s", stage), &sns.TopicSubscriptionArgs{
			Topic:    messagesTopic.Arn,
			Protocol: pulumi.String("sqs"),
			Endpoint: messagesQueue.Arn,
		})
		if err != nil {
			return err
		}

		// SQS Queue Policy to allow SNS to send messages
		queuePolicy, err := sqs.NewQueuePolicy(ctx, fmt.Sprintf("rez-agent-messages-queue-policy-%s", stage), &sqs.QueuePolicyArgs{
			QueueUrl: messagesQueue.Url,
			Policy: pulumi.All(messagesQueue.Arn, messagesTopic.Arn).ApplyT(func(args []interface{}) string {
				queueArn := args[0].(string)
				topicArn := args[1].(string)
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [{
						"Effect": "Allow",
						"Principal": {"Service": "sns.amazonaws.com"},
						"Action": "sqs:SendMessage",
						"Resource": "%s",
						"Condition": {
							"ArnEquals": {"aws:SourceArn": "%s"}
						}
					}]
				}`, queueArn, topicArn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// ========================================
		// Systems Manager Parameters
		// ========================================
		_, err = ssm.NewParameter(ctx, fmt.Sprintf("rez-agent-ntfy-url-%s", stage), &ssm.ParameterArgs{
			Name:  pulumi.String(fmt.Sprintf("/rez-agent/%s/ntfy-url", stage)),
			Type:  pulumi.String("String"),
			Value: pulumi.String(ntfyUrl),
			Tags:  commonTags,
		})
		if err != nil {
			return err
		}

		// ========================================
		// IAM Roles and Policies
		// ========================================

		// Scheduler Lambda Role
		schedulerRole, err := iam.NewRole(ctx, fmt.Sprintf("rez-agent-scheduler-role-%s", stage), &iam.RoleArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-scheduler-role-%s", stage)),
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Principal": {"Service": "lambda.amazonaws.com"},
					"Action": "sts:AssumeRole"
				}]
			}`),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// Scheduler Lambda Policy
		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("rez-agent-scheduler-policy-%s", stage), &iam.RolePolicyArgs{
			Role: schedulerRole.Name,
			Policy: pulumi.All(messagesTable.Arn, messagesTopic.Arn).ApplyT(func(args []interface{}) string {
				tableArn := args[0].(string)
				topicArn := args[1].(string)
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [
						{
							"Effect": "Allow",
							"Action": [
								"dynamodb:PutItem",
								"dynamodb:UpdateItem"
							],
							"Resource": "%s"
						},
						{
							"Effect": "Allow",
							"Action": ["sns:Publish"],
							"Resource": "%s"
						},
						{
							"Effect": "Allow",
							"Action": [
								"logs:CreateLogGroup",
								"logs:CreateLogStream",
								"logs:PutLogEvents"
							],
							"Resource": "arn:aws:logs:*:*:*"
						},
						{
							"Effect": "Allow",
							"Action": [
								"xray:PutTraceSegments",
								"xray:PutTelemetryRecords"
							],
							"Resource": "*"
						}
					]
				}`, tableArn, topicArn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// Processor Lambda Role
		processorRole, err := iam.NewRole(ctx, fmt.Sprintf("rez-agent-processor-role-%s", stage), &iam.RoleArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-processor-role-%s", stage)),
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Principal": {"Service": "lambda.amazonaws.com"},
					"Action": "sts:AssumeRole"
				}]
			}`),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// Processor Lambda Policy
		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("rez-agent-processor-policy-%s", stage), &iam.RolePolicyArgs{
			Role: processorRole.Name,
			Policy: pulumi.All(messagesTable.Arn, messagesQueue.Arn).ApplyT(func(args []interface{}) string {
				tableArn := args[0].(string)
				queueArn := args[1].(string)
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [
						{
							"Effect": "Allow",
							"Action": [
								"dynamodb:GetItem",
								"dynamodb:UpdateItem",
								"dynamodb:Query"
							],
							"Resource": ["%s", "%s/*"]
						},
						{
							"Effect": "Allow",
							"Action": [
								"sqs:ReceiveMessage",
								"sqs:DeleteMessage",
								"sqs:GetQueueAttributes"
							],
							"Resource": "%s"
						},
						{
							"Effect": "Allow",
							"Action": [
								"ssm:GetParameter",
								"ssm:GetParameters"
							],
							"Resource": "arn:aws:ssm:*:*:parameter/rez-agent/%s/*"
						},
						{
							"Effect": "Allow",
							"Action": [
								"logs:CreateLogGroup",
								"logs:CreateLogStream",
								"logs:PutLogEvents"
							],
							"Resource": "arn:aws:logs:*:*:*"
						},
						{
							"Effect": "Allow",
							"Action": [
								"xray:PutTraceSegments",
								"xray:PutTelemetryRecords"
							],
							"Resource": "*"
						}
					]
				}`, tableArn, tableArn, queueArn, stage)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// WebAPI Lambda Role
		webapiRole, err := iam.NewRole(ctx, fmt.Sprintf("rez-agent-webapi-role-%s", stage), &iam.RoleArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-webapi-role-%s", stage)),
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Principal": {"Service": "lambda.amazonaws.com"},
					"Action": "sts:AssumeRole"
				}]
			}`),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// WebAPI Lambda Policy
		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("rez-agent-webapi-policy-%s", stage), &iam.RolePolicyArgs{
			Role: webapiRole.Name,
			Policy: pulumi.All(messagesTable.Arn, messagesTopic.Arn).ApplyT(func(args []interface{}) string {
				tableArn := args[0].(string)
				topicArn := args[1].(string)
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [
						{
							"Effect": "Allow",
							"Action": [
								"dynamodb:Query",
								"dynamodb:GetItem",
								"dynamodb:PutItem",
								"dynamodb:UpdateItem"
							],
							"Resource": ["%s", "%s/*"]
						},
						{
							"Effect": "Allow",
							"Action": ["sns:Publish"],
							"Resource": "%s"
						},
						{
							"Effect": "Allow",
							"Action": [
								"logs:CreateLogGroup",
								"logs:CreateLogStream",
								"logs:PutLogEvents"
							],
							"Resource": "arn:aws:logs:*:*:*"
						},
						{
							"Effect": "Allow",
							"Action": [
								"xray:PutTraceSegments",
								"xray:PutTelemetryRecords"
							],
							"Resource": "*"
						}
					]
				}`, tableArn, tableArn, topicArn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// ========================================
		// CloudWatch Log Groups
		// ========================================
		schedulerLogGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("rez-agent-scheduler-logs-%s", stage), &cloudwatch.LogGroupArgs{
			Name:            pulumi.String(fmt.Sprintf("/aws/lambda/rez-agent-scheduler-%s", stage)),
			RetentionInDays: pulumi.Int(logRetentionDays),
			Tags:            commonTags,
		})
		if err != nil {
			return err
		}

		processorLogGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("rez-agent-processor-logs-%s", stage), &cloudwatch.LogGroupArgs{
			Name:            pulumi.String(fmt.Sprintf("/aws/lambda/rez-agent-processor-%s", stage)),
			RetentionInDays: pulumi.Int(logRetentionDays),
			Tags:            commonTags,
		})
		if err != nil {
			return err
		}

		webapiLogGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("rez-agent-webapi-logs-%s", stage), &cloudwatch.LogGroupArgs{
			Name:            pulumi.String(fmt.Sprintf("/aws/lambda/rez-agent-webapi-%s", stage)),
			RetentionInDays: pulumi.Int(logRetentionDays),
			Tags:            commonTags,
		})
		if err != nil {
			return err
		}

		// ========================================
		// Lambda Functions
		// ========================================

		// Scheduler Lambda
		schedulerLambda, err := lambda.NewFunction(ctx, fmt.Sprintf("rez-agent-scheduler-%s", stage), &lambda.FunctionArgs{
			Name:    pulumi.String(fmt.Sprintf("rez-agent-scheduler-%s", stage)),
			Runtime: pulumi.String("provided.al2"),
			Role:    schedulerRole.Arn,
			Handler: pulumi.String("bootstrap"),
			Code:    pulumi.NewFileArchive("../build/scheduler.zip"),
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"DYNAMODB_TABLE": messagesTable.Name,
					"SNS_TOPIC_ARN":  messagesTopic.Arn,
					"STAGE":          pulumi.String(stage),
				},
			},
			MemorySize: pulumi.Int(256),
			Timeout:    pulumi.Int(60),
			TracingConfig: &lambda.FunctionTracingConfigArgs{
				Mode: pulumi.String(map[bool]string{true: "Active", false: "PassThrough"}[enableXRay]),
			},
			Tags: commonTags,
		}, pulumi.DependsOn([]pulumi.Resource{schedulerLogGroup}))
		if err != nil {
			return err
		}

		// Processor Lambda
		processorLambda, err := lambda.NewFunction(ctx, fmt.Sprintf("rez-agent-processor-%s", stage), &lambda.FunctionArgs{
			Name:    pulumi.String(fmt.Sprintf("rez-agent-processor-%s", stage)),
			Runtime: pulumi.String("provided.al2"),
			Role:    processorRole.Arn,
			Handler: pulumi.String("bootstrap"),
			Code:    pulumi.NewFileArchive("../build/processor.zip"),
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"DYNAMODB_TABLE": messagesTable.Name,
					"NTFY_URL":       pulumi.String(ntfyUrl),
					"STAGE":          pulumi.String(stage),
				},
			},
			MemorySize: pulumi.Int(512),
			Timeout:    pulumi.Int(300),
			TracingConfig: &lambda.FunctionTracingConfigArgs{
				Mode: pulumi.String(map[bool]string{true: "Active", false: "PassThrough"}[enableXRay]),
			},
			Tags: commonTags,
		}, pulumi.DependsOn([]pulumi.Resource{processorLogGroup}))
		if err != nil {
			return err
		}

		// SQS Event Source Mapping for Processor Lambda
		_, err = lambda.NewEventSourceMapping(ctx, fmt.Sprintf("rez-agent-processor-sqs-trigger-%s", stage), &lambda.EventSourceMappingArgs{
			EventSourceArn: messagesQueue.Arn,
			FunctionName:   processorLambda.Arn,
			BatchSize:      pulumi.Int(10),
			Enabled:        pulumi.Bool(true),
		}, pulumi.DependsOn([]pulumi.Resource{queuePolicy}))
		if err != nil {
			return err
		}

		// WebAPI Lambda
		webapiLambda, err := lambda.NewFunction(ctx, fmt.Sprintf("rez-agent-webapi-%s", stage), &lambda.FunctionArgs{
			Name:    pulumi.String(fmt.Sprintf("rez-agent-webapi-%s", stage)),
			Runtime: pulumi.String("provided.al2"),
			Role:    webapiRole.Arn,
			Handler: pulumi.String("bootstrap"),
			Code:    pulumi.NewFileArchive("../build/webapi.zip"),
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"DYNAMODB_TABLE": messagesTable.Name,
					"SNS_TOPIC_ARN":  messagesTopic.Arn,
					"STAGE":          pulumi.String(stage),
				},
			},
			MemorySize: pulumi.Int(256),
			Timeout:    pulumi.Int(30),
			TracingConfig: &lambda.FunctionTracingConfigArgs{
				Mode: pulumi.String(map[bool]string{true: "Active", false: "PassThrough"}[enableXRay]),
			},
			Tags: commonTags,
		}, pulumi.DependsOn([]pulumi.Resource{webapiLogGroup}))
		if err != nil {
			return err
		}

		// Lambda permission for ALB to invoke WebAPI
		_, err = lambda.NewPermission(ctx, fmt.Sprintf("rez-agent-webapi-alb-permission-%s", stage), &lambda.PermissionArgs{
			Action:    pulumi.String("lambda:InvokeFunction"),
			Function:  webapiLambda.Name,
			Principal: pulumi.String("elasticloadbalancing.amazonaws.com"),
		})
		if err != nil {
			return err
		}

		// ========================================
		// Application Load Balancer
		// ========================================

		// Get default VPC
		defaultVpc, err := ec2.LookupVpc(ctx, &ec2.LookupVpcArgs{
			Default: pulumi.BoolRef(true),
		})
		if err != nil {
			return err
		}

		// Get subnets in default VPC
		subnets, err := ec2.GetSubnets(ctx, &ec2.GetSubnetsArgs{
			Filters: []ec2.GetSubnetsFilter{
				{
					Name:   "vpc-id",
					Values: []string{defaultVpc.Id},
				},
			},
		})
		if err != nil {
			return err
		}

		// Security Group for ALB
		albSecurityGroup, err := ec2.NewSecurityGroup(ctx, fmt.Sprintf("rez-agent-alb-sg-%s", stage), &ec2.SecurityGroupArgs{
			Name:        pulumi.String(fmt.Sprintf("rez-agent-alb-sg-%s", stage)),
			Description: pulumi.String("Security group for rez-agent ALB"),
			VpcId:       pulumi.String(defaultVpc.Id),
			Ingress: ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Protocol:   pulumi.String("tcp"),
					FromPort:   pulumi.Int(80),
					ToPort:     pulumi.Int(80),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
			Egress: ec2.SecurityGroupEgressArray{
				&ec2.SecurityGroupEgressArgs{
					Protocol:   pulumi.String("-1"),
					FromPort:   pulumi.Int(0),
					ToPort:     pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// Application Load Balancer
		alb, err := lb.NewLoadBalancer(ctx, fmt.Sprintf("rez-agent-alb-%s", stage), &lb.LoadBalancerArgs{
			Name:             pulumi.String(fmt.Sprintf("rez-agent-alb-%s", stage)),
			Internal:         pulumi.Bool(false),
			LoadBalancerType: pulumi.String("application"),
			SecurityGroups:   pulumi.StringArray{albSecurityGroup.ID()},
			Subnets:          toPulumiStringArray(subnets.Ids),
			Tags:             commonTags,
		})
		if err != nil {
			return err
		}

		// Target Group for WebAPI Lambda
		webapiTargetGroup, err := lb.NewTargetGroup(ctx, fmt.Sprintf("rez-agent-webapi-tg-%s", stage), &lb.TargetGroupArgs{
			Name:       pulumi.String(fmt.Sprintf("rez-agent-webapi-tg-%s", stage)),
			TargetType: pulumi.String("lambda"),
			HealthCheck: &lb.TargetGroupHealthCheckArgs{
				Enabled:            pulumi.Bool(true),
				Path:               pulumi.String("/api/health"),
				Matcher:            pulumi.String("200"),
				Interval:           pulumi.Int(30),
				Timeout:            pulumi.Int(5),
				HealthyThreshold:   pulumi.Int(2),
				UnhealthyThreshold: pulumi.Int(2),
			},
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// Attach WebAPI Lambda to Target Group
		_, err = lb.NewTargetGroupAttachment(ctx, fmt.Sprintf("rez-agent-webapi-tg-attachment-%s", stage), &lb.TargetGroupAttachmentArgs{
			TargetGroupArn: webapiTargetGroup.Arn,
			TargetId:       webapiLambda.Arn,
		})
		if err != nil {
			return err
		}

		// ALB Listener
		_, err = lb.NewListener(ctx, fmt.Sprintf("rez-agent-alb-listener-%s", stage), &lb.ListenerArgs{
			LoadBalancerArn: alb.Arn,
			Port:            pulumi.Int(80),
			Protocol:        pulumi.String("HTTP"),
			DefaultActions: lb.ListenerDefaultActionArray{
				&lb.ListenerDefaultActionArgs{
					Type:           pulumi.String("forward"),
					TargetGroupArn: webapiTargetGroup.Arn,
				},
			},
		})
		if err != nil {
			return err
		}

		// ========================================
		// EventBridge Scheduler
		// ========================================

		// EventBridge Scheduler Role
		schedulerExecutionRole, err := iam.NewRole(ctx, fmt.Sprintf("rez-agent-eventbridge-scheduler-role-%s", stage), &iam.RoleArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-eventbridge-scheduler-role-%s", stage)),
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Principal": {"Service": "scheduler.amazonaws.com"},
					"Action": "sts:AssumeRole"
				}]
			}`),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// EventBridge Scheduler Policy
		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("rez-agent-eventbridge-scheduler-policy-%s", stage), &iam.RolePolicyArgs{
			Role: schedulerExecutionRole.Name,
			Policy: schedulerLambda.Arn.ApplyT(func(arn string) string {
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [{
						"Effect": "Allow",
						"Action": "lambda:InvokeFunction",
						"Resource": "%s"
					}]
				}`, arn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// EventBridge Scheduler
		_, err = scheduler.NewSchedule(ctx, fmt.Sprintf("rez-agent-daily-scheduler-%s", stage), &scheduler.ScheduleArgs{
			Name:               pulumi.String(fmt.Sprintf("rez-agent-daily-scheduler-%s", stage)),
			ScheduleExpression: pulumi.String(schedulerCron),
			FlexibleTimeWindow: &scheduler.ScheduleFlexibleTimeWindowArgs{
				Mode: pulumi.String("OFF"),
			},
			Target: &scheduler.ScheduleTargetArgs{
				Arn:     schedulerLambda.Arn,
				RoleArn: schedulerExecutionRole.Arn,
				RetryPolicy: &scheduler.ScheduleTargetRetryPolicyArgs{
					MaximumRetryAttempts:     pulumi.Int(3),
					MaximumEventAgeInSeconds: pulumi.Int(3600),
				},
			},
		})
		if err != nil {
			return err
		}

		// ========================================
		// CloudWatch Alarms
		// ========================================

		// DLQ Alarm
		_, err = cloudwatch.NewMetricAlarm(ctx, fmt.Sprintf("rez-agent-dlq-alarm-%s", stage), &cloudwatch.MetricAlarmArgs{
			Name:               pulumi.String(fmt.Sprintf("rez-agent-dlq-messages-%s", stage)),
			ComparisonOperator: pulumi.String("GreaterThanThreshold"),
			EvaluationPeriods:  pulumi.Int(1),
			MetricName:         pulumi.String("ApproximateNumberOfMessagesVisible"),
			Namespace:          pulumi.String("AWS/SQS"),
			Period:             pulumi.Int(300),
			Statistic:          pulumi.String("Average"),
			Threshold:          pulumi.Float64(1),
			AlarmDescription:   pulumi.String("Alert when messages appear in DLQ"),
			Dimensions: pulumi.StringMap{
				"QueueName": dlq.Name,
			},
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// Processor Lambda Error Alarm
		_, err = cloudwatch.NewMetricAlarm(ctx, fmt.Sprintf("rez-agent-processor-errors-%s", stage), &cloudwatch.MetricAlarmArgs{
			Name:               pulumi.String(fmt.Sprintf("rez-agent-processor-errors-%s", stage)),
			ComparisonOperator: pulumi.String("GreaterThanThreshold"),
			EvaluationPeriods:  pulumi.Int(2),
			MetricName:         pulumi.String("Errors"),
			Namespace:          pulumi.String("AWS/Lambda"),
			Period:             pulumi.Int(300),
			Statistic:          pulumi.String("Sum"),
			Threshold:          pulumi.Float64(5),
			AlarmDescription:   pulumi.String("Alert when processor Lambda has errors"),
			Dimensions: pulumi.StringMap{
				"FunctionName": processorLambda.Name,
			},
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// ========================================
		// Exports
		// ========================================
		ctx.Export("dynamodbTableName", messagesTable.Name)
		ctx.Export("dynamodbTableArn", messagesTable.Arn)
		ctx.Export("snsTopicArn", messagesTopic.Arn)
		ctx.Export("sqsQueueUrl", messagesQueue.Url)
		ctx.Export("sqsQueueArn", messagesQueue.Arn)
		ctx.Export("dlqUrl", dlq.Url)
		ctx.Export("dlqArn", dlq.Arn)
		ctx.Export("schedulerLambdaArn", schedulerLambda.Arn)
		ctx.Export("processorLambdaArn", processorLambda.Arn)
		ctx.Export("webapiLambdaArn", webapiLambda.Arn)
		ctx.Export("albDnsName", alb.DnsName)
		ctx.Export("albArn", alb.Arn)
		ctx.Export("webapiUrl", alb.DnsName.ApplyT(func(dns string) string {
			return fmt.Sprintf("http://%s", dns)
		}).(pulumi.StringOutput))

		return nil
	})
}

// Helper function to convert []string to pulumi.StringArray
func toPulumiStringArray(strs []string) pulumi.StringArray {
	result := make(pulumi.StringArray, len(strs))
	for i, s := range strs {
		result[i] = pulumi.String(s)
	}
	return result
}
