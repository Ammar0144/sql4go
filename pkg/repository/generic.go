package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/ammar0144/sql4go/pkg/db"
	"github.com/ammar0144/sql4go/pkg/redis"

	"github.com/cespare/xxhash/v2"
	"gorm.io/gorm"
)

// Cache key constants for consistent key generation
const (
	cacheKeyPrefix     = "sql4go"
	cacheKeySeparator  = ":"
	cacheKeyHashLength = 12 // Balance between uniqueness and key length
)

// GenericRepository provides comprehensive CRUD operations with intelligent caching
// It automatically handles cache-first reads and relationship-aware invalidation
type GenericRepository[T Entity] struct {
	db         *gorm.DB
	dbManager  *db.Manager
	redis      *redis.Manager
	entityType reflect.Type
	tableName  string
	primaryKey string
	dbName     string // Database name for cache key isolation
}

// NewGenericRepository creates a new generic repository with GORM and Redis integration
func NewGenericRepository[T Entity](dbManager *db.Manager, redisManager *redis.Manager) Repository[T] {
	// Obtain the reflect.Type for the generic type parameter T in a safe way
	entityType := reflect.TypeOf((*T)(nil)).Elem()

	// Create a model instance suitable for assertions and for calling Entity methods
	var model interface{}
	if entityType.Kind() == reflect.Ptr {
		model = reflect.New(entityType.Elem()).Interface()
	} else {
		model = reflect.New(entityType).Interface()
	}

	// Assert the model implements Entity
	ent, ok := model.(Entity)
	if !ok {
		panic(fmt.Sprintf("entity type %v does not implement repository.Entity", entityType))
	}

	// Verify TableName()
	tableName := ent.TableName()
	if tableName == "" {
		panic(fmt.Sprintf("entity type %v returned empty TableName(), Entity interface not properly implemented", entityType))
	}

	// Extract database name from GORM connection
	dbName := extractDatabaseName(dbManager.DB())

	return &GenericRepository[T]{
		db:         dbManager.DB(),
		dbManager:  dbManager,
		redis:      redisManager,
		entityType: entityType,
		tableName:  tableName,
		primaryKey: func() string {
			// Prefer extracting the DB column name for primary key via GORM schema
			if db := dbManager.DB(); db != nil {
				if pk := extractPrimaryKeyNameFromDB(db, entityType); pk != "" {
					return pk
				}
			}
			return extractPrimaryKeyName(entityType)
		}(),
		dbName: dbName,
	}
}

// NewGenericRepositoryDBOnly creates a repository without Redis (database only)
// For cases where caching is not needed
func NewGenericRepositoryDBOnly[T Entity](manager *db.Manager) Repository[T] {
	return NewGenericRepository[T](manager, nil)
}

// withQueryTimeout wraps a context with the configured query timeout
func (r *GenericRepository[T]) withQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.dbManager != nil && r.dbManager.Config() != nil {
		timeout := r.dbManager.Config().QueryTimeout
		if timeout > 0 {
			return context.WithTimeout(ctx, timeout)
		}
	}
	// Return context without timeout if not configured
	return ctx, func() {}
}

// ============================================================================
// READ OPERATIONS - Cache-First Implementation
// ============================================================================

// FindByID finds a record by ID with cache-first strategy
func (r *GenericRepository[T]) FindByID(ctx context.Context, id interface{}) (*T, bool, bool, error) {
	// Input validation
	if id == nil {
		return nil, false, false, fmt.Errorf("id cannot be nil")
	}

	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, false, false, fmt.Errorf("context cancelled before operation: %w", err)
	}

	// Generate cache key
	cacheKey := r.generateCacheKey("find_by_id", fmt.Sprintf("%v", id))

	// Try cache first
	if r.redis != nil {
		var entity T
		if err := r.redis.GetValue(ctx, cacheKey, &entity); err == nil {
			return &entity, true, false, nil // Cache hit
		} else if !redis.IsKeyNotFound(err) {
			// Unexpected cache error; continue to DB (best-effort cache)
		}
	}

	// Cache miss - query database (use primary key lookup to avoid injecting column names)
	var entity T
	result := r.db.WithContext(ctx).First(&entity, id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, false, false, nil // Not found, not an error
		}
		return nil, false, false, fmt.Errorf("database error: %w", result.Error)
	}

	// Cache the result
	cacheStored := false
	if r.redis != nil {
		if err := r.redis.SetValue(ctx, cacheKey, entity); err == nil {
			cacheStored = true
		}
		// Ignore cache errors - best effort
	}

	return &entity, false, cacheStored, nil // From DB, cacheStored status
}

