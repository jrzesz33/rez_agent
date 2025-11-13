package models

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/aws/aws-sdk-go-v2/service/scheduler/types"
)

// ScheduleStatus represents the current state of a schedule
type ScheduleStatus string

const (
	// ScheduleStatusActive indicates the schedule is active and running
	ScheduleStatusActive ScheduleStatus = "active"
	// ScheduleStatusPaused indicates the schedule is temporarily paused
	ScheduleStatusPaused ScheduleStatus = "paused"
	// ScheduleStatusDeleted indicates the schedule has been deleted
	ScheduleStatusDeleted ScheduleStatus = "deleted"
	// ScheduleStatusError indicates the schedule creation/update failed
	ScheduleStatusError ScheduleStatus = "error"
)

// IsValid checks if the schedule status value is valid
func (s ScheduleStatus) IsValid() bool {
	switch s {
	case ScheduleStatusActive, ScheduleStatusPaused, ScheduleStatusDeleted, ScheduleStatusError:
		return true
	default:
		return false
	}
}

// String returns the string representation of the schedule status
func (s ScheduleStatus) String() string {
	return string(s)
}

// TargetType represents the type of action the schedule will trigger
type TargetType string

const (
	// TargetTypeWebAction triggers a web action (HTTP API call)
	TargetTypeWebAction TargetType = "web_action"
	// TargetTypeNotification sends a notification
	TargetTypeNotification TargetType = "notification"
	// TargetTypeNotification sends a notification
	TargetTypeScheduler TargetType = "scheduled"
	// TargetTypeCustom triggers a custom action
	TargetTypeCustom TargetType = "custom"
)

// IsValid checks if the target type value is valid
func (t TargetType) IsValid() bool {
	switch t {
	case TargetTypeWebAction, TargetTypeNotification, TargetTypeCustom:
		return true
	default:
		return false
	}
}

// String returns the string representation of the target type
func (t TargetType) String() string {
	return string(t)
}

// Schedule represents a dynamically created EventBridge Schedule
type Schedule struct {
	// ID is the unique identifier for the schedule (sched_<timestamp>_<random>)
	ID string `json:"id" dynamodbav:"id"`

	// Name is a human-readable name for the schedule
	Name string `json:"name" dynamodbav:"name"`

	// Description is an optional description of what the schedule does
	Description string `json:"description,omitempty" dynamodbav:"description,omitempty"`

	// ScheduleExpression is the cron or rate expression (e.g., "cron(0 12 * * ? *)")
	ScheduleExpression string `json:"schedule_expression" dynamodbav:"schedule_expression"`

	// Timezone is the IANA timezone for cron expressions (default: UTC)
	Timezone string `json:"timezone" dynamodbav:"timezone"`

	// TargetType is the type of action to trigger
	TargetType TargetType `json:"target_type" dynamodbav:"target_type"`

	// TargetTopicArn is the SNS topic ARN to publish to when triggered
	TargetTopicArn string `json:"target_topic_arn" dynamodbav:"target_topic_arn"`

	// Payload is the action-specific data (stored as JSON string)
	Payload string `json:"payload" dynamodbav:"payload"`

	// EventBridgeArn is the ARN of the created EventBridge Schedule
	EventBridgeArn string `json:"eventbridge_arn,omitempty" dynamodbav:"eventbridge_arn,omitempty"`

	// EventBridgeName is the name of the EventBridge Schedule resource
	EventBridgeName string `json:"eventbridge_name,omitempty" dynamodbav:"eventbridge_name,omitempty"`

	// Status is the current state of the schedule
	Status ScheduleStatus `json:"status" dynamodbav:"status"`

	// CreatedBy is the user/system that created the schedule
	CreatedBy string `json:"created_by" dynamodbav:"created_by"`

	// CreatedDate is when the schedule was created
	CreatedDate time.Time `json:"created_date" dynamodbav:"created_date"`

	// UpdatedDate is when the schedule was last updated
	UpdatedDate time.Time `json:"updated_date" dynamodbav:"updated_date"`

	// LastTriggered is when the schedule was last executed (optional)
	LastTriggered *time.Time `json:"last_triggered,omitempty" dynamodbav:"last_triggered,omitempty"`

	// ExecutionCount tracks how many times the schedule has executed
	ExecutionCount int64 `json:"execution_count" dynamodbav:"execution_count"`

	// ErrorMessage contains error details if Status is Error
	ErrorMessage string `json:"error_message,omitempty" dynamodbav:"error_message,omitempty"`

	// Stage is the environment (dev, stage, prod)
	Stage Stage `json:"stage" dynamodbav:"stage"`
	// CreateScheduleReq is the AWS SDK input used to create the EventBridge Schedule
	CreateRequest *scheduler.CreateScheduleInput
}

