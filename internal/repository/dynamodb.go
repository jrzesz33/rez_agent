package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/yourusername/rez_agent/internal/models"
)

// MessageRepository defines the interface for message persistence operations
type MessageRepository interface {
	SaveMessage(ctx context.Context, message *models.Message) error
	GetMessage(ctx context.Context, id string) (*models.Message, error)
	ListMessages(ctx context.Context, stage *models.Stage, status *models.Status, limit int) ([]*models.Message, error)
	UpdateStatus(ctx context.Context, id string, status models.Status, errorMessage string) error
}

// DynamoDBRepository implements MessageRepository using DynamoDB
type DynamoDBRepository struct {
	client    *dynamodb.Client
	tableName string
}

// NewDynamoDBRepository creates a new DynamoDB repository instance
func NewDynamoDBRepository(client *dynamodb.Client, tableName string) *DynamoDBRepository {
	return &DynamoDBRepository{
		client:    client,
		tableName: tableName,
	}
}

// SaveMessage saves a message to DynamoDB
func (r *DynamoDBRepository) SaveMessage(ctx context.Context, message *models.Message) error {
	av, err := attributevalue.MarshalMap(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      av,
	}

	_, err = r.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to save message to DynamoDB: %w", err)
	}

	return nil
}

// GetMessage retrieves a message by ID from DynamoDB
func (r *DynamoDBRepository) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	}

	result, err := r.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get message from DynamoDB: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("message not found: %s", id)
	}

	var message models.Message
	err = attributevalue.UnmarshalMap(result.Item, &message)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &message, nil
}

// ListMessages retrieves messages with optional filtering by stage and status
func (r *DynamoDBRepository) ListMessages(ctx context.Context, stage *models.Stage, status *models.Status, limit int) ([]*models.Message, error) {
	// Build filter expression
	var filterExpression string
	expressionAttributeValues := make(map[string]types.AttributeValue)
	expressionAttributeNames := make(map[string]string)

	if stage != nil {
		filterExpression = "#stage = :stage"
		expressionAttributeNames["#stage"] = "stage"
		expressionAttributeValues[":stage"] = &types.AttributeValueMemberS{Value: stage.String()}
	}

	if status != nil {
		if filterExpression != "" {
			filterExpression += " AND "
		}
		filterExpression += "#status = :status"
		expressionAttributeNames["#status"] = "status"
		expressionAttributeValues[":status"] = &types.AttributeValueMemberS{Value: status.String()}
	}

	// Set default limit if not specified
	if limit <= 0 {
		limit = 100
	}

	input := &dynamodb.ScanInput{
		TableName: aws.String(r.tableName),
		Limit:     aws.Int32(int32(limit)),
	}

	if filterExpression != "" {
		input.FilterExpression = aws.String(filterExpression)
		input.ExpressionAttributeValues = expressionAttributeValues
		input.ExpressionAttributeNames = expressionAttributeNames
	}

	result, err := r.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan messages from DynamoDB: %w", err)
	}

	messages := make([]*models.Message, 0, len(result.Items))
	for _, item := range result.Items {
		var message models.Message
		err = attributevalue.UnmarshalMap(item, &message)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}
		messages = append(messages, &message)
	}

	return messages, nil
}

// UpdateStatus updates the status of a message in DynamoDB
func (r *DynamoDBRepository) UpdateStatus(ctx context.Context, id string, status models.Status, errorMessage string) error {
	updateExpression := "SET #status = :status, updated_date = :updated_date"
	expressionAttributeNames := map[string]string{
		"#status": "status",
	}

	// Use current timestamp as string
	updatedDate := fmt.Sprintf("%d", aws.ToTime(aws.Time(time.Now())).Unix())

	expressionAttributeValues := map[string]types.AttributeValue{
		":status":       &types.AttributeValueMemberS{Value: status.String()},
		":updated_date": &types.AttributeValueMemberS{Value: updatedDate},
	}

	if errorMessage != "" {
		updateExpression += ", error_message = :error_message"
		expressionAttributeValues[":error_message"] = &types.AttributeValueMemberS{Value: errorMessage}
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	}

	_, err := r.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update message status in DynamoDB: %w", err)
	}

	return nil
}
