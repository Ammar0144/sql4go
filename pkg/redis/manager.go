package redis

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache key constants for consistent key generation across the application
const (
	cacheKeyPrefix        = "sql4go"
	cacheKeySeparator     = ":"
	cacheDependencyPrefix = "deps"
	cacheMetadataSuffix   = "_internal:meta"  // Internal suffix to prevent user key collisions
	cacheChunkPrefix      = "_internal:chunk" // Internal prefix for chunk keys
)

// Manager manages Redis connections and cache operations
type Manager struct {
	config        *Config
	client        redis.UniversalClient
	clusterClient *redis.ClusterClient
	metrics       *Metrics
}

// NewManager creates a new Redis cache manager
func NewManager(config *Config) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid redis config: %w", err)
	}

	manager := &Manager{
		config:  config,
		metrics: NewMetrics(),
	}

	// Initialize Redis client based on configuration
	if err := manager.initializeClient(); err != nil {
		return nil, fmt.Errorf("failed to initialize redis client: %w", err)
	}

	return manager, nil
}

// initializeClient sets up the Redis client based on configuration
func (m *Manager) initializeClient() error {
	if !m.config.Enabled {
		return nil // Skip initialization if cache is disabled
	}

	if m.config.IsClusterMode() {
		// Redis Cluster configuration
		m.clusterClient = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:           m.config.Cluster.Addresses,
			Username:        m.config.Cluster.Username,
			Password:        m.config.Cluster.Password,
			PoolSize:        m.config.PoolSize,
			MinIdleConns:    m.config.MinIdleConns,
			ConnMaxLifetime: m.config.MaxConnAge,
			PoolTimeout:     m.config.PoolTimeout,
			ConnMaxIdleTime: m.config.IdleTimeout,
			ReadTimeout:     m.config.ReadTimeout,
			WriteTimeout:    m.config.WriteTimeout,
			DialTimeout:     m.config.DialTimeout,
		})
		m.client = m.clusterClient
	} else {
		// Single Redis instance configuration
		m.client = redis.NewClient(&redis.Options{
			Addr:            m.config.GetAddr(),
			Password:        m.config.Password,
			DB:              m.config.Database,
			PoolSize:        m.config.PoolSize,
			MinIdleConns:    m.config.MinIdleConns,
			ConnMaxLifetime: m.config.MaxConnAge,
			PoolTimeout:     m.config.PoolTimeout,
			ConnMaxIdleTime: m.config.IdleTimeout,
			ReadTimeout:     m.config.ReadTimeout,
			WriteTimeout:    m.config.WriteTimeout,
			DialTimeout:     m.config.DialTimeout,
		})
	}

	return nil
}

// Config returns the manager's configuration
func (m *Manager) Config() *Config {
	return m.config
}

// Close closes the Redis connection
func (m *Manager) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// Ping tests the Redis connection
// Returns nil if cache is disabled (not an error condition)
// Returns ErrClientNotInitialized if client is not initialized
// Returns ErrConnectionFailed if ping fails
func (m *Manager) Ping(ctx context.Context) error {
	// Cache disabled is not an error - it's a valid configuration state
	if !m.config.Enabled {
		return nil
	}

	// Client not initialized when cache is enabled - this is an error
	if m.client == nil {
		return ErrClientNotInitialized
	}

	// Test actual connection
	result := m.client.Ping(ctx)
	if result.Err() != nil {
		return fmt.Errorf("%w: %v", ErrConnectionFailed, result.Err())
	}

	return nil
}

// checkClient validates that cache is enabled and client is initialized
// Returns ErrCacheDisabled if cache is disabled
// Returns ErrClientNotInitialized if client is nil
func (m *Manager) checkClient() error {
	if !m.config.Enabled {
		return ErrCacheDisabled
	}
	if m.client == nil {
		return ErrClientNotInitialized
	}
	return nil
}

