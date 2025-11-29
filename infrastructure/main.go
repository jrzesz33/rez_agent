package main

import (
	"fmt"
	"log"
	"runtime/debug"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/dynamodb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/s3"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/scheduler"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/sns"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/sqs"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) (err error) {
		// Add panic recovery with detailed logging
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC RECOVERED: %v", r)
				log.Printf("Stack trace:\n%s", debug.Stack())
				err = fmt.Errorf("panic occurred: %v", r)
			}
		}()

		log.Printf("Starting Pulumi infrastructure deployment...")
		// Load configuration
		cfg := config.New(ctx, "")

		log.Printf("Loading configuration values...")
		stage := cfg.Get("stage")
		if stage == "" {
			stage = "dev"
			log.Printf("Using default stage: %s", stage)
		}

		ntfyUrl := cfg.Get("ntfyUrl")
		if ntfyUrl == "" {
			return fmt.Errorf("required config 'ntfyUrl' is missing")
		}

		logRetentionDays := cfg.GetInt("logRetentionDays")
		if logRetentionDays == 0 {
			logRetentionDays = 7
			log.Printf("Using default logRetentionDays: %d", logRetentionDays)
		}

		enableXRay := cfg.GetBool("enableXRay")
		log.Printf("X-Ray tracing enabled: %v", enableXRay)

		schedulerCron := cfg.Get("schedulerCron")
		if schedulerCron == "" {
			schedulerCron = "cron(0 12 * * ? *)" // Default: daily at noon UTC
			log.Printf("Using default schedulerCron: %s", schedulerCron)
		}

		log.Printf("Configuration loaded successfully: stage=%s, logRetentionDays=%d, enableXRay=%v", stage, logRetentionDays, enableXRay)

		// Common tags
		commonTags := pulumi.StringMap{
			"Project":     pulumi.String("rez-agent"),
			"Stage":       pulumi.String(stage),
			"ManagedBy":   pulumi.String("pulumi"),
			"Environment": pulumi.String(stage),
		}

		// ========================================
		// S3 Bucket for Lambda Deployment Artifacts
		// ========================================
		log.Printf("Creating S3 bucket for Lambda deployment artifacts...")
		lambdaDeploymentBucket, err := s3.NewBucket(ctx, fmt.Sprintf("rez-agent-lambda-deployments-%s", stage), &s3.BucketArgs{
			Bucket: pulumi.String(fmt.Sprintf("rez-agent-lambda-deployments-%s", stage)),
			ForceDestroy: pulumi.Bool(true),
			Tags:   commonTags,
		})
		if err != nil {
			return fmt.Errorf("failed to create Lambda deployment S3 bucket: %w", err)
		}

		// Block public access to the deployment bucket
		_, err = s3.NewBucketPublicAccessBlock(ctx, fmt.Sprintf("rez-agent-lambda-deployments-pab-%s", stage), &s3.BucketPublicAccessBlockArgs{
			Bucket:                lambdaDeploymentBucket.ID(),
			BlockPublicAcls:       pulumi.Bool(true),
			BlockPublicPolicy:     pulumi.Bool(true),
			IgnorePublicAcls:      pulumi.Bool(true),
			RestrictPublicBuckets: pulumi.Bool(true),
		})
		if err != nil {
			return fmt.Errorf("failed to create bucket public access block: %w", err)
		}

		// Upload agent.zip to S3 (for large Lambda packages > 50MB)
		log.Printf("Uploading agent.zip to S3...")
		agentZipObject, err := s3.NewBucketObject(ctx, fmt.Sprintf("rez-agent-agent-code-%s", stage), &s3.BucketObjectArgs{
			Bucket: lambdaDeploymentBucket.ID(),
			Key:    pulumi.String(fmt.Sprintf("agent-%s.zip", stage)),
			Source: pulumi.NewFileArchive("../build/agent.zip"),
			Tags:   commonTags,
		})
		if err != nil {
			return fmt.Errorf("failed to upload agent.zip to S3: %w", err)
		}

		// ========================================
		// S3 Bucket for Agent Logs
		// ========================================
		log.Printf("Creating S3 bucket for agent logs...")
		agentLogsBucket, err := s3.NewBucket(ctx, fmt.Sprintf("rez-agent-logs-%s", stage), &s3.BucketArgs{
			Bucket: pulumi.String(fmt.Sprintf("rez-agent-logs-%s", stage)),
			ForceDestroy: pulumi.Bool(true),
			Tags:   commonTags,
		})
		if err != nil {
			return fmt.Errorf("failed to create agent logs S3 bucket: %w", err)
		}

		// Block public access to the logs bucket
		_, err = s3.NewBucketPublicAccessBlock(ctx, fmt.Sprintf("rez-agent-logs-pab-%s", stage), &s3.BucketPublicAccessBlockArgs{
			Bucket:                agentLogsBucket.ID(),
			BlockPublicAcls:       pulumi.Bool(true),
			BlockPublicPolicy:     pulumi.Bool(true),
			IgnorePublicAcls:      pulumi.Bool(true),
			RestrictPublicBuckets: pulumi.Bool(true),
		})
		if err != nil {
			return fmt.Errorf("failed to create logs bucket public access block: %w", err)
		}

		// Configure lifecycle policy for agent logs (auto-delete after 90 days)
		_, err = s3.NewBucketLifecycleConfigurationV2(ctx, fmt.Sprintf("rez-agent-logs-lifecycle-%s", stage), &s3.BucketLifecycleConfigurationV2Args{
			Bucket: agentLogsBucket.ID(),
			Rules: s3.BucketLifecycleConfigurationV2RuleArray{
				&s3.BucketLifecycleConfigurationV2RuleArgs{
					Id:     pulumi.String("delete-old-logs"),
					Status: pulumi.String("Enabled"),
					Expiration: &s3.BucketLifecycleConfigurationV2RuleExpirationArgs{
						Days: pulumi.Int(90),
					},
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create logs bucket lifecycle policy: %w", err)
		}

		// ========================================
		// DynamoDB Table
		// ========================================
		log.Printf("Creating DynamoDB messages table...")
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
		// DynamoDB Table for Schedules
		// ========================================
		schedulesTable, err := dynamodb.NewTable(ctx, fmt.Sprintf("rez-agent-schedules-%s", stage), &dynamodb.TableArgs{
			Name:        pulumi.String(fmt.Sprintf("rez-agent-schedules-%s", stage)),
			BillingMode: pulumi.String("PAY_PER_REQUEST"),
			HashKey:     pulumi.String("id"),
			Attributes: dynamodb.TableAttributeArray{
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("id"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("status"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("created_date"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("created_by"),
					Type: pulumi.String("S"),
				},
			},
			GlobalSecondaryIndexes: dynamodb.TableGlobalSecondaryIndexArray{
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("status-created_date-index"),
					HashKey:        pulumi.String("status"),
					RangeKey:       pulumi.String("created_date"),
					ProjectionType: pulumi.String("ALL"),
				},
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("created_by-index"),
					HashKey:        pulumi.String("created_by"),
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

		// Agent Response Topic (for receiving tool results)
		agentResponseTopic, err := sns.NewTopic(ctx, fmt.Sprintf("rez-agent-agent-responses-%s", stage), &sns.TopicArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-agent-responses-%s", stage)),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// Schedule Creation Topic (for dynamically creating EventBridge Schedules)
		scheduleCreationTopic, err := sns.NewTopic(ctx, fmt.Sprintf("rez-agent-schedule-creation-%s", stage), &sns.TopicArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-schedule-creation-%s", stage)),
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

		// Agent Response Queue
		agentResponseDlq, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-agent-responses-dlq-%s", stage), &sqs.QueueArgs{
			Name:                    pulumi.String(fmt.Sprintf("rez-agent-agent-responses-dlq-%s", stage)),
			MessageRetentionSeconds: pulumi.Int(1209600), // 14 days
			Tags:                    commonTags,
		})
		if err != nil {
			return err
		}

		agentResponseQueue, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-agent-responses-%s", stage), &sqs.QueueArgs{
			Name:                     pulumi.String(fmt.Sprintf("rez-agent-agent-responses-%s", stage)),
			VisibilityTimeoutSeconds: pulumi.Int(300),     // 5 minutes
			MessageRetentionSeconds:  pulumi.Int(1209600), // 14 days
			RedrivePolicy: agentResponseDlq.Arn.ApplyT(func(arn string) string {
				return fmt.Sprintf(`{"deadLetterTargetArn":"%s","maxReceiveCount":3}`, arn)
			}).(pulumi.StringOutput),
			Tags: commonTags,
		})
		if err != nil {
			return err
		}

		// Schedule Creation DLQ
		scheduleCreationDlq, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-schedule-creation-dlq-%s", stage), &sqs.QueueArgs{
			Name:                    pulumi.String(fmt.Sprintf("rez-agent-schedule-creation-dlq-%s", stage)),
			MessageRetentionSeconds: pulumi.Int(1209600), // 14 days
			Tags:                    commonTags,
		})
		if err != nil {
			return err
		}

		// Schedule Creation Queue
		scheduleCreationQueue, err := sqs.NewQueue(ctx, fmt.Sprintf("rez-agent-schedule-creation-%s", stage), &sqs.QueueArgs{
			Name:                     pulumi.String(fmt.Sprintf("rez-agent-schedule-creation-%s", stage)),
			VisibilityTimeoutSeconds: pulumi.Int(60),      // 1 minute (schedule creation should be quick)
			MessageRetentionSeconds:  pulumi.Int(1209600), // 14 days
			RedrivePolicy: scheduleCreationDlq.Arn.ApplyT(func(arn string) string {
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
		// Agent Response Topic -> Queue Subscription
		_, err = sns.NewTopicSubscription(ctx, fmt.Sprintf("rez-agent-agent-responses-subscription-%s", stage), &sns.TopicSubscriptionArgs{
			Topic:              agentResponseTopic.Arn,
			Protocol:           pulumi.String("sqs"),
			Endpoint:           agentResponseQueue.Arn,
			RawMessageDelivery: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		// Schedule Creation Topic -> Schedule Creation Queue
		_, err = sns.NewTopicSubscription(ctx, fmt.Sprintf("rez-agent-schedule-creation-subscription-%s", stage), &sns.TopicSubscriptionArgs{
			Topic:              scheduleCreationTopic.Arn,
			Protocol:           pulumi.String("sqs"),
			Endpoint:           scheduleCreationQueue.Arn,
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

		// Agent Response Queue Policy
		agentResponseQueuePolicy, err := sqs.NewQueuePolicy(ctx, fmt.Sprintf("rez-agent-agent-responses-queue-policy-%s", stage), &sqs.QueuePolicyArgs{
			QueueUrl: agentResponseQueue.Url,
			Policy: pulumi.All(agentResponseQueue.Arn, agentResponseTopic.Arn).ApplyT(func(args []interface{}) string {
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

		// Schedule Creation Queue Policy
		scheduleCreationQueuePolicy, err := sqs.NewQueuePolicy(ctx, fmt.Sprintf("rez-agent-schedule-creation-queue-policy-%s", stage), &sqs.QueuePolicyArgs{
			QueueUrl: scheduleCreationQueue.Url,
			Policy: pulumi.All(scheduleCreationQueue.Arn, scheduleCreationTopic.Arn).ApplyT(func(args []interface{}) string {
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
			Policy: pulumi.All(
				messagesTable.Arn,
				schedulesTable.Arn,
				notificationsTopic.Arn,
				webActionsTopic.Arn,
				scheduleCreationQueue.Arn,
				agentLogsBucket.Arn,
			).ApplyT(func(args []interface{}) string {
				messagesTableArn := args[0].(string)
				schedulesTableArn := args[1].(string)
				notificationsTopicArn := args[2].(string)
				webActionsTopicArn := args[3].(string)
				scheduleCreationQueueArn := args[4].(string)
				agentLogsBucketArn := args[5].(string)
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [
						{
							"Effect": "Allow",
							"Action": [
								"dynamodb:PutItem",
								"dynamodb:UpdateItem",
								"dynamodb:GetItem",
								"dynamodb:Query"
							],
							"Resource": ["%s", "%s/*", "%s", "%s/*"]
						},
						{
							"Effect": "Allow",
							"Action": ["sns:Publish"],
							"Resource": ["%s", "%s"]
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
								"s3:PutObject",
								"s3:PutObjectAcl"
							],
							"Resource": "%s/*"
						},
						{
							"Effect": "Allow",
							"Action": [
								"scheduler:CreateSchedule",
								"scheduler:GetSchedule",
								"scheduler:UpdateSchedule",
								"scheduler:DeleteSchedule"
							],
							"Resource": "arn:aws:scheduler:*:*:schedule/default/*"
						},
						{
							"Effect": "Allow",
							"Action": ["iam:PassRole"],
							"Resource": "arn:aws:iam::*:role/rez-agent-eventbridge-scheduler-execution-role-%s"
						},
						{
							"Effect": "Allow",
							"Action": [
								"bedrock:InvokeModel",
								"bedrock:InvokeModelWithResponseStream"
							],
							"Resource": "*"
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
				}`, messagesTableArn, messagesTableArn, schedulesTableArn, schedulesTableArn,
					notificationsTopicArn, webActionsTopicArn, scheduleCreationQueueArn, agentLogsBucketArn, stage)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// EventBridge Scheduler Execution Role (for dynamically created schedules)
		// This role is passed to EventBridge Scheduler to invoke the scheduler Lambda
		eventBridgeSchedulerExecutionRole, err := iam.NewRole(ctx, fmt.Sprintf("rez-agent-eventbridge-scheduler-execution-role-%s", stage), &iam.RoleArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-eventbridge-scheduler-execution-role-%s", stage)),
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

		// EventBridge Scheduler Execution Role Policy
		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("rez-agent-eventbridge-scheduler-execution-policy-%s", stage), &iam.RolePolicyArgs{
			Role: eventBridgeSchedulerExecutionRole.Name,
			Policy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Effect": "Allow",
						"Action": ["lambda:InvokeFunction"],
						"Resource": "arn:aws:lambda:*:*:function:rez-agent-scheduler-*"
					},
					{
						"Effect": "Allow",
						"Action": ["sns:Publish"],
						"Resource": "*"
					},
					{
						"Effect": "Allow",
						"Action": ["sqs:SendMessage"],
						"Resource": "arn:aws:sqs:*:*:rez-agent-schedule-creation-queue-*"
					}
				]
			}`),
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
			Policy: pulumi.All(
				messagesTable.Arn,
				schedulesTable.Arn,
				webActionsTopic.Arn,
				notificationsTopic.Arn,
				scheduleCreationTopic.Arn,
			).ApplyT(func(args []interface{}) string {
				messagesTableArn := args[0].(string)
				schedulesTableArn := args[1].(string)
				webActionsTopicArn := args[2].(string)
				notificationsTopicArn := args[3].(string)
				scheduleCreationTopicArn := args[4].(string)
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
							"Resource": ["%s", "%s/*", "%s", "%s/*"]
						},
						{
							"Effect": "Allow",
							"Action": ["sns:Publish"],
							"Resource": ["%s", "%s", "%s"]
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
				}`, messagesTableArn, messagesTableArn, schedulesTableArn, schedulesTableArn,
					webActionsTopicArn, notificationsTopicArn, scheduleCreationTopicArn)
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
		// API Gateway HTTP API (created early for MCP URL)
		// ========================================

		httpApi, err := apigatewayv2.NewApi(ctx, fmt.Sprintf("rez-agent-api-%s", stage), &apigatewayv2.ApiArgs{
			Name:         pulumi.String(fmt.Sprintf("rez-agent-api-%s", stage)),
			ProtocolType: pulumi.String("HTTP"),
			Description:  pulumi.String("HTTP API for rez-agent web interface"),
			Tags:         commonTags,
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
					"DYNAMODB_TABLE_NAME":            messagesTable.Name,
					"SCHEDULES_TABLE_NAME":           schedulesTable.Name,
					"WEB_ACTIONS_TOPIC_ARN":          webActionsTopic.Arn,       // Topic-based routing
					"NOTIFICATIONS_TOPIC_ARN":        notificationsTopic.Arn,    // Topic-based routing
					"SCHEDULE_CREATION_TOPIC_ARN":    scheduleCreationTopic.Arn, // For publishing new schedule requests
					"SCHEDULE_CREATION_QUEUE_URL":    scheduleCreationQueue.Url, // For receiving schedule creation requests
					"WEB_ACTION_SQS_QUEUE_URL":       webActionsQueue.Url,
					"NOTIFICATION_SQS_QUEUE_URL":     notificationsQueue.Url,
					"EVENTBRIDGE_EXECUTION_ROLE_ARN": eventBridgeSchedulerExecutionRole.Arn,
					"BEDROCK_MODEL_ID":               pulumi.String("amazon.nova-lite-v1:0"),
					"AGENT_LOGS_BUCKET":              agentLogsBucket.ID(),
					"MCP_SERVER_URL": httpApi.ApiEndpoint.ApplyT(func(endpoint string) string {
						return fmt.Sprintf("%s/mcp", endpoint)
					}).(pulumi.StringOutput),
					"STAGE": pulumi.String(stage),
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
					"DYNAMODB_TABLE_NAME":         messagesTable.Name,
					"SCHEDULES_TABLE_NAME":        schedulesTable.Name,
					"WEB_ACTIONS_TOPIC_ARN":       webActionsTopic.Arn,       // Topic-based routing
					"NOTIFICATIONS_TOPIC_ARN":     notificationsTopic.Arn,    // Topic-based routing
					"AGENT_RESPONSE_TOPIC_ARN":    agentResponseTopic.Arn,    // Topic-based routing
					"SCHEDULE_CREATION_TOPIC_ARN": scheduleCreationTopic.Arn, // Schedule management
					"WEB_ACTION_SQS_QUEUE_URL":    webActionsQueue.Url,
					"NOTIFICATION_SQS_QUEUE_URL":  notificationsQueue.Url,
					"STAGE":                       pulumi.String(stage),
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
			Policy: pulumi.All(messagesTable.Arn, webActionResultsTable.Arn, webActionsQueue.Arn, webActionsTopic.Arn, notificationsQueue.Arn, notificationsTopic.Arn, agentResponseTopic.Arn).ApplyT(func(args []interface{}) string {
				tableArn := args[0].(string)
				webActionResultsArn := args[1].(string)
				waQueueArn := args[2].(string)
				waTtopicArn := args[3].(string)
				noQueueArn := args[4].(string)
				noTtopicArn := args[5].(string)
				agentResponseTopicArn := args[6].(string)
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
							"Resource": ["%s","%s","%s"]
						},
						{
							"Effect": "Allow",
							"Action": [
								"secretsmanager:GetSecretValue"
							],
							"Resource": "arn:aws:secretsmanager:*:*:secret:rez-agent/*"
						}
					]
				}`, tableArn, tableArn, webActionResultsArn, webActionResultsArn, waQueueArn, noQueueArn, waTtopicArn, noTtopicArn, agentResponseTopicArn)
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

		// Note: AGENT_RESPONSE_TOPIC_ARN will be added after agent infrastructure is created

		// WebAction Lambda Function
		webactionLambda, err := lambda.NewFunction(ctx, fmt.Sprintf("rez-agent-webaction-%s", stage), &lambda.FunctionArgs{
			Name:    pulumi.String(fmt.Sprintf("rez-agent-webaction-%s", stage)),
			Runtime: pulumi.String("provided.al2"),
			Role:    webactionRole.Arn,
			Handler: pulumi.String("bootstrap"),
			Code:    pulumi.NewFileArchive("../build/webaction.zip"),
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"DYNAMODB_TABLE_NAME":         messagesTable.Name,
					"WEB_ACTIONS_TOPIC_ARN":       webActionsTopic.Arn,    // Topic-based routing
					"NOTIFICATIONS_TOPIC_ARN":     notificationsTopic.Arn, // Topic-based routing
					"WEB_ACTION_SQS_QUEUE_URL":    webActionsQueue.Url,
					"NOTIFICATION_SQS_QUEUE_URL":  notificationsQueue.Url,
					"AGENT_RESPONSE_TOPIC_ARN":    agentResponseTopic.Arn,    // Now available
					"SCHEDULE_CREATION_TOPIC_ARN": scheduleCreationTopic.Arn, // Schedule management
					"STAGE":                       pulumi.String(stage),
					"GOLF_SECRET_NAME":            pulumi.String(fmt.Sprintf("rez-agent/golf/credentials-%s", stage)),
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

		// SQS Event Source Mapping for Scheduler Lambda (Schedule Creation Queue)
		_, err = lambda.NewEventSourceMapping(ctx, fmt.Sprintf("rez-agent-scheduler-sqs-trigger-%s", stage), &lambda.EventSourceMappingArgs{
			EventSourceArn: scheduleCreationQueue.Arn,
			FunctionName:   schedulerLambda.Arn,
			BatchSize:      pulumi.Int(10),
			Enabled:        pulumi.Bool(true),
			// No filter criteria needed - dedicated queue for schedule creation
		}, pulumi.DependsOn([]pulumi.Resource{scheduleCreationQueuePolicy}))
		if err != nil {
			return err
		}

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
		// MCP Lambda Function
		// ========================================

		log.Printf("Creating MCP Lambda function...")

		// MCP Lambda Role
		mcpRole, err := iam.NewRole(ctx, fmt.Sprintf("rez-agent-mcp-role-%s", stage), &iam.RoleArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-mcp-role-%s", stage)),
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

		// MCP Lambda Policy
		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("rez-agent-mcp-policy-%s", stage), &iam.RolePolicyArgs{
			Role: mcpRole.Name,
			Policy: pulumi.All(messagesTable.Arn, notificationsTopic.Arn).ApplyT(func(args []interface{}) string {
				tableArn := args[0].(string)
				topicArn := args[1].(string)
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
							"Resource": ["%s", "%s/*"]
						},
						{
							"Effect": "Allow",
							"Action": ["sns:Publish"],
							"Resource": "%s"
						},
						{
							"Effect": "Allow",
							"Action": ["secretsmanager:GetSecretValue"],
							"Resource": "arn:aws:secretsmanager:*:*:secret:rez-agent/*"
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

		// MCP Lambda Log Group
		mcpLogGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("rez-agent-mcp-logs-%s", stage), &cloudwatch.LogGroupArgs{
			Name:            pulumi.String(fmt.Sprintf("/aws/lambda/rez-agent-mcp-%s", stage)),
			RetentionInDays: pulumi.Int(logRetentionDays),
			Tags:            commonTags,
		})
		if err != nil {
			return err
		}

		// MCP Lambda Function
		mcpLambda, err := lambda.NewFunction(ctx, fmt.Sprintf("rez-agent-mcp-%s", stage), &lambda.FunctionArgs{
			Name:    pulumi.String(fmt.Sprintf("rez-agent-mcp-%s", stage)),
			Runtime: pulumi.String("provided.al2"),
			Role:    mcpRole.Arn,
			Handler: pulumi.String("bootstrap"),
			Code:    pulumi.NewFileArchive("../build/mcp.zip"),
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"MCP_SERVER_NAME":            pulumi.String("rez-agent-mcp"),
					"MCP_SERVER_VERSION":         pulumi.String("1.0.0"),
					"DYNAMODB_TABLE_NAME":        messagesTable.Name,
					"NOTIFICATIONS_TOPIC_ARN":    notificationsTopic.Arn,
					"NOTIFICATION_SQS_QUEUE_URL": notificationsQueue.Url,
					"NTFY_URL":                   pulumi.String(ntfyUrl),
					"STAGE":                      pulumi.String(stage),
					"GOLF_SECRET_NAME":           pulumi.String(fmt.Sprintf("rez-agent/golf/credentials-%s", stage)),
					"WEATHER_API_KEY_SECRET":     pulumi.String(fmt.Sprintf("rez-agent/weather/api-key-%s", stage)),
				},
			},
			MemorySize: pulumi.Int(512),
			Timeout:    pulumi.Int(30),
			TracingConfig: &lambda.FunctionTracingConfigArgs{
				Mode: pulumi.String(map[bool]string{true: "Active", false: "PassThrough"}[enableXRay]),
			},
			Tags: commonTags,
		}, pulumi.DependsOn([]pulumi.Resource{mcpLogGroup}))
		if err != nil {
			return err
		}

		// Lambda permission for API Gateway to invoke MCP
		_, err = lambda.NewPermission(ctx, fmt.Sprintf("rez-agent-mcp-apigw-permission-%s", stage), &lambda.PermissionArgs{
			Action:    pulumi.String("lambda:InvokeFunction"),
			Function:  mcpLambda.Name,
			Principal: pulumi.String("apigateway.amazonaws.com"),
			SourceArn: httpApi.ExecutionArn.ApplyT(func(arn string) string {
				return fmt.Sprintf("%s/*/*", arn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// API Gateway Integration for MCP
		mcpApiIntegration, err := apigatewayv2.NewIntegration(ctx, fmt.Sprintf("rez-agent-mcp-api-integration-%s", stage), &apigatewayv2.IntegrationArgs{
			ApiId:                httpApi.ID(),
			IntegrationType:      pulumi.String("AWS_PROXY"),
			IntegrationUri:       mcpLambda.Arn,
			IntegrationMethod:    pulumi.String("POST"),
			PayloadFormatVersion: pulumi.String("2.0"),
		})
		if err != nil {
			return err
		}

		// API Gateway Route for MCP
		_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("rez-agent-mcp-route-%s", stage), &apigatewayv2.RouteArgs{
			ApiId:    httpApi.ID(),
			RouteKey: pulumi.String("POST /mcp"),
			Target: mcpApiIntegration.ID().ApplyT(func(id string) string {
				return fmt.Sprintf("integrations/%s", id)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		log.Printf("MCP Lambda function created successfully")

		// ========================================
		// AI Agent Infrastructure
		// ========================================

		// Agent Session DynamoDB Table
		agentSessionTable, err := dynamodb.NewTable(ctx, fmt.Sprintf("rez-agent-sessions-%s", stage), &dynamodb.TableArgs{
			Name:        pulumi.String(fmt.Sprintf("rez-agent-sessions-%s", stage)),
			BillingMode: pulumi.String("PAY_PER_REQUEST"),
			HashKey:     pulumi.String("session_id"),
			Attributes: dynamodb.TableAttributeArray{
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("session_id"),
					Type: pulumi.String("S"),
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

		// Agent Lambda Role
		agentRole, err := iam.NewRole(ctx, fmt.Sprintf("rez-agent-agent-role-%s", stage), &iam.RoleArgs{
			Name: pulumi.String(fmt.Sprintf("rez-agent-agent-role-%s", stage)),
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

		// Agent Lambda Policy
		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("rez-agent-agent-policy-%s", stage), &iam.RolePolicyArgs{
			Role: agentRole.Name,
			Policy: pulumi.All(
				agentSessionTable.Arn,
				messagesTable.Arn,
				webActionsTopic.Arn,
				notificationsTopic.Arn,
				agentResponseTopic.Arn,
				agentResponseQueue.Arn,
			).ApplyT(func(args []interface{}) string {
				sessionTableArn := args[0].(string)
				messagesTableArn := args[1].(string)
				webActionsTopicArn := args[2].(string)
				notificationsTopicArn := args[3].(string)
				agentResponseTopicArn := args[4].(string)
				agentResponseQueueArn := args[5].(string)
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [
						{
							"Effect": "Allow",
							"Action": [
								"dynamodb:GetItem",
								"dynamodb:PutItem",
								"dynamodb:UpdateItem",
								"dynamodb:Query"
							],
							"Resource": ["%s", "%s", "%s/*"]
						},
						{
							"Effect": "Allow",
							"Action": ["sns:Publish"],
							"Resource": ["%s", "%s", "%s"]
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
								"bedrock:InvokeModel",
								"bedrock:InvokeModelWithResponseStream"
							],
							"Resource": "*"
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
				}`, sessionTableArn, messagesTableArn, messagesTableArn,
					webActionsTopicArn, notificationsTopicArn, agentResponseTopicArn,
					agentResponseQueueArn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// Agent Lambda Log Group
		agentLogGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("rez-agent-agent-logs-%s", stage), &cloudwatch.LogGroupArgs{
			Name:            pulumi.String(fmt.Sprintf("/aws/lambda/rez-agent-agent-%s", stage)),
			RetentionInDays: pulumi.Int(logRetentionDays),
			Tags:            commonTags,
		})
		if err != nil {
			return err
		}

		// Agent Lambda Function (using S3 for large package)
		log.Printf("Creating agent Lambda function from S3...")
		agentLambda, err := lambda.NewFunction(ctx, fmt.Sprintf("rez-agent-agent-%s", stage), &lambda.FunctionArgs{
			Name:           pulumi.String(fmt.Sprintf("rez-agent-agent-%s", stage)),
			Runtime:        pulumi.String("python3.12"),
			Role:           agentRole.Arn,
			Handler:        pulumi.String("main.lambda_handler"),
			S3Bucket:       lambdaDeploymentBucket.ID(),
			S3Key:          agentZipObject.Key,
			SourceCodeHash: agentZipObject.Etag, // Use ETag to detect file changes (works without versioning)
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"DYNAMODB_TABLE_NAME":      messagesTable.Name,
					"AGENT_SESSION_TABLE_NAME": agentSessionTable.Name,
					"WEB_ACTIONS_TOPIC_ARN":    webActionsTopic.Arn,
					"NOTIFICATIONS_TOPIC_ARN":  notificationsTopic.Arn,
					"AGENT_RESPONSE_TOPIC_ARN": agentResponseTopic.Arn,
					"AGENT_RESPONSE_QUEUE_URL": agentResponseQueue.Url,
					"STAGE":                    pulumi.String(stage),
					// MCP Server Configuration
					"MCP_SERVER_URL": httpApi.ApiEndpoint.ApplyT(func(endpoint string) string {
						return fmt.Sprintf("%s/mcp", endpoint)
					}).(pulumi.StringOutput),
					// Note: MCP_API_KEY should be set via AWS Parameter Store or Secrets Manager
					// For now, omitting it (MCP Lambda will allow unauthenticated requests for internal use)
					// Bedrock LLM Configuration
					"BEDROCK_MODEL_ID":    pulumi.String("us.anthropic.claude-sonnet-4-20250514-v1:0"),
					"BEDROCK_PROVIDER":    pulumi.String("anthropic"),
					"BEDROCK_REGION":      pulumi.String("us-east-1"),
					"BEDROCK_TEMPERATURE": pulumi.String("0.5"),
					"BEDROCK_MAX_TOKENS":  pulumi.String("4096"),
				},
			},
			MemorySize: pulumi.Int(1024),
			Timeout:    pulumi.Int(300),
			TracingConfig: &lambda.FunctionTracingConfigArgs{
				Mode: pulumi.String(map[bool]string{true: "Active", false: "PassThrough"}[enableXRay]),
			},
			Tags: commonTags,
		}, pulumi.DependsOn([]pulumi.Resource{agentLogGroup, agentZipObject}))
		if err != nil {
			return err
		}

		// SQS Event Source Mapping for Agent Lambda (Agent Response Queue)
		_, err = lambda.NewEventSourceMapping(ctx, fmt.Sprintf("rez-agent-agent-sqs-trigger-%s", stage), &lambda.EventSourceMappingArgs{
			EventSourceArn: agentResponseQueue.Arn,
			FunctionName:   agentLambda.Arn,
			BatchSize:      pulumi.Int(10),
			Enabled:        pulumi.Bool(true),
			// No filter criteria needed - dedicated queue for notifications
		}, pulumi.DependsOn([]pulumi.Resource{agentResponseQueuePolicy}))
		if err != nil {
			return err
		}

		// Lambda permission for API Gateway to invoke Agent
		_, err = lambda.NewPermission(ctx, fmt.Sprintf("rez-agent-agent-apigw-permission-%s", stage), &lambda.PermissionArgs{
			Action:    pulumi.String("lambda:InvokeFunction"),
			Function:  agentLambda.Name,
			Principal: pulumi.String("apigateway.amazonaws.com"),
			SourceArn: httpApi.ExecutionArn.ApplyT(func(arn string) string {
				return fmt.Sprintf("%s/*/*", arn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// API Gateway Integration for Agent
		agentApiIntegration, err := apigatewayv2.NewIntegration(ctx, fmt.Sprintf("rez-agent-agent-api-integration-%s", stage), &apigatewayv2.IntegrationArgs{
			ApiId:                httpApi.ID(),
			IntegrationType:      pulumi.String("AWS_PROXY"),
			IntegrationUri:       agentLambda.Arn,
			IntegrationMethod:    pulumi.String("POST"),
			PayloadFormatVersion: pulumi.String("2.0"),
		})
		if err != nil {
			return err
		}

		// API Gateway Route for Agent (POST for chat)
		_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("rez-agent-agent-api-route-%s", stage), &apigatewayv2.RouteArgs{
			ApiId:    httpApi.ID(),
			RouteKey: pulumi.String("POST /agent"),
			Target: agentApiIntegration.ID().ApplyT(func(id string) string {
				return fmt.Sprintf("integrations/%s", id)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// API Gateway Route for Agent Card (GET for A2A discovery)
		_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("rez-agent-agent-card-route-%s", stage), &apigatewayv2.RouteArgs{
			ApiId:    httpApi.ID(),
			RouteKey: pulumi.String("GET /agent/card"),
			Target: agentApiIntegration.ID().ApplyT(func(id string) string {
				return fmt.Sprintf("integrations/%s", id)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// API Gateway Route for Agent Card (well-known path for A2A discovery)
		_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("rez-agent-agent-wellknown-route-%s", stage), &apigatewayv2.RouteArgs{
			ApiId:    httpApi.ID(),
			RouteKey: pulumi.String("GET /agent/.well-known/agent-card"),
			Target: agentApiIntegration.ID().ApplyT(func(id string) string {
				return fmt.Sprintf("integrations/%s", id)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// API Gateway Route for Agent UI Interface
		_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("rez-agent-agent-ui-route-%s", stage), &apigatewayv2.RouteArgs{
			ApiId:    httpApi.ID(),
			RouteKey: pulumi.String("GET /agent/ui"),
			Target: agentApiIntegration.ID().ApplyT(func(id string) string {
				return fmt.Sprintf("integrations/%s", id)
			}).(pulumi.StringOutput),
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
		ctx.Export("agentLambdaArn", agentLambda.Arn)
		ctx.Export("mcpLambdaArn", mcpLambda.Arn)

		// Agent Infrastructure
		ctx.Export("agentResponseTopicArn", agentResponseTopic.Arn)
		ctx.Export("agentResponseQueueUrl", agentResponseQueue.Url)
		ctx.Export("agentResponseQueueArn", agentResponseQueue.Arn)
		ctx.Export("agentSessionTableName", agentSessionTable.Name)
		ctx.Export("agentSessionTableArn", agentSessionTable.Arn)

		// S3 Buckets
		ctx.Export("lambdaDeploymentBucket", lambdaDeploymentBucket.ID())
		ctx.Export("agentLogsBucket", agentLogsBucket.ID())

		// API Gateway
		ctx.Export("apiGatewayId", httpApi.ID())
		ctx.Export("apiGatewayEndpoint", httpApi.ApiEndpoint)
		ctx.Export("webapiUrl", httpApi.ApiEndpoint)

		// Schedule-related exports
		ctx.Export("schedulesTableName", schedulesTable.Name)
		ctx.Export("schedulesTableArn", schedulesTable.Arn)
		ctx.Export("scheduleCreationTopicArn", scheduleCreationTopic.Arn)
		ctx.Export("eventBridgeSchedulerExecutionRoleArn", eventBridgeSchedulerExecutionRole.Arn)

		return nil
	})
}
