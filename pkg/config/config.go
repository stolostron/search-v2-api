// Copyright Contributors to the Open Cluster Management project

package config

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strconv"

	klog "k8s.io/klog/v2"
)

var DEVELOPMENT_MODE = false // Do not change this. See config_development.go to enable.
var Cfg = new()

// Defines the configurable options for this microservice.
type Config struct {
	HubName                     string //Display Name of the cluster where ACM is deployed
	API_SERVER_URL              string // address for Kubernetes API Server
	AuthCacheTTL                int    // Time-to-live (milliseconds) of Authentication (TokenReview) cache.
	SharedCacheTTL              int    // Time-to-live (milliseconds) of common resources (shared across users) cache.
	UserCacheTTL                int    // Time-to-live (milliseconds) of namespaced resources (specifc to users) cache.
	ContextPath                 string
	DBHost                      string
	DBMinConns                  int32 // Overrides pgxpool.Config{ MinConns } Default: 2
	DBMaxConns                  int32 // Overrides pgxpool.Config{ MaxConns } Default: 10
	DBMaxConnIdleTime           int   // Overrides pgxpool.Config{ MaxConnIdleTime } Default: 5 min
	DBMaxConnLifeTime           int   // Overrides pgxpool.Config{ MaxConnLifetime } Default: 5 min
	DBMaxConnLifeJitter         int   // Overrides pgxpool.Config{ MaxConnLifetimeJitter } Default: 1 min
	DBName                      string
	DBPass                      string
	DBPort                      int
	DBUser                      string
	DevelopmentMode             bool             // Indicates if running in local development mode.
	Features                    featureFlags     // Enable or disable features.
	Federation                  federationConfig // Federated search configuration.
	HttpPort                    int
	PlaygroundMode              bool   // Enable the GraphQL Playground client.
	PodNamespace                string // Kubernetes namespace where the pod is running.
	QueryLimit                  uint   // The default LIMIT to use on queries. Client can override. Default: 1000
	RelationLevel               int    // The number of levels/hops for finding relationships for a particular resource
	SlowLog                     int    // Logs queries slower than the specified duration in ms. Default: 300ms
	SubscriptionRefreshInterval int    // Duration in seconds between subscription polls.        Default: 10 seconds
	SubscriptionRefreshTimeout  int    // Minutes a subscription will stay open before timeout.  Default: 5 minutes
}

// Define feature flags.
type featureFlags struct {
	FederatedSearch     bool // Enables federated search
	FineGrainedRbac     bool // Enables fine-grained RBAC
	SubscriptionEnabled bool // Enables GraphQL Subscriptions
}

// Http Client Pool Transport settings for federated client pool.
type httpClientPool struct {
	MaxConnsPerHost       int
	MaxIdleConns          int
	MaxIdleConnPerHost    int
	MaxIdleConnTimeout    int
	ResponseHeaderTimeout int
	RequestTimeout        int // Timeout for outbound federated requests.
}

// Federated search configuration options.
type federationConfig struct {
	GlobalHubName  string         // Identifies the global hub cluster, similar to local-cluster
	ConfigCacheTTL int            // Time-to-live (milliseconds) of federation config cache.
	HttpPool       httpClientPool // Transport settings for federated client pool.
}

