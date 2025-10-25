package redis

import (
	"sync/atomic"
	"time"
)

// Metrics tracks cache performance statistics
type Metrics struct {
	// Cache hit/miss counters
	cacheHits   atomic.Uint64
	cacheMisses atomic.Uint64
	cacheErrors atomic.Uint64

	// Operation counters
	getOperations    atomic.Uint64
	setOperations    atomic.Uint64
	deleteOperations atomic.Uint64

	// Timing metrics (in nanoseconds)
	totalGetLatency    atomic.Uint64
	totalSetLatency    atomic.Uint64
	totalDeleteLatency atomic.Uint64

	// Large value metrics
	compressionSaves  atomic.Uint64 // Bytes saved via compression
	chunkedOperations atomic.Uint64

	// Invalidation metrics
	invalidationCount atomic.Uint64
	dependencyCount   atomic.Uint64
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordCacheHit increments cache hit counter
func (m *Metrics) RecordCacheHit() {
	m.cacheHits.Add(1)
}

// RecordCacheMiss increments cache miss counter
func (m *Metrics) RecordCacheMiss() {
	m.cacheMisses.Add(1)
}

// RecordCacheError increments cache error counter
func (m *Metrics) RecordCacheError() {
	m.cacheErrors.Add(1)
}

// RecordGet records a get operation with latency
func (m *Metrics) RecordGet(duration time.Duration) {
	m.getOperations.Add(1)
	m.totalGetLatency.Add(uint64(duration.Nanoseconds()))
}

// RecordSet records a set operation with latency
func (m *Metrics) RecordSet(duration time.Duration) {
	m.setOperations.Add(1)
	m.totalSetLatency.Add(uint64(duration.Nanoseconds()))
}

// RecordDelete records a delete operation with latency
func (m *Metrics) RecordDelete(duration time.Duration) {
	m.deleteOperations.Add(1)
	m.totalDeleteLatency.Add(uint64(duration.Nanoseconds()))
}

// RecordCompression records bytes saved via compression
func (m *Metrics) RecordCompression(bytesSaved uint64) {
	m.compressionSaves.Add(bytesSaved)
}

// RecordChunked increments chunked operation counter
func (m *Metrics) RecordChunked() {
	m.chunkedOperations.Add(1)
}

// RecordInvalidation increments invalidation counter
func (m *Metrics) RecordInvalidation() {
	m.invalidationCount.Add(1)
}

// RecordDependency increments dependency counter
func (m *Metrics) RecordDependency() {
	m.dependencyCount.Add(1)
}

// GetSnapshot returns a snapshot of current metrics
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	hits := m.cacheHits.Load()
	misses := m.cacheMisses.Load()
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	getOps := m.getOperations.Load()
	setOps := m.setOperations.Load()
	deleteOps := m.deleteOperations.Load()

	var avgGetLatency, avgSetLatency, avgDeleteLatency time.Duration
	if getOps > 0 {
		avgGetLatency = time.Duration(m.totalGetLatency.Load() / getOps)
	}
	if setOps > 0 {
		avgSetLatency = time.Duration(m.totalSetLatency.Load() / setOps)
	}
	if deleteOps > 0 {
		avgDeleteLatency = time.Duration(m.totalDeleteLatency.Load() / deleteOps)
	}

	return MetricsSnapshot{
		CacheHits:             hits,
		CacheMisses:           misses,
		CacheErrors:           m.cacheErrors.Load(),
		CacheHitRate:          hitRate,
		GetOperations:         getOps,
		SetOperations:         setOps,
		DeleteOperations:      deleteOps,
		AvgGetLatency:         avgGetLatency,
		AvgSetLatency:         avgSetLatency,
		AvgDeleteLatency:      avgDeleteLatency,
		CompressionBytesSaved: m.compressionSaves.Load(),
		ChunkedOperations:     m.chunkedOperations.Load(),
		InvalidationCount:     m.invalidationCount.Load(),
		DependencyCount:       m.dependencyCount.Load(),
	}
}

// Reset resets all metrics counters
func (m *Metrics) Reset() {
	m.cacheHits.Store(0)
	m.cacheMisses.Store(0)
	m.cacheErrors.Store(0)
	m.getOperations.Store(0)
	m.setOperations.Store(0)
	m.deleteOperations.Store(0)
	m.totalGetLatency.Store(0)
	m.totalSetLatency.Store(0)
	m.totalDeleteLatency.Store(0)
	m.compressionSaves.Store(0)
	m.chunkedOperations.Store(0)
	m.invalidationCount.Store(0)
	m.dependencyCount.Store(0)
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	// Cache metrics
	CacheHits    uint64
	CacheMisses  uint64
	CacheErrors  uint64
	CacheHitRate float64 // Percentage

	// Operation counts
	GetOperations    uint64
	SetOperations    uint64
	DeleteOperations uint64

	// Latency metrics
	AvgGetLatency    time.Duration
	AvgSetLatency    time.Duration
	AvgDeleteLatency time.Duration

	// Large value metrics
	CompressionBytesSaved uint64
	ChunkedOperations     uint64

	// Invalidation metrics
	InvalidationCount uint64
	DependencyCount   uint64
}