// Get retrieves a value from cache
func (m *Manager) Get(ctx context.Context, key string) ([]byte, error) {
	if err := m.checkClient(); err != nil {
		return nil, err
	}

	start := time.Now()
	result := m.client.Get(ctx, key)
	m.metrics.RecordGet(time.Since(start))

	if result.Err() == redis.Nil {
		m.metrics.RecordCacheMiss()
		return nil, ErrKeyNotFound // Key not found
	}

	if result.Err() != nil {
		m.metrics.RecordCacheError()
		return nil, fmt.Errorf("redis get error: %w", result.Err())
	}

	m.metrics.RecordCacheHit()
	return []byte(result.Val()), nil
}

// Set stores a value in cache with TTL
func (m *Manager) Set(ctx context.Context, key string, value []byte) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	start := time.Now()
	result := m.client.Set(ctx, key, value, m.config.DefaultTTL)
	m.metrics.RecordSet(time.Since(start))

	return result.Err()
}

// SetWithTTL stores a value in cache with custom TTL
func (m *Manager) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	result := m.client.Set(ctx, key, value, ttl)
	return result.Err()
}

// Delete removes a key from cache
func (m *Manager) Delete(ctx context.Context, key string) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	start := time.Now()
	result := m.client.Del(ctx, key)
	m.metrics.RecordDelete(time.Since(start))

	return result.Err()
}

// DeleteKeys removes multiple keys from cache
func (m *Manager) DeleteKeys(ctx context.Context, keys []string) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	if len(keys) == 0 {
		return nil
	}

	result := m.client.Del(ctx, keys...)
	return result.Err()
}

// InvalidatePattern removes keys matching a pattern using SCAN instead of KEYS
// SCAN is non-blocking and production-safe, unlike KEYS which blocks the Redis server
func (m *Manager) InvalidatePattern(ctx context.Context, pattern string) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	// Use SCAN to iterate through keys without blocking Redis
	var cursor uint64
	var keys []string
	const scanBatchSize = 100 // Process keys in batches

	for {
		// SCAN returns a cursor and a batch of keys
		var batch []string
		var err error

		batch, cursor, err = m.client.Scan(ctx, cursor, pattern, scanBatchSize).Result()
		if err != nil {
			return fmt.Errorf("failed to scan keys with pattern %s: %w", pattern, err)
		}

		keys = append(keys, batch...)

		// Delete keys in batches to avoid large atomic operations
		if len(batch) > 0 {
			if err := m.client.Del(ctx, batch...).Err(); err != nil {
				return fmt.Errorf("failed to delete batch: %w", err)
			}
			m.metrics.RecordInvalidation()
		}

		// cursor == 0 means we've iterated through all keys
		if cursor == 0 {
			break
		}
	}

	return nil
}

// InvalidateRelationships invalidates related cache keys based on entity relationships
func (m *Manager) InvalidateRelationships(ctx context.Context, entityType string, entityID interface{}) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	// Build patterns for relationship invalidation
	patterns := m.buildInvalidationPatterns(entityType, entityID)

	// Invalidate each pattern
	for _, pattern := range patterns {
		if err := m.InvalidatePattern(ctx, pattern); err != nil {
			// Log error but continue with other patterns
			continue
		}
	}

	return nil
}

// buildInvalidationPatterns creates cache key patterns for relationship invalidation
func (m *Manager) buildInvalidationPatterns(entityType string, entityID interface{}) []string {
	patterns := []string{
		// Base entity patterns
		fmt.Sprintf("%s%s%s%s*", cacheKeyPrefix, cacheKeySeparator, entityType, cacheKeySeparator),
		fmt.Sprintf("%s%s%s%sfind_by_id%s%v", cacheKeyPrefix, cacheKeySeparator, entityType, cacheKeySeparator, cacheKeySeparator, entityID),
	}

	// Add custom invalidation patterns if configured
	if customPatterns, exists := m.config.Invalidation.KeyPatterns[entityType]; exists {
		for _, customPattern := range customPatterns {
			// Replace placeholders with actual entity ID
			pattern := strings.ReplaceAll(customPattern, "{id}", fmt.Sprintf("%v", entityID))
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}

// WarmCache preloads commonly accessed data
func (m *Manager) WarmCache(ctx context.Context, entities []string) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	if !m.config.WarmUp.Enabled {
		return nil // Cache warming is disabled
	}

	// This is a placeholder for cache warming logic
	// In a real implementation, this would:
	// 1. Query the database for commonly accessed data
	// 2. Store the results in Redis with appropriate TTL
	// 3. Use configured warming strategies (startup, scheduled, etc.)

	return nil
}

// AddDependency links a cache key to an entity for relationship-aware invalidation
func (m *Manager) AddDependency(ctx context.Context, entityType string, entityID interface{}, cacheKey string) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	// Create dependency key: "sql4go:deps:customer:123"
	dependencyKey := fmt.Sprintf("%s%s%s%s%s%s%v", cacheKeyPrefix, cacheKeySeparator, cacheDependencyPrefix, cacheKeySeparator, entityType, cacheKeySeparator, entityID)

	// Add cache key to the set of dependencies for this entity
	result := m.client.SAdd(ctx, dependencyKey, cacheKey)
	if result.Err() != nil {
		return fmt.Errorf("failed to add dependency: %w", result.Err())
	}

	m.metrics.RecordDependency()

	// Set TTL on dependency key to prevent memory leaks
	if res := m.client.Expire(ctx, dependencyKey, m.config.DefaultTTL*2); res.Err() != nil {
		// record cache error but do not fail the whole operation
		m.metrics.RecordCacheError()
	}

	return nil
}

