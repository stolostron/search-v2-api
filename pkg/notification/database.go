// Copyright Contributors to the Open Cluster Management project
package notification

import (
	"context"
	"embed"
	"fmt"

	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"k8s.io/klog/v2"
)

//go:embed setup.sql
var setupSQL embed.FS

// SetupDatabaseTriggers creates the necessary PostgreSQL triggers and functions for notifications
func SetupDatabaseTriggers(ctx context.Context) error {
	if !config.Cfg.Features.NotificationEnabled {
		klog.V(2).Info("PostgreSQL notifications are disabled, skipping trigger setup")
		return nil
	}

	klog.Info("Setting up PostgreSQL triggers for notifications...")

	pool := db.GetConnPool(ctx)
	if pool == nil {
		return fmt.Errorf("failed to get database connection pool")
	}

	// Read the setup SQL file
	sqlContent, err := setupSQL.ReadFile("setup.sql")
	if err != nil {
		return fmt.Errorf("failed to read setup.sql: %w", err)
	}

	// Execute the SQL commands
	_, err = pool.Exec(ctx, string(sqlContent))
	if err != nil {
		return fmt.Errorf("failed to execute database setup: %w", err)
	}

	klog.Info("Successfully set up PostgreSQL triggers for notifications")
	return nil
}

// RemoveDatabaseTriggers removes the notification triggers and functions
func RemoveDatabaseTriggers(ctx context.Context) error {
	klog.Info("Removing PostgreSQL triggers for notifications...")

	pool := db.GetConnPool(ctx)
	if pool == nil {
		return fmt.Errorf("failed to get database connection pool")
	}

	// SQL to remove triggers and functions
	removeSQL := `
		DROP TRIGGER IF EXISTS search_resources_notify_trigger ON search.resources;
		DROP FUNCTION IF EXISTS search.notify_resources_change();
		DROP FUNCTION IF EXISTS search.notify_resources_change_filtered();
	`

	_, err := pool.Exec(ctx, removeSQL)
	if err != nil {
		return fmt.Errorf("failed to remove database triggers: %w", err)
	}

	klog.Info("Successfully removed PostgreSQL triggers for notifications")
	return nil
}

// TestNotificationSetup tests the notification system by inserting, updating, and deleting a test record
func TestNotificationSetup(ctx context.Context) error {
	if !config.Cfg.Features.NotificationEnabled {
		return fmt.Errorf("notifications are disabled")
	}

	klog.Info("Testing PostgreSQL notification setup...")

	pool := db.GetConnPool(ctx)
	if pool == nil {
		return fmt.Errorf("failed to get database connection pool")
	}

	testUID := "test-notification-uid"
	testCluster := "test-cluster"
	testData := map[string]interface{}{
		"kind":      "Pod",
		"name":      "test-notification-pod",
		"namespace": "default",
		"test":      true,
	}

	// Clean up any existing test data
	_, err := pool.Exec(ctx, "DELETE FROM search.resources WHERE uid = $1", testUID)
	if err != nil {
		klog.Warningf("Failed to clean up existing test data: %v", err)
	}

	// Test INSERT
	klog.V(2).Info("Testing INSERT notification...")
	_, err = pool.Exec(ctx,
		"INSERT INTO search.resources (uid, cluster, data) VALUES ($1, $2, $3)",
		testUID, testCluster, testData)
	if err != nil {
		return fmt.Errorf("failed to insert test record: %w", err)
	}

	// Test UPDATE
	klog.V(2).Info("Testing UPDATE notification...")
	testData["updated"] = true
	_, err = pool.Exec(ctx,
		"UPDATE search.resources SET data = $1 WHERE uid = $2",
		testData, testUID)
	if err != nil {
		return fmt.Errorf("failed to update test record: %w", err)
	}

	// Test DELETE
	klog.V(2).Info("Testing DELETE notification...")
	_, err = pool.Exec(ctx, "DELETE FROM search.resources WHERE uid = $1", testUID)
	if err != nil {
		return fmt.Errorf("failed to delete test record: %w", err)
	}

	klog.Info("PostgreSQL notification test completed successfully")
	return nil
}
