# PostgreSQL LISTEN/NOTIFY for Real-Time Search Updates

This package implements real-time notifications for the search-v2-api using PostgreSQL's LISTEN/NOTIFY functionality. It provides an alternative to polling-based subscriptions by delivering immediate notifications when data in the `search.resources` table changes.

## Overview

The notification system consists of:

1. **Database Triggers**: PostgreSQL triggers on `search.resources` that send notifications on INSERT, UPDATE, and DELETE operations
2. **Notification Listener**: Go service that listens for PostgreSQL notifications
3. **Filter System**: Conditional filtering to only receive relevant notifications based on user-defined criteria
4. **GraphQL Integration**: Enhanced subscriptions that use real-time notifications instead of polling

## Configuration

### Environment Variables

Set these environment variables to enable and configure the notification system:

```bash
# Enable the notification feature
FEATURE_NOTIFICATION=true

# PostgreSQL notification channel name (default: search_resources_changes)
NOTIFICATION_CHANNEL_NAME=search_resources_changes

# Buffer size for notification channels (default: 1000)
NOTIFICATION_BUFFER_SIZE=1000

# Reconnection delay in milliseconds (default: 5000)
NOTIFICATION_RECONNECT_DELAY=5000

# Maximum retry attempts (default: 3)
NOTIFICATION_MAX_RETRIES=3
```

### Feature Flags

The notification system is controlled by feature flags in your configuration:

```go
Features: featureFlags{
    NotificationEnabled: true,  // Enable PostgreSQL LISTEN/NOTIFY
    SubscriptionEnabled: true,  // Enable GraphQL subscriptions
}
```

## Database Setup

### Automatic Setup

The application automatically sets up the required triggers when it starts:

1. Creates the `search.notify_resources_change()` function
2. Creates the `search_resources_notify_trigger` trigger on `search.resources`
3. Optionally creates a filtered version of the trigger function

### Manual Setup

If you prefer to set up the triggers manually, run the SQL commands in [`setup.sql`](setup.sql):

```sql
-- Connect to your PostgreSQL database
psql -d your_database -f pkg/notification/setup.sql
```

### Testing the Setup

You can test the notification system:

```go
import "github.com/stolostron/search-v2-api/pkg/notification"

// Test the database setup
err := notification.TestNotificationSetup(ctx)
if err != nil {
    log.Fatalf("Notification test failed: %v", err)
}
```

## Usage

### Basic Subscription

Create a real-time subscription for all changes:

```go
manager := notification.GetNotificationManager()

// Create a filter for all operations
filter := notification.NewFilterBuilder().
    WithOperations("INSERT", "UPDATE", "DELETE").
    Build()

// Subscribe to notifications
subscription, err := manager.Subscribe("my-subscription", filter, nil)
if err != nil {
    return err
}

// Listen for notifications
go func() {
    for payload := range subscription.Channel {
        fmt.Printf("Received %s operation on %s\n", payload.Operation, payload.UID)
        // Handle the notification
    }
}()
```

### Filtered Subscriptions

Create subscriptions with specific filtering criteria:

```go
// Only receive notifications for Pods in the default namespace
filter := notification.NewFilterBuilder().
    WithKinds("Pod").
    WithNamespaces("default").
    WithOperations("INSERT", "UPDATE", "DELETE").
    Build()

subscription, err := manager.Subscribe("pod-subscription", filter, nil)
```

### GraphQL Subscriptions

The existing GraphQL subscription endpoint automatically uses real-time notifications when enabled:

```graphql
subscription {
  experimentalSearch(input: [{
    filters: [{
      property: "kind"
      values: ["Pod"]
    }]
  }]) {
    count
    items
    related {
      kind
      count
    }
  }
}
```

## Filtering

### Available Filter Options

```go
type NotificationFilter struct {
    Kinds       []string               // Filter by resource kinds (e.g., "Pod", "Deployment")
    Namespaces  []string               // Filter by namespaces
    Clusters    []string               // Filter by clusters
    Labels      map[string]string      // Filter by labels
    Properties  map[string]interface{} // Filter by other properties
    Operations  []string               // Filter by operations (INSERT, UPDATE, DELETE)
}
```

### RBAC Integration

Filters automatically respect RBAC permissions:

```go
// Create a filter that respects user's RBAC permissions
filter, err := notification.CreateSubscriptionFilter(ctx, searchInput)
if err != nil {
    return err
}
```

The system automatically:
- Restricts clusters to those the user has access to
- Filters namespaces based on user permissions
- Applies cluster-admin privileges when appropriate

### Filter Examples

```go
// Monitor only Deployment changes in production namespaces
filter := notification.NewFilterBuilder().
    WithKinds("Deployment").
    WithNamespaces("prod-web", "prod-api").
    WithOperations("UPDATE", "DELETE").
    Build()

// Monitor resources with specific labels
filter := notification.NewFilterBuilder().
    WithLabel("environment", "production").
    WithLabel("team", "platform").
    Build()

// Monitor specific clusters
filter := notification.NewFilterBuilder().
    WithClusters("us-east-1", "us-west-2").
    WithKinds("Node", "Pod").
    Build()
```

## Integration with Existing Code

### Fallback to Polling

The system gracefully falls back to polling-based subscriptions if:
- PostgreSQL notifications are disabled
- Database connection fails
- Filter creation fails

### Subscription Management

```go
manager := notification.GetNotificationManager()

// List active subscriptions
subscriptions := manager.ListSubscriptions()

// Get specific subscription
subscription, exists := manager.GetSubscription("my-sub-id")

// Clean up subscription
err := manager.Unsubscribe("my-sub-id")
```

## Performance Considerations

### Throttling

The system includes built-in throttling to prevent overwhelming the search API:

- Search queries are throttled based on `SUBSCRIPTION_REFRESH_INTERVAL`
- Rapid notifications are batched to avoid excessive database queries

### Buffer Management

- Notification channels have configurable buffer sizes
- Full channels will drop notifications (logged as warnings)
- Consider increasing `NOTIFICATION_BUFFER_SIZE` for high-throughput scenarios

### Resource Usage

- Each subscription creates a goroutine for handling notifications
- PostgreSQL connection is shared across all subscriptions
- Automatic cleanup when subscriptions are cancelled

## Monitoring and Logging

### Log Levels

- `V(1)`: High-level operations (start/stop, connections)
- `V(2)`: Subscription lifecycle (create/delete subscriptions)
- `V(3)`: Notification processing
- `V(4)`: Detailed notification flow and throttling

### Example Log Output

```
I1210 15:30:45.123456 1 listener.go:95] Starting PostgreSQL notification listener...
I1210 15:30:45.234567 1 listener.go:112] PostgreSQL notification listener started on channel: search_resources_changes
I1210 15:30:45.345678 1 filter.go:234] Created notification subscription: search-subscription-1702218645123456789
I1210 15:30:46.456789 1 listener.go:234] Received notification on channel search_resources_changes: {"operation":"UPDATE","uid":"cluster1/pod-123","timestamp":1702218646}
```

## Troubleshooting

### Common Issues

1. **Notifications not received**
   - Check that `FEATURE_NOTIFICATION=true`
   - Verify database triggers are installed
   - Check PostgreSQL logs for connection issues

2. **High CPU usage**
   - Increase `NOTIFICATION_BUFFER_SIZE`
   - Add more specific filters to reduce notification volume
   - Check for notification loops

3. **Connection errors**
   - Verify database credentials and connectivity
   - Check `NOTIFICATION_RECONNECT_DELAY` setting
   - Monitor `NOTIFICATION_MAX_RETRIES` configuration

### Debug Commands

```bash
# Check if triggers are installed
psql -d your_db -c "\df search.notify_resources_change"

# Listen to notifications manually
psql -d your_db -c "LISTEN search_resources_changes;"

# Test notification with manual insert
psql -d your_db -c "INSERT INTO search.resources (uid, cluster, data) VALUES ('test', 'test-cluster', '{\"kind\":\"Pod\"}');"
```

## Security Considerations

- Notifications respect existing RBAC permissions
- Users only receive notifications for resources they have access to
- No sensitive data is exposed in notification payloads
- Database triggers run with appropriate PostgreSQL permissions

## Migration from Polling

To migrate from polling-based to real-time subscriptions:

1. Set `FEATURE_NOTIFICATION=true`
2. Restart the application
3. Existing GraphQL subscriptions automatically use real-time notifications
4. Monitor logs to ensure proper operation
5. Optionally tune configuration parameters for your workload 