// AddMultipleDependencies links a cache key to multiple entities
// dependencies: map[entityType] -> []entityIDs
func (m *Manager) AddMultipleDependencies(ctx context.Context, dependencies map[string][]interface{}, cacheKey string) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	// Use pipeline for atomic operation
	pipe := m.client.Pipeline()

	for entityType, ids := range dependencies {
		for _, entityID := range ids {
			dependencyKey := fmt.Sprintf("%s%s%s%s%s%s%v", cacheKeyPrefix, cacheKeySeparator, cacheDependencyPrefix, cacheKeySeparator, entityType, cacheKeySeparator, entityID)
			pipe.SAdd(ctx, dependencyKey, cacheKey)
			pipe.Expire(ctx, dependencyKey, m.config.DefaultTTL*2)
		}
	}

	_, err := pipe.Exec(ctx)
	return err
}

// InvalidateEntityDependencies clears all caches that depend on a specific entity
func (m *Manager) InvalidateEntityDependencies(ctx context.Context, entityType string, entityID interface{}) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	dependencyKey := fmt.Sprintf("%s%s%s%s%s%s%v", cacheKeyPrefix, cacheKeySeparator, cacheDependencyPrefix, cacheKeySeparator, entityType, cacheKeySeparator, entityID)

	// Get all cache keys that depend on this entity
	result := m.client.SMembers(ctx, dependencyKey)
	if result.Err() != nil && result.Err() != redis.Nil {
		return fmt.Errorf("failed to get dependencies: %w", result.Err())
	}

	dependentKeys := result.Val()
	if len(dependentKeys) == 0 {
		return nil // No dependencies to invalidate
	}

	// Delete all dependent cache keys (including chunked and metadata keys)
	for _, cacheKey := range dependentKeys {
		if err := m.DeleteLarge(ctx, cacheKey); err != nil {
			// Log error but continue with other keys
			continue
		}
	}

	// Clean up the dependency set itself
	m.client.Del(ctx, dependencyKey)

	return nil
}

// SetWithDependencies stores a value and registers its dependencies in one operation
func (m *Manager) SetWithDependencies(ctx context.Context, cacheKey string, value []byte, dependencies map[string][]interface{}) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	// Use pipeline for atomic operation
	pipe := m.client.Pipeline()

	// 1. Store the cache value
	pipe.Set(ctx, cacheKey, value, m.config.DefaultTTL)

	// 2. Register all dependencies
	for entityType, ids := range dependencies {
		for _, entityID := range ids {
			dependencyKey := fmt.Sprintf("%s%s%s%s%s%s%v", cacheKeyPrefix, cacheKeySeparator, cacheDependencyPrefix, cacheKeySeparator, entityType, cacheKeySeparator, entityID)
			pipe.SAdd(ctx, dependencyKey, cacheKey)
			pipe.Expire(ctx, dependencyKey, m.config.DefaultTTL*2)
		}
	}

	_, err := pipe.Exec(ctx)
	return err
}