// NewSchedule creates a new schedule with default values
func NewSchedule(
	msg *Message,
	createdBy string,
	targetTopicArn string,
	stage Stage,
	execRoleArn string,
) (*Schedule, error) {

	var scheduleOut Schedule

	now := time.Now().UTC()
	scheduleOut.ID = generateScheduleID(now)
	scheduleOut.Name = msg.Arguments["name"].(string)
	scheduleOut.ScheduleExpression = msg.Arguments["schedule_expression"].(string)
	scheduleOut.Timezone = msg.Arguments["timezone"].(string)
	scheduleOut.TargetType = TargetType(msg.Arguments["target_type"].(string))
	scheduleOut.Status = ScheduleStatusActive
	scheduleOut.CreatedBy = createdBy
	scheduleOut.CreatedDate = now
	scheduleOut.UpdatedDate = now
	scheduleOut.ExecutionCount = 0
	scheduleOut.TargetTopicArn = targetTopicArn
	scheduleOut.Stage = stage
	// Validate timezone
	if scheduleOut.Timezone == "" {
		scheduleOut.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(scheduleOut.Timezone); err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", scheduleOut.Timezone, err)
	}

	if msg.Arguments["description"] != nil {
		scheduleOut.Description = msg.Arguments["description"].(string)
	}
	// Generate EventBridge name (must be unique and conform to naming rules)
	eventBridgeName := generateEventBridgeName(scheduleOut.Name, stage)

	// Build the new Message for the Payload

	// Only include relevant arguments for the schedule target
	_newArgs := make(map[string]interface{})
	if MessageType(scheduleOut.TargetType) == MessageTypeWebAction {
		if msg.Arguments["operation"] != nil {
			_newArgs["operation"] = msg.Arguments["operation"]
		} else {
			return nil, fmt.Errorf("missing required argument 'operation' for web_action target")
		}
	}
	payloadMsg := NewMessage(
		createdBy,
		_newArgs,
		"1.0",
		stage,
		MessageType(scheduleOut.TargetType),
		msg.Payload)

	payloadBytes, err := json.Marshal(payloadMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schedule payload: %w", err)
	}
	// Create the schedule targeting the SQS queue
	scheduleOut.CreateRequest = &scheduler.CreateScheduleInput{
		Name:                       aws.String(eventBridgeName),
		ScheduleExpression:         aws.String(scheduleOut.ScheduleExpression),
		ScheduleExpressionTimezone: &scheduleOut.Timezone,
		State:                      types.ScheduleStateEnabled,
		Description:                aws.String(scheduleOut.Description),
		FlexibleTimeWindow: &types.FlexibleTimeWindow{
			Mode: types.FlexibleTimeWindowModeOff,
		},
		Target: &types.Target{
			Arn:     aws.String(targetTopicArn),
			RoleArn: aws.String(execRoleArn),
			Input:   aws.String(string(payloadBytes)),
		},
	}
	err = scheduleOut.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid schedule data: %w", err)
	}
	return &scheduleOut, nil
}

// generateScheduleID generates a unique schedule ID
func generateScheduleID(t time.Time) string {
	// Format: sched_<timestamp>_<random_hex>
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("sched_%s_%s", t.Format("20060102150405"), randomHex)
}

// generateEventBridgeName generates a unique EventBridge Schedule name
// EventBridge names must be unique and follow: ^[0-9a-zA-Z-_.]+$
func generateEventBridgeName(name string, stage Stage) string {
	// Sanitize name: replace spaces/special chars with hyphens
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)

	// Truncate if too long (max 64 chars for EventBridge, leave room for stage and timestamp)
	if len(sanitized) > 30 {
		sanitized = sanitized[:30]
	}

	// Add timestamp and stage for uniqueness
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s-%s-%d", sanitized, stage.String(), timestamp)
}

// GetPayloadMap returns the payload as a map
func (s *Schedule) GetPayloadMap() (map[string]interface{}, error) {
	var payloadMap map[string]interface{}
	if err := json.Unmarshal([]byte(s.Payload), &payloadMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return payloadMap, nil
}

// MarkActive updates the schedule status to active
func (s *Schedule) MarkActive() {
	s.Status = ScheduleStatusActive
	s.UpdatedDate = time.Now().UTC()
}

// MarkPaused updates the schedule status to paused
func (s *Schedule) MarkPaused() {
	s.Status = ScheduleStatusPaused
	s.UpdatedDate = time.Now().UTC()
}

// MarkDeleted updates the schedule status to deleted
func (s *Schedule) MarkDeleted() {
	s.Status = ScheduleStatusDeleted
	s.UpdatedDate = time.Now().UTC()
}

// MarkError updates the schedule status to error with an error message
func (s *Schedule) MarkError(errorMessage string) {
	s.Status = ScheduleStatusError
	s.ErrorMessage = errorMessage
	s.UpdatedDate = time.Now().UTC()
}

// UpdateEventBridgeArn sets the EventBridge Schedule ARN after creation
func (s *Schedule) UpdateEventBridgeArn(arn string) {
	s.EventBridgeArn = arn
	s.UpdatedDate = time.Now().UTC()
}

// RecordExecution updates the last triggered time and increments execution count
func (s *Schedule) RecordExecution() {
	now := time.Now().UTC()
	s.LastTriggered = &now
	s.ExecutionCount++
	s.UpdatedDate = now
}

// Validate checks if the schedule has valid required fields
func (s *Schedule) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("schedule name is required")
	}
	if s.ScheduleExpression == "" {
		return fmt.Errorf("schedule expression is required")
	}
	if !s.TargetType.IsValid() {
		return fmt.Errorf("invalid target type: %s", s.TargetType)
	}

	if !s.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", s.Status)
	}

	// Validate timezone
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", s.Timezone, err)
	}

	// Validate schedule expression format
	if err := ValidateScheduleExpression(s.ScheduleExpression); err != nil {
		return fmt.Errorf("invalid schedule expression: %w", err)
	}

	return nil
}

