package repository

import (
	"context"
)

// Repository defines the generic repository interface
type Repository[T any] interface {
	// Queries (Read Operations - Cache-First)
	FindByID(ctx context.Context, id interface{}) (*T, error)
	FindAll(ctx context.Context) ([]T, error)
	FindWhere(ctx context.Context, query interface{}, args ...interface{}) ([]T, error)
	First(ctx context.Context, query interface{}, args ...interface{}) (*T, error)
	Count(ctx context.Context) (int64, error)
	Exists(ctx context.Context, id interface{}) (bool, error)

	// GORM Query Methods (Cached)
	Preload(ctx context.Context, associations ...string) Repository[T]
	Joins(ctx context.Context, query string, args ...interface{}) Repository[T]
	Order(ctx context.Context, value interface{}) Repository[T]
	Limit(ctx context.Context, limit int) Repository[T]
	Offset(ctx context.Context, offset int) Repository[T]

	// Commands (Write Operations - Relationship-Aware Cache Invalidation)
	Create(ctx context.Context, entity *T) error
	Update(ctx context.Context, entity *T) error
	Delete(ctx context.Context, id interface{}) error

	// Batch Operations
	CreateBatch(ctx context.Context, entities []*T) error
	UpdateBatch(ctx context.Context, entities []*T) error

	// Cache Management
	InvalidateCache(ctx context.Context) error
	WarmCache(ctx context.Context) error
}
