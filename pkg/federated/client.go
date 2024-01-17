// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	config "github.com/stolostron/search-v2-api/pkg/config"
)

// shared HTTP transport and client for efficient connection reuse as per
// godoc: https://cs.opensource.google/go/go/+/go1.21.5:src/net/http/transport.go;l=95 and
// https://stuartleeks.com/posts/connection-re-use-in-golang-with-http-client/
var tr = &http.Transport{
	MaxIdleConns:          config.Cfg.FedClientPool.MaxIdleConns,
	IdleConnTimeout:       time.Duration(config.Cfg.FedClientPool.MaxIdleConnTimeout) * time.Millisecond,
	ResponseHeaderTimeout: time.Duration(config.Cfg.FedClientPool.ResponseHeaderTimeout) * time.Millisecond,
	DisableKeepAlives:     false,
	TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13, // TODO: Verify if 1.3 is ok now. It caused issues in the past.
	},
	MaxConnsPerHost:     config.Cfg.FedClientPool.MaxConnsPerHost,
	MaxIdleConnsPerHost: config.Cfg.FedClientPool.MaxIdleConnPerHost,
}

var httpClientPool = sync.Pool{
	New: func() interface{} {
		return &http.Client{
			Transport: tr,
			Timeout:   time.Duration(config.Cfg.FedClientPool.RequestTimeout) * time.Millisecond,
		}
	},
}

/**
// HTTPClientPool represents an interface for an HTTP client pool.
type HTTPClientPool interface {
	Get() HTTPClient
	Put(HTTPClient)
}

// HTTPClient is an interface for an HTTP client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
	SetTLSClientConfig(*tls.Config)
}

// RealHTTPClientPool is a real implementation of the HTTPClientPool interface.
type RealHTTPClientPool struct {
}

func (p *RealHTTPClientPool) Get() HTTPClient {
	client := httpClientPool.Get().(HTTPClient)
	return client
}

func (p *RealHTTPClientPool) Put(client HTTPClient) {
	httpClientPool.Put(client)
}

// RealHTTPClient is a real implementation of the HTTPClient interface.
type RealHTTPClient struct {
	*http.Client
}

// SetTLSClientConfig sets the TLS client configuration for the HTTP client.
func (c *RealHTTPClient) SetTLSClientConfig(config *tls.Config) {
	c.Transport.(*http.Transport).TLSClientConfig = config
}
*/
