package repository

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jrzesz33/rez_agent/internal/models"
)

// WebActionResultRepository defines the interface for web action result persistence
type WebActionResultRepository interface {
	SaveResult(ctx context.Context, result *models.WebActionResult) error
	GetResult(ctx context.Context, id string) (*models.WebActionResult, error)
	GetResultByMessageID(ctx context.Context, messageID string) (*models.WebActionResult, error)
}

// DynamoDBWebActionRepository implements WebActionResultRepository using DynamoDB
type DynamoDBWebActionRepository struct {
	client    *dynamodb.Client
	tableName string
}

// NewDynamoDBWebActionRepository creates a new web action result repository
func NewDynamoDBWebActionRepository(client *dynamodb.Client, tableName string) *DynamoDBWebActionRepository {
	return &DynamoDBWebActionRepository{
		client:    client,
		tableName: tableName,
	}
}

// SaveResult saves a web action result to DynamoDB with TTL
func (r *DynamoDBWebActionRepository) SaveResult(ctx context.Context, result *models.WebActionResult) error {
	av, err := attributevalue.MarshalMap(result)
	if err != nil {
		return fmt.Errorf("failed to marshal web action result: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      av,
	}

	_, err = r.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to save web action result to DynamoDB: %w", err)
	}

	return nil
}

// GetResult retrieves a web action result by ID
func (r *DynamoDBWebActionRepository) GetResult(ctx context.Context, id string) (*models.WebActionResult, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	}

	resp, err := r.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get web action result from DynamoDB: %w", err)
	}

	if resp.Item == nil {
		return nil, fmt.Errorf("web action result not found: %s", id)
	}

	var result models.WebActionResult
	err = attributevalue.UnmarshalMap(resp.Item, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal web action result: %w", err)
	}

	return &result, nil
}

// GetResultByMessageID retrieves a web action result by message ID using a GSI
func (r *DynamoDBWebActionRepository) GetResultByMessageID(ctx context.Context, messageID string) (*models.WebActionResult, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("message_id-index"),
		KeyConditionExpression: aws.String("message_id = :messageID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":messageID": &types.AttributeValueMemberS{Value: messageID},
		},
		Limit: aws.Int32(1),
	}

	resp, err := r.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query web action result by message ID: %w", err)
	}

	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("web action result not found for message: %s", messageID)
	}

	var result models.WebActionResult
	err = attributevalue.UnmarshalMap(resp.Items[0], &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal web action result: %w", err)
	}

	return &result, nil
}
