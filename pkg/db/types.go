package db

import (
	"time"

	"gorm.io/gorm"
)

// Config holds MySQL/GORM database configuration
type Config struct {
	// Connection Settings
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Database string `json:"database" yaml:"database"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`

	// Connection Pool Settings
	MaxOpenConns    int           `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time" yaml:"conn_max_idle_time"`

	// MySQL Specific Settings
	Charset   string `json:"charset" yaml:"charset"`     // Default: utf8mb4
	Collation string `json:"collation" yaml:"collation"` // Default: utf8mb4_unicode_ci
	TimeZone  string `json:"timezone" yaml:"timezone"`   // Default: UTC

	// GORM Settings
	DisableForeignKeyConstraintWhenMigrating bool          `json:"disable_foreign_key_constraint_when_migrating" yaml:"disable_foreign_key_constraint_when_migrating"`
	SkipDefaultTransaction                   bool          `json:"skip_default_transaction" yaml:"skip_default_transaction"`
	PrepareStmt                              bool          `json:"prepare_stmt" yaml:"prepare_stmt"`
	QueryTimeout                             time.Duration `json:"query_timeout" yaml:"query_timeout"`

	// SSL Configuration
	SSL SSLConfig `json:"ssl" yaml:"ssl"`

	// Logging Configuration
	Logging LoggingConfig `json:"logging" yaml:"logging"`
}

// SSLConfig holds SSL/TLS configuration for MySQL
type SSLConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	CertFile   string `json:"cert_file" yaml:"cert_file"`
	KeyFile    string `json:"key_file" yaml:"key_file"`
	CAFile     string `json:"ca_file" yaml:"ca_file"`
	SkipVerify bool   `json:"skip_verify" yaml:"skip_verify"` // Skip certificate verification (not recommended for production)
	ServerName string `json:"server_name" yaml:"server_name"`
	MinVersion string `json:"min_version" yaml:"min_version"` // TLS1.2, TLS1.3
}

// LoggingConfig controls database logging behavior
type LoggingConfig struct {
	// General Logging
	Level  string `json:"level" yaml:"level"`   // debug, info, warn, error
	Format string `json:"format" yaml:"format"` // json, text

	// Database Logging
	LogQueries         bool          `json:"log_queries" yaml:"log_queries"`
	LogSlowQueries     bool          `json:"log_slow_queries" yaml:"log_slow_queries"`
	SlowQueryThreshold time.Duration `json:"slow_query_threshold" yaml:"slow_query_threshold"`
	LogQueryParameters bool          `json:"log_query_parameters" yaml:"log_query_parameters"`

	// Performance Logging
	LogPerformanceMetrics bool          `json:"log_performance_metrics" yaml:"log_performance_metrics"`
	MetricsInterval       time.Duration `json:"metrics_interval" yaml:"metrics_interval"`
}

// Manager manages database connections
type Manager struct {
	config *Config
	db     *gorm.DB
}