// FindAll finds all records with caching
func (r *GenericRepository[T]) FindAll(ctx context.Context) ([]T, bool, bool, error) {
	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, false, false, fmt.Errorf("context cancelled before operation: %w", err)
	}

	cacheKey := r.generateCacheKey("find_all", "")

	// Try cache first
	if r.redis != nil {
		var entities []T
		if err := r.redis.GetLargeValue(ctx, cacheKey, &entities); err == nil {
			return entities, true, false, nil // Cache hit
		} else if !redis.IsKeyNotFound(err) {
			// Unexpected cache error; continue to DB
		}
	}

	// Cache miss - query database
	var entities []T
	result := r.db.WithContext(ctx).Find(&entities)
	if result.Error != nil {
		return nil, false, false, fmt.Errorf("database error: %w", result.Error)
	}

	// Cache the result
	cacheStored := false
	if r.redis != nil {
		if err := r.redis.SetLargeValue(ctx, cacheKey, entities); err == nil {
			cacheStored = true
		}
		// Ignore cache errors - best effort
	}

	return entities, false, cacheStored, nil // From DB, cacheStored status
}

// FindWhere finds records with conditions and caching
func (r *GenericRepository[T]) FindWhere(ctx context.Context, query interface{}, args ...interface{}) ([]T, bool, bool, error) {
	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, false, false, fmt.Errorf("context cancelled before operation: %w", err)
	}

	// Validate query type - don't cache *gorm.DB queries as they're not deterministic
	shouldCache := true
	if _, isGormDB := query.(*gorm.DB); isGormDB {
		shouldCache = false
	}

	// Generate cache key from query and args
	var cacheKey string
	if shouldCache {
		cacheKey = r.generateCacheKeyFromQuery("find_where", query, args...)
	}

	// Try cache first (only if cacheable)
	if r.redis != nil && shouldCache {
		var entities []T
		if err := r.redis.GetLargeValue(ctx, cacheKey, &entities); err == nil {
			return entities, true, false, nil // Cache hit
		} else if !redis.IsKeyNotFound(err) {
			// Unexpected cache error; continue to DB
		}
	}

	// Cache miss - query database
	var entities []T
	result := r.db.WithContext(ctx).Where(query, args...).Find(&entities)
	if result.Error != nil {
		return nil, false, false, fmt.Errorf("database error: %w", result.Error)
	}

	// Cache the result with dependencies (only if cacheable)
	cacheStored := false
	if r.redis != nil && shouldCache {
		dependencies := r.extractDependenciesFromEntities(entities)
		if data, err := r.marshalEntities(entities); err == nil {
			// best-effort cache store; ignore cache errors here
			if err := r.redis.SetLargeWithDependencies(ctx, cacheKey, data, dependencies); err == nil {
				cacheStored = true
			}
		}
	}

	return entities, false, cacheStored, nil // From DB, cacheStored status
}

