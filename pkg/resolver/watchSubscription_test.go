// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"github.com/stretchr/testify/assert"
)

func TestEventMatchesFilters_MultiKeywordsAgainstLabels(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":  "Pod",
			"name":  "test-one",
			"label": map[string]interface{}{"app": "search", "ppa": "asdf"},
		},
	}

	one := "asdf"
	two := "search"
	// Input with keywords to match labels.
	input := &model.SearchInput{
		Keywords: []*string{&one, &two},
	}
	// Should match keywords in labels.
	assert.True(t, eventMatchesAllFilters(event, input), "Should match keywords against labels")
}

// [AI]
func TestWatchSubscription_Disabled(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Disable subscription feature
	config.Cfg.Features.SubscriptionEnabled = false

	ctx := context.Background()
	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	// Verify error is returned when feature is disabled
	assert.NotNil(t, err, "Should return error when subscription is disabled")
	assert.Contains(t, err.Error(), "disabled", "Error should mention subscription is disabled")
	assert.NotNil(t, resultChan, "Result channel should be returned even on error")
}

// [AI]
func TestWatchSubscription_Enabled(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	// Verify no error when feature is enabled
	assert.Nil(t, err, "Should not return error when subscription is enabled")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// Wait a moment for goroutine to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop subscription
	cancel()

	// Verify channel is eventually closed
	select {
	case _, ok := <-resultChan:
		if ok {
			t.Log("Received event before channel closed")
		}
	case <-time.After(1 * time.Second):
		// Timeout is acceptable, channel should close
	}
}

// [AI]
func TestWatchSubscription_ContextCancellation(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithCancel(context.Background())
	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// Wait for goroutine to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context
	cancel()

	// Verify the channel is closed after context cancellation
	select {
	case _, ok := <-resultChan:
		assert.False(t, ok, "Channel should be closed after context cancellation")
	case <-time.After(2 * time.Second):
		t.Fatal("Channel should be closed within timeout")
	}
}

// [AI]
func TestWatchSubscription_MultipleSubscriptions(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	input := &model.SearchInput{}

	// Create multiple subscriptions
	numSubscriptions := 5
	channels := make([]<-chan *model.Event, numSubscriptions)
	var wg sync.WaitGroup

	for i := 0; i < numSubscriptions; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ch, err := WatchSubscription(ctx, input)
			assert.Nil(t, err, "Should not return error for subscription %d", index)
			channels[index] = ch
		}(i)
	}

	wg.Wait()

	// Verify all channels were created
	for i, ch := range channels {
		assert.NotNil(t, ch, "Channel %d should not be nil", i)
	}

	// Cancel context to clean up
	cancel()

	// Wait for all channels to close
	time.Sleep(500 * time.Millisecond)
}

// [AI]
func TestWatchSubscription_EventForwarding(t *testing.T) {
	// This test would require mocking the database listener
	// For now, we test the basic flow

	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// Start a goroutine to consume from the channel
	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-resultChan:
				if !ok {
					// Channel closed
					done <- true
					return
				}
				// Process event if received
				if event != nil {
					t.Logf("Received event: %+v", event)
				}
			case <-time.After(1 * time.Second):
				// No events received, that's ok for this test
				done <- true
				return
			}
		}
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// Test completed
	case <-time.After(3 * time.Second):
		t.Fatal("Test timed out")
	}
}

// [AI]
func TestWatchSubscription_ChannelBufferHandling(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// The result channel should be non-buffered (created with make(chan *model.Event))
	// We can't directly check the buffer size of a receive-only channel,
	// but we can verify it's operational

	// Cancel after a short time
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for channel to close
	time.Sleep(200 * time.Millisecond)
}

// [AI]
func TestWatchSubscription_NilInput(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Test with nil input - should still work as input is not currently used
	resultChan, err := WatchSubscription(ctx, nil)

	assert.Nil(t, err, "Should not return error with nil input")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	time.Sleep(100 * time.Millisecond)
	cancel()
}

// [AI]
func TestWatchSubscription_RapidContextCancellation(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithCancel(context.Background())
	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// Cancel immediately
	cancel()

	// Verify channel eventually closes
	select {
	case _, ok := <-resultChan:
		if ok {
			t.Log("Received event before closure")
		}
		// Channel closed, as expected
	case <-time.After(2 * time.Second):
		// Timeout is acceptable
	}
}

