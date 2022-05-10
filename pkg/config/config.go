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

var Cfg = new()

// Define a Config type to hold our config properties.
type Config struct {
	API_SERVER_URL string // address for Kubernetes API Server
	ContextPath    string
	DB_HOST        string
	DB_NAME        string
	DB_PASS        string
	DB_PORT        int
	DB_USER        string
	HttpPort       int
	PlaygroundMode bool // Enable the GraphQL Playground client.
	QueryLimit     int  // The default LIMIT to use on queries. Client can override.
	RelationLevel  int  // The number of levels/hops for finding relationships for a particular resource
	// Placeholder for future use.
	// QueryLoopLimit          int // number of queries handled at a time
	// RBAC_INACTIVITY_TIMEOUT int
	// RBAC_POLL_INTERVAL      int
}

func new() *Config {
	// If environment variables are set, use default values
	// Simply put, the order of preference is env -> default values (from left to right)
	conf := &Config{
		API_SERVER_URL: getEnv("API_SERVER_URL", "https://kubernetes.default.svc"),
		ContextPath:    getEnv("CONTEXT_PATH", "/searchapi"),
		DB_HOST:        getEnv("DB_HOST", "localhost"),
		DB_NAME:        getEnv("DB_NAME", ""),
		DB_PASS:        getEnv("DB_PASS", ""),
		DB_PORT:        getEnvAsInt("DB_PORT", int(5432)),
		DB_USER:        getEnv("DB_USER", ""),
		HttpPort:       getEnvAsInt("HTTP_PORT", 4010),
		PlaygroundMode: getEnvAsBool("PLAYGROUND_MODE", false),
		QueryLimit:     getEnvAsInt("QUERY_LIMIT", 10000),
		RelationLevel:  getEnvAsInt("RELATION_LEVEL", 3),
		// Placeholder for future use.
		// QueryLoopLimit:          getEnvAsInt("QUERY_LOOP_LIMIT", 5000),
		// RBAC_INACTIVITY_TIMEOUT: getEnvAsInt("RBAC_INACTIVITY_TIMEOUT", 600000), // 10 minutes
		// RBAC_POLL_INTERVAL:      getEnvAsInt("RBAC_POLL_INTERVAL", 60000),       // 1 minute
	}
	conf.DB_PASS = url.QueryEscape(conf.DB_PASS)
	return conf
}

// Format and print environment to logger.
func (cfg *Config) PrintConfig() {
	// Make a copy to redact secrets and sensitive information.
	tmp := *cfg
	tmp.DB_PASS = "[REDACTED]"

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
	if cfg.DB_NAME == "" {
		return errors.New("required environment DB_NAME is not set")
	}
	if cfg.DB_USER == "" {
		return errors.New("required environment DB_USER is not set")
	}
	if cfg.DB_PASS == "" {
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

// Helper to read an environment variable into a bool or return default value
func getEnvAsBool(name string, defaultVal bool) bool {
	valStr := getEnv(name, "")
	if val, err := strconv.ParseBool(valStr); err == nil {
		return val
	}

	return defaultVal
}
