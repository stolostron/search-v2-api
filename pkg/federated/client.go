package federated

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// shared HTTP transport and client for efficient connection reuse as per
// godoc: https://cs.opensource.google/go/go/+/go1.21.5:src/net/http/transport.go;l=95 and
// https://stuartleeks.com/posts/connection-re-use-in-golang-with-http-client/
var tr = &http.Transport{
	MaxIdleConns:          10,               // TODO: make it configurable
	IdleConnTimeout:       15 * time.Second, // TODO: make it configurable, use ms for consistency.
	ResponseHeaderTimeout: 15 * time.Second, // TODO: make it configurable, use ms for consistency.
	DisableKeepAlives:     false,            // TODO: make it configurable
	TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13, // TODO: Verify if 1.3 is ok now. It caused issues in the past.
	},
	// MaxIdleConnsPerHost: 1, // TODO: make it configurable
	// MaxConnsPerHost:     2, // TODO: make it configurable
}

var httpClientPool = sync.Pool{
	New: func() interface{} {
		return &http.Client{
			Transport: tr,
			Timeout:   time.Second * 60, //make it configurable
		}
	},
}

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
	client := &RealHTTPClient{
		client: httpClientPool.Get().(*http.Client),
	}
	return client
}

func (p *RealHTTPClientPool) Put(client HTTPClient) {
	realClient, ok := client.(*RealHTTPClient)
	if !ok {
		klog.Error("Trying to put an invalid client type into the pool")
		return
	}
	httpClientPool.Put(realClient.client)
}

// RealHTTPClient is a real implementation of the HTTPClient interface.
type RealHTTPClient struct {
	client *http.Client
}

// Do implements the HTTPClient interface for RealHTTPClient.
func (c *RealHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

// SetTLSClientConfig sets the TLS client configuration for the HTTP client.
func (c *RealHTTPClient) SetTLSClientConfig(config *tls.Config) {
	c.client.Transport.(*http.Transport).TLSClientConfig = config
}