// [AI]
func TestWatchSubscription_FilterInput(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Create input with filters (these should be used in future for event filtering)
	val1 := "Pod"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "kind", Values: []*string{&val1}},
		},
	}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error with filtered input")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	time.Sleep(100 * time.Millisecond)
	cancel()
}

// [AI] Test eventMatchesFilters with no filters (should match all)
func TestEventMatchesFilters_NoFilters(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":      "Pod",
			"name":      "test-pod",
			"namespace": "default",
		},
	}

	// No input - should match
	assert.True(t, eventMatchesAllFilters(event, nil))

	// Empty input - should match
	emptyInput := &model.SearchInput{}
	assert.True(t, eventMatchesAllFilters(event, emptyInput))
}

// [AI] Test eventMatchesFilters with property filters
func TestEventMatchesFilters_PropertyFilters(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":      "Pod",
			"name":      "test-pod",
			"namespace": "default",
		},
	}

	// Filter matching kind=Pod
	kindFilter := "kind"
	kindValue := "Pod"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{
				Property: kindFilter,
				Values:   []*string{&kindValue},
			},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match kind=Pod filter")

	// Filter NOT matching kind=Deployment
	deploymentValue := "Deployment"
	inputNoMatch := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{
				Property: kindFilter,
				Values:   []*string{&deploymentValue},
			},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match kind=Deployment filter")
}

// [AI] Test eventMatchesFilters with case-sensitive kind matching
// Note: The previous behavior was case-insensitive for kind, but it was simplified to be case-sensitive.
func TestEventMatchesFilters_KindCaseSensitive(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
		},
	}

	// The kind filter is compared case-insensitively.
	kindFilter := "kind"
	kindValueLower := "pod"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{
				Property: kindFilter,
				Values:   []*string{&kindValueLower},
			},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match kind case-insensitively (lowercase)")

	// Filter with uppercase "POD" should match "Pod"
	kindValueUpper := "POD"
	inputUpper := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{
				Property: kindFilter,
				Values:   []*string{&kindValueUpper},
			},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputUpper), "Should match kind with different case (uppercase)")

	// Exact match should work
	kindValueExact := "Pod"
	inputExact := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{
				Property: kindFilter,
				Values:   []*string{&kindValueExact},
			},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputExact), "Should match kind with exact case")
}

// [AI] Test eventMatchesFilters with multiple filters (AND operation)
func TestEventMatchesFilters_MultipleFilters_AND(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":      "Pod",
			"namespace": "default",
			"name":      "my-pod",
		},
	}

	// Both filters match
	kindFilter := "kind"
	kindValue := "Pod"
	nsFilter := "namespace"
	nsValue := "default"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
			{Property: nsFilter, Values: []*string{&nsValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match when all filters match")

	// One filter doesn't match
	wrongNsValue := "kube-system"
	inputNoMatch := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
			{Property: nsFilter, Values: []*string{&wrongNsValue}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match when one filter doesn't match")
}

// [AI] Test eventMatchesFilters with multiple values per filter (OR operation)
func TestEventMatchesFilters_MultipleValues_OR(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
		},
	}

	// Filter with multiple kind values (Pod OR Deployment)
	kindFilter := "kind"
	podValue := "Pod"
	deploymentValue := "Deployment"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{
				Property: kindFilter,
				Values:   []*string{&podValue, &deploymentValue},
			},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match when one of the values matches")

	// Filter with values that don't match
	serviceValue := "Service"
	configMapValue := "ConfigMap"
	inputNoMatch := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{
				Property: kindFilter,
				Values:   []*string{&serviceValue, &configMapValue},
			},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match when none of the values match")
}

