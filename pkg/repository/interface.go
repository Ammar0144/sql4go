package repository

import (
	"context"
)

// Repository defines the generic repository interface
type Repository[T any] interface {
	// Queries (Read Operations - Cache-First)
	// Returns: (result, cacheHit, cacheStored, error)
	// - cacheHit: true if data retrieved from Redis cache
	// - cacheStored: true if data successfully stored to Redis after DB query
	FindByID(ctx context.Context, id interface{}) (*T, bool, bool, error)
	FindAll(ctx context.Context) ([]T, bool, bool, error)
	FindWhere(ctx context.Context, query interface{}, args ...interface{}) ([]T, bool, bool, error)
	First(ctx context.Context, query interface{}, args ...interface{}) (*T, bool, bool, error)
	Count(ctx context.Context) (int64, bool, bool, error)
	Exists(ctx context.Context, id interface{}) (bool, bool, bool, error)

	// GORM Query Methods (Cached)
	Preload(ctx context.Context, associations ...string) Repository[T]
	Joins(ctx context.Context, query string, args ...interface{}) Repository[T]
	Order(ctx context.Context, value interface{}) Repository[T]
	Limit(ctx context.Context, limit int) Repository[T]
	Offset(ctx context.Context, offset int) Repository[T]

	// Commands (Write Operations - Relationship-Aware Cache Invalidation)
	// Returns: (cacheInvalidated, error)
	// - cacheInvalidated: true if related caches were successfully invalidated
	Create(ctx context.Context, entity *T) (bool, error)
	Update(ctx context.Context, entity *T) (bool, error)
	Delete(ctx context.Context, id interface{}) (bool, error)

	// Batch Operations
	CreateBatch(ctx context.Context, entities []*T) error
	UpdateBatch(ctx context.Context, entities []*T) error

	// Cache Management
	InvalidateCache(ctx context.Context) error
	WarmCache(ctx context.Context) error
}
