// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"sync"
	"time"

	config "github.com/stolostron/search-v2-api/pkg/config"
	"k8s.io/klog/v2"
)

// Returns a client to process the federated request.
func GetHttpClient(remoteService RemoteSearchService) HTTPClient {
	// Get http client from pool.
	client := httpClientPool.Get().(*RealHTTPClient)

	return client
}

// shared HTTP transport and client for efficient connection reuse as per
// godoc: https://cs.opensource.google/go/go/+/go1.21.5:src/net/http/transport.go;l=95 and
// https://stuartleeks.com/posts/connection-re-use-in-golang-with-http-client/
var tr = &http.Transport{
	MaxIdleConns:          config.Cfg.Federation.HttpPool.MaxIdleConns,
	IdleConnTimeout:       time.Duration(config.Cfg.Federation.HttpPool.MaxIdleConnTimeout) * time.Millisecond,
	ResponseHeaderTimeout: time.Duration(config.Cfg.Federation.HttpPool.ResponseHeaderTimeout) * time.Millisecond,
	DisableKeepAlives:     false,
	TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    x509.NewCertPool(),
	},
	MaxConnsPerHost:     config.Cfg.Federation.HttpPool.MaxConnsPerHost,
	MaxIdleConnsPerHost: config.Cfg.Federation.HttpPool.MaxIdleConnPerHost,
}

var httpClientPool = sync.Pool{
	New: func() interface{} {
		klog.V(6).Infof("Creating new RealHTTPClient from pool.")
		return &RealHTTPClient{
			&http.Client{
				Transport: tr,
				Timeout:   time.Duration(config.Cfg.Federation.HttpPool.RequestTimeout) * time.Millisecond,
			},
		}
	},
}

// HTTPClient is an interface for an HTTP client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// RealHTTPClient is a real implementation of the HTTPClient interface.
type RealHTTPClient struct {
	*http.Client
}

// Do implements the HTTPClient interface for RealHTTPClient.
func (c RealHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}
