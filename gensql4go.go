// Package sql4go provides a minimal GORM-based database library
// with intelligent caching for high read, low write applications.
package sql4go

import (
	"github.com/ammar0144/sql4go/pkg/db"
	"github.com/ammar0144/sql4go/pkg/redis"
	"github.com/ammar0144/sql4go/pkg/repository"
)

// Config represents database configuration
type Config = db.Config

// NewManager creates a new database manager
func NewManager(config *Config) (*db.Manager, error) {
	return db.NewManager(config)
}

// Entity interface that all repository entities must implement
type Entity = repository.Entity

// Repository provides the generic repository interface
type Repository[T Entity] interface {
	repository.Repository[T]
}

// RedisConfig represents Redis configuration
type RedisConfig = redis.Config

// NewRepository creates a new repository instance
// If redisManager is nil, operates in database-only mode
// If redisManager is provided, automatically enables intelligent caching
func NewRepository[T Entity](dbManager *db.Manager, redisManager *redis.Manager) Repository[T] {
	return repository.NewGenericRepository[T](dbManager, redisManager)
}

// NewRedisManager creates a new Redis manager
func NewRedisManager(config *RedisConfig) (*redis.Manager, error) {
	return redis.NewManager(config)
}
