// Copyright Contributors to the Open Cluster Management project
package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v4"
	pgxpool "github.com/jackc/pgx/v4/pgxpool"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/metric"
	klog "k8s.io/klog/v2"
)

var pool *pgxpool.Pool
var timeLastPing time.Time

func initializePool(ctx context.Context) {
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
	klog.Infof("Initializing connection to PostgreSQL using: %s", redactedDbConn)

	config, configErr := pgxpool.ParseConfig(dbConnString)
	if configErr != nil {
		klog.Error("Error parsing database connection configuration.", configErr)
	}

	config.ConnConfig.BuildStatementCache = nil

	// config.MaxConnIdleTime = 1 * time.Minute
	// config.MinConns = 2
	config.MaxConns = int32(cfg.DBMaxConns)

	config.AfterRelease = func(conn *pgx.Conn) bool {
		klog.Infof(">>> Releasing connection.  %+v", conn)
		_, err := conn.Exec(ctx, "DISCARD ALL")
		if err != nil {
			klog.Infof("Error during DISCARD ALL.  %s", err)
			return false
		}
		return true
	}

	conn, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		klog.Errorf("Unable to connect to database: %+v\n", err)
		metric.DBConnectionFailed.WithLabelValues("DBConnect").Inc()
	}

	pool = conn
}

func GetConnPool(ctx context.Context) *pgxpool.Pool {
	if pool == nil {
		initializePool(ctx)
	}

	if pool != nil {
		// Skip database ping if checked less than 1 second ago.
		if time.Since(timeLastPing) < time.Second {
			return pool
		}
		err := pool.Ping(ctx)
		if err != nil {
			klog.Error("Unable to get a database connection. ", err)
			metric.DBConnectionFailed.WithLabelValues("DBPing").Inc()
			return nil
		}
		timeLastPing = time.Now()
		klog.Info("Successfully connected to database!")
	}
	return pool
}
