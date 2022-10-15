// Copyright Contributors to the Open Cluster Management project
package database

import (
	"context"
	"fmt"
	"strings"

	pgxpool "github.com/jackc/pgx/v4/pgxpool"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/metric"
	klog "k8s.io/klog/v2"
)

var pool *pgxpool.Pool

func initializePool() {
	klog.Info("Initializing database connection pool.")
	cfg := config.Cfg

	dbConnString := fmt.Sprint(
		"host=", cfg.DBHost,
		" port=", cfg.DBPort,
		" user=", cfg.DBUser,
		" password=", cfg.DBPass,
		" dbname=", cfg.DBName,
		" sslmode=require", // https://www.postgresql.org/docs/current/libpq-connect.html
	)

	// Remove password from connection log.
	redactedDbConn := strings.ReplaceAll(dbConnString, "password="+cfg.DBPass, "password=[REDACTED]")
	klog.Infof("Connecting to PostgreSQL using: %s", redactedDbConn)

	config, configErr := pgxpool.ParseConfig(dbConnString)
	if configErr != nil {
		klog.Error("Error parsing database connection configuration.", configErr)
	}

	conn, err := pgxpool.ConnectConfig(context.TODO(), config)
	if err != nil {
		klog.Errorf("Unable to connect to database: %+v\n", err)
		metric.DBConnectionFailed.WithLabelValues("DBConnect").Inc()
	}

	pool = conn
}

func GetConnection() *pgxpool.Pool {
	if pool == nil {
		initializePool()
		metric.DBConnectionSuccess.WithLabelValues("DBConnect").Inc()
	}

	if pool != nil {
		err := pool.Ping(context.TODO())
		if err != nil {
			klog.Error("Unable to get a database connection. ", err)
			metric.DBConnectionFailed.WithLabelValues("DBPing").Inc()
			// Here we may need to add retry.
			return nil
		}
		metric.DBConnectionSuccess.WithLabelValues("DBPing").Inc()
		klog.Info("Successfully connected to database!")
		return pool
	}
	return nil
}