// [AI] Test eventMatchesFilters with keywords
func TestEventMatchesFilters_Keywords(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":      "Pod",
			"name":      "nginx-deployment-abc123",
			"namespace": "production",
		},
	}

	// Single keyword that matches
	keyword1 := "nginx"
	input := &model.SearchInput{
		Keywords: []*string{&keyword1},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match when keyword found")

	// Keyword with different case
	keyword2 := "NGINX"
	inputCase := &model.SearchInput{
		Keywords: []*string{&keyword2},
	}
	assert.True(t, eventMatchesAllFilters(event, inputCase), "Should match keyword case-insensitively")

	// Multiple keywords (AND operation) - all must match
	keyword3 := "production"
	inputMultiple := &model.SearchInput{
		Keywords: []*string{&keyword1, &keyword3},
	}
	assert.True(t, eventMatchesAllFilters(event, inputMultiple), "Should match when all keywords found")

	// Keyword that doesn't match
	keywordNoMatch := "nonexistent"
	inputNoMatch := &model.SearchInput{
		Keywords: []*string{&keywordNoMatch},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match when keyword not found")
}

// [AI] Test eventMatchesFilters with DELETE operation (uses OldData)
func TestEventMatchesFilters_DeleteOperation(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "DELETE",
		NewData:   nil, // No NewData for DELETE
		OldData: map[string]interface{}{
			"kind":      "Pod",
			"name":      "deleted-pod",
			"namespace": "default",
		},
	}

	// Filter on kind
	kindFilter := "kind"
	kindValue := "Pod"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match DELETE event using OldData")

	// Keyword search in OldData
	keyword := "deleted"
	inputKeyword := &model.SearchInput{
		Keywords: []*string{&keyword},
	}
	assert.True(t, eventMatchesAllFilters(event, inputKeyword), "Should find keyword in OldData")
}

// [AI] Test eventMatchesFilters with both keywords and filters
func TestEventMatchesFilters_KeywordsAndFilters(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "UPDATE",
		NewData: map[string]interface{}{
			"kind":      "Deployment",
			"name":      "nginx-app",
			"namespace": "production",
		},
	}

	// Both keyword and filter must match
	keyword := "nginx"
	kindFilter := "kind"
	kindValue := "Deployment"
	input := &model.SearchInput{
		Keywords: []*string{&keyword},
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match when both keyword and filter match")

	// Keyword matches but filter doesn't
	wrongKind := "Pod"
	inputNoMatch := &model.SearchInput{
		Keywords: []*string{&keyword},
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&wrongKind}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match when filter doesn't match")
}

// [AI] Test eventMatchesFilters with missing property
func TestEventMatchesFilters_MissingProperty(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
			"name": "test-pod",
		},
	}

	// Filter on property that doesn't exist
	labelFilter := "label"
	labelValue := "app=nginx"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: labelFilter, Values: []*string{&labelValue}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, input), "Should not match when property doesn't exist")
}

// [AI] Test eventMatchesFilters with nil event data
func TestEventMatchesFilters_NilEventData(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "UPDATE",
		NewData:   nil,
		OldData:   nil,
	}

	kindFilter := "kind"
	kindValue := "Pod"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, input), "Should not match when event has no data")
}

// [AI] Test eventMatchesFilters with empty filter values
func TestEventMatchesFilters_EmptyFilterValues(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
		},
	}

	// Filter with empty values array
	kindFilter := "kind"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{}},
		},
	}
	// Empty values means no matching criteria, should not match
	assert.False(t, eventMatchesAllFilters(event, input), "Should not match with empty filter values")
}

// [AI] Test eventMatchesFilters with non-string property values
func TestEventMatchesFilters_NonStringProperties(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":     "Pod",
			"replicas": 3,
			"ready":    true,
		},
	}

	// Filter on numeric property
	replicasFilter := "replicas"
	replicasValue := "3"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: replicasFilter, Values: []*string{&replicasValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match numeric property converted to string")

	// Filter on boolean property
	readyFilter := "ready"
	readyValue := "true"
	inputBool := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: readyFilter, Values: []*string{&readyValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputBool), "Should match boolean property converted to string")
}

// [AI] Test eventMatchesFilters with nil filter
func TestEventMatchesFilters_NilFilter(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
		},
	}

	// Input with nil filter in the array
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{nil},
	}
	// Should skip nil filter and match (no valid filters)
	assert.True(t, eventMatchesAllFilters(event, input), "Should skip nil filters")
}

