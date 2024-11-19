// Copyright Contributors to the Open Cluster Management project
package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	pgxpool "github.com/jackc/pgx/v4/pgxpool"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/metrics"
	"k8s.io/klog/v2"
)

var pool *pgxpool.Pool
var timeLastPing time.Time

// Checks new connection is healthy before using it.
func afterConnect(ctx context.Context, c *pgx.Conn) error {
	if err := c.Ping(ctx); err != nil {
		klog.V(7).Info("New DB connection from pool was unhealthy. ", err)
		return err
	}
	return nil
}

// Checks idle connection is healthy before using it.
func beforeAcquire(ctx context.Context, c *pgx.Conn) bool {
	if err := c.Ping(ctx); err != nil {
		klog.V(7).Info("Idle DB connection from pool is unhealthy, destroying it. ", err)
		return false
	}
	return true
}

// Release resources used by the connection before returning to the pool.
func afterRelease(c *pgx.Conn) bool {
	klog.V(7).Info("Releasing DB connection back to pool.")
	_, err := c.Exec(context.TODO(), "DISCARD ALL")
	if err != nil {
		klog.Error("Error discarding connection state.", err)
		return false // Discard failed, don't return to pool.
	}
	return true
}

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

	config.AfterConnect = afterConnect   // Checks new connection health before using it.
	config.BeforeAcquire = beforeAcquire // Checks idle connection health before using it.
	config.AfterRelease = afterRelease
	// Add jitter to prevent all connections from being closed at same time.
	config.MaxConnLifetimeJitter = time.Duration(cfg.DBMaxConnLifeJitter) * time.Millisecond
	config.MaxConns = int32(cfg.DBMaxConns)
	config.MaxConnIdleTime = time.Duration(cfg.DBMaxConnIdleTime) * time.Millisecond
	config.MaxConnLifetime = time.Duration(cfg.DBMaxConnLifeTime) * time.Millisecond
	config.MinConns = int32(cfg.DBMinConns)

	klog.Infof("Using pgxpool.Config %+v", config)

	conn, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		klog.Errorf("Unable to connect to database: %+v\n", err)
		metrics.DBConnectionFailed.Inc()
	} else {
		klog.Info("Successfully connected to database!")
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
			klog.Error("Unable to get a healthy database connection. ", err)
			metrics.DBConnectionFailed.Inc()
			return nil
		}
		timeLastPing = time.Now()
		klog.V(5).Info("Database pool connection is healthy.")
	}
	return pool
}