// First finds the first record matching conditions
func (r *GenericRepository[T]) First(ctx context.Context, query interface{}, args ...interface{}) (*T, bool, bool, error) {
	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, false, false, fmt.Errorf("context cancelled before operation: %w", err)
	}

	// Validate query type - don't cache *gorm.DB queries
	shouldCache := true
	if _, isGormDB := query.(*gorm.DB); isGormDB {
		shouldCache = false
	}

	var cacheKey string
	if shouldCache {
		cacheKey = r.generateCacheKeyFromQuery("first", query, args...)
	}

	// Try cache first (only if cacheable)
	if r.redis != nil && shouldCache {
		var entity T
		if err := r.redis.GetValue(ctx, cacheKey, &entity); err == nil {
			return &entity, true, false, nil // Cache hit
		} else if !redis.IsKeyNotFound(err) {
			// Unexpected cache error; continue to DB
		}
	}

	// Cache miss - query database
	var entity T
	result := r.db.WithContext(ctx).Where(query, args...).First(&entity)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, false, false, nil // Not found, not an error
		}
		return nil, false, false, fmt.Errorf("database error: %w", result.Error)
	}

	// Cache the result (only if cacheable)
	cacheStored := false
	if r.redis != nil && shouldCache {
		if err := r.redis.SetValue(ctx, cacheKey, entity); err == nil {
			cacheStored = true
		}
		// Ignore cache errors - best effort
	}

	return &entity, false, cacheStored, nil // From DB, cacheStored status
}

// Count counts records with caching
func (r *GenericRepository[T]) Count(ctx context.Context) (int64, bool, bool, error) {
	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return 0, false, false, fmt.Errorf("context cancelled before operation: %w", err)
	}

	cacheKey := r.generateCacheKey("count", "")

	// Try cache first
	if r.redis != nil {
		var count int64
		if err := r.redis.GetValue(ctx, cacheKey, &count); err == nil {
			return count, true, false, nil // Cache hit
		} else if !redis.IsKeyNotFound(err) {
			// Unexpected cache error; continue to DB
		}
	}

	// Cache miss - query database
	var count int64
	var entity T
	result := r.db.WithContext(ctx).Model(&entity).Count(&count)
	if result.Error != nil {
		return 0, false, false, fmt.Errorf("database error: %w", result.Error)
	}

	// Cache the result
	cacheStored := false
	if r.redis != nil {
		if err := r.redis.SetValue(ctx, cacheKey, count); err == nil {
			cacheStored = true
		}
		// Ignore cache errors - best effort
	}

	return count, false, cacheStored, nil // From DB, cacheStored status
}

// Exists checks if a record exists by ID
func (r *GenericRepository[T]) Exists(ctx context.Context, id interface{}) (bool, bool, bool, error) {
	entity, cacheHit, cacheStored, err := r.FindByID(ctx, id)
	if err != nil {
		return false, false, false, err
	}
	return entity != nil, cacheHit, cacheStored, nil
}

// ============================================================================
// QUERY BUILDER METHODS - Chainable GORM Operations
// ============================================================================

// Preload specifies associations to preload (returns new repository instance)
func (r *GenericRepository[T]) Preload(ctx context.Context, associations ...string) Repository[T] {
	newRepo := *r
	db := newRepo.db
	for _, association := range associations {
		db = db.Preload(association)
	}
	newRepo.db = db
	return &newRepo
}

// Joins specifies joins to perform
func (r *GenericRepository[T]) Joins(ctx context.Context, query string, args ...interface{}) Repository[T] {
	newRepo := *r
	newRepo.db = newRepo.db.Joins(query, args...)
	return &newRepo
}

// Order specifies ordering
func (r *GenericRepository[T]) Order(ctx context.Context, value interface{}) Repository[T] {
	newRepo := *r
	newRepo.db = newRepo.db.Order(value)
	return &newRepo
}

// Limit specifies limit
func (r *GenericRepository[T]) Limit(ctx context.Context, limit int) Repository[T] {
	if limit < 0 {
		limit = 0 // Normalize negative values to 0
	}
	newRepo := *r
	newRepo.db = newRepo.db.Limit(limit)
	return &newRepo
}

// Offset specifies offset
func (r *GenericRepository[T]) Offset(ctx context.Context, offset int) Repository[T] {
	if offset < 0 {
		offset = 0 // Normalize negative values to 0
	}
	newRepo := *r
	newRepo.db = newRepo.db.Offset(offset)
	return &newRepo
}

