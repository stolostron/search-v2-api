package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/stolostron/search-v2-api/graph/generated"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/resolver"
	klog "k8s.io/klog/v2"
)

func (r *mutationResolver) DeleteSearch(ctx context.Context, resource *string) (*string, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) SaveSearch(ctx context.Context, resource *string) (*string, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) Search(ctx context.Context, input []*model.SearchInput) ([]*resolver.SearchResult, error) {
	klog.V(3).Infof("--------- Received Search query with %d inputs ---------\n", len(input))
	return resolver.Search(ctx, input)
}

func (r *queryResolver) Messages(ctx context.Context) ([]*model.Message, error) {
	klog.V(3).Infoln("Received Messages query")
	return resolver.Messages(ctx)
}

func (r *queryResolver) SearchSchema(ctx context.Context) (map[string]interface{}, error) {
	klog.V(3).Infoln("Received SearchSchema query")
	return resolver.SearchSchemaResolver(ctx)
}

func (r *queryResolver) SavedSearches(ctx context.Context) ([]*model.UserSearch, error) {
	klog.V(3).Infoln("Received SavedSearches query")

	savedSrches := make([]*model.UserSearch, 0)
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
	klog.V(3).Infof("Received SearchComplete query with input property **%s** and limit %d", property, limit)
	return resolver.SearchComplete(ctx, property, query, limit)
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
