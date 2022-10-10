// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/stolostron/search-v2-api/graph/model"
)

func Test_Messages_DisabledCluster(t *testing.T) {
	// Build mock
	mockMessage := Message{
		cache: &MockCache{
			disabled: map[string]struct{}{"managed1": {}},
		},
	}
	//Execute the function
	res, err := mockMessage.messageResults(context.Background())

	// Validate
	messages := make([]*model.Message, 0)
	kind := "information"
	desc := "Search is disabled on some of your managed clusters."
	message := model.Message{ID: "S20",
		Kind:        &kind,
		Description: &desc}
	messages = append(messages, &message)

	if !reflect.DeepEqual(messages, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", messages, res)
	}
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)
	}
}

func Test_Messages_Error(t *testing.T) {
	// Build mock
	mockMessage := Message{
		cache: &MockCache{
			disabled: map[string]struct{}{"managed1": {}},
			err:      fmt.Errorf("err running query"),
		},
	}
	// Execute the function
	res, errRes := mockMessage.messageResults(context.Background())

	// Validate
	if !reflect.DeepEqual([]*model.Message{}, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", []*model.Message{}, res)
	}
	if errRes == nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", fmt.Errorf("err running query"), errRes)
	}
}

func Test_Messages_MultipleDisabledClusters(t *testing.T) {
	// Build mock
	mockMessage := Message{
		cache: &MockCache{
			disabled: map[string]struct{}{"managed1": {}, "managed2": {}},
			err:      nil,
		},
	}
	// Execute the function
	res, err := mockMessage.messageResults(context.Background())

	// Validate
	messages := make([]*model.Message, 0)
	kind := "information"
	desc := "Search is disabled on some of your managed clusters."
	message := model.Message{ID: "S20",
		Kind:        &kind,
		Description: &desc}
	messages = append(messages, &message)

	if !reflect.DeepEqual(messages, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", messages, res)
	}
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)
	}
}

func Test_Message_NoDisabledClusters(t *testing.T) {
	// Build mock
	mockMessage := Message{
		cache: &MockCache{
			disabled: map[string]struct{}{},
			err:      nil,
		},
	}
	// Execute the function
	res, err := mockMessage.messageResults(context.Background())

	// Validate
	if !reflect.DeepEqual([]*model.Message{}, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", []*model.Message{}, res)
	}
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)
	}
}