// ============================================================================
// WRITE OPERATIONS - Cache Invalidation Implementation
// ============================================================================

// Create creates a new record with automatic cache invalidation
func (r *GenericRepository[T]) Create(ctx context.Context, entity *T) (bool, error) {
	// Input validation
	if entity == nil {
		return false, fmt.Errorf("entity cannot be nil")
	}

	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// Execute database operation
	if err := r.db.WithContext(ctx).Create(entity).Error; err != nil {
		return false, fmt.Errorf("database error: %w", err)
	}

	// Invalidate related caches
	cacheInvalidated := false
	if r.redis != nil {
		r.invalidateEntityCaches(ctx, *entity)
		cacheInvalidated = true // Best effort - assume success
	}

	return cacheInvalidated, nil
}

// Update updates a record with relationship-aware cache invalidation
func (r *GenericRepository[T]) Update(ctx context.Context, entity *T) (bool, error) {
	// Input validation
	if entity == nil {
		return false, fmt.Errorf("entity cannot be nil")
	}

	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// Execute database operation
	if err := r.db.WithContext(ctx).Save(entity).Error; err != nil {
		return false, fmt.Errorf("database error: %w", err)
	}

	// Invalidate related caches
	cacheInvalidated := false
	if r.redis != nil {
		r.invalidateEntityCaches(ctx, *entity)
		cacheInvalidated = true // Best effort - assume success
	}

	return cacheInvalidated, nil
}

// Delete deletes a record by ID with cache invalidation
func (r *GenericRepository[T]) Delete(ctx context.Context, id interface{}) (bool, error) {
	// Input validation
	if id == nil {
		return false, fmt.Errorf("id cannot be nil")
	}

	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// First get the entity to invalidate relationships
	// Use GORM's safe primary key lookup instead of string formatting to prevent SQL injection
	var entity T
	if err := r.db.WithContext(ctx).First(&entity, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil // Entity doesn't exist, no error
		}
		return false, fmt.Errorf("database error while finding entity to delete: %w", err)
	}

	// Execute database operation
	if err := r.db.WithContext(ctx).Delete(&entity).Error; err != nil {
		return false, fmt.Errorf("database error: %w", err)
	}

	// Invalidate related caches
	cacheInvalidated := false
	if r.redis != nil {
		r.invalidateEntityCaches(ctx, entity)
		cacheInvalidated = true // Best effort - assume success
	}

	return cacheInvalidated, nil
}

// CreateBatch creates multiple records in batch with cache invalidation
func (r *GenericRepository[T]) CreateBatch(ctx context.Context, entities []*T) error {
	if len(entities) == 0 {
		return nil
	}

	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// Execute batch database operation
	if err := r.db.WithContext(ctx).Create(&entities).Error; err != nil {
		return fmt.Errorf("batch create error: %w", err)
	}

	// Invalidate related caches for all entities
	if r.redis != nil {
		for _, entity := range entities {
			if entity != nil {
				r.invalidateEntityCaches(ctx, *entity)
			}
		}
	}

	return nil
}

// UpdateBatch updates multiple records in batch with cache invalidation
func (r *GenericRepository[T]) UpdateBatch(ctx context.Context, entities []*T) error {
	if len(entities) == 0 {
		return nil
	}

	// Apply query timeout
	ctx, cancel := r.withQueryTimeout(ctx)
	defer cancel()

	// Execute batch database operation
	if err := r.db.WithContext(ctx).Save(&entities).Error; err != nil {
		return fmt.Errorf("batch update error: %w", err)
	}

	// Invalidate related caches for all entities
	if r.redis != nil {
		for _, entity := range entities {
			if entity != nil {
				r.invalidateEntityCaches(ctx, *entity)
			}
		}
	}

	return nil
}

// InvalidateCache invalidates all caches for this entity type in this database
func (r *GenericRepository[T]) InvalidateCache(ctx context.Context) error {
	if r.redis == nil {
		return nil
	}

	// Invalidate all caches for this table in this database
	pattern := fmt.Sprintf("sql4go:%s:%s:*", r.dbName, r.tableName)
	return r.redis.InvalidatePattern(ctx, pattern)
}