// SetLargeWithDependencies stores a large value and registers its dependencies
func (m *Manager) SetLargeWithDependencies(ctx context.Context, cacheKey string, value []byte, dependencies map[string][]interface{}) error {
	// First store the large value
	if err := m.SetLarge(ctx, cacheKey, value); err != nil {
		return err
	}

	// Then register dependencies
	return m.AddMultipleDependencies(ctx, dependencies, cacheKey)
}

// SetJSONWithDependencies stores JSON value and registers its dependencies
func (m *Manager) SetJSONWithDependencies(ctx context.Context, cacheKey string, value interface{}, dependencies map[string][]interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return m.SetWithDependencies(ctx, cacheKey, data, dependencies)
}

// GetDependencies returns all cache keys that depend on an entity
func (m *Manager) GetDependencies(ctx context.Context, entityType string, entityID interface{}) ([]string, error) {
	if err := m.checkClient(); err != nil {
		return nil, err
	}

	dependencyKey := fmt.Sprintf("%s%s%s%s%s%s%v", cacheKeyPrefix, cacheKeySeparator, cacheDependencyPrefix, cacheKeySeparator, entityType, cacheKeySeparator, entityID)

	result := m.client.SMembers(ctx, dependencyKey)
	if result.Err() == redis.Nil {
		return []string{}, nil // No dependencies
	}

	return result.Val(), result.Err()
}

// GetStats returns Redis connection and performance statistics
func (m *Manager) GetStats(ctx context.Context) (map[string]interface{}, error) {
	if err := m.checkClient(); err != nil {
		return nil, err
	}

	stats := make(map[string]interface{})

	// Get Redis server info
	info := m.client.Info(ctx, "memory", "stats")
	if info.Err() != nil {
		return nil, fmt.Errorf("failed to get redis info: %w", info.Err())
	}

	stats["redis_info"] = info.Val()
	return stats, nil
}

// SetJSON stores a JSON-serializable value in cache
func (m *Manager) SetJSON(ctx context.Context, key string, value interface{}) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return m.Set(ctx, key, data)
}

// GetJSON retrieves and unmarshals a JSON value from cache
func (m *Manager) GetJSON(ctx context.Context, key string, target interface{}) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	// Get already tracks metrics, so this will be counted
	data, err := m.Get(ctx, key)
	if err != nil {
		return err
	}

	if data == nil {
		return ErrKeyNotFound
	}

	return json.Unmarshal(data, target)
}

// Exists checks if a key exists in cache
func (m *Manager) Exists(ctx context.Context, key string) (bool, error) {
	if err := m.checkClient(); err != nil {
		return false, err
	}

	result := m.client.Exists(ctx, key)
	if result.Err() != nil {
		return false, result.Err()
	}

	return result.Val() > 0, nil
}

// getLargeValueConfig returns large value configuration with fallback to defaults
func (m *Manager) getLargeValueConfig() (maxSize, chunkSize, compressThreshold int, enableCompression, enableChunking bool) {
	config := m.config.LargeValue

	maxSize = config.MaxValueSize
	if maxSize <= 0 {
		maxSize = 1024 * 1024 * 10 // 10MB default
	}

	chunkSize = config.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 1024 * 1024 * 2 // 2MB default
	}

	compressThreshold = config.CompressThreshold
	if compressThreshold <= 0 {
		compressThreshold = 1024 * 100 // 100KB default
	}

	enableCompression = config.EnableCompression
	enableChunking = config.EnableChunking

	return
}

// SetLarge stores large values using compression and chunking if needed
func (m *Manager) SetLarge(ctx context.Context, key string, value []byte) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	maxSize, chunkSize, compressThreshold, enableCompression, enableChunking := m.getLargeValueConfig()

	// Check if value exceeds maximum allowed size
	if len(value) > maxSize {
		return fmt.Errorf("value too large: %d bytes exceeds maximum %d bytes", len(value), maxSize)
	}

	// Compress if enabled and value is large enough
	processedValue := value
	compressed := false

	if enableCompression && len(value) > compressThreshold {
		compressedValue, err := m.compressData(value)
		if err != nil {
			return fmt.Errorf("failed to compress large value: %w", err)
		}

		// Use compressed version if it's smaller
		if len(compressedValue) < len(value) {
			processedValue = compressedValue
			compressed = true
			// Record compression savings
			bytesSaved := uint64(len(value) - len(compressedValue))
			m.metrics.RecordCompression(bytesSaved)
		}
	}

	// Check if chunking is needed and enabled
	if enableChunking && len(processedValue) > chunkSize {
		m.metrics.RecordChunked()
		return m.setChunked(ctx, key, processedValue, compressed, chunkSize)
	}

	// Store normally with compression metadata
	return m.setWithMetadata(ctx, key, processedValue, compressed, false)
}