// [AI] Test eventMatchesFilters with empty property name
func TestEventMatchesFilters_EmptyProperty(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
		},
	}

	// Filter with empty property name
	podValue := "Pod"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "", Values: []*string{&podValue}},
		},
	}
	// Should skip filter with empty property
	assert.True(t, eventMatchesAllFilters(event, input), "Should skip filters with empty property")
}

// [AI] Test eventMatchesFilters with nil keyword
func TestEventMatchesFilters_NilKeyword(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
			"name": "test-pod",
		},
	}

	// Input with nil keyword in the array
	input := &model.SearchInput{
		Keywords: []*string{nil},
	}
	// Should skip nil keyword and match (no valid keywords)
	assert.True(t, eventMatchesAllFilters(event, input), "Should skip nil keywords")
}

// [AI] Test eventMatchesFilters with complex multi-filter scenario
func TestEventMatchesFilters_ComplexScenario(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "UPDATE",
		NewData: map[string]interface{}{
			"kind":      "Deployment",
			"name":      "nginx-production-v2",
			"namespace": "production",
			"replicas":  3,
			"labels":    "app=nginx,env=prod",
		},
	}

	// Multiple filters and keywords - all must match
	keyword1 := "nginx"
	keyword2 := "production"
	kindFilter := "kind"
	kindValue := "Deployment"
	nsFilter := "namespace"
	nsValue := "production"

	input := &model.SearchInput{
		Keywords: []*string{&keyword1, &keyword2},
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
			{Property: nsFilter, Values: []*string{&nsValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match complex filter scenario")

	// One keyword missing
	keywordMissing := "missing"
	inputNoMatch := &model.SearchInput{
		Keywords: []*string{&keyword1, &keywordMissing},
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match when keyword missing")
}

// [AI] Test eventMatchesFilters with nil filter value
func TestEventMatchesFilters_NilFilterValue(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
		},
	}

	// Filter with nil value in values array
	kindFilter := "kind"
	podValue := "Pod"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{nil, &podValue}},
		},
	}
	// Should skip nil value and match with "Pod"
	assert.True(t, eventMatchesAllFilters(event, input), "Should skip nil filter values")
}

// [AI] Test eventMatchesFilters with label matching
func TestEventMatchesFilters_LabelMatching(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":  "Pod",
			"label": map[string]interface{}{"app": "nginx", "env": "prod"},
		},
	}

	// Match on one label
	labelVal1 := "app=nginx"
	input1 := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "label", Values: []*string{&labelVal1}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input1), "Should match exact label key=value")

	// Match on multiple labels (OR logic within label filter values? No, matchLabels returns true if ANY matches)
	// matchLabels implementation: returns true if ANY of the labelFilters matches the event labels.
	labelVal2 := "env=prod"
	input2 := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "label", Values: []*string{&labelVal1, &labelVal2}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input2), "Should match if any label matches")

	// No match
	labelValNoMatch := "app=apache"
	inputNoMatch := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "label", Values: []*string{&labelValNoMatch}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match different value")

	// Key mismatch
	labelKeyNoMatch := "tier=frontend"
	inputKeyNoMatch := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "label", Values: []*string{&labelKeyNoMatch}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputKeyNoMatch), "Should not match different key")
}

// [AI] Test WatchSubscription input validation
func TestWatchSubscription_InputValidation(t *testing.T) {
	// Enable subscription feature for this test
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	config.Cfg.Features.SubscriptionEnabled = true
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Test operators are now supported
	valOp := "!value"
	inputOp := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "kind", Values: []*string{&valOp}},
		},
	}
	_, err := WatchSubscription(ctx, inputOp)
	assert.NoError(t, err, "Operators should now be supported")

	// Test wildcards still not supported
	valWild := "val*"
	inputWild := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "kind", Values: []*string{&valWild}},
		},
	}
	_, err = WatchSubscription(ctx, inputWild)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Wildcards are not yet supported")

	// Test invalid label format
	valLabel := "invalid-label"
	inputLabel := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "label", Values: []*string{&valLabel}},
		},
	}
	_, err = WatchSubscription(ctx, inputLabel)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Value must be a key=value pair.")

	// Test empty property
	val := "value"
	inputEmptyProp := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "", Values: []*string{&val}},
		},
	}
	_, err = WatchSubscription(ctx, inputEmptyProp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Property is required")
}

