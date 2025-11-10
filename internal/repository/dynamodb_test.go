package repository

import (
	"context"
	"testing"

	"github.com/jrzesz33/rez_agent/internal/models"
)

// Note: These are basic unit tests. For integration tests with actual DynamoDB,
// you would use localstack or DynamoDB Local with testcontainers

func TestNewDynamoDBRepository(t *testing.T) {
	tableName := "test-table"
	repo := NewDynamoDBRepository(nil, tableName)

	if repo == nil {
		t.Error("NewDynamoDBRepository() returned nil")
	}
	if repo.tableName != tableName {
		t.Errorf("tableName = %v, want %v", repo.tableName, tableName)
	}
}

func TestDynamoDBRepository_Interface(t *testing.T) {
	// Verify that DynamoDBRepository implements MessageRepository
	var _ MessageRepository = (*DynamoDBRepository)(nil)
}

// Mock test to ensure method signatures are correct
func TestDynamoDBRepository_MethodSignatures(t *testing.T) {
	repo := &DynamoDBRepository{
		client:    nil,
		tableName: "test-table",
	}

	ctx := context.Background()
	_payload := make(map[string]interface{})
	_payload["key"] = "value"
	msg := models.NewMessage("test", nil, "1.0", models.StageDev, models.MessageTypeHelloWorld, _payload)

	// These will fail at runtime due to nil client, but ensure method signatures compile
	t.Run("SaveMessage signature", func(t *testing.T) {
		if repo.client == nil {
			t.Skip("skipping test with nil client")
		}
		_ = repo.SaveMessage(ctx, msg)
	})

	t.Run("GetMessage signature", func(t *testing.T) {
		if repo.client == nil {
			t.Skip("skipping test with nil client")
		}
		_, _ = repo.GetMessage(ctx, "test-id")
	})

	t.Run("ListMessages signature", func(t *testing.T) {
		if repo.client == nil {
			t.Skip("skipping test with nil client")
		}
		stage := models.StageDev
		status := models.StatusCreated
		_, _ = repo.ListMessages(ctx, &stage, &status, 10)
	})

	t.Run("UpdateStatus signature", func(t *testing.T) {
		if repo.client == nil {
			t.Skip("skipping test with nil client")
		}
		_ = repo.UpdateStatus(ctx, "test-id", models.StatusCompleted, "")
	})
}
