// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"time"

	"github.com/stolostron/search-v2-api/graph/model"
)

func SearchSubscription(ctx context.Context, input []*model.SearchInput) (<-chan []*SearchResult, error) {
	ch := make(chan []*SearchResult)

	// You can (and probably should) handle your channels in a central place outside of `schema.resolvers.go`.
	// For this example we'll simply use a Goroutine with a simple loop.
	go func() {
		// Handle deregistration of the channel here. Note the `defer`
		defer close(ch)

		for {
			// Send the search results every 10 seconds.
			time.Sleep(10 * time.Second)
			fmt.Println("SearchSubscription new poll interval")
			
			// TODO 2nd return item is error... needs to be handled
			searchResult, _ := Search(ctx, input)

			// The subscription may have got closed due to the client disconnecting.
			// Hence we do send in a select block with a check for context cancellation.
			// This avoids goroutine getting blocked forever or panicking,
			select {
			case <-ctx.Done(): // This runs when context gets cancelled. Subscription closes.
				fmt.Println("Subscription Closed")
				// Handle deregistration of the channel here. `close(ch)`
				return // Remember to return to end the routine.
			
			case ch <- searchResult: // This is the actual send.
				// Our message went through, do nothing	
			}
		}
	}()

	// We return the channel and no error.
	return ch, nil
}