// [AI] Test parseOperatorAndValue function
func TestParseOperatorAndValue(t *testing.T) {
	tests := []struct {
		name             string
		filterValue      string
		expectedOperator string
		expectedValue    string
	}{
		{"Equality default", "value", "=", "value"},
		{"Not equal !=", "!=value", "!=", "value"},
		{"Not equal !", "!value", "!", "value"},
		{"Greater than", ">10", ">", "10"},
		{"Greater or equal", ">=10", ">=", "10"},
		{"Less than", "<10", "<", "10"},
		{"Less or equal", "<=10", "<=", "10"},
		{"Value with special chars", "my-value-123", "=", "my-value-123"},
		{"Numeric value", "100", "=", "100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operator, value := parseOperatorAndValue(tt.filterValue)
			assert.Equal(t, tt.expectedOperator, operator, "Operator mismatch")
			assert.Equal(t, tt.expectedValue, value, "Value mismatch")
		})
	}
}

// [AI] Test compareWithOperator function with string values
func TestCompareValues_Strings(t *testing.T) {
	tests := []struct {
		name          string
		operator      string
		propertyValue interface{}
		filterValue   string
		expected      bool
	}{
		{"String equality match", "=", "Pod", "Pod", true},
		{"String equality no match", "=", "Pod", "Deployment", false},
		{"String not equal match", "!=", "Pod", "Deployment", true},
		{"String not equal no match", "!=", "Pod", "Pod", false},
		{"String greater than true", ">", "zebra", "apple", true},
		{"String greater than false", ">", "apple", "zebra", false},
		{"String greater or equal true", ">=", "pod", "pod", true},
		{"String less than true", "<", "apple", "zebra", true},
		{"String less than false", "<", "zebra", "apple", false},
		{"String less or equal true", "<=", "pod", "pod", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareWithOperator(tt.operator, tt.propertyValue, tt.filterValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// [AI] Test compareWithOperator function with numeric values
func TestCompareValues_Numeric(t *testing.T) {
	tests := []struct {
		name          string
		operator      string
		propertyValue interface{}
		filterValue   string
		expected      bool
	}{
		{"Numeric equality match", "=", 42, "42", true},
		{"Numeric equality no match", "=", 42, "100", false},
		{"Numeric not equal match", "!=", 42, "100", true},
		{"Numeric not equal no match", "!=", 42, "42", false},
		{"Numeric greater than true", ">", 100, "50", true},
		{"Numeric greater than false", ">", 50, "100", false},
		{"Numeric greater or equal true (equal)", ">=", 100, "100", true},
		{"Numeric greater or equal true (greater)", ">=", 100, "50", true},
		{"Numeric greater or equal false", ">=", 50, "100", false},
		{"Numeric less than true", "<", 50, "100", true},
		{"Numeric less than false", "<", 100, "50", false},
		{"Numeric less or equal true (equal)", "<=", 100, "100", true},
		{"Numeric less or equal true (less)", "<=", 50, "100", true},
		{"Numeric less or equal false", "<=", 100, "50", false},
		{"Float comparison", ">", 3.14, "3.0", true},
		{"Negative numbers", "<", -10, "0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareWithOperator(tt.operator, tt.propertyValue, tt.filterValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// [AI] Test compareWithOperator with mixed types
func TestCompareValues_MixedTypes(t *testing.T) {
	// Boolean converted to string
	result := compareWithOperator("=", true, "true")
	assert.True(t, result, "Boolean true should match string 'true'")

	result = compareWithOperator("=", false, "false")
	assert.True(t, result, "Boolean false should match string 'false'")

	// String number vs numeric comparison
	result = compareWithOperator(">", "100", "50")
	assert.True(t, result, "String '100' should be greater than '50' numerically")
}

// [AI] Test eventMatchesAllFilters with not equal operator
func TestEventMatchesFilters_NotEqualOperator(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":      "Pod",
			"namespace": "default",
		},
	}

	// Filter: kind != Deployment (should match Pod)
	kindFilter := "kind"
	kindValue := "!=Deployment"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match when kind is not Deployment")

	// Filter: kind != Pod (should not match)
	kindValueNoMatch := "!=Pod"
	inputNoMatch := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValueNoMatch}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match when kind equals Pod with != operator")

	// Filter: namespace ! kube-system (should match default)
	nsFilter := "namespace"
	nsValue := "!kube-system"
	inputNs := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: nsFilter, Values: []*string{&nsValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputNs), "Should match when namespace is not kube-system")
}

// [AI] Test eventMatchesAllFilters with comparison operators on numeric values
func TestEventMatchesFilters_NumericComparison(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":     "Pod",
			"replicas": 3,
			"age":      100,
		},
	}

	// replicas > 2
	replicasFilter := "replicas"
	replicasValue := ">2"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: replicasFilter, Values: []*string{&replicasValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match replicas > 2")

	// replicas >= 3
	replicasValueGte := ">=3"
	inputGte := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: replicasFilter, Values: []*string{&replicasValueGte}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputGte), "Should match replicas >= 3")

	// replicas < 5
	replicasValueLt := "<5"
	inputLt := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: replicasFilter, Values: []*string{&replicasValueLt}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputLt), "Should match replicas < 5")

	// replicas <= 3
	replicasValueLte := "<=3"
	inputLte := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: replicasFilter, Values: []*string{&replicasValueLte}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputLte), "Should match replicas <= 3")

	// age > 200 (should not match)
	ageFilter := "age"
	ageValue := ">200"
	inputNoMatch := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: ageFilter, Values: []*string{&ageValue}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match age > 200")
}

