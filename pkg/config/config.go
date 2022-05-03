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

const (
	API_SERVER_URL           = "https://kubernetes.default.svc"
	DEFAULT_CONTEXT_PATH     = "/searchapi"
	DEFAULT_QUERY_LIMIT      = 10000
	DEFAULT_QUERY_LOOP_LIMIT = 5000
	HTTP_PORT                = 4010
	RBAC_POLL_INTERVAL       = 60000  // 1 minute
	RBAC_INACTIVITY_TIMEOUT  = 600000 // 10 minutes
	SERVICEACCT_TOKEN        = ""
	DEFAULT_DB_PASS          = ""
	DEFAULT_DB_HOST          = "localhost"
	DEFAULT_DB_USER          = ""
	DEFAULT_DB_NAME          = ""
	DEFAULT_DB_PORT          = int(5432)
)

// Define a config type to hold our config properties.

var Cfg = new()

type Config struct {
	API_SERVER_URL          string // address for API_SERVER
	ContextPath             string
	defaultQueryLimit       int // number of queries handled at a time
	defaultQueryLoopLimit   int
	HttpPort                int
	RBAC_POLL_INTERVAL      int
	RBAC_INACTIVITY_TIMEOUT int
	SERVICEACCT_TOKEN       string
	DB_HOST                 string
	DB_USER                 string
	DB_NAME                 string
	DB_PASS                 string
	DB_PORT                 int
	PlaygroundMode          bool
}

func new() *Config {
	// If environment variables are set, use those values constants
	// Simply put, the order of preference is env -> default constants (from left to right)
	conf := &Config{
		API_SERVER_URL:          getEnv("API_SERVER_URL", API_SERVER_URL),
		ContextPath:             getEnv("CONTEXT_PATH", DEFAULT_CONTEXT_PATH),
		defaultQueryLimit:       getEnvAsInt("QUERY_LIMIT", DEFAULT_QUERY_LIMIT),
		defaultQueryLoopLimit:   getEnvAsInt("QUERY_LOOP_LIMIT", DEFAULT_QUERY_LOOP_LIMIT),
		HttpPort:                getEnvAsInt("HTTP_PORT", HTTP_PORT),
		RBAC_POLL_INTERVAL:      getEnvAsInt("RBAC_POLL_INTERVAL", RBAC_POLL_INTERVAL),
		RBAC_INACTIVITY_TIMEOUT: getEnvAsInt("RBAC_INACTIVITY_TIMEOUT", RBAC_INACTIVITY_TIMEOUT),
		SERVICEACCT_TOKEN:       getEnv("SERVICEACCT_TOKEN", SERVICEACCT_TOKEN),
		DB_PASS:                 getEnv("DB_PASS", DEFAULT_DB_PASS),
		DB_HOST:                 getEnv("DB_HOST", DEFAULT_DB_HOST),
		DB_NAME:                 getEnv("DB_NAME", DEFAULT_DB_NAME),
		DB_USER:                 getEnv("DB_USER", DEFAULT_DB_USER),
		DB_PORT:                 getEnvAsInt("DB_PORT", DEFAULT_DB_PORT),
		PlaygroundMode:          getEnvAsBool("PLAYGROUND_MODE", false),
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
