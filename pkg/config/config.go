/*
Copyright (c) 2021 Red Hat, Inc.
*/
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"

	klog "k8s.io/klog/v2"
)

const (
	API_SERVER_URL           = "https://kubernetes.default.svc"
	DEFAULT_CONTEXT_PATH     = "/searchapi"
	DEFAULT_QUERY_LIMIT      = 10000
	DEFAULT_QUERY_LOOP_LIMIT = 5000
	HTTP_PORT                = 4010
	RBAC_POLL_INTERVAL       = 60000
	RBAC_INACTIVITY_TIMEOUT  = 600000
	SERVICEACCT_TOKEN        = ""
	DEFAULT_DB_PASSWORD      = "P1{zoPopgjA4>Hw4^zP;C^=g"
	DEFAULT_DB_HOST          = "localhost"
	DEFAULT_DB_USER          = "hippo"
	DEFAULT_DB_NAME          = "hippo"
	DEFAULT_DB_PORT          = int(5432)
)

// Define a config type to hold our config properties.
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
	DB_PASSWORD             string
	DB_PORT                 int
}

var Cfg = Config{}

func New() Config {
	klog.Info("In Config New")
	// If environment variables are set, use those values constants
	// Simply put, the order of preference is env -> default constants (from left to right)
	setDefault(&Cfg.API_SERVER_URL, "API_SERVER_URL", API_SERVER_URL)
	setDefault(&Cfg.ContextPath, "CONTEXT_PATH", DEFAULT_CONTEXT_PATH)
	setDefaultInt(&Cfg.defaultQueryLimit, "QUERY_LIMIT", DEFAULT_QUERY_LIMIT)
	setDefaultInt(&Cfg.defaultQueryLoopLimit, "QUERY_LOOP_LIMIT", DEFAULT_QUERY_LOOP_LIMIT)
	setDefaultInt(&Cfg.HttpPort, "HTTP_PORT", HTTP_PORT)
	setDefaultInt(&Cfg.RBAC_POLL_INTERVAL, "RBAC_POLL_INTERVAL", RBAC_POLL_INTERVAL)
	setDefaultInt(&Cfg.RBAC_INACTIVITY_TIMEOUT, "RBAC_INACTIVITY_TIMEOUT", RBAC_INACTIVITY_TIMEOUT)
	setDefault(&Cfg.SERVICEACCT_TOKEN, "SERVICEACCT_TOKEN", SERVICEACCT_TOKEN)
	setDefault(&Cfg.DB_PASSWORD, "DB_PASSWORD", DEFAULT_DB_PASSWORD)
	Cfg.DB_PASSWORD = url.QueryEscape(Cfg.DB_PASSWORD)
	setDefault(&Cfg.DB_HOST, "DB_HOST", DEFAULT_DB_HOST)
	setDefault(&Cfg.DB_NAME, "DB_NAME", DEFAULT_DB_NAME)
	setDefault(&Cfg.DB_USER, "DB_USER", DEFAULT_DB_USER)
	setDefaultInt(&Cfg.DB_PORT, "DB_PORT", DEFAULT_DB_PORT)
	return Cfg

}

// Format and print environment to logger.
func (cfg *Config) PrintConfig() {
	// Make a copy to redact secrets and sensitive information.
	tmp := cfg
	// tmp.DB_PASSWORD = "[REDACTED]"

	// Convert to JSON for nicer formatting.
	cfgJSON, err := json.MarshalIndent(tmp, "", "\t")
	if err != nil {
		klog.Warning("Encountered a problem formatting configuration. ", err)
		klog.Infof("Configuration %#v\n", tmp)
	}
	klog.Infof("Using configuration:\n%s\n", string(cfgJSON))
}

func setDefault(field *string, env, defaultVal string) {
	if val := os.Getenv(env); val != "" {
		if env == "DB_PASSWORD" {
			klog.Infof("Using %s from environment", env)
		} else {
			klog.Infof("Using %s from environment: %s", env, val)
		}
		*field = val
	} else if *field == "" && defaultVal != "" {
		// Skip logging when running tests to reduce confusing output.
		if !strings.HasSuffix(os.Args[0], ".test") {
			klog.Infof("%s not set, using default value: %s", env, defaultVal)
		}
		*field = defaultVal
	}
}

func setDefaultInt(field *int, env string, defaultVal int) {
	if val := os.Getenv(env); val != "" {
		klog.Infof("Using %s from environment: %s", env, val)
		var err error
		*field, err = strconv.Atoi(val)
		if err != nil {
			klog.Error("Error parsing env [", env, "].  Expected an integer.  Original error: ", err)
		}
	} else if *field == 0 && defaultVal != 0 {
		// Skip logging when running tests to reduce confusing output.
		if !strings.HasSuffix(os.Args[0], ".test") {
			klog.Infof("No %s from file or environment, using default value: %d", env, defaultVal)
		}
		*field = defaultVal
	}
}