// WarmCache preloads commonly accessed data
func (r *GenericRepository[T]) WarmCache(ctx context.Context) error {
	if r.redis == nil {
		return nil
	}

	// Warm up common queries (ignore errors - best effort)
	_, _, _, _ = r.FindAll(ctx) // Cache all entities
	_, _, _, _ = r.Count(ctx)   // Cache count

	return nil
}

// ============================================================================
// HELPER METHODS - Cache Key Generation and Management
// ============================================================================

// generateCacheKey creates a cache key for simple operations with database isolation
func (r *GenericRepository[T]) generateCacheKey(operation, suffix string) string {
	if suffix == "" {
		return fmt.Sprintf("%s%s%s%s%s%s%s", cacheKeyPrefix, cacheKeySeparator, r.dbName, cacheKeySeparator, r.tableName, cacheKeySeparator, operation)
	}
	return fmt.Sprintf("%s%s%s%s%s%s%s%s%s", cacheKeyPrefix, cacheKeySeparator, r.dbName, cacheKeySeparator, r.tableName, cacheKeySeparator, operation, cacheKeySeparator, suffix)
}

// generateCacheKeyFromQuery creates a cache key from query and parameters with database isolation
func (r *GenericRepository[T]) generateCacheKeyFromQuery(operation string, query interface{}, args ...interface{}) string {
	// Handle different query types for consistent cache key generation
	var queryStr string

	switch q := query.(type) {
	case string:
		// Simple string query: "status = ? AND active = ?"
		queryStr = q
	case map[string]interface{}:
		// Map query: map[string]interface{}{"status": "active"}
		// Sort keys for consistent hashing
		data, err := json.Marshal(q)
		if err != nil {
			// Fallback to string representation if marshal fails
			queryStr = fmt.Sprintf("%v", q)
		} else {
			queryStr = string(data)
		}
	case *gorm.DB:
		// If someone passes a *gorm.DB, we can't reliably cache it
		// Use a warning marker in the key to signal this shouldn't be cached
		queryStr = "UNCACHEABLE_GORM_DB"
	default:
		// Fallback: use reflection to get a string representation
		// This handles structs and other types
		queryStr = fmt.Sprintf("%T:%v", query, query)
	}

	// Serialize args consistently
	argsData, err := json.Marshal(args)
	if err != nil {
		// Fallback to string representation if marshal fails
		argsData = []byte(fmt.Sprintf("%v", args))
	}
	argsStr := string(argsData)

	combined := queryStr + cacheKeySeparator + argsStr

	// Create hash for consistent, short keys using xxhash (fast non-cryptographic hash)
	hash := xxhash.Sum64String(combined)
	hashStr := fmt.Sprintf("%016x", hash)
	return fmt.Sprintf("%s%s%s%s%s%s%s%s%s", cacheKeyPrefix, cacheKeySeparator, r.dbName, cacheKeySeparator, r.tableName, cacheKeySeparator, operation, cacheKeySeparator, hashStr[:cacheKeyHashLength])
}

