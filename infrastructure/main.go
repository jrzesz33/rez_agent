package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/dynamodb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
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
			// Note: Removed RangeKey to simplify operations since id is unique
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
		// DynamoDB Table for Web Action Results
		// ========================================
		webActionResultsTable, err := dynamodb.NewTable(ctx, fmt.Sprintf("rez-agent-web-action-results-%s", stage), &dynamodb.TableArgs{
			Name:        pulumi.String(fmt.Sprintf("rez-agent-web-action-results-%s", stage)),
			BillingMode: pulumi.String("PAY_PER_REQUEST"),
			HashKey:     pulumi.String("id"),
			Attributes: dynamodb.TableAttributeArray{
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("id"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("message_id"),
					Type: pulumi.String("S"),
				},
			},
			GlobalSecondaryIndexes: dynamodb.TableGlobalSecondaryIndexArray{
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("message_id-index"),
					HashKey:        pulumi.String("message_id"),
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
		// SNS Topics (Topic-based routing)
		// ========================================

		// Web Actions Topic
		webActionsTopic, err := sns.NewTopic(ctx, fmt.Sprintf("rez-agent-web-actions-%s", stage), &sns.TopicArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-web-actions-%s", stage)),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// Notifications Topic (for scheduler, manual messages, etc.)
		notificationsTopic, err := sns.NewTopic(ctx, fmt.Sprintf("rez-agent-notifications-%s", stage), &sns.TopicArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-notifications-%s", stage)),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// ========================================
		// SQS Queues (Separate queues per message type)
		// ========================================

		// Dead Letter Queues
		webActionsDlq, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-web-actions-dlq-%s", stage), &sqs.QueueArgs{
			Name:                    pulumi.String(fmt.Sprintf("rez-agent-web-actions-dlq-%s", stage)),
			MessageRetentionSeconds: pulumi.Int(1209600), // 14 days
			Tags:                    commonTags,
		})
		if err != nil {
			return err
		}

		notificationsDlq, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-notifications-dlq-%s", stage), &sqs.QueueArgs{
			Name:                    pulumi.String(fmt.Sprintf("rez-agent-notifications-dlq-%s", stage)),
			MessageRetentionSeconds: pulumi.Int(1209600), // 14 days
			Tags:                    commonTags,
		})
		if err != nil {
			return err
		}

		// Web Actions Queue
		webActionsQueue, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-web-actions-%s", stage), &sqs.QueueArgs{
			Name:                     pulumi.String(fmt.Sprintf("rez-agent-web-actions-%s", stage)),
			VisibilityTimeoutSeconds: pulumi.Int(300),     // 5 minutes
			MessageRetentionSeconds:  pulumi.Int(1209600), // 14 days
			RedrivePolicy: webActionsDlq.Arn.ApplyT(func(arn string) string {
				return fmt.Sprintf(`{"deadLetterTargetArn":"%s","maxReceiveCount":3}`, arn)
			}).(pulumi.StringOutput),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// Notifications Queue
		notificationsQueue, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-notifications-%s", stage), &sqs.QueueArgs{
			Name:                     pulumi.String(fmt.Sprintf("rez-agent-notifications-%s", stage)),
			VisibilityTimeoutSeconds: pulumi.Int(300),     // 5 minutes
			MessageRetentionSeconds:  pulumi.Int(1209600), // 14 days
			RedrivePolicy: notificationsDlq.Arn.ApplyT(func(arn string) string {
				return fmt.Sprintf(`{"deadLetterTargetArn":"%s","maxReceiveCount":3}`, arn)
			}).(pulumi.StringOutput),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// ========================================
		// SNS to SQS Subscriptions
		// ========================================

		// Web Actions Topic -> Web Actions Queue
		_, err = sns.NewTopicSubscription(ctx, fmt.Sprintf("rez-agent-web-actions-subscription-%s", stage), &sns.TopicSubscriptionArgs{
			Topic:              webActionsTopic.Arn,
			Protocol:           pulumi.String("sqs"),
			Endpoint:           webActionsQueue.Arn,
			RawMessageDelivery: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		// Notifications Topic -> Notifications Queue
		_, err = sns.NewTopicSubscription(ctx, fmt.Sprintf("rez-agent-notifications-subscription-%s", stage), &sns.TopicSubscriptionArgs{
			Topic:              notificationsTopic.Arn,
			Protocol:           pulumi.String("sqs"),
			Endpoint:           notificationsQueue.Arn,
			RawMessageDelivery: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		// ========================================
		// SQS Queue Policies
		// ========================================

		// Web Actions Queue Policy
		qPolicy, err := sqs.NewQueuePolicy(ctx, fmt.Sprintf("rez-agent-web-actions-queue-policy-%s", stage), &sqs.QueuePolicyArgs{
			QueueUrl: webActionsQueue.Url,
			Policy: pulumi.All(webActionsQueue.Arn, webActionsTopic.Arn).ApplyT(func(args []interface{}) string {
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

		// Notifications Queue Policy
		nPolicy, err := sqs.NewQueuePolicy(ctx, fmt.Sprintf("rez-agent-notifications-queue-policy-%s", stage), &sqs.QueuePolicyArgs{
			QueueUrl: notificationsQueue.Url,
			Policy: pulumi.All(notificationsQueue.Arn, notificationsTopic.Arn).ApplyT(func(args []interface{}) string {
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
			Policy: pulumi.All(messagesTable.Arn, notificationsTopic.Arn).ApplyT(func(args []interface{}) string {
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
			Policy: pulumi.All(messagesTable.Arn, notificationsQueue.Arn).ApplyT(func(args []interface{}) string {
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
			Policy: pulumi.All(messagesTable.Arn, webActionsTopic.Arn, notificationsTopic.Arn).ApplyT(func(args []interface{}) string {
				tableArn := args[0].(string)
				webActionsTopicArn := args[1].(string)
				notificationsTopicArn := args[2].(string)
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
							"Resource": ["%s", "%s"]
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
				}`, tableArn, tableArn, webActionsTopicArn, notificationsTopicArn)
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
					"DYNAMODB_TABLE_NAME":        messagesTable.Name,
					"WEB_ACTIONS_TOPIC_ARN":      webActionsTopic.Arn,    // Topic-based routing
					"NOTIFICATIONS_TOPIC_ARN":    notificationsTopic.Arn, // Topic-based routing
					"WEB_ACTION_SQS_QUEUE_URL":   webActionsQueue.Url,
					"NOTIFICATION_SQS_QUEUE_URL": notificationsQueue.Url,
					"STAGE":                      pulumi.String(stage),
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
					"DYNAMODB_TABLE_NAME":        messagesTable.Name,
					"WEB_ACTIONS_TOPIC_ARN":      webActionsTopic.Arn,    // Topic-based routing
					"NOTIFICATIONS_TOPIC_ARN":    notificationsTopic.Arn, // Topic-based routing
					"WEB_ACTION_SQS_QUEUE_URL":   webActionsQueue.Url,
					"NOTIFICATION_SQS_QUEUE_URL": notificationsQueue.Url,
					"NTFY_URL":                   pulumi.String(ntfyUrl),
					"STAGE":                      pulumi.String(stage),
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

		// SQS Event Source Mapping for Processor Lambda (Notifications Queue)
		_, err = lambda.NewEventSourceMapping(ctx, fmt.Sprintf("rez-agent-processor-sqs-trigger-%s", stage), &lambda.EventSourceMappingArgs{
			EventSourceArn: notificationsQueue.Arn,
			FunctionName:   processorLambda.Arn,
			BatchSize:      pulumi.Int(10),
			Enabled:        pulumi.Bool(true),
			// No filter criteria needed - dedicated queue for notifications
		}, pulumi.DependsOn([]pulumi.Resource{nPolicy}))
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
					"DYNAMODB_TABLE_NAME":        messagesTable.Name,
					"WEB_ACTIONS_TOPIC_ARN":      webActionsTopic.Arn,    // Topic-based routing
					"NOTIFICATIONS_TOPIC_ARN":    notificationsTopic.Arn, // Topic-based routing
					"WEB_ACTION_SQS_QUEUE_URL":   webActionsQueue.Url,
					"NOTIFICATION_SQS_QUEUE_URL": notificationsQueue.Url,
					"STAGE":                      pulumi.String(stage),
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

		// ========================================
		// WebAction Lambda
		// ========================================

		// WebAction Lambda Role
		webactionRole, err := iam.NewRole(ctx, fmt.Sprintf("rez-agent-webaction-role-%s", stage), &iam.RoleArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-webaction-role-%s", stage)),
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

		// Attach basic Lambda execution policy
		_, err = iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("rez-agent-webaction-basic-execution-%s", stage), &iam.RolePolicyAttachmentArgs{
			Role:      webactionRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		})
		if err != nil {
			return err
		}

		// WebAction Lambda Policy
		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("rez-agent-webaction-policy-%s", stage), &iam.RolePolicyArgs{
			Role: webactionRole.Name,
			Policy: pulumi.All(messagesTable.Arn, webActionResultsTable.Arn, webActionsQueue.Arn, webActionsTopic.Arn, notificationsQueue.Arn, notificationsTopic.Arn).ApplyT(func(args []interface{}) string {
				tableArn := args[0].(string)
				webActionResultsArn := args[1].(string)
				waQueueArn := args[2].(string)
				waTtopicArn := args[3].(string)
				noQueueArn := args[4].(string)
				noTtopicArn := args[5].(string)
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [
						{
							"Effect": "Allow",
							"Action": [
								"dynamodb:GetItem",
								"dynamodb:PutItem",
								"dynamodb:UpdateItem"
							],
							"Resource": [
								"%s",
								"%s/*"
							]
						},
						{
							"Effect": "Allow",
							"Action": [
								"dynamodb:PutItem",
								"dynamodb:GetItem",
								"dynamodb:Query"
							],
							"Resource": [
								"%s",
								"%s/*"
							]
						},
						{
							"Effect": "Allow",
							"Action": [
								"sqs:ReceiveMessage",
								"sqs:DeleteMessage",
								"sqs:GetQueueAttributes"
							],
							"Resource": ["%s","%s"]
						},
						{
							"Effect": "Allow",
							"Action": ["sns:Publish"],
							"Resource": ["%s","%s"]
						},
						{
							"Effect": "Allow",
							"Action": [
								"secretsmanager:GetSecretValue"
							],
							"Resource": "arn:aws:secretsmanager:*:*:secret:rez-agent/*"
						}
					]
				}`, tableArn, tableArn, webActionResultsArn, webActionResultsArn, waQueueArn, noQueueArn, waTtopicArn, noTtopicArn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// WebAction Lambda Log Group
		webactionLogGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("rez-agent-webaction-logs-%s", stage), &cloudwatch.LogGroupArgs{
			Name:            pulumi.String(fmt.Sprintf("/aws/lambda/rez-agent-webaction-%s", stage)),
			RetentionInDays: pulumi.Int(logRetentionDays),
			Tags:            commonTags,
		})
		if err != nil {
			return err
		}

		// WebAction Lambda Function
		webactionLambda, err := lambda.NewFunction(ctx, fmt.Sprintf("rez-agent-webaction-%s", stage), &lambda.FunctionArgs{
			Name:    pulumi.String(fmt.Sprintf("rez-agent-webaction-%s", stage)),
			Runtime: pulumi.String("provided.al2"),
			Role:    webactionRole.Arn,
			Handler: pulumi.String("bootstrap"),
			Code:    pulumi.NewFileArchive("../build/webaction.zip"),
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"DYNAMODB_TABLE_NAME":        messagesTable.Name,
					"WEB_ACTIONS_TOPIC_ARN":      webActionsTopic.Arn,    // Topic-based routing
					"NOTIFICATIONS_TOPIC_ARN":    notificationsTopic.Arn, // Topic-based routing
					"WEB_ACTION_SQS_QUEUE_URL":   webActionsQueue.Url,
					"NOTIFICATION_SQS_QUEUE_URL": notificationsQueue.Url,
					"STAGE":                      pulumi.String(stage),
					"GOLF_SECRET_NAME":           pulumi.String(fmt.Sprintf("rez-agent/golf/credentials-%s", stage)),
				},
			},
			MemorySize: pulumi.Int(512),
			Timeout:    pulumi.Int(300),
			TracingConfig: &lambda.FunctionTracingConfigArgs{
				Mode: pulumi.String(map[bool]string{true: "Active", false: "PassThrough"}[enableXRay]),
			},
			Tags: commonTags,
		}, pulumi.DependsOn([]pulumi.Resource{webactionLogGroup}))
		if err != nil {
			return err
		}

		// WebAction Lambda SQS Event Source Mapping (Web Actions Queue)
		_, err = lambda.NewEventSourceMapping(ctx, fmt.Sprintf("rez-agent-webaction-sqs-trigger-%s", stage), &lambda.EventSourceMappingArgs{
			EventSourceArn: webActionsQueue.Arn,
			FunctionName:   webactionLambda.Arn,
			BatchSize:      pulumi.Int(1),
			Enabled:        pulumi.Bool(true),
			// No filter criteria needed - dedicated queue for web actions
		}, pulumi.DependsOn([]pulumi.Resource{qPolicy}))
		if err != nil {
			return err
		}

		// ========================================
		// API Gateway HTTP API
		// ========================================

		// API Gateway HTTP API
		httpApi, err := apigatewayv2.NewApi(ctx, fmt.Sprintf("rez-agent-api-%s", stage), &apigatewayv2.ApiArgs{
			Name:         pulumi.String(fmt.Sprintf("rez-agent-api-%s", stage)),
			ProtocolType: pulumi.String("HTTP"),
			Description:  pulumi.String("HTTP API for rez-agent web interface"),
			Tags:         commonTags,
		})
		if err != nil {
			return err
		}

		// Lambda permission for API Gateway to invoke WebAPI
		_, err = lambda.NewPermission(ctx, fmt.Sprintf("rez-agent-webapi-apigw-permission-%s", stage), &lambda.PermissionArgs{
			Action:    pulumi.String("lambda:InvokeFunction"),
			Function:  webapiLambda.Name,
			Principal: pulumi.String("apigateway.amazonaws.com"),
			SourceArn: httpApi.ExecutionArn.ApplyT(func(arn string) string {
				return fmt.Sprintf("%s/*/*", arn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// API Gateway Integration
		httpApiIntegration, err := apigatewayv2.NewIntegration(ctx, fmt.Sprintf("rez-agent-api-integration-%s", stage), &apigatewayv2.IntegrationArgs{
			ApiId:                httpApi.ID(),
			IntegrationType:      pulumi.String("AWS_PROXY"),
			IntegrationUri:       webapiLambda.Arn,
			IntegrationMethod:    pulumi.String("POST"),
			PayloadFormatVersion: pulumi.String("2.0"),
		})
		if err != nil {
			return err
		}

		// API Gateway Route (catch-all)
		_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("rez-agent-api-route-%s", stage), &apigatewayv2.RouteArgs{
			ApiId:    httpApi.ID(),
			RouteKey: pulumi.String("$default"),
			Target: httpApiIntegration.ID().ApplyT(func(id string) string {
				return fmt.Sprintf("integrations/%s", id)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// API Gateway Stage (auto-deploy)
		_, err = apigatewayv2.NewStage(ctx, fmt.Sprintf("rez-agent-api-stage-%s", stage), &apigatewayv2.StageArgs{
			ApiId:      httpApi.ID(),
			Name:       pulumi.String("$default"),
			AutoDeploy: pulumi.Bool(true),
			AccessLogSettings: &apigatewayv2.StageAccessLogSettingsArgs{
				DestinationArn: webapiLogGroup.Arn,
				Format:         pulumi.String(`{"requestId":"$context.requestId","ip":"$context.identity.sourceIp","requestTime":"$context.requestTime","httpMethod":"$context.httpMethod","routeKey":"$context.routeKey","status":"$context.status","protocol":"$context.protocol","responseLength":"$context.responseLength"}`),
			},
			Tags: commonTags,
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
		// Stack Outputs
		// ========================================

		// DynamoDB
		ctx.Export("dynamodbTableName", messagesTable.Name)
		ctx.Export("dynamodbTableArn", messagesTable.Arn)

		// SNS Topics
		ctx.Export("webActionsTopicArn", webActionsTopic.Arn)
		ctx.Export("notificationsTopicArn", notificationsTopic.Arn)

		// SQS Queues
		ctx.Export("webActionsQueueUrl", webActionsQueue.Url)
		ctx.Export("webActionsQueueArn", webActionsQueue.Arn)
		ctx.Export("notificationsQueueUrl", notificationsQueue.Url)
		ctx.Export("notificationsQueueArn", notificationsQueue.Arn)

		// Dead Letter Queues
		ctx.Export("webActionsDlqUrl", webActionsDlq.Url)
		ctx.Export("webActionsDlqArn", webActionsDlq.Arn)
		ctx.Export("notificationsDlqUrl", notificationsDlq.Url)
		ctx.Export("notificationsDlqArn", notificationsDlq.Arn)

		// Lambda Functions
		ctx.Export("schedulerLambdaArn", schedulerLambda.Arn)
		ctx.Export("processorLambdaArn", processorLambda.Arn)
		ctx.Export("webactionLambdaArn", webactionLambda.Arn)
		ctx.Export("webapiLambdaArn", webapiLambda.Arn)

		// API Gateway
		ctx.Export("apiGatewayId", httpApi.ID())
		ctx.Export("apiGatewayEndpoint", httpApi.ApiEndpoint)
		ctx.Export("webapiUrl", httpApi.ApiEndpoint)

		return nil
	})
}