// GetLarge retrieves large values, handling decompression and chunk reassembly
func (m *Manager) GetLarge(ctx context.Context, key string) ([]byte, error) {
	if err := m.checkClient(); err != nil {
		return nil, err
	}

	// Check if this is a chunked value
	isChunked, err := m.isChunkedValue(ctx, key)
	if err != nil {
		return nil, err
	}

	var data []byte
	var compressed bool

	if isChunked {
		data, compressed, err = m.getChunked(ctx, key)
		if err != nil {
			return nil, err
		}
	} else {
		data, compressed, err = m.getWithMetadata(ctx, key)
		if err != nil {
			return nil, err
		}
	}

	// Decompress if needed
	if compressed {
		return m.decompressData(data)
	}

	return data, nil
}

// SetLargeJSON stores large JSON values with compression and chunking
func (m *Manager) SetLargeJSON(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return m.SetLarge(ctx, key, data)
}

// GetLargeJSON retrieves and unmarshals large JSON values
func (m *Manager) GetLargeJSON(ctx context.Context, key string, target interface{}) error {
	// GetLarge handles all metrics tracking internally
	data, err := m.GetLarge(ctx, key)
	if err != nil {
		return err
	}

	if data == nil {
		return ErrKeyNotFound
	}

	return json.Unmarshal(data, target)
}