// marshalEntities converts entities to bytes for Redis storage
func (r *GenericRepository[T]) marshalEntities(entities []T) ([]byte, error) {
	data, err := json.Marshal(entities)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// extractDependenciesFromEntities builds dependency map for relationship invalidation
func (r *GenericRepository[T]) extractDependenciesFromEntities(entities []T) map[string][]interface{} {
	dependencies := make(map[string][]interface{})

	if len(entities) == 0 {
		return dependencies
	}

	for _, entity := range entities {
		// Skip zero-value entities
		var zero T
		if reflect.DeepEqual(entity, zero) {
			continue
		}

		// Add this entity's dependency
		pkValue := entity.GetPrimaryKeyValue()
		if pkValue != nil {
			dependencies[r.tableName] = append(dependencies[r.tableName], pkValue)
		}

		// First, check if entity manually implements RelationshipAware
		if relEntity, ok := any(entity).(RelationshipAware); ok {
			relationships := relEntity.GetRelationships()
			for _, relatedEntities := range relationships {
				for _, related := range relatedEntities {
					if related.EntityID != nil {
						dependencies[related.EntityType] = append(dependencies[related.EntityType], related.EntityID)
					}
				}
			}
		} else {
			// Automatic relationship detection using GORM reflection
			if pkValue != nil {
				autoRelationships := extractRelationshipsFromEntity(entity, pkValue)
				for _, relatedEntities := range autoRelationships {
					for _, related := range relatedEntities {
						if related.EntityID != nil {
							dependencies[related.EntityType] = append(dependencies[related.EntityType], related.EntityID)
						}
					}
				}
			}
		}
	}

	return dependencies
}

// invalidateEntityCaches handles cache invalidation for entity changes
func (r *GenericRepository[T]) invalidateEntityCaches(ctx context.Context, entity T) {
	// Invalidate all caches for this entity type (ignore errors - best effort)
	_ = r.InvalidateCache(ctx)

	// Invalidate specific entity dependencies (ignore errors - best effort)
	_ = r.redis.InvalidateEntityDependencies(ctx, r.tableName, entity.GetPrimaryKeyValue())

	// Handle relationship-aware invalidation
	var relationships map[string][]RelatedEntity

	// First check if entity manually implements RelationshipAware
	if relEntity, ok := any(entity).(RelationshipAware); ok {
		relationships = relEntity.GetRelationships()
	} else {
		// Use automatic GORM relationship detection
		relationships = extractRelationshipsFromEntity(entity, entity.GetPrimaryKeyValue())
	}

	// Invalidate all related entity caches (ignore errors - best effort)
	for _, relatedEntities := range relationships {
		for _, related := range relatedEntities {
			if related.EntityID != nil {
				_ = r.redis.InvalidateEntityDependencies(ctx, related.EntityType, related.EntityID)
			}
		}
	}
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// extractPrimaryKeyName extracts the primary key field name from entity type
// Uses reflection to find the field tagged as primary key or defaults to "id"
func extractPrimaryKeyName(entityType reflect.Type) string {
	// Handle pointer types
	if entityType.Kind() == reflect.Ptr {
		entityType = entityType.Elem()
	}

	// Look for field with gorm:"primaryKey" tag
	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		gormTag := field.Tag.Get("gorm")

		if strings.Contains(gormTag, "primaryKey") || strings.Contains(gormTag, "primary_key") {
			return strings.ToLower(field.Name)
		}
	}

	// Look for common primary key field names
	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		fieldName := strings.ToLower(field.Name)

		if fieldName == "id" || fieldName == "uuid" {
			return fieldName
		}
	}

	// Default fallback
	return "id"
}

// extractPrimaryKeyNameFromDB tries to obtain the primary key column name using GORM schema
// Returns empty string if it cannot be determined
func extractPrimaryKeyNameFromDB(gormDB *gorm.DB, entityType reflect.Type) string {
	if gormDB == nil || entityType == nil {
		return ""
	}

	// Create a model instance suitable for GORM's schema parser
	var model interface{}
	if entityType.Kind() == reflect.Ptr {
		model = reflect.New(entityType.Elem()).Interface()
	} else {
		// reflect.New returns *T even for non-pointer types, which is acceptable for GORM
		model = reflect.New(entityType).Interface()
	}

	stmt := &gorm.Statement{DB: gormDB}
	if err := stmt.Parse(model); err != nil {
		return ""
	}

	if stmt.Schema == nil {
		return ""
	}

	// Use PrimaryFields if present
	if len(stmt.Schema.PrimaryFields) > 0 {
		if f := stmt.Schema.PrimaryFields[0]; f != nil {
			return f.DBName
		}
	}

	return ""
}

