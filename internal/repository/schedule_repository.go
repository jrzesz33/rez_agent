package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jrzesz33/rez_agent/internal/models"
)

// ScheduleRepository defines the interface for schedule data operations
type ScheduleRepository interface {
	// SaveSchedule saves a new schedule to DynamoDB
	SaveSchedule(ctx context.Context, schedule *models.Schedule) error

	// GetSchedule retrieves a schedule by ID
	GetSchedule(ctx context.Context, id string) (*models.Schedule, error)

	// UpdateSchedule updates an existing schedule
	UpdateSchedule(ctx context.Context, schedule *models.Schedule) error

	// UpdateScheduleStatus updates only the status of a schedule
	UpdateScheduleStatus(ctx context.Context, id string, status models.ScheduleStatus, errorMessage string) error

	// ListSchedulesByStatus lists schedules with a specific status
	ListSchedulesByStatus(ctx context.Context, status models.ScheduleStatus) ([]*models.Schedule, error)

	// ListSchedulesByCreator lists schedules created by a specific user/system
	ListSchedulesByCreator(ctx context.Context, createdBy string) ([]*models.Schedule, error)

	// DeleteSchedule marks a schedule as deleted
	DeleteSchedule(ctx context.Context, id string) error
}

// DynamoDBScheduleRepository implements ScheduleRepository using DynamoDB
type DynamoDBScheduleRepository struct {
	client    *dynamodb.Client
	tableName string
}

// NewDynamoDBScheduleRepository creates a new DynamoDB-based schedule repository
func NewDynamoDBScheduleRepository(client *dynamodb.Client, tableName string) *DynamoDBScheduleRepository {
	return &DynamoDBScheduleRepository{
		client:    client,
		tableName: tableName,
	}
}

// SaveSchedule saves a new schedule to DynamoDB
func (r *DynamoDBScheduleRepository) SaveSchedule(ctx context.Context, schedule *models.Schedule) error {
	item, err := attributevalue.MarshalMap(schedule)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
		// Ensure schedule doesn't already exist
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	}

	_, err = r.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to save schedule: %w", err)
	}

	return nil
}

// GetSchedule retrieves a schedule by ID
func (r *DynamoDBScheduleRepository) GetSchedule(ctx context.Context, id string) (*models.Schedule, error)  {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	}

	result, err := r.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("schedule not found: %s", id)
	}

	var schedule models.Schedule
	err = attributevalue.UnmarshalMap(result.Item, &schedule)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal schedule: %w", err)
	}

	return &schedule, nil
}

// UpdateSchedule updates an existing schedule
func (r *DynamoDBScheduleRepository) UpdateSchedule(ctx context.Context, schedule *models.Schedule) error {
	item, err := attributevalue.MarshalMap(schedule)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	}

	_, err = r.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update schedule: %w", err)
	}

	return nil
}

// UpdateScheduleStatus updates only the status of a schedule
func (r *DynamoDBScheduleRepository) UpdateScheduleStatus(ctx context.Context, id string, status models.ScheduleStatus, errorMessage string) error {
	updateExpr := "SET #status = :status, updated_date = :updated_date"
	exprAttrNames := map[string]string{
		"#status": "status",
	}
	exprAttrValues := map[string]types.AttributeValue{
		":status":       &types.AttributeValueMemberS{Value: string(status)},
		":updated_date": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
	}

	if errorMessage != "" {
		updateExpr += ", error_message = :error_message"
		exprAttrValues[":error_message"] = &types.AttributeValueMemberS{Value: errorMessage}
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeNames:  exprAttrNames,
		ExpressionAttributeValues: exprAttrValues,
	}

	_, err := r.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update schedule status: %w", err)
	}

	return nil
}

// ListSchedulesByStatus lists schedules with a specific status
func (r *DynamoDBScheduleRepository) ListSchedulesByStatus(ctx context.Context, status models.ScheduleStatus) ([]*models.Schedule, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("status-created_date-index"),
		KeyConditionExpression: aws.String("#status = :status"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: string(status)},
		},
	}

	result, err := r.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query schedules by status: %w", err)
	}

	schedules := make([]*models.Schedule, 0, len(result.Items))
	for _, item := range result.Items {
		var schedule models.Schedule
		err = attributevalue.UnmarshalMap(item, &schedule)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal schedule: %w", err)
		}
		schedules = append(schedules, &schedule)
	}

	return schedules, nil
}

// ListSchedulesByCreator lists schedules created by a specific user/system
func (r *DynamoDBScheduleRepository) ListSchedulesByCreator(ctx context.Context, createdBy string) ([]*models.Schedule, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("created_by-index"),
		KeyConditionExpression: aws.String("created_by = :created_by"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":created_by": &types.AttributeValueMemberS{Value: createdBy},
		},
	}

	result, err := r.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query schedules by creator: %w", err)
	}

	schedules := make([]*models.Schedule, 0, len(result.Items))
	for _, item := range result.Items {
		var schedule models.Schedule
		err = attributevalue.UnmarshalMap(item, &schedule)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal schedule: %w", err)
		}
		schedules = append(schedules, &schedule)
	}

	return schedules, nil
}

// DeleteSchedule marks a schedule as deleted
func (r *DynamoDBScheduleRepository) DeleteSchedule(ctx context.Context, id string) error {
	return r.UpdateScheduleStatus(ctx, id, models.ScheduleStatusDeleted, "")
}