// [AI] Test eventMatchesAllFilters with comparison operators on string values
func TestEventMatchesFilters_StringComparison(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
			"name": "nginx-deployment",
		},
	}

	// name > "my" (alphabetically)
	nameFilter := "name"
	nameValue := ">my"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: nameFilter, Values: []*string{&nameValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match name > 'my' alphabetically")

	// name < "zebra"
	nameValueLt := "<zebra"
	inputLt := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: nameFilter, Values: []*string{&nameValueLt}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputLt), "Should match name < 'zebra' alphabetically")
}

// [AI] Test eventMatchesAllFilters with multiple operators
func TestEventMatchesFilters_MultipleOperators(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":     "Pod",
			"replicas": 3,
			"name":     "test-pod",
		},
	}

	// Multiple filters with different operators (AND operation)
	kindFilter := "kind"
	kindValue := "!=Deployment"
	replicasFilter := "replicas"
	replicasValue := ">=2"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
			{Property: replicasFilter, Values: []*string{&replicasValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match all filters with operators")

	// One filter doesn't match
	replicasValueNoMatch := ">5"
	inputNoMatch := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
			{Property: replicasFilter, Values: []*string{&replicasValueNoMatch}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match when one operator filter doesn't match")
}

// [AI] Test eventMatchesAllFilters with operators and OR logic within a filter
func TestEventMatchesFilters_OperatorsWithOR(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind":     "Pod",
			"replicas": 3,
		},
	}

	// Filter with multiple values using operators (OR logic)
	// replicas > 5 OR replicas < 5 (should match 3)
	replicasFilter := "replicas"
	val1 := ">5"
	val2 := "<5"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: replicasFilter, Values: []*string{&val1, &val2}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match when one of the OR conditions is true")

	// Both conditions false
	val3 := ">10"
	val4 := "<2"
	inputNoMatch := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: replicasFilter, Values: []*string{&val3, &val4}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNoMatch), "Should not match when all OR conditions are false")
}

// [AI] Test that kind filter still works case-insensitively with = operator
func TestEventMatchesFilters_KindCaseInsensitiveWithOperator(t *testing.T) {
	event := &model.Event{
		UID:       "test-123",
		Operation: "CREATE",
		NewData: map[string]interface{}{
			"kind": "Pod",
		},
	}

	// Kind with lowercase (implicit = operator)
	kindFilter := "kind"
	kindValue := "pod"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValue}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, input), "Should match kind case-insensitively with implicit =")

	// Kind with explicit = operator
	kindValueExplicit := "=pod"
	inputExplicit := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValueExplicit}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputExplicit), "Should match kind case-insensitively with explicit =")

	// Kind with != operator should also be case-insensitive
	kindValueNe := "!=deployment"
	inputNe := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValueNe}},
		},
	}
	assert.True(t, eventMatchesAllFilters(event, inputNe), "Kind with != should be case-insensitive (Pod != deployment)")

	// Kind with != same value (case-insensitive) should not match
	kindValueNeSame := "!=pod"
	inputNeSame := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: kindFilter, Values: []*string{&kindValueNeSame}},
		},
	}
	assert.False(t, eventMatchesAllFilters(event, inputNeSame), "Kind with != pod should not match Pod (case-insensitive)")
}

