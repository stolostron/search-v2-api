// Copyright Contributors to the Open Cluster Management project
package fedresolver

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/federatedsearch/graph/model"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/metrics"
	"k8s.io/klog/v2"
)

type SearchResult struct {
	context    context.Context
	input      *model.SearchInput
	level      int                 // The number of levels/hops for finding relationships for a particular resource
	pool       pgxpoolmock.PgxPool // Used to mock database pool in tests
	uids       []*string           // List of uids from search result to be used to get relatioinships.
	routeToken []map[string]string
	wg         sync.WaitGroup // Used to serialize search query and relatioinships query.
}

type resultPayload struct {
	Data struct {
		Search []struct {
			Count int                      `json:"count"`
			Items []map[string]interface{} `json:"items"`
		} `json:"search"`
	} `json:"data"`
}

const ErrorMsg string = "Error building Search query"

func getTokenAndRoute() []map[string]string {
	route1, route1Present := os.LookupEnv("ROUTE1")
	token1, token1Present := os.LookupEnv("TOKEN1")
	if route1Present && token1Present {
		return []map[string]string{{route1: token1}}
	} else {
		return []map[string]string{}
	}
}

func Search(ctx context.Context, input []*model.SearchInput) ([]*SearchResult, error) {
	defer metrics.SlowLog("SearchResolver", 0)()
	// For each input, create a SearchResult resolver.
	srchResult := make([]*SearchResult, len(input))
	routeToken := getTokenAndRoute()

	// Proceed if user's rbac data exists
	if len(input) > 0 {
		for index, in := range input {
			srchResult[index] = &SearchResult{
				input:      in,
				pool:       db.GetConnPool(ctx),
				routeToken: routeToken,
				context:    ctx,
			}
		}
	}
	return srchResult, nil
}

func createRequest(route string, token string, payloadBuffer *strings.Reader) (http.Client, *http.Request) {
	req, err := http.NewRequest(http.MethodPost, route, payloadBuffer)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		klog.Infof("error making http request: %s\n", err)
	}
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 30 * time.Second,
	}
	return client, req
}
func callRoute(method string, routeAndToken map[string]string, input model.SearchInput) interface{} {
	var returnResult interface{}
	var count int
	var items []map[string]interface{}

	if len(routeAndToken) == 0 {
		klog.Warning("No routes and tokens provided.")
		return returnResult
	}
	for route, token := range routeAndToken {
		payloadBuffer := strings.NewReader("{\"query\":\"{\\n  search(\\n    input: [{keywords: [], filters: [{property: \\\"kind\\\", values: \\\"Node\\\"}], limit: 1000}]\\n  ) {\\n    count\\n    items\\n  }\\n}\",\"variables\":{}}")
		client, req := createRequest(route, token, payloadBuffer)

		res, err := client.Do(req)
		if err != nil {
			fmt.Printf("client: error making http request: %s\n", err)
		}
		klog.Infof("client: got response:  %+v", res)
		klog.Infof("client: status code: %d\n", res.StatusCode)
		resultBytes, err := io.ReadAll(res.Body) //json.Marshal(res)
		// klog.Info("res.Body: ", string(resultBytes))

		if err != nil {
			klog.Info("Error reading resultBytes", err)
		}
		result := resultPayload{}
		err = json.Unmarshal(resultBytes, &result)
		if err != nil {
			klog.Error("Error unmarshaling search result")
		}

		switch method {
		case "count":
			count = result.Data.Search[0].Count
			klog.Info("count: ", count)
			returnResult = count
		case "items":
			items = result.Data.Search[0].Items
			klog.Info("count: ", count)
			returnResult = items
		default:
			klog.Infof("%s not implemented ", method)
			klog.Info("res.Body: ", string(resultBytes))
			continue
		}

	}

	return returnResult
}
func (s *SearchResult) Count() int {
	var count int

	klog.Info("Resolving SearchResult:Count()")
	var wg sync.WaitGroup
	for _, routeAndToken := range s.routeToken {
		wg.Add(1)
		routeAndToken := routeAndToken
		// Wrap the worker call in a closure that makes sure to tell
		// the WaitGroup that this worker is done.
		go func() {
			defer wg.Done()
			count += callRoute("count", routeAndToken, *s.input).(int)
		}()
	}

	// Block until the WaitGroup counter goes back to 0;
	// all the workers notified they're done.
	wg.Wait()
	return count
}

func (s *SearchResult) Items() []map[string]interface{} {
	s.wg.Add(1)
	defer s.wg.Done()
	items := []map[string]interface{}{}
	klog.Info("Resolving SearchResult:Items()")
	// s.buildSearchQuery(s.context, false, false)

	var wg sync.WaitGroup
	// var result int
	for _, routeAndToken := range s.routeToken {
		wg.Add(1)
		routeAndToken := routeAndToken
		// Wrap the worker call in a closure that makes sure to tell
		// the WaitGroup that this worker is done.
		go func() {
			defer wg.Done()
			tmpItems := callRoute("items", routeAndToken, *s.input)
			tmpItemsArray, _ := tmpItems.([]map[string]interface{})
			items = append(items, tmpItemsArray...)
		}()
	}

	// Block until the WaitGroup counter goes back to 0;
	// all the workers notified they're done.
	wg.Wait()

	return items
}

func (s *SearchResult) Related(ctx context.Context) []SearchRelatedResult {
	klog.Info("Resolving SearchResult:Related()")
	if s.context == nil {
		s.context = ctx
	}
	if s.uids == nil {
		s.Uids()
	}
	// Wait for search to complete before resolving relationships.
	s.wg.Wait()
	// Log if this function is slow.
	defer metrics.SlowLog(fmt.Sprintf("SearchResult::Related() - uids: %d levels: %d", len(s.uids), s.level),
		500*time.Millisecond)()

	return []SearchRelatedResult{}
}

func (s *SearchResult) Uids() {
	klog.Info("Resolving SearchResult:Uids()")
}