func new() *Config {
	// If environment variables are set, use default values
	// Simply put, the order of preference is env -> default values (from left to right)
	conf := &Config{
		HubName:             getEnv("HUB_NAME", ""),
		API_SERVER_URL:      getEnv("API_SERVER_URL", "https://kubernetes.default.svc"),
		AuthCacheTTL:        getEnvAsInt("AUTH_CACHE_TTL", 60000),    // 1 minute
		SharedCacheTTL:      getEnvAsInt("SHARED_CACHE_TTL", 300000), // 5 minutes
		UserCacheTTL:        getEnvAsInt("USER_CACHE_TTL", 300000),   // 5 minutes
		ContextPath:         getEnv("CONTEXT_PATH", "/searchapi"),
		DBHost:              getEnv("DB_HOST", "localhost"),
		DBMaxConns:          getEnvAsInt32("DB_MAX_CONNS", int32(10)),          // 10     Overrides pgxpool default (4)
		DBMaxConnIdleTime:   getEnvAsInt("DB_MAX_CONN_IDLE_TIME", 5*60*1000),   // 5 min, Overrides pgxpool default (30)
		DBMaxConnLifeJitter: getEnvAsInt("DB_MAX_CONN_LIFE_JITTER", 1*60*1000), // 1 min, Overrides pgxpool default
		DBMaxConnLifeTime:   getEnvAsInt("DB_MAX_CONN_LIFE_TIME", 5*60*1000),   // 5 min, Overrides pgxpool default (60)
		DBMinConns:          getEnvAsInt32("DB_MIN_CONNS", int32(2)),           // 2      Overrides pgxpool default (0)
		DBName:              getEnv("DB_NAME", ""),
		DBPass:              getEnv("DB_PASS", ""),
		DBPort:              getEnvAsInt("DB_PORT", 5432),
		DBUser:              getEnv("DB_USER", ""),
		DevelopmentMode:     DEVELOPMENT_MODE,
		Features: featureFlags{
			FederatedSearch:     getEnvAsBool("FEATURE_FEDERATED_SEARCH", false),  // In Dev mode default is true.
			FineGrainedRbac:     getEnvAsBool("FEATURE_FINE_GRAINED_RBAC", false), // In Dev mode default is true.
			SubscriptionEnabled: getEnvAsBool("FEATURE_SUBSCRIPTION", false),      // In Dev mode default is true.
		},
		Federation: federationConfig{
			GlobalHubName:  getEnv("GLOBAL_HUB_NAME", "global-hub"),
			ConfigCacheTTL: getEnvAsInt("FEDERATION_CONFIG_CACHE_TTL", 2*60*1000), // 2 mins
			HttpPool: httpClientPool{ // Default values for federated client pool.
				MaxConnsPerHost:       getEnvAsInt("MAX_CONNS_PER_HOST", 2),
				MaxIdleConns:          getEnvAsInt("MAX_IDLE_CONNS", 10),
				MaxIdleConnPerHost:    getEnvAsInt("MAX_IDLE_CONN_PER_HOST", 2),
				MaxIdleConnTimeout:    getEnvAsInt("MAX_IDLE_CONN_TIMEOUT", 15*1000),     // 15 seconds.
				ResponseHeaderTimeout: getEnvAsInt("RESPONSE_HEADER_TIMEOUT", 15*1000),   // 15 seconds.
				RequestTimeout:        getEnvAsInt("FEDERATED_REQUEST_TIMEOUT", 60*1000), // 60 seconds.
			},
		},
		HttpPort:       getEnvAsInt("HTTP_PORT", 4010),
		PlaygroundMode: getEnvAsBool("PLAYGROUND_MODE", false),
		PodNamespace:   getEnv("POD_NAMESPACE", "open-cluster-management"),
		QueryLimit:     getEnvAsUint("QUERY_LIMIT", uint(1000)),
		SlowLog:        getEnvAsInt("SLOW_LOG", 300),
		// Setting default level to 0 to check if user has explicitly set this variable
		// This will be updated to 1 for default searches and 3 for applications - unless set by the user
		RelationLevel:               getEnvAsInt("RELATION_LEVEL", 0),
		SubscriptionRefreshInterval: getEnvAsInt("SUBSCRIPTION_REFRESH_INTERVAL", 10*1000),  // 10 seconds
		SubscriptionRefreshTimeout:  getEnvAsInt("SUBSCRIPTION_REFRESH_TIMEOUT", 5*60*1000), // 5 minutes
	}
	conf.DBPass = url.QueryEscape(conf.DBPass)
	return conf
}

// Format and print environment to logger.
func (cfg *Config) PrintConfig() {
	// Make a copy to redact secrets and sensitive information.
	tmp := *cfg
	tmp.DBPass = "[REDACTED]"

	// Convert to JSON for nicer formatting.
	cfgJSON, err := json.MarshalIndent(tmp, "", "\t")
	if err != nil {
		klog.Warning("Encountered a problem formatting configuration. ", err)
		klog.Infof("Configuration %#v\n", tmp)
	}
	klog.Infof("Using configuration:\n%s\n", string(cfgJSON))
}

// Validate required configuration.
func (cfg *Config) Validate() error {
	if cfg.DBName == "" {
		return errors.New("required environment DB_NAME is not set")
	}
	if cfg.DBUser == "" {
		return errors.New("required environment DB_USER is not set")
	}
	if cfg.DBPass == "" {
		return errors.New("required environment DB_PASS is not set")
	}
	return nil
}

// Simple helper function to read an environment or return a default value
func getEnv(key string, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

// Simple helper function to read an environment variable into integer or return a default value
func getEnvAsInt(name string, defaultVal int) int {
	valueStr := getEnv(name, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultVal
}

// Helper function to read an environment variable into integer32 or return a default value
func getEnvAsInt32(name string, defaultVal int32) int32 {
	valueStr := getEnv(name, "")
	if value, err := strconv.ParseInt(valueStr, 10, 32); err == nil {
		return int32(value)
	}
	return defaultVal
}

// Helper function to read an environment variable into unsigned integer or return a default value
func getEnvAsUint(name string, defaultVal uint) uint {
	valueStr := getEnv(name, "")
	if value, err := strconv.ParseUint(valueStr, 10, 32); err == nil {
		return uint(value)
	}
	return defaultVal
}

// Helper to read an environment variable into a bool or return default value
func getEnvAsBool(name string, defaultVal bool) bool {
	valStr := getEnv(name, "")
	if val, err := strconv.ParseBool(valStr); err == nil {
		return val
	}

	return defaultVal
}
