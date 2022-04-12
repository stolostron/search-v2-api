// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
)

func Test_Messages_Query(t *testing.T) {
	// Create a SearchSchemaResolver instance with a mock connection pool.
	resolver, _ := newMockSearchSchema(t)

	sql := `SELECT COUNT(DISTINCT("cluster")) FROM "search"."resources" WHERE (("cluster" NOT IN ('', 'local-cluster')) AND ("cluster" NOT IN ((SELECT DISTINCT "data"->>'namespace' AS "cluster" FROM "search"."resources" WHERE (("data"->>'kind' = 'ManagedClusterAddOn') AND ("data"->>'name' = 'search-collector'))))))`
	// Execute function
	resolver.messageQuery(context.TODO())

	// Verify response
	if resolver.query != sql {
		t.Errorf("Expected sql query: %s but got %s", sql, resolver.query)
	}
}

func Test_Message_Results(t *testing.T) {
	// Create a SearchSchemaResolver instance with a mock connection pool.
	resolver, mockPool := newMockSearchSchema(t)

	// Mock the database queries.
	mockRow := &Row{MockValue: 1}

	// Mock the database query
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(`SELECT COUNT(DISTINCT("cluster")) FROM "search"."resources" WHERE (("cluster" NOT IN ('', 'local-cluster')) AND ("cluster" NOT IN ((SELECT DISTINCT "data"->>'namespace' AS "cluster" FROM "search"."resources" WHERE (("data"->>'kind' = 'ManagedClusterAddOn') AND ("data"->>'name' = 'search-collector'))))))`),
	).Return(mockRow)
	resolver.messageQuery(context.TODO())
	//Execute the function
	res, err := resolver.messageResults()

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