// extractRelationshipsFromEntity automatically detects GORM relationships using reflection
// This eliminates the need to manually implement RelationshipAware interface
// Respects max relationship depth to prevent excessive recursion
func extractRelationshipsFromEntity(entity interface{}, entityID interface{}) map[string][]RelatedEntity {
	return extractRelationshipsFromEntityWithDepth(entity, entityID, 0, 3) // Default max depth of 3
}

// extractRelationshipsFromEntityWithDepth is the internal implementation with depth tracking
func extractRelationshipsFromEntityWithDepth(entity interface{}, entityID interface{}, currentDepth, maxDepth int) map[string][]RelatedEntity {
	relationships := make(map[string][]RelatedEntity)

	// Enforce maximum depth to prevent excessive recursion
	if currentDepth >= maxDepth {
		return relationships
	}

	// Safely get entity type
	entityType := reflect.TypeOf(entity)
	if entityType == nil {
		return relationships // Return empty for nil interface
	}
	if entityType.Kind() == reflect.Ptr {
		entityType = entityType.Elem()
	}

	// Safely get entity value - prevent panic on nil pointer
	entityValue := reflect.ValueOf(entity)
	if entityValue.Kind() == reflect.Ptr {
		if entityValue.IsNil() {
			return relationships // Return empty for nil pointer
		}
		entityValue = entityValue.Elem()
	}

	// Verify we have a valid, non-zero value
	if !entityValue.IsValid() {
		return relationships
	}

	// Scan all fields for GORM relationship tags
	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		gormTag := field.Tag.Get("gorm")

		if gormTag == "" {
			continue
		}

		// Determine relationship type and target entity
		relationType, targetEntity := parseGORMRelationship(field, gormTag)
		if relationType == "" || targetEntity == "" {
			continue
		}

		var relatedEntityID interface{}

		// For belongs_to, get the foreign key value
		if relationType == "belongs_to" {
			foreignKey := extractForeignKeyFromTag(gormTag)
			if foreignKey == "" {
				foreignKey = field.Name + "ID" // GORM default convention
			}

			// Find the foreign key field value
			for j := 0; j < entityType.NumField(); j++ {
				if strings.EqualFold(entityType.Field(j).Name, foreignKey) {
					fieldValue := entityValue.Field(j)
					if fieldValue.IsValid() && !fieldValue.IsZero() {
						relatedEntityID = fieldValue.Interface()
					}
					break
				}
			}
		} else {
			// For has_one/has_many, use current entity's ID
			relatedEntityID = entityID
		}

		relationships[relationType] = append(relationships[relationType], RelatedEntity{
			EntityType: targetEntity,
			EntityID:   relatedEntityID,
		})
	}

	return relationships
}

// parseGORMRelationship parses GORM tag to extract relationship type and target entity
func parseGORMRelationship(field reflect.StructField, gormTag string) (relationType, targetEntity string) {
	// Check for explicit relationship types in GORM tags
	if strings.Contains(gormTag, "foreignKey:") {
		// This field likely defines a relationship
		fieldType := field.Type

		// Handle slice types (has_many)
		if fieldType.Kind() == reflect.Slice {
			fieldType = fieldType.Elem()
			relationType = "has_many"
		} else {
			relationType = "has_one"
		}

		// Handle pointer types
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// Get target entity name (convert struct name to table name)
		if fieldType.Kind() == reflect.Struct {
			targetEntity = convertStructNameToTableName(fieldType.Name())
		}
	} else if strings.Contains(gormTag, "references:") {
		// This indicates a belongs_to relationship
		relationType = "belongs_to"
		fieldType := field.Type

		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Struct {
			targetEntity = convertStructNameToTableName(fieldType.Name())
		}
	}

	// Auto-detect based on field characteristics if no explicit tags
	if relationType == "" {
		fieldType := field.Type

		if fieldType.Kind() == reflect.Slice {
			// Slice of structs = has_many
			elemType := fieldType.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				relationType = "has_many"
				targetEntity = convertStructNameToTableName(elemType.Name())
			}
		} else {
			// Single struct = has_one or belongs_to
			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}
			if fieldType.Kind() == reflect.Struct {
				// Check if there's a corresponding foreign key field
				if hasCorrespondingForeignKey(field.Name) {
					relationType = "belongs_to"
				} else {
					relationType = "has_one"
				}
				targetEntity = convertStructNameToTableName(fieldType.Name())
			}
		}
	}

	return relationType, targetEntity
}

