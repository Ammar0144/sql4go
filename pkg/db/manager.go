package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	// instance holds the singleton database manager
	// Protected by once for thread-safe initialization
	instance *Manager
	once     sync.Once
)

// Singleton Lifecycle Documentation:
//
// The singleton pattern ensures only one database connection pool exists per application.
// This is appropriate for most applications to avoid connection pool exhaustion.
//
// Thread-Safety Guarantees:
//   - NewSingletonManager is safe for concurrent calls
//   - Once initialized, the instance is immutable and safe for concurrent access
//   - First call determines the configuration; subsequent calls return the same instance
//
// Lifecycle Pattern:
//   1. Call NewSingletonManager(config) once at application startup
//   2. Use the returned Manager throughout the application lifetime
//   3. Call Close() at application shutdown to release resources
//
// For testing environments with multiple configuration changes, avoid the singleton
// and use NewManager(config) directly instead.

// NewDefaultManager creates a database manager with minimal configuration
func NewDefaultManager(host, database, username, password string) (*Manager, error) {
	config := &Config{
		Host:            host,
		Database:        database,
		Username:        username,
		Password:        password,
		Port:            3306,
		Charset:         "utf8mb4",
		Collation:       "utf8mb4_unicode_ci",
		TimeZone:        "UTC",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
		PrepareStmt:     true,
		QueryTimeout:    30 * time.Second,
	}

	return NewManager(config)
}

// NewManager creates a new database manager instance with full configuration
func NewManager(config *Config) (*Manager, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	logLevel := getLogLevel(config.Logging.Level)
	gormConfig := &gorm.Config{
		SkipDefaultTransaction:                   config.SkipDefaultTransaction,
		DisableForeignKeyConstraintWhenMigrating: config.DisableForeignKeyConstraintWhenMigrating,
		PrepareStmt:                              config.PrepareStmt,
		Logger:                                   logger.Default.LogMode(logLevel),
	}

	db, err := gorm.Open(mysql.Open(config.GetDSN()), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	return &Manager{
		config: config,
		db:     db,
	}, nil
}

// NewSingletonManager returns the singleton database manager instance
//
// IMPORTANT: Singleton Initialization Behavior
//   - The first call to NewSingletonManager initializes the singleton instance
//   - If the first call fails, the singleton remains uninitialized permanently
//   - Subsequent calls return the error from the first failed attempt
//   - There is no automatic retry mechanism - this is by design for production stability
//   - Once successfully initialized, subsequent calls ignore the config parameter
//
// Error Recovery:
//   - For testing: Use NewManager(config) directly instead of the singleton
//   - For production: Ensure the first call uses valid configuration
//   - To reset in tests: Call ResetSingleton() (if implemented) or restart the application
//
// Thread-Safety:
//   - This function is safe for concurrent calls
//   - The initialization only happens once, protected by sync.Once
func NewSingletonManager(config *Config) (*Manager, error) {
	var initErr error
	once.Do(func() {
		instance, initErr = NewManager(config)
	})

	// Handle case where initialization failed
	if instance == nil {
		if initErr != nil {
			return nil, fmt.Errorf("singleton initialization failed (permanent until restart): %w", initErr)
		}
		return nil, fmt.Errorf("singleton initialization failed with unknown error (permanent until restart)")
	}

	return instance, nil
}

// DB returns the GORM database instance
func (m *Manager) DB() *gorm.DB {
	return m.db
}

// SqlDB returns the underlying sql.DB instance
func (m *Manager) SqlDB() (*sql.DB, error) {
	return m.db.DB()
}

// Close closes the database connection
func (m *Manager) Close() error {
	if m.db != nil {
		sqlDB, err := m.db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// Config returns the manager's configuration
func (m *Manager) Config() *Config {
	return m.config
}

// Ping tests the database connection
func (m *Manager) Ping(ctx context.Context) error {
	sqlDB, err := m.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Stats returns database connection statistics
func (m *Manager) Stats() (sql.DBStats, error) {
	sqlDB, err := m.db.DB()
	if err != nil {
		return sql.DBStats{}, err
	}
	return sqlDB.Stats(), nil
}

func getLogLevel(level string) logger.LogLevel {
	switch strings.ToLower(level) {
	case "info":
		return logger.Info
	case "warn":
		return logger.Warn
	case "error":
		return logger.Error
	case "silent":
		return logger.Silent
	default:
		return logger.Error // Default to error
	}
}