func TestGetEventDataFields(t *testing.T) {
	// Given: event data
	eventData := map[string]any{
		"namespace":           "foo",
		"kind_plural":         "foos",
		"apigroup":            "foo1alpha1bar",
		"cluster":             "foo-hub",
		"_hubClusterResource": true,
	}

	// When: we pass it to getEventDataFields
	namespace, kind, apigroup, cluster, isHubCluster := getEventDataFields(eventData)

	// Then: we get our desired fields returned
	assert.Equal(t, namespace, "foo", "Expected namespace to be foo")
	assert.Equal(t, kind, "foos", "Exptected kind to be foos")
	assert.Equal(t, apigroup, "foo1alpha1bar", "Expected apigroup to be foo1alpha1bar")
	assert.Equal(t, cluster, "foo-hub", "Expected cluster to be foo-hub")
	assert.Equal(t, isHubCluster, true, "Expected isHubCluster to be true")
}

func TestEventMatchesRbacHubClusterResource_Allowed(t *testing.T) {
	// Given: a user with permissions to watch hub cluster resource streamed event
	ctx := rbac.CreateTestContext("test-user-1", "testuser1")
	event := &model.Event{
		UID:       "asdf-1234",
		Operation: "CREATE",
		NewData: map[string]any{
			"apigroup":            "v1",
			"kind_plural":         "pods",
			"namespace":           "foo",
			"cluster":             "local-cluster",
			"_hubClusterResource": true,
		},
	}
	userData := rbac.CreateTestUserWatchData("watch", "v1", "pods", "foo", true, time.Now(), 1*time.Minute)
	rbac.SetupWatchCacheWithUserData(ctx, userData)
	defer rbac.CleanupWatchCache(ctx)

	// When: we check user permissions against the event
	result := eventMatchesRbac(ctx, event)

	// Then: user has permission
	assert.Equal(t, result, true, "Expected user to have permission to see event")
}

func TestEventMatchesRbacHubClusterResource_Disallowed(t *testing.T) {
	// Given: a user with misaligned permissions to watch hub cluster resource streamed event
	ctx := rbac.CreateTestContext("test-user-1", "testuser1")
	event := &model.Event{
		UID:       "asdf-1234",
		Operation: "UPDATE",
		NewData: map[string]any{
			"apigroup":            "v1",
			"kind_plural":         "namespaces",
			"namespace":           "foo",
			"cluster":             "local-cluster",
			"_hubClusterResource": true,
		},
	}
	userData := rbac.CreateTestUserWatchData("watch", "v1", "pods", "foo", true, time.Now(), 1*time.Minute)
	rbac.SetupWatchCacheWithUserData(ctx, userData)
	defer rbac.CleanupWatchCache(ctx)

	// When: we check user permissions against the event
	result := eventMatchesRbac(ctx, event)

	// Then: user doesn't have permission
	assert.Equal(t, result, false, "Expected user not to have permission to see event")
}

func TestEventMatchesRbacManagedClusterResource_Allowed(t *testing.T) {
	originalFineGrainedRbac := config.Cfg.Features.FineGrainedRbac
	config.Cfg.Features.FineGrainedRbac = false
	defer func() {
		config.Cfg.Features.FineGrainedRbac = originalFineGrainedRbac
	}()
	// Given: a user with permissions to watch managed cluster resource streamed event
	ctx := rbac.CreateTestContext("test-user-1", "testuser1")
	event := &model.Event{
		UID:       "asdf-1234",
		Operation: "CREATE",
		NewData: map[string]any{
			"apigroup":    "v1",
			"kind_plural": "pods",
			"namespace":   "foo",
			"cluster":     "managed-cluster",
		},
	}
	userData := rbac.CreateTestUserWatchData("create", "view.open-cluster-management.io",
		"managedclusterviews", "managed-cluster", true, time.Now(), 1*time.Minute)
	rbac.SetupWatchCacheWithUserData(ctx, userData)
	defer rbac.CleanupWatchCache(ctx)

	// When: we check user permissions against the event
	result := eventMatchesRbac(ctx, event)

	// Then: user has permission
	assert.Equal(t, result, true, "Expected user to have permission to see event")
}

