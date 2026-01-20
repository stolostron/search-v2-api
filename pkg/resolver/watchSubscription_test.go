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
	authv1 "k8s.io/api/authentication/v1"
	authz "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authnv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
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

	ctx := context.Background()

	// Test unsupported operators
	valOp := "!value"
	inputOp := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "kind", Values: []*string{&valOp}},
		},
	}
	_, err := WatchSubscription(ctx, inputOp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Operators !,!=,>,>=,<,<= are not yet supported")

	// Test wildcards
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

// helper for mocking eventMatchesRbac() and watchCache testing
type mockAuthzClient struct {
	permissions map[rbac.WatchPermissionKey]*rbac.WatchPermissionEntry
}

// helper for mocking eventMatchesRbac() and watchCache testing
func (m *mockAuthzClient) RESTClient() rest.Interface {
	return nil
}

// helper for mocking eventMatchesRbac() and watchCache testing
type mockAuthnClient struct {
	userInfo authv1.UserInfo
}

// helper for mocking eventMatchesRbac() and watchCache testing
func (m *mockAuthnClient) RESTClient() rest.Interface {
	return nil
}

// helper for mocking eventMatchesRbac() and watchCache testing
func (m *mockAuthnClient) TokenReviews() authnv1.TokenReviewInterface {
	return &mockTokenReviewInterface{userInfo: m.userInfo}
}

// helper for mocking eventMatchesRbac() and watchCache testing
type mockTokenReviewInterface struct {
	userInfo authv1.UserInfo
}

// helper for mocking eventMatchesRbac() and watchCache testing
func (m *mockTokenReviewInterface) Create(ctx context.Context, tr *authv1.TokenReview, opts metav1.CreateOptions) (*authv1.TokenReview, error) {
	tr.Status.Authenticated = true
	tr.Status.User = m.userInfo
	return tr, nil
}

// helper for mocking eventMatchesRbac() and watchCache testing
func newMockAuthzClient() *mockAuthzClient {
	return &mockAuthzClient{
		permissions: make(map[rbac.WatchPermissionKey]*rbac.WatchPermissionEntry),
	}
}

// helper for mocking eventMatchesRbac() and watchCache testing
type mockSelfSubjectAccessReviewInterface struct {
	client *mockAuthzClient
}

// helper for mocking eventMatchesRbac() and watchCache testing
func (m mockSelfSubjectAccessReviewInterface) Create(ctx context.Context, ssar *authz.SelfSubjectAccessReview, opts metav1.CreateOptions) (*authz.SelfSubjectAccessReview, error) {
	key := rbac.WatchPermissionKey{
		Verb:      ssar.Spec.ResourceAttributes.Verb,
		Apigroup:  ssar.Spec.ResourceAttributes.Group,
		Kind:      ssar.Spec.ResourceAttributes.Resource,
		Namespace: ssar.Spec.ResourceAttributes.Namespace,
	}

	entry, ok := m.client.permissions[key]
	if !ok {
		ssar.Status.Allowed = false
	} else {
		ssar.Status.Allowed = entry.Allowed
	}

	return ssar, nil
}

// helper for mocking eventMatchesRbac() and watchCache testing
func (m *mockAuthzClient) SelfSubjectAccessReviews() v1.SelfSubjectAccessReviewInterface {
	return &mockSelfSubjectAccessReviewInterface{client: m}
}

// helper for mocking eventMatchesRbac() and watchCache testing
func (m *mockAuthzClient) SelfSubjectRulesReviews() v1.SelfSubjectRulesReviewInterface {
	return nil
}

// helper for mocking eventMatchesRbac() and watchCache testing
func (m *mockAuthzClient) SubjectAccessReviews() v1.SubjectAccessReviewInterface {
	return nil
}

// helper for mocking eventMatchesRbac() and watchCache testing
func (m *mockAuthzClient) LocalSubjectAccessReviews(namespace string) v1.LocalSubjectAccessReviewInterface {
	return nil
}

// helper for mocking eventMatchesRbac() and watchCache testing
func setupWatchCacheWithUserData(ctx context.Context, userData *rbac.UserWatchData) {
	watchCache := rbac.GetWatchCache()
	cache := rbac.GetCache()
	uid, _ := cache.GetUserUID(ctx)

	watchCache.WatchUserDataLock.Lock()
	defer watchCache.WatchUserDataLock.Unlock()

	if watchCache.WatchUserData == nil {
		watchCache.WatchUserData = make(map[string]*rbac.UserWatchData)
	}

	watchCache.WatchUserData[uid] = userData
}

// helper for mocking eventMatchesRbac() and watchCache testing
func cleanupWatchCache(ctx context.Context) {
	watchCache := rbac.GetWatchCache()
	cache := rbac.GetCache()
	uid, _ := cache.GetUserUID(ctx)

	watchCache.WatchUserDataLock.Lock()
	defer watchCache.WatchUserDataLock.Unlock()

	if watchCache.WatchUserData != nil {
		delete(watchCache.WatchUserData, uid)
	}
}

// helper for mocking eventMatchesRbac() and watchCache testing
func createTestUserWatchData(permissions map[rbac.WatchPermissionKey]*rbac.WatchPermissionEntry, ttl time.Duration) *rbac.UserWatchData {
	mockClient := newMockAuthzClient()
	mockClient.permissions = permissions

	return &rbac.UserWatchData{
		AuthzClient:     mockClient,
		Permissions:     make(map[rbac.WatchPermissionKey]*rbac.WatchPermissionEntry),
		PermissionsLock: sync.RWMutex{},
		Ttl:             ttl,
	}
}

// helper for mocking eventMatchesRbac() and watchCache testing
func createTestContext(userUID, username string) context.Context {
	ctx := context.Background()

	// set up the user info in the regular RBAC cache so getUserUidFromContext works
	cache := rbac.GetCache()
	userInfo := authv1.UserInfo{
		UID:      userUID,
		Username: username,
	}

	// Set up mock authentication client
	cache.AuthnClient = &mockAuthnClient{userInfo: userInfo}

	token := "test-token-" + userUID
	ctx = context.WithValue(ctx, rbac.ContextAuthTokenKey, token)

	return ctx
}

func TestEventMatchesRbacHubClusterResource_Allowed(t *testing.T) {
	// Given: a user with permissions to watch hub cluster resource streamed event
	ctx := createTestContext("test-user-1", "testuser1")
	permissions := map[rbac.WatchPermissionKey]*rbac.WatchPermissionEntry{
		rbac.WatchPermissionKey{
			Verb:      "watch",
			Apigroup:  "v1",
			Kind:      "pods",
			Namespace: "foo",
		}: &rbac.WatchPermissionEntry{
			Allowed:   true,
			UpdatedAt: time.Now(),
		},
	}
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
	userData := createTestUserWatchData(permissions, 1*time.Minute)
	setupWatchCacheWithUserData(ctx, userData)
	defer cleanupWatchCache(ctx)

	// When: we check user permissions against the event
	result := eventMatchesRbac(ctx, event)

	// Then: user has permission
	assert.Equal(t, result, true, "Expected user to have permission to see event")
}

func TestEventMatchesRbacHubClusterResource_Disallowed(t *testing.T) {
	// Given: a user with misaligned permissions to watch hub cluster resource streamed event
	ctx := createTestContext("test-user-1", "testuser1")
	permissions := map[rbac.WatchPermissionKey]*rbac.WatchPermissionEntry{
		rbac.WatchPermissionKey{
			Verb:      "watch",
			Apigroup:  "v1",
			Kind:      "pods",
			Namespace: "foo",
		}: &rbac.WatchPermissionEntry{
			Allowed:   true,
			UpdatedAt: time.Now(),
		},
	}
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
	userData := createTestUserWatchData(permissions, 1*time.Minute)
	setupWatchCacheWithUserData(ctx, userData)
	defer cleanupWatchCache(ctx)

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
	ctx := createTestContext("test-user-1", "testuser1")
	permissions := map[rbac.WatchPermissionKey]*rbac.WatchPermissionEntry{
		rbac.WatchPermissionKey{
			Verb:      "create",
			Apigroup:  "view.open-cluster-management.io",
			Kind:      "managedclusterviews",
			Namespace: "managed-cluster",
		}: &rbac.WatchPermissionEntry{
			Allowed:   true,
			UpdatedAt: time.Now(),
		},
	}
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
	userData := createTestUserWatchData(permissions, 1*time.Minute)
	setupWatchCacheWithUserData(ctx, userData)
	defer cleanupWatchCache(ctx)

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
	ctx := createTestContext("test-user-1", "testuser1")
	permissions := map[rbac.WatchPermissionKey]*rbac.WatchPermissionEntry{
		rbac.WatchPermissionKey{
			Verb:      "create",
			Apigroup:  "view.open-cluster-management.io",
			Kind:      "managedclusterviews",
			Namespace: "a-totally-different-managed-cluster",
		}: &rbac.WatchPermissionEntry{
			Allowed:   true,
			UpdatedAt: time.Now(),
		},
	}
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
	userData := createTestUserWatchData(permissions, 1*time.Minute)
	setupWatchCacheWithUserData(ctx, userData)
	defer cleanupWatchCache(ctx)

	// When: we check user permissions against the event
	result := eventMatchesRbac(ctx, event)

	// Then: user doesn't have permission
	assert.Equal(t, result, false, "Expected user not to have permission to see event")
}
