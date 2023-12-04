package fedresolver

import (
	"net/http"
	"os"
	"strings"

	"k8s.io/klog/v2"
)

func getTokenAndRoute() []map[string]string {
	routeToken := []map[string]string{}

	route1, route1Present := os.LookupEnv("ROUTE1")
	token1, token1Present := os.LookupEnv("TOKEN1")

	if route1Present && token1Present {
		routeToken = append(routeToken, map[string]string{route1: token1})
	}
	route2, route2Present := os.LookupEnv("ROUTE2")
	token2, token2Present := os.LookupEnv("TOKEN2")
	if route2Present && token2Present {
		routeToken = append(routeToken, map[string]string{route2: token2})
	}

	if len(routeToken) == 0 {
		klog.Info("No route and tokens found")
	}

	return routeToken
}

func createRequest(route string, token string, payloadBuffer *strings.Reader) *http.Request {
	req, err := http.NewRequest(http.MethodPost, route, payloadBuffer)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		klog.Infof("error making http request: %s\n", err)
	}
	return req
}