// ValidateScheduleExpression validates EventBridge Scheduler expression syntax
func ValidateScheduleExpression(expr string) error {
	// EventBridge Scheduler supports:
	// 1. rate() expressions: rate(value unit) where unit is minute(s), hour(s), or day(s)
	// 2. cron() expressions: cron(Minutes Hours Day-of-month Month Day-of-week Year)
	// 3. at() expressions: at(yyyy-mm-ddThh:mm:ss) for one-time schedules

	expr = strings.TrimSpace(expr)

	if strings.HasPrefix(expr, "rate(") && strings.HasSuffix(expr, ")") {
		// rate(N minute|hour|day)
		rateContent := expr[5 : len(expr)-1]
		parts := strings.Fields(rateContent)
		if len(parts) != 2 {
			return fmt.Errorf("rate expression must have format: rate(value unit)")
		}
		// TODO: Add more validation for rate values if needed
		return nil
	}
	//0 12 * * ? *
	if strings.HasPrefix(expr, "cron(") && strings.HasSuffix(expr, ")") {
		// cron(Minutes Hours Day-of-month Month Day-of-week Year)
		cronContent := expr[5 : len(expr)-1]
		parts := strings.Fields(cronContent)
		if len(parts) != 6 {
			return fmt.Errorf("cron expression must have 5 fields: Minutes Hours Day-of-month Month Day-of-week Year")
		}
		// TODO: Add more detailed cron field validation if needed
		return nil
	}

	if strings.HasPrefix(expr, "at(") && strings.HasSuffix(expr, ")") {
		// at(yyyy-mm-ddThh:mm:ss)
		atContent := expr[3 : len(expr)-1]
		// Validate ISO 8601 format
		if _, err := time.Parse(time.RFC3339, atContent); err != nil {
			// Try without timezone
			if _, err := time.Parse("2006-01-02T15:04:05", atContent); err != nil {
				return fmt.Errorf("at() expression must be in ISO 8601 format: yyyy-mm-ddThh:mm:ss")
			}
		}
		return nil
	}

	return fmt.Errorf("schedule expression must start with rate(), cron(), or at()")
}

/*/ ScheduleCreationRequest represents the request to create a schedule
type ScheduleCreationRequest struct {
	Action   string             `json:"action"` // "create", "update", "delete", "pause", "resume"
	Schedule ScheduleDefinition `json:"schedule"`
}*/

// ScheduleDefinition represents the schedule configuration in API requests
type ScheduleDefinition struct {
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description,omitempty"`
	ScheduleExpression string                 `json:"schedule_expression"`
	Timezone           string                 `json:"timezone,omitempty"`
	TargetType         string                 `json:"target_type"`
	Payload            map[string]interface{} `json:"payload"`
}

// Validate checks if the schedule definition is valid
func (sd *ScheduleDefinition) Validate() error {
	if sd.Name == "" {
		return fmt.Errorf("schedule name is required")
	}
	if sd.ScheduleExpression == "" {
		return fmt.Errorf("schedule expression is required")
	}
	if sd.TargetType == "" {
		return fmt.Errorf("target type is required")
	}

	// Validate target type
	targetType := TargetType(sd.TargetType)
	if !targetType.IsValid() {
		return fmt.Errorf("invalid target type: %s (must be one of: web_action, notification, custom)", sd.TargetType)
	}

	// Validate schedule expression
	if err := ValidateScheduleExpression(sd.ScheduleExpression); err != nil {
		return fmt.Errorf("invalid schedule expression: %w", err)
	}

	// Validate timezone if provided
	if sd.Timezone != "" {
		if _, err := time.LoadLocation(sd.Timezone); err != nil {
			return fmt.Errorf("invalid timezone %q: %w", sd.Timezone, err)
		}
	}

	return nil
}