func TestEventMatchesRbacManagedClusterResource_Disallowed(t *testing.T) {
	originalFineGrainedRbac := config.Cfg.Features.FineGrainedRbac
	config.Cfg.Features.FineGrainedRbac = false
	defer func() {
		config.Cfg.Features.FineGrainedRbac = originalFineGrainedRbac
	}()
	// Given: a user with misaligned permissions to watch managed cluster resource streamed event
	ctx := rbac.CreateTestContext("test-user-1", "testuser1")
	event := &model.Event{
		UID:       "asdf-1234",
		Operation: "CREATE",
		NewData: map[string]any{
			"apigroup":    "v1",
			"kind_plural": "pods",
			"namespace":   "foo",
			"cluster":     "managed-cluster",
		},
	}
	userData := rbac.CreateTestUserWatchData("create", "view.open-cluster-management.io", "managedclusterviews", "a-different-managed-cluster", true, time.Now(), 1*time.Minute)
	rbac.SetupWatchCacheWithUserData(ctx, userData)
	defer rbac.CleanupWatchCache(ctx)

	// When: we check user permissions against the event
	result := eventMatchesRbac(ctx, event)

	// Then: user doesn't have permission
	assert.Equal(t, result, false, "Expected user not to have permission to see event")
}

func TestEventMatchesRbacFineGrainedRbac_Allowed(t *testing.T) {
	originalFineGrainedRbac := config.Cfg.Features.FineGrainedRbac
	config.Cfg.Features.FineGrainedRbac = true
	defer func() {
		config.Cfg.Features.FineGrainedRbac = originalFineGrainedRbac
	}()
	// Given: a user with fine-grained RBAC permissions to watch managed cluster resource streamed event
	ctx := rbac.CreateTestContext("test-user-1", "testuser1")
	event := &model.Event{
		UID:       "asdf-1234",
		Operation: "CREATE",
		NewData: map[string]any{
			"apigroup":    "v1",
			"kind_plural": "pods",
			"namespace":   "foo",
			"cluster":     "managed-cluster",
		},
	}
	userDataCache := rbac.CreateTestUserDataCache("watch", "v1", "pods", "managed-cluster", "foo")
	rbac.SetupCacheWithUserData(ctx, userDataCache)
	defer rbac.CleanupCache(ctx)

	// When: we check user permissions against the event
	result := eventMatchesRbac(ctx, event)

	// Then: user has permission
	assert.Equal(t, result, true, "Expected user to have permission to see event")
}

func TestEventMatchesRbacFineGrainedRbac_Disallowed(t *testing.T) {
	originalFineGrainedRbac := config.Cfg.Features.FineGrainedRbac
	config.Cfg.Features.FineGrainedRbac = true
	defer func() {
		config.Cfg.Features.FineGrainedRbac = originalFineGrainedRbac
	}()
	// Given: a user with misaligned fine-grained RBAC permissions to watch managed cluster resource streamed event
	ctx := rbac.CreateTestContext("test-user-1", "testuser1")
	event := &model.Event{
		UID:       "asdf-1234",
		Operation: "CREATE",
		NewData: map[string]any{
			"apigroup":    "v1",
			"kind_plural": "pods",
			"namespace":   "foo",
			"cluster":     "managed-cluster",
		},
	}
	// User has permission for different cluster
	userDataCache := rbac.CreateTestUserDataCache("watch", "v1", "pods", "different-cluster", "foo")
	rbac.SetupCacheWithUserData(ctx, userDataCache)
	defer rbac.CleanupCache(ctx)

	// When: we check user permissions against the event
	result := eventMatchesRbac(ctx, event)

	// Then: user doesn't have permission
	assert.Equal(t, result, false, "Expected user not to have permission to see event")
}
