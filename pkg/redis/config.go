package redis

import (
	"fmt"
	"time"
)

// Config holds Redis cache configuration
type Config struct {
	// Cache Strategy
	Enabled      bool          `json:"enabled" yaml:"enabled"`
	Strategy     CacheStrategy `json:"strategy" yaml:"strategy"` // read_through, write_through, write_behind
	DefaultTTL   time.Duration `json:"default_ttl" yaml:"default_ttl"`
	NullCacheTTL time.Duration `json:"null_cache_ttl" yaml:"null_cache_ttl"` // Cache null results

	// Redis Connection
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Password string `json:"password" yaml:"password"`
	Database int    `json:"database" yaml:"database"`

	// Connection Pool
	PoolSize           int           `json:"pool_size" yaml:"pool_size"`
	MinIdleConns       int           `json:"min_idle_conns" yaml:"min_idle_conns"`
	MaxConnAge         time.Duration `json:"max_conn_age" yaml:"max_conn_age"`
	PoolTimeout        time.Duration `json:"pool_timeout" yaml:"pool_timeout"`
	IdleTimeout        time.Duration `json:"idle_timeout" yaml:"idle_timeout"`
	IdleCheckFrequency time.Duration `json:"idle_check_frequency" yaml:"idle_check_frequency"`

	// Performance
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`
	DialTimeout  time.Duration `json:"dial_timeout" yaml:"dial_timeout"`

	// Clustering (for Redis Cluster)
	Cluster ClusterConfig `json:"cluster" yaml:"cluster"`

	// Cache Invalidation
	Invalidation InvalidationConfig `json:"invalidation" yaml:"invalidation"`

	// Cache Warming
	WarmUp WarmUpConfig `json:"warm_up" yaml:"warm_up"`

	// Cache Metrics
	EnableMetrics bool `json:"enable_metrics" yaml:"enable_metrics"`

	// Cache Logging
	Logging LoggingConfig `json:"logging" yaml:"logging"`

	// Large Value Handling
	LargeValue LargeValueConfig `json:"large_value" yaml:"large_value"`
}

// ClusterConfig for Redis Cluster setup
type ClusterConfig struct {
	Enabled   bool     `json:"enabled" yaml:"enabled"`
	Addresses []string `json:"addresses" yaml:"addresses"`
	Username  string   `json:"username" yaml:"username"`
	Password  string   `json:"password" yaml:"password"`
}

// InvalidationConfig controls relationship-aware cache invalidation
type InvalidationConfig struct {
	// Relationship Detection
	AutoDetectRelationships bool     `json:"auto_detect_relationships" yaml:"auto_detect_relationships"`
	MaxRelationshipDepth    int      `json:"max_relationship_depth" yaml:"max_relationship_depth"`
	IgnoreRelationships     []string `json:"ignore_relationships" yaml:"ignore_relationships"`

	// Invalidation Strategy
	Strategy           InvalidationStrategy `json:"strategy" yaml:"strategy"` // immediate, batch, async
	BatchSize          int                  `json:"batch_size" yaml:"batch_size"`
	BatchFlushInterval time.Duration        `json:"batch_flush_interval" yaml:"batch_flush_interval"`

	// Pattern-based Invalidation
	KeyPatterns map[string][]string `json:"key_patterns" yaml:"key_patterns"` // entity -> patterns to invalidate
}

// WarmUpConfig controls cache warming strategies
type WarmUpConfig struct {
	Enabled    bool     `json:"enabled" yaml:"enabled"`
	Strategies []string `json:"strategies" yaml:"strategies"` // startup, schedule, on_demand

	// Scheduled warming
	CronExpression string        `json:"cron_expression" yaml:"cron_expression"`
	WarmUpTimeout  time.Duration `json:"warm_up_timeout" yaml:"warm_up_timeout"`

	// Entities to warm
	Entities []string `json:"entities" yaml:"entities"`
}

// LoggingConfig controls Redis cache logging behavior
type LoggingConfig struct {
	LogCacheHits     bool `json:"log_cache_hits" yaml:"log_cache_hits"`
	LogCacheMisses   bool `json:"log_cache_misses" yaml:"log_cache_misses"`
	LogInvalidations bool `json:"log_invalidations" yaml:"log_invalidations"`
}

// LargeValueConfig controls handling of large cache values
type LargeValueConfig struct {
	MaxValueSize      int  `json:"max_value_size" yaml:"max_value_size"`         // Maximum size per key (bytes)
	ChunkSize         int  `json:"chunk_size" yaml:"chunk_size"`                 // Size per chunk (bytes)
	CompressThreshold int  `json:"compress_threshold" yaml:"compress_threshold"` // Auto-compress above this size
	EnableCompression bool `json:"enable_compression" yaml:"enable_compression"` // Enable/disable compression
	EnableChunking    bool `json:"enable_chunking" yaml:"enable_chunking"`       // Enable/disable chunking
}

// Cache strategy enums
type CacheStrategy string

const (
	CacheStrategyReadThrough  CacheStrategy = "read_through"
	CacheStrategyWriteThrough CacheStrategy = "write_through"
	CacheStrategyWriteBehind  CacheStrategy = "write_behind"
	CacheStrategyLazyLoading  CacheStrategy = "lazy_loading"
)

// Invalidation strategy enums
type InvalidationStrategy string

const (
	InvalidationImmediate InvalidationStrategy = "immediate"
	InvalidationBatch     InvalidationStrategy = "batch"
	InvalidationAsync     InvalidationStrategy = "async"
)

// DefaultConfig returns a Redis configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:            true,
		Strategy:           CacheStrategyReadThrough,
		DefaultTTL:         time.Hour,
		NullCacheTTL:       time.Minute * 5,
		Host:               "localhost",
		Port:               6379,
		Database:           0,
		PoolSize:           10,
		MinIdleConns:       3,
		MaxConnAge:         time.Hour,
		PoolTimeout:        time.Second * 4,
		IdleTimeout:        time.Minute * 5,
		IdleCheckFrequency: time.Minute,
		ReadTimeout:        time.Second * 3,
		WriteTimeout:       time.Second * 3,
		DialTimeout:        time.Second * 5,
		Cluster: ClusterConfig{
			Enabled: false,
		},
		Invalidation: InvalidationConfig{
			AutoDetectRelationships: true,
			MaxRelationshipDepth:    3,
			Strategy:                InvalidationImmediate,
			BatchSize:               100,
			BatchFlushInterval:      time.Millisecond * 100,
		},
		WarmUp: WarmUpConfig{
			Enabled:       false,
			WarmUpTimeout: time.Minute * 5,
		},
		EnableMetrics: true,
		Logging: LoggingConfig{
			LogCacheHits:     false,
			LogCacheMisses:   true,
			LogInvalidations: true,
		},
		LargeValue: LargeValueConfig{
			MaxValueSize:      1024 * 1024 * 10, // 10MB max per key
			ChunkSize:         1024 * 1024 * 2,  // 2MB per chunk
			CompressThreshold: 1024 * 100,       // Compress values larger than 100KB
			EnableCompression: true,
			EnableChunking:    true,
		},
	}
}

// Validate checks if the Redis configuration is valid
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // Skip validation if cache is disabled
	}

	if c.Host == "" {
		return fmt.Errorf("redis host is required when cache is enabled")
	}
	if c.Port <= 0 {
		return fmt.Errorf("redis port must be positive")
	}
	if c.DefaultTTL <= 0 {
		return fmt.Errorf("default_ttl must be positive when cache is enabled")
	}
	if c.Invalidation.MaxRelationshipDepth < 1 {
		return fmt.Errorf("max_relationship_depth must be at least 1")
	}
	if c.PoolSize < 1 {
		return fmt.Errorf("pool_size must be at least 1")
	}

	return nil
}

// GetAddr returns the Redis connection address
func (c *Config) GetAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// IsClusterMode returns true if Redis cluster is enabled
func (c *Config) IsClusterMode() bool {
	return c.Cluster.Enabled && len(c.Cluster.Addresses) > 0
}
