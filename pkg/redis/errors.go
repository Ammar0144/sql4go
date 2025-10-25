package redis

import "errors"

// Sentinel errors for Redis operations
var (
	// ErrCacheDisabled is returned when attempting operations on a disabled cache
	ErrCacheDisabled = errors.New("redis cache is disabled")

	// ErrClientNotInitialized is returned when the Redis client is nil
	ErrClientNotInitialized = errors.New("redis client not initialized")

	// ErrKeyNotFound is returned when a cache key doesn't exist (not an error condition)
	ErrKeyNotFound = errors.New("cache key not found")

	// ErrConnectionFailed is returned when Redis connection cannot be established
	ErrConnectionFailed = errors.New("redis connection failed")

	// ErrInvalidKey is returned for invalid cache key operations
	ErrInvalidKey = errors.New("invalid cache key")

	// ErrSerializationFailed is returned when JSON marshaling/unmarshaling fails
	ErrSerializationFailed = errors.New("cache serialization failed")
)

// IsCacheDisabled checks if an error is ErrCacheDisabled
func IsCacheDisabled(err error) bool {
	return errors.Is(err, ErrCacheDisabled)
}

// IsKeyNotFound checks if an error is ErrKeyNotFound
func IsKeyNotFound(err error) bool {
	return errors.Is(err, ErrKeyNotFound)
}

// IsConnectionFailed checks if an error is ErrConnectionFailed
func IsConnectionFailed(err error) bool {
	return errors.Is(err, ErrConnectionFailed)
}
