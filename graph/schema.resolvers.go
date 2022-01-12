package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/open-cluster-management/search-v2-api/graph/generated"
	"github.com/open-cluster-management/search-v2-api/graph/model"
	"github.com/open-cluster-management/search-v2-api/pkg/schema"
	klog "k8s.io/klog/v2"
)

func (r *mutationResolver) DeleteSearch(ctx context.Context, resource *string) (*string, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) SaveSearch(ctx context.Context, resource *string) (*string, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) Search(ctx context.Context, input []*model.SearchInput) ([]*model.SearchResult, error) {
	// var count int
	klog.Infof("--------- Received Search query with %d inputs ---------\n", len(input))
	return schema.Search(ctx, input)
}

func (r *queryResolver) Messages(ctx context.Context) ([]*model.Message, error) {
	klog.Infoln("Received Messages query")

	messages := make([]*model.Message, 0)
	kind := "Informational"
	desc := "Trial search-v2-api"
	message1 := model.Message{ID: "1", Kind: &kind, Description: &desc}
	messages = append(messages, &message1)
	return messages, nil
}

func (r *queryResolver) SearchSchema(ctx context.Context) (map[string]interface{}, error) {
	klog.Infoln("Received SearchSchema query")
	return schema.SearchSchema(ctx)
}

func (r *queryResolver) SavedSearches(ctx context.Context) ([]*model.UserSearch, error) {
	klog.Infoln("Received SavedSearches query")
	savedSrches := []*model.UserSearch{}
	// savedSrches := make([]*model.UserSearch, 0)
	// id := "1"
	// name := "savedSrch1"
	// srchText := "Trial savedSrch1"
	// desc := "Trial search-v2-api savedSrch1"
	// savedSrch1 := model.UserSearch{ID: &id, Name: &name, Description: &desc, SearchText: &srchText}
	// savedSrches = append(savedSrches, &savedSrch1)
	// return savedSrches, nil
	return savedSrches, nil
}

func (r *queryResolver) SearchComplete(ctx context.Context, property string, query *model.SearchInput, limit *int) ([]*string, error) {
	klog.Infof("Received SearchComplete query with input property **%s** and limit %d", property, limit)
	return schema.SearchComplete(ctx, property, query, limit)
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