// compressData compresses data using gzip
func (m *Manager) compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompressData decompresses gzip data
func (m *Manager) decompressData(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// setChunked stores large values in chunks
func (m *Manager) setChunked(ctx context.Context, key string, data []byte, compressed bool, chunkSize int) error {
	chunkCount := (len(data) + chunkSize - 1) / chunkSize // Ceiling division

	// Use pipeline for atomic chunked storage
	pipe := m.client.Pipeline()

	// Store metadata with internal suffix to prevent collisions
	metadataKey := key + cacheMetadataSuffix
	metadata := fmt.Sprintf("chunked:%t:%d", compressed, chunkCount)
	pipe.Set(ctx, metadataKey, metadata, m.config.DefaultTTL)

	// Store chunks with internal prefix to prevent collisions
	for i := 0; i < chunkCount; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(data) {
			end = len(data)
		}

		chunkKey := fmt.Sprintf("%s%s:%d", key, cacheChunkPrefix, i)
		pipe.Set(ctx, chunkKey, data[start:end], m.config.DefaultTTL)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// getChunked retrieves and reassembles chunked values
func (m *Manager) getChunked(ctx context.Context, key string) ([]byte, bool, error) {
	metadataKey := key + cacheMetadataSuffix

	metadataResult := m.client.Get(ctx, metadataKey)
	if metadataResult.Err() == redis.Nil {
		return nil, false, ErrKeyNotFound // Consistent error for missing keys
	}
	if metadataResult.Err() != nil {
		return nil, false, metadataResult.Err()
	}

	metadata := metadataResult.Val()
	parts := strings.Split(metadata, ":")
	if len(parts) != 3 || parts[0] != "chunked" {
		return nil, false, fmt.Errorf("invalid chunk metadata: %s", metadata)
	}

	compressed := parts[1] == "true"
	chunkCount, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, false, fmt.Errorf("invalid chunk count in metadata: %s", parts[2])
	}

	// Retrieve all chunks
	var chunks [][]byte
	for i := 0; i < chunkCount; i++ {
		chunkKey := fmt.Sprintf("%s%s:%d", key, cacheChunkPrefix, i)
		chunkResult := m.client.Get(ctx, chunkKey)
		if chunkResult.Err() != nil {
			return nil, false, fmt.Errorf("failed to get chunk %d: %w", i, chunkResult.Err())
		}
		chunks = append(chunks, []byte(chunkResult.Val()))
	}

	// Reassemble chunks
	var result bytes.Buffer
	for _, chunk := range chunks {
		result.Write(chunk)
	}

	return result.Bytes(), compressed, nil
}

// setWithMetadata stores value with compression metadata
func (m *Manager) setWithMetadata(ctx context.Context, key string, data []byte, compressed, chunked bool) error {
	if compressed {
		metadataKey := key + cacheMetadataSuffix
		metadata := fmt.Sprintf("single:%t:1", compressed)

		pipe := m.client.Pipeline()
		pipe.Set(ctx, metadataKey, metadata, m.config.DefaultTTL)
		pipe.Set(ctx, key, data, m.config.DefaultTTL)

		_, err := pipe.Exec(ctx)
		return err
	}

	// Store normally without metadata for uncompressed values
	return m.Set(ctx, key, data)
}

// getWithMetadata retrieves value with compression metadata
func (m *Manager) getWithMetadata(ctx context.Context, key string) ([]byte, bool, error) {
	metadataKey := key + cacheMetadataSuffix

	metadataResult := m.client.Get(ctx, metadataKey)
	if metadataResult.Err() == redis.Nil {
		// No metadata, try regular get (uncompressed)
		data, err := m.Get(ctx, key)
		return data, false, err
	}

	if metadataResult.Err() != nil {
		return nil, false, metadataResult.Err()
	}

	metadata := metadataResult.Val()
	parts := strings.Split(metadata, ":")
	if len(parts) != 3 || parts[0] != "single" {
		return nil, false, fmt.Errorf("invalid metadata: %s", metadata)
	}

	compressed := parts[1] == "true"

	data, err := m.Get(ctx, key)
	return data, compressed, err
}

// isChunkedValue checks if a key represents a chunked value
func (m *Manager) isChunkedValue(ctx context.Context, key string) (bool, error) {
	metadataKey := key + cacheMetadataSuffix

	result := m.client.Get(ctx, metadataKey)
	if result.Err() == redis.Nil {
		return false, nil // No metadata found
	}

	if result.Err() != nil {
		return false, result.Err()
	}

	metadata := result.Val()
	return strings.HasPrefix(metadata, "chunked:"), nil
}

// DeleteLarge deletes large values including all chunks and metadata
func (m *Manager) DeleteLarge(ctx context.Context, key string) error {
	if err := m.checkClient(); err != nil {
		return err
	}

	// Check if chunked
	isChunked, err := m.isChunkedValue(ctx, key)
	if err != nil {
		return err
	}

	keysToDelete := []string{key}

	if isChunked {
		// Get chunk count from metadata
		metadataKey := key + cacheMetadataSuffix
		metadataResult := m.client.Get(ctx, metadataKey)
		if metadataResult.Err() == nil {
			metadata := metadataResult.Val()
			parts := strings.Split(metadata, ":")
			if len(parts) == 3 {
				if chunkCount, err := strconv.Atoi(parts[2]); err == nil {
					// Add all chunk keys
					for i := 0; i < chunkCount; i++ {
						chunkKey := fmt.Sprintf("%s%s:%d", key, cacheChunkPrefix, i)
						keysToDelete = append(keysToDelete, chunkKey)
					}
				}
			}
		}
		keysToDelete = append(keysToDelete, metadataKey)
	} else {
		// Check for metadata key anyway (compressed single values)
		metadataKey := key + cacheMetadataSuffix
		exists, _ := m.Exists(ctx, metadataKey)
		if exists {
			keysToDelete = append(keysToDelete, metadataKey)
		}
	}

	return m.DeleteKeys(ctx, keysToDelete)
}

// GetMetrics returns current cache performance metrics
func (m *Manager) GetMetrics() MetricsSnapshot {
	if m.metrics == nil {
		return MetricsSnapshot{}
	}
	return m.metrics.GetSnapshot()
}

// ResetMetrics resets all performance metrics counters
func (m *Manager) ResetMetrics() {
	if m.metrics != nil {
		m.metrics.Reset()
	}
}
