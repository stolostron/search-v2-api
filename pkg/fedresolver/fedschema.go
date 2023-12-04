package fedresolver

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// Define the httpClient interface
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// clientWrapper is a generic wrapper around http.Client or any httpClient implementation
type clientWrapper struct {
	client httpClient
}

func (c *clientWrapper) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

// Your SearchSchema struct with an interface for the HTTP client
type SearchSchema struct {
	routeToken []map[string]string
	client     httpClient
}

type schemaPayload struct {
	Data struct {
		SearchSchema map[string][]string `json:"searchSchema"`
	} `json:"data"`
}

func SearchSchemaResolver(ctx context.Context) (map[string]interface{}, error) {
	routeToken := getTokenAndRoute()

	// Create an http.Client
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 30 * time.Second,
	}

	// Wrap the http.Client with the clientWrapper
	wrappedClient := &clientWrapper{client: httpClient}

	// Create a SearchSchema instance with the wrapped client
	searchSchemaResult := &SearchSchema{
		routeToken: routeToken,
		client:     wrappedClient,
	}

	return searchSchemaResult.searchSchemaResults(ctx)
}

func (s *SearchSchema) searchSchemaResults(ctx context.Context) (map[string]interface{}, error) {
	klog.Info("Resolving searchSchemaResults()")
	srchSchema := map[string]interface{}{}
	var schemaErr error
	propArray := []interface{}{}
	if schemaErr == nil {
		klog.Info("schemaErr is nill here")
	}
	// These default properties are always present and we want them at the top.
	defaultschema := []string{"cluster", "kind", "label", "name", "namespace", "status"}
	// Use a map to remove duplicates efficiently.
	defaultschemaMap := map[string]struct{}{}
	newschemaMap := map[string]struct{}{}
	for _, key := range defaultschema {
		defaultschemaMap[key] = struct{}{}
		propArray = append(propArray, key)
	}
	// Return the default properties if there is error in processing the rest
	srchSchema["allProperties"] = propArray

	if len(s.routeToken) == 0 {
		klog.Warning("No routes and tokens provided.")
		return srchSchema, fmt.Errorf("error in search schema call: No leaf hub routes and tokens provided")
	}

	var wg sync.WaitGroup
	for _, routeAndToken := range s.routeToken {
		wg.Add(1)
		routeAndToken := routeAndToken
		// Wrap the worker call in a closure that makes sure to tell
		// the WaitGroup that this worker is done.
		go func() {
			defer wg.Done()
			// Q: What to do if one call errors out?
			props, err := s.callSchemaRoute(routeAndToken)
			if err != nil && err.Error() != "" {
				schemaErr = err
				klog.Warning("Error calling route: ", err.Error())
			} else {
				// klog.Info("props:", props)
				for _, prop := range props {
					// Check if the property is already captured or if it is an internal-only property
					if _, present := defaultschemaMap[prop]; !present && prop[0:1] != "_" {
						newschemaMap[prop] = struct{}{}
						klog.Info("newschemaMap:", newschemaMap)
					}
				}
			}
		}()
	}

	// Block until the WaitGroup counter goes back to 0;
	// all the workers notified they're done.
	wg.Wait()
	for key := range newschemaMap {
		propArray = append(propArray, key)
	}

	srchSchema["allProperties"] = propArray
	klog.Info("schemaErr: ", schemaErr)
	if schemaErr != nil && schemaErr.Error() != "" {
		return srchSchema, fmt.Errorf("error fetching searchschema: %s", schemaErr)
	} else {
		return srchSchema, nil
	}
}

func (s *SearchSchema) callSchemaRoute(routeAndToken map[string]string) ([]string, error) {
	klog.Info("len(routeAndToken):", len(routeAndToken))

	var httpErr error
	result := schemaPayload{}

	for route, token := range routeAndToken {
		result = schemaPayload{}
		klog.Info("route: ", route, " token: ", token)
		payloadBuffer := strings.NewReader("{\"query\":\"{\\n  searchSchema \\n}\"}")

		req := createRequest(route, token, payloadBuffer)

		res, err := s.client.Do(req)
		if err != nil {
			klog.Warningf("error fetching searchschema, error making http request: %s\n", err)
			httpErr = err
			continue
		}
		klog.Infof("client: got response:  %+v", res.Body)
		if res != nil {
			klog.Infof("client: status code: %d\n", res.StatusCode)
			resultBytes, err := io.ReadAll(res.Body)
			if err != nil {
				klog.Info("Error reading resultBytes", err)
			}

			// Q: Should we return the error?
			err = json.Unmarshal(resultBytes, &result)
			if err != nil {
				klog.Error("error fetching searchschema, while unmarshaling search result: ", err)
				httpErr = err
			}
		}
	}
	returnRes := result.Data.SearchSchema["allProperties"]
	klog.Info("returnRes: ", returnRes)
	if httpErr != nil {
		return returnRes, fmt.Errorf("error fetching searchschema: %s", httpErr)
	} else {
		return returnRes, nil
	}
}