// extractForeignKeyFromTag extracts foreign key field name from GORM tag
func extractForeignKeyFromTag(gormTag string) string {
	parts := strings.Split(gormTag, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "foreignKey:") {
			return strings.TrimPrefix(part, "foreignKey:")
		}
	}
	return ""
}

// convertStructNameToTableName converts struct name to table name using GORM conventions
// WARNING: This uses basic English pluralization rules which will fail for irregular nouns.
// For production use, it's STRONGLY RECOMMENDED that your entities implement the Entity.TableName()
// method to return the correct table name explicitly.
//
// Known limitations:
//   - "Person" -> "persons" (should be "people")
//   - "Child" -> "childs" (should be "children")
//   - "Datum" -> "datums" (should be "data")
//   - "Status" -> "statuses" (happens to be correct)
//
// Example proper implementation:
//
//	type User struct {
//	    ID   uint
//	    Name string
//	}
//	func (User) TableName() string { return "users" }
func convertStructNameToTableName(structName string) string {
	tableName := strings.ToLower(structName)

	// Basic English pluralization rules (NOT comprehensive)
	if strings.HasSuffix(tableName, "y") && !isVowel(tableName[len(tableName)-2]) {
		// city -> cities, but day -> days
		tableName = strings.TrimSuffix(tableName, "y") + "ies"
	} else if strings.HasSuffix(tableName, "s") || strings.HasSuffix(tableName, "x") ||
		strings.HasSuffix(tableName, "z") || strings.HasSuffix(tableName, "ch") ||
		strings.HasSuffix(tableName, "sh") {
		tableName += "es"
	} else {
		tableName += "s"
	}

	return tableName
}

// isVowel checks if a byte represents a vowel
func isVowel(b byte) bool {
	return b == 'a' || b == 'e' || b == 'i' || b == 'o' || b == 'u'
}

// hasCorrespondingForeignKey checks if there's a foreign key field for the given relationship
func hasCorrespondingForeignKey(fieldName string) bool {
	// This is a simplified check - in a real implementation, you'd scan the struct
	// for fields that match the pattern like UserID for a User field
	return strings.HasSuffix(fieldName, "ID") ||
		strings.Contains(strings.ToLower(fieldName), "id")
}

// extractDatabaseName extracts the database name from GORM DB connection
// NOTE: This implementation is MySQL-specific and uses MySQL's SELECT DATABASE() function.
// For other database systems (PostgreSQL, SQLite, etc.), this would need to be adapted.
func extractDatabaseName(gormDB *gorm.DB) string {
	if gormDB == nil {
		return "unknown"
	}

	// Get the underlying SQL database
	sqlDB, err := gormDB.DB()
	if err != nil {
		return "unknown"
	}

	// Verify connection is alive before querying
	if err := sqlDB.Ping(); err != nil {
		return "unknown"
	}

	// Try to get database name from GORM's migrator first (preferred method)
	if config := gormDB.Config; config != nil {
		if migrator := gormDB.Migrator(); migrator != nil {
			if dbName := migrator.CurrentDatabase(); dbName != "" {
				return dbName
			}
		}
	}

	// Fallback: Execute MySQL-specific query to get current database name
	// NOTE: This only works with MySQL/MariaDB. For other databases:
	//   - PostgreSQL: SELECT current_database()
	//   - SQLite: PRAGMA database_list
	//   - SQL Server: SELECT DB_NAME()
	var dbName string
	result := gormDB.Raw("SELECT DATABASE()").Scan(&dbName)
	if result.Error == nil && dbName != "" {
		return dbName
	}

	// Final fallback
	return "default_db"
}
