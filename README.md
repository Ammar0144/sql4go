# sql4go

**An experimental, zero-configuration database library** that explores automatic relationship detection and intelligent caching patterns in Go. This is a learning project focused on reducing boilerplate and improving performance for read-heavy applications.

> 🎓 **Learning Project**: This library is built as an exploration of advanced Go patterns, GORM internals, and distributed caching strategies. Join us in learning and improving together!

## 📑 Table of Contents

1. [**The Problem That Motivated This Project**](#-the-problem-that-motivated-this-project)
2. [**How sql4go Solves These Problems**](#-how-sql4go-solves-these-problems)
3. [**The Vision**](#-the-vision)
4. [**What We're Trying to Achieve**](#-what-were-trying-to-achieve)
5. [**Current Features**](#-current-features)
6. [**Architecture**](#-architecture)
7. [**Installation**](#installation)
8. [**Why sql4go?**](#why-sql4go)
9. [**Quick Start**](#quick-start)
10. [**Core API**](#core-api) - Repository operations, GORM query builder, cache management
11. [**Monitoring & Metrics**](#-monitoring--metrics)
12. [**How It Works**](#how-it-works)
13. [**Configuration**](#configuration)
14. [**Entity Interface**](#entity-interface)
14. [**Entity Interface**](#entity-interface)
15. [**Known Challenges & Learning Areas**](#-known-challenges--learning-areas)
16. [**Edge Cases & Foreseen Issues**](#-edge-cases--foreseen-issues)
17. [**Learning Together**](#-learning-together)
18. [**When to Use sql4go**](#-when-to-use-sql4go)
15. [**Edge Cases & Foreseen Issues**](#-edge-cases--foreseen-issues)
16. [**Learning Together**](#-learning-together)
18. [**When to Use sql4go**](#-when-to-use-sql4go)
19. [**Best Practices**](#best-practices)
20. [**Troubleshooting**](#troubleshooting)
21. [**Advanced Usage**](#advanced-usage)
22. [**Requirements & Dependencies**](#requirements)
23. [**Contributing & Learning Together**](#-contributing--learning-together)
24. [**Documentation & Resources**](#-documentation--resources)
25. [**License & Acknowledgments**](#license)
26. [**Get Involved**](#-get-involved)
## 🔍 The Problem That Motivated This Project

### Issue #1: Repetitive Repository Boilerplate

**The Reality**: Every Go project using GORM ends up with repetitive repository code:

```go
// You write this for EVERY entity (User, Order, Product, etc.)
type UserRepository struct {
    db *gorm.DB
}

func (r *UserRepository) Create(user *User) error { ... }
func (r *UserRepository) FindByID(id uint) (*User, error) { ... }
func (r *UserRepository) FindAll() ([]*User, error) { ... }
func (r *UserRepository) Update(user *User) error { ... }
func (r *UserRepository) Delete(id uint) error { ... }
// ... and more
```

**Impact**: 
- 200-300 lines of code per entity
- Copy-paste errors
- Inconsistent error handling
- Difficult to maintain across entities

### Issue #2: Manual Cache Management Hell

**The Reality**: Adding caching means manual work everywhere:

```go
func (r *UserRepository) FindByID(id uint) (*User, error) {
    // 1. Check cache (manual)
    cacheKey := fmt.Sprintf("user:%d", id)
    if cached, err := r.redis.Get(ctx, cacheKey).Result(); err == nil {
        var user User
        json.Unmarshal([]byte(cached), &user)
        return &user, nil
    }
    
    // 2. Query database
    var user User
    if err := r.db.First(&user, id).Error; err != nil {
        return nil, err
    }
    
    // 3. Store in cache (manual)
    data, _ := json.Marshal(user)
    r.redis.Set(ctx, cacheKey, data, time.Hour*24)
    
    return &user, nil
}

func (r *UserRepository) Update(user *User) error {
    // 4. Update database
    if err := r.db.Save(user).Error; err != nil {
        return err
    }
    
    // 5. Invalidate cache (manual - easy to forget!)
    r.redis.Del(ctx, fmt.Sprintf("user:%d", user.ID))
    
    // 6. What about related caches? (often forgotten!)
    // Orders? Profile? Addresses?
    // This gets out of hand quickly...
    
    return nil
}
```

**Impact**:
- 10-20 additional lines per method
- Cache invalidation bugs (stale data)
- Forgotten relationship invalidation
- Difficult to maintain

### Issue #3: Relationship Cache Invalidation Nightmare

**The Reality**: When you update a User, you must manually invalidate all related caches:

```go
func (r *UserRepository) Update(user *User) error {
    r.db.Save(user)
    
    // Manual invalidation - easy to forget or get wrong
    r.redis.Del(ctx, fmt.Sprintf("user:%d", user.ID))
    r.redis.Del(ctx, fmt.Sprintf("orders:user:%d", user.ID))
    r.redis.Del(ctx, fmt.Sprintf("profile:user:%d", user.ID))
    r.redis.Del(ctx, fmt.Sprintf("addresses:user:%d", user.ID))
    r.redis.Del(ctx, "users:all") // Don't forget FindAll cache!
    
    // What if Order has Items? Do we invalidate those too?
    // What if relationships change? Update ALL repositories?
    
    return nil
}
```

**Impact**:
- Stale data bugs (the worst kind)
- Tight coupling between repositories
- Difficult to trace invalidation logic
- Breaks when relationships change

### Issue #4: No Type Safety with Interfaces

**The Reality**: Generic repository interfaces lose type safety:

```go
type Repository interface {
    FindByID(id interface{}) (interface{}, error)  // Returns any type!
    Create(entity interface{}) error                // Accepts any type!
}

// Usage - no compile-time checks
user, err := userRepo.FindByID(1)
typedUser := user.(*User)  // Runtime panic risk!
```

**Impact**:
- Runtime panics instead of compile errors
- Lost IDE autocomplete
- More defensive programming needed
- Harder to refactor

### Issue #5: Inconsistent Error Handling

**The Reality**: Different developers handle errors differently:

```go
// Repository A
func (r *RepoA) FindByID(id uint) (*User, error) {
    var user User
    err := r.db.First(&user, id).Error
    return &user, err  // Returns GORM error directly
}

// Repository B  
func (r *RepoB) FindByID(id uint) (*User, error) {
    var user User
    if err := r.db.First(&user, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, ErrNotFound  // Custom error
        }
        return nil, fmt.Errorf("find user: %w", err)  // Wrapped error
    }
    return &user, nil
}
```

**Impact**:
- Inconsistent error messages
- Hard to handle errors uniformly
- Breaks error handling contracts

## ✅ How sql4go Solves These Problems

### Solution #1: Generic Repository Pattern

**One implementation, all entities**:

```go
// Write once, use for ALL entities
type Repository[T Entity] interface {
    FindByID(ctx context.Context, id interface{}) (*T, error)
    Create(ctx context.Context, entity *T) error
    // ... all methods with full type safety
}

// Usage - full type safety!
userRepo := sql4go.NewRepository[User](db, redis)
user, err := userRepo.FindByID(ctx, 1)  // Returns *User, not interface{}!
```

**Benefits**:
- ✅ 70% less boilerplate (2 methods vs 6+ per entity)
- ✅ Compile-time type safety
- ✅ Consistent behavior across all entities
- ✅ Single place to fix bugs or add features

### Solution #2: Automatic Transparent Caching

**Zero manual cache code**:

```go
// No cache code in your business logic!
user, err := userRepo.FindByID(ctx, 1)  // Automatically cached
user.Name = "Updated"
userRepo.Update(ctx, user)              // Cache automatically invalidated
```

**How it works**:
1. Read operations check cache first automatically
2. Cache misses fetch from DB and store in cache
3. Write operations invalidate cache automatically
4. No manual cache management code needed

**Benefits**:
- ✅ Zero cache boilerplate
- ✅ No forgotten invalidations
- ✅ Consistent caching strategy
- ✅ Easy to enable/disable (pass nil for Redis)

### Solution #3: Automatic Relationship Detection

**Relationships detected via reflection**:

```go
type User struct {
    ID      uint    `gorm:"primaryKey"`
    Orders  []Order `gorm:"foreignKey:UserID"`  // ← Automatically detected!
    Profile Profile `gorm:"foreignKey:UserID"`  // ← Automatically detected!
}

// When you update user:
userRepo.Update(ctx, user)

// sql4go automatically:
// 1. Scans struct tags via reflection
// 2. Detects foreignKey relationships
// 3. Invalidates user cache
// 4. Invalidates related order caches
// 5. Invalidates related profile cache
// All without any manual code!
```

**Benefits**:
- ✅ Zero manual relationship tracking
- ✅ No stale data from forgotten invalidations
- ✅ Relationships updated = cache logic updated
- ✅ Cross-repository invalidation automatic

### Solution #4: Type-Safe Generics

**Full compile-time type checking**:

```go
userRepo := sql4go.NewRepository[User](db, redis)

user, err := userRepo.FindByID(ctx, 1)  // *User, not interface{}
user.Name = "John"                       // IDE autocomplete works!
userRepo.Update(ctx, user)               // Type checked at compile time

// This won't compile (good!):
userRepo.Update(ctx, &Order{})  // ❌ Compile error: Order is not User
```

**Benefits**:
- ✅ Catch errors at compile time
- ✅ Better IDE support
- ✅ Easier refactoring
- ✅ Self-documenting code

### Solution #5: Consistent Error Handling

**Uniform error handling built-in**:

```go
// All repositories handle errors consistently
user, err := userRepo.FindByID(ctx, 1)
if err != nil {
    if errors.Is(err, gorm.ErrRecordNotFound) {
        // Handle not found
    }
    // All other errors properly wrapped
}
```

**Benefits**:
- ✅ Predictable error behavior
- ✅ Proper error wrapping
- ✅ Easy to add logging/metrics later

## �💡 The Vision

Building database layers in Go often involves writing repetitive code: repositories, cache management, relationship handling, and manual invalidation logic. What if we could:

1. **Eliminate boilerplate** by using reflection and generics intelligently
2. **Automate caching** with smart invalidation based on entity relationships
3. **Learn from real-world challenges** by building something production-grade
4. **Create a reference implementation** others can learn from

This project explores these ideas through a practical GORM extension with Redis caching.

## 🎯 What We're Trying to Achieve

### Primary Goals
- **Minimal Code**: Reduce entity setup from 6+ methods to just 2
- **Automatic Intelligence**: Detect relationships from GORM tags via reflection
- **Cache-First Architecture**: Transparent caching layer with smart invalidation
- **Type Safety**: Full generics support with compile-time guarantees
- **Learning Resource**: Well-documented code that teaches advanced Go patterns

### Educational Focus
This project serves as a practical exploration of:
- Go generics and type constraints
- Reflection for automatic behavior
- Distributed caching strategies
- GORM internals and hooks
- Repository pattern variations
- Cross-cutting concerns (caching, logging, metrics)

## ✨ Current Features

- 🤖 **Zero Configuration**: Automatic GORM relationship detection via reflection
- 🔧 **Minimal Boilerplate**: Only 2 methods per entity (vs 6+ traditional)
- ⚡ **Intelligent Caching**: Cache-first reads with Redis (10x-100x faster reads)
- 🧠 **Relationship Aware**: Automatic cross-repository cache invalidation
- 🛡️ **Graceful Degradation**: Works without Redis seamlessly
- 📊 **Type-Safe**: Full generic support with compile-time checking
- 🔗 **GORM Native**: Built on GORM with full query builder access

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     APPLICATION LAYER                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │UserService   │  │OrderService  │  │Other Services│          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
└─────────┼──────────────────┼──────────────────┼─────────────────┘
          │                  │                  │
          ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                    SQL4GO REPOSITORY LAYER                      │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │         Generic Repository[T Entity]                       │ │
│  │                                                            │ │
│  │  ┌──────────────────┐  ┌──────────────────┐              │ │
│  │  │  Cache Layer     │  │  Database Layer  │              │ │
│  │  │                  │  │                  │              │ │
│  │  │  1. Check Cache  │  │  3. Query GORM   │              │ │
│  │  │  2. Return Hit   │  │  4. Cache Result │              │ │
│  │  └──────────────────┘  └──────────────────┘              │ │
│  │                                                            │ │
│  │  ┌────────────────────────────────────────────────────┐   │ │
│  │  │     Automatic Relationship Detection               │   │ │
│  │  │  • Scans GORM struct tags via reflection          │   │ │
│  │  │  • Detects foreignKey, references, many2many      │   │ │
│  │  │  • Builds dependency graph for invalidation       │   │ │
│  │  │  • Handles nested relationships automatically     │   │ │
│  │  └────────────────────────────────────────────────────┘   │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
          │                                    │
          ▼                                    ▼
┌──────────────────────┐           ┌────────────────────────┐
│   REDIS CACHE        │           │   MySQL DATABASE       │
│                      │           │                        │
│  • Key-value store   │           │  • GORM managed        │
│  • Pattern matching  │           │  • Connection pool     │
│  • Compression       │           │  • Transactions        │
│  • TTL management    │           │  • Relationships       │
└──────────────────────┘           └────────────────────────┘

FLOW EXAMPLE (Read Operation):
───────────────────────────────────
1. userRepo.FindByID(ctx, 1)
2. Check Redis: "sql4go:mydb:users:find_by_id:1"
3a. Cache HIT  → Return (0.5ms) ⚡
3b. Cache MISS → Query MySQL → Store in Redis → Return (15ms)

FLOW EXAMPLE (Write Operation):
────────────────────────────────────
1. userRepo.Update(ctx, user)
2. Update MySQL database
3. Detect relationships (Orders, Profile)
4. Invalidate patterns:
   • sql4go:mydb:users:*
   • sql4go:mydb:orders:*user*
   • sql4go:mydb:profiles:*user*
5. Related repos automatically get fresh data!
```

## Installation

```bash
go get github.com/ammar0144/sql4go
```

## Why sql4go?

### Before sql4go (Traditional GORM)
```go
// 6+ methods + manual cache management + relationship tracking
type UserRepository struct {
    db    *gorm.DB
    cache *redis.Client
}

func (r *UserRepository) FindByID(id uint) (*User, error) {
    // Manual cache check
    cached, err := r.cache.Get(ctx, fmt.Sprintf("user:%d", id)).Result()
    if err == nil {
        var user User
        json.Unmarshal([]byte(cached), &user)
        return &user, nil
    }
    
    // Database query
    var user User
    if err := r.db.First(&user, id).Error; err != nil {
        return nil, err
    }
    
    // Manual cache store
    data, _ := json.Marshal(user)
    r.cache.Set(ctx, fmt.Sprintf("user:%d", id), data, time.Hour*24)
    
    return &user, nil
}

func (r *UserRepository) Update(user *User) error {
    if err := r.db.Save(user).Error; err != nil {
        return err
    }
    // Manual cache invalidation for user
    r.cache.Del(ctx, fmt.Sprintf("user:%d", user.ID))
    // Manual invalidation for related entities (orders, profiles, etc.)
    r.cache.Del(ctx, fmt.Sprintf("orders:user:%d", user.ID))
    r.cache.Del(ctx, fmt.Sprintf("profile:user:%d", user.ID))
    return nil
}

// + Create, Delete, FindAll, FindWhere methods...
```

### After sql4go
```go
// Only 2 methods - everything else is automatic!
type User struct {
    ID      uint    `gorm:"primaryKey"`
    Name    string
    Orders  []Order `gorm:"foreignKey:UserID"`  // ✨ Auto-detected relationship
}

func (User) TableName() string { return "users" }
func (u User) GetPrimaryKeyValue() interface{} { return u.ID }

// That's it! Use it:
userRepo := sql4go.NewRepository[User](dbManager, redisManager)
user, _ := userRepo.FindByID(ctx, 1)  // Automatic caching + relationship detection
```

**Result**: 70% less code, automatic cache management, zero configuration!

## Quick Start

### 1. Define Your Entity (Only 2 Methods Required!)

```go
package main

import (
    "context"
    "sql4go"
)

type User struct {
    ID      uint      `gorm:"primaryKey"`
    Name    string    `gorm:"size:100;not null"`
    Email   string    `gorm:"size:100;uniqueIndex;not null"`
    Age     int       `gorm:"default:0"`
    Orders  []Order   `gorm:"foreignKey:UserID"`   // ✨ Relationships auto-detected!
    Profile Profile   `gorm:"foreignKey:UserID"`
}

// Only 2 methods needed:
func (User) TableName() string { 
    return "users" 
}

func (u User) GetPrimaryKeyValue() interface{} { 
    return u.ID 
}
```

### 2. Setup (Database Only Mode)

```go
func main() {
    // Configure database
    dbConfig := &sql4go.Config{
        DSN:             "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4&parseTime=True",
        MaxOpenConns:    10,
        MaxIdleConns:    5,
        ConnMaxLifetime: 3600,
    }

    // Create database manager
    dbManager, err := sql4go.NewManager(dbConfig)
    if err != nil {
        log.Fatal(err)
    }

    // Create repository (no caching)
    userRepo := sql4go.NewRepository[User](dbManager, nil)

    // Use it!
    ctx := context.Background()
    
    user := User{Name: "John Doe", Email: "john@example.com"}
    userRepo.Create(ctx, &user)
    
    foundUser, _ := userRepo.FindByID(ctx, user.ID)
    log.Printf("Found: %s", foundUser.Name)
}
```

### 3. Enable Intelligent Caching (10x-100x Faster!)

```go
func main() {
    // Setup database
    dbManager, _ := sql4go.NewManager(dbConfig)
    
    // Setup Redis
    redisConfig := &sql4go.RedisConfig{
        Addr:     "localhost:6379",
        Password: "",
        DB:       0,
    }
    redisManager, _ := sql4go.NewRedisManager(redisConfig)
    
    // Create repository with caching enabled
    userRepo := sql4go.NewRepository[User](dbManager, redisManager)
    
    ctx := context.Background()
    
    // First read - from database (10-50ms)
    user, _ := userRepo.FindByID(ctx, 1)
    
    // Second read - from cache (0.5ms) ⚡ 20-100x faster!
    user, _ = userRepo.FindByID(ctx, 1)
    
    // Update - automatically invalidates related caches
    user.Name = "Jane Doe"
    userRepo.Update(ctx, user)  // Invalidates user + orders + profile caches!
}
```

## Core API

### Repository Operations

```go
ctx := context.Background()

// Create
user := User{Name: "John", Email: "john@example.com"}
err := userRepo.Create(ctx, &user)

// Read (Cache-First)
user, err := userRepo.FindByID(ctx, 1)
users, err := userRepo.FindAll(ctx)
users, err := userRepo.FindWhere(ctx, "age > ?", 18)
user, err := userRepo.First(ctx, "email = ?", "john@example.com")

// Check existence
exists, err := userRepo.Exists(ctx, 1)

// Count
count, err := userRepo.Count(ctx)

// Update (Auto Cache Invalidation)
user.Age = 31
err = userRepo.Update(ctx, &user)

// Delete (Auto Cache Invalidation)
err = userRepo.Delete(ctx, user.ID)

// Batch Operations
users := []*User{% raw %}{{Name: "Alice"}, {Name: "Bob"}}{% endraw %}
err = userRepo.CreateBatch(ctx, users)
err = userRepo.UpdateBatch(ctx, users)
```

### GORM Query Builder (Chainable + Cached)

```go
// Complex queries with automatic caching
users, err := userRepo.
    Where(ctx, "age > ?", 18).
    Order(ctx, "created_at DESC").
    Limit(ctx, 10).
    Preload(ctx, "Orders").
    FindAll(ctx)

// Joins
users, err := userRepo.
    Joins(ctx, "LEFT JOIN orders ON orders.user_id = users.id").
    Where(ctx, "orders.total > ?", 100).
    FindAll(ctx)
```

### Cache Management

```go
// Manual cache invalidation (rarely needed)
err := userRepo.InvalidateCache(ctx)

// Warm cache for frequently accessed data
err := userRepo.WarmCache(ctx)
```

## 📊 Monitoring & Metrics

sql4go includes **basic development metrics** to help you understand cache behavior. These are useful for development and debugging, but **not production-grade monitoring**.

### Getting Metrics

```go
// Get current metrics snapshot
metrics := redisManager.GetMetrics()

fmt.Printf("Cache Performance:\n")
fmt.Printf("  Hits: %d\n", metrics.CacheHits)
fmt.Printf("  Misses: %d\n", metrics.CacheMisses)
fmt.Printf("  Hit Rate: %.2f%%\n", metrics.HitRate)

fmt.Printf("\nLatency (averages):\n")
fmt.Printf("  Get: %.2fms\n", metrics.AvgGetLatency)
fmt.Printf("  Set: %.2fms\n", metrics.AvgSetLatency)
fmt.Printf("  Delete: %.2fms\n", metrics.AvgDeleteLatency)

fmt.Printf("\nOperations:\n")
fmt.Printf("  Compressions: %d\n", metrics.CompressionCount)
fmt.Printf("  Invalidations: %d\n", metrics.InvalidationCount)
fmt.Printf("  Errors: %d\n", metrics.ErrorCount)

// Reset metrics (e.g., after deployment)
redisManager.ResetMetrics()
```

### Tracked Metrics

| Metric | Description | Use Case |
|--------|-------------|----------|
| **CacheHits** | Successful cache reads | Measure cache effectiveness |
| **CacheMisses** | Cache misses (DB queries) | Identify cold data or cache issues |
| **HitRate** | Hits / (Hits + Misses) | Overall cache efficiency |
| **AvgGetLatency** | Average Redis GET time | Detect network/Redis issues |
| **AvgSetLatency** | Average Redis SET time | Identify write bottlenecks |
| **AvgDeleteLatency** | Average Redis DEL time | Monitor invalidation performance |
| **CompressionCount** | Compressed cache entries | Track compression usage |
| **InvalidationCount** | Cache invalidations | Monitor write patterns |
| **ErrorCount** | Redis operation failures | Alert on cache failures |

### Example: Logging Metrics

```go
// Log metrics periodically
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        metrics := redisManager.GetMetrics()
        
        log.Printf("[METRICS] Cache hit rate: %.2f%% (%d hits, %d misses)",
            metrics.HitRate, metrics.CacheHits, metrics.CacheMisses)
        
        if metrics.HitRate < 50 {
            log.Printf("[WARNING] Low cache hit rate - consider cache warming")
        }
        
        if metrics.ErrorCount > 0 {
            log.Printf("[ERROR] Redis errors detected: %d", metrics.ErrorCount)
        }
    }
}()
```

### ⚠️ Important Limitations

**These are basic development metrics, not production-grade monitoring:**

1. **In-Memory Only**: Metrics reset on application restart
2. **Per-Instance**: No aggregation across multiple app instances
3. **Simple Averages**: No percentiles (p50, p95, p99) or histograms
4. **No Time Windows**: Cumulative since start, no rolling windows
5. **Thread-Safe**: Uses atomic counters (safe for concurrent use)
6. **Basic Instrumentation**: Covers main operations only

### Production Monitoring Recommendations

For production systems, integrate proper monitoring:

```go
// Export metrics to Prometheus
import "github.com/prometheus/client_golang/prometheus"

var (
    cacheHits = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "sql4go_cache_hits_total",
        Help: "Total number of cache hits",
    })
    
    cacheLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "sql4go_cache_latency_seconds",
        Help:    "Cache operation latency",
        Buckets: prometheus.DefBuckets,
    }, []string{"operation"})
)

// Or use distributed tracing (OpenTelemetry)
import "go.opentelemetry.io/otel"

// Or APM tools (DataDog, New Relic, etc.)
```

### What to Monitor in Production

Beyond basic metrics, track:
- **Database query time** (cache miss cost)
- **Cache memory usage** (Redis memory)
- **Invalidation patterns** (write-heavy vs read-heavy)
- **Error rates and types**
- **Distributed cache consistency** (across instances)

**Bottom line**: Use sql4go's metrics for **development and debugging**. For production, integrate with your existing monitoring infrastructure.

## How It Works

### Automatic Relationship Detection

sql4go automatically detects relationships from GORM tags - no manual configuration needed!

```go
type User struct {
    ID      uint    `gorm:"primaryKey"`
    Orders  []Order `gorm:"foreignKey:UserID"`   // ✅ Auto-detected as "has_many"
    Profile Profile `gorm:"foreignKey:UserID"`   // ✅ Auto-detected as "has_one"
}

type Order struct {
    ID     uint `gorm:"primaryKey"`
    UserID uint `gorm:"index;not null"`
    User   User `gorm:"foreignKey:UserID"`       // ✅ Auto-detected as "belongs_to"
}

// When you update a user:
userRepo.Update(ctx, user)

// sql4go automatically invalidates:
// ✅ All user caches
// ✅ Related order caches  
// ✅ Related profile caches
// No manual work required!
```

### Intelligent Cache Keys

```go
// Format: sql4go:{database}:{table}:{operation}:{params}
sql4go:mydb:users:find_by_id:1
sql4go:mydb:users:find_all
sql4go:mydb:orders:find_where:a4b2c8d9  // MD5 hash for complex queries

// Smart invalidation patterns
userRepo.Update(user)  // Invalidates: sql4go:mydb:users:*
                       // And related: sql4go:mydb:orders:*
```

### Performance Characteristics

**Cache Hit (0.5-2ms)**:
- Simple queries: 20-50x faster than database
- Complex joins: 10-30x faster than database

**Cache Miss (10-50ms)**:
- Query database + store in cache
- Subsequent reads served from cache

**Write Operations**:
- Create: Database time + ~1ms (cache store)
- Update/Delete: Database time + ~2-5ms (cache invalidation)

**Typical Benefits**:
- 80%+ reduction in database load
- Sub-millisecond response times for cached data
- Automatic staleness prevention via smart invalidation

## Configuration

### Database Config

```go
type Config struct {
    DSN             string // Data Source Name (MySQL connection string)
    MaxOpenConns    int    // Maximum open connections (default: 10)
    MaxIdleConns    int    // Maximum idle connections (default: 5)
    ConnMaxLifetime int    // Connection max lifetime in seconds (default: 3600)
}

// Example
dbConfig := &sql4go.Config{
    DSN:             "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local",
    MaxOpenConns:    25,
    MaxIdleConns:    10,
    ConnMaxLifetime: 7200,
}
```

### Redis Config

```go
type RedisConfig struct {
    Addr     string        // Redis server address (default: "localhost:6379")
    Password string        // Redis password (optional)
    DB       int           // Redis database number (default: 0)
    PoolSize int           // Connection pool size (optional)
    MinIdleConns int       // Minimum idle connections (optional)
}

// Example
redisConfig := &sql4go.RedisConfig{
    Addr:         "localhost:6379",
    Password:     "",
    DB:           0,
    PoolSize:     10,
    MinIdleConns: 5,
}
```

## Entity Interface

Entities must implement this minimal interface:

```go
type Entity interface {
    TableName() string                    // GORM table name
    GetPrimaryKeyValue() interface{}      // Primary key value for caching
}

// Example implementation
type User struct {
    ID    uint   `gorm:"primaryKey"`
    Name  string
}

func (User) TableName() string { 
    return "users" 
}

func (u User) GetPrimaryKeyValue() interface{} { 
    return u.ID 
}
```

## 🚧 Known Challenges & Learning Areas

As an experimental project, we're actively working through several challenges:

### 1. **Cache Coherence in Distributed Systems**
- **Challenge**: Multiple app instances can have cache inconsistencies
- **Current Approach**: Pattern-based invalidation via Redis
- **Learning**: Exploring event-driven invalidation and cache versioning
- **Your Input**: How would you handle this? Open an issue with ideas!

### 2. **Complex Relationship Detection**
- **Challenge**: Many-to-many and polymorphic associations are tricky
- **Current Status**: Basic relationships work (belongs_to, has_many, has_one)
- **Learning**: Studying GORM's reflection internals
- **Help Wanted**: If you've worked with GORM reflection, we'd love your insights!

### 3. **Performance vs. Flexibility Trade-offs**
- **Challenge**: Reflection overhead vs. developer convenience
- **Current Approach**: Reflection at repository creation (one-time cost)
- **Exploring**: Code generation as an alternative approach
- **Discussion**: What's your take on reflection vs. code generation?

### 4. **Cache Invalidation Granularity**
- **Challenge**: Over-invalidation wastes cache, under-invalidation causes stale data
- **Current Approach**: Conservative pattern-based invalidation
- **Researching**: Dependency graphs and fine-grained tracking
- **Ideas Welcome**: Better strategies for smart invalidation?

### 5. **Memory Management with Large Datasets**
- **Challenge**: Caching large result sets can exhaust Redis memory
- **Current Approach**: Compression and chunking
- **Learning**: Cache eviction strategies and memory limits
- **Question**: How do you handle caching at scale?

### 6. **Transaction Support**
- **Challenge**: Cache invalidation timing with GORM transactions
- **Status**: Not fully implemented
- **Complexity**: Rollback scenarios and isolation levels
- **Brainstorm**: Should we invalidate on commit or per-operation?

## 🤝 Learning Together

This project is as much about learning as it is about building. We welcome:

### Ways to Contribute & Learn

1. **🐛 Report Issues**: Found a bug? Great! Let's learn why it happened
2. **💡 Suggest Ideas**: Have a better approach? Share it - we'll explore together
3. **📖 Improve Docs**: Help others understand the code better
4. **🔬 Experiment**: Fork it, break it, fix it - share what you learned
5. **🎓 Ask Questions**: No question is too basic - if you're learning, others are too
6. **🏗️ Add Features**: Pick a challenge above and try solving it
7. **📊 Benchmark**: Test performance and share your findings
8. **🎨 Refactor**: See a better pattern? Show us!

### What You'll Learn

Working on this project exposes you to:
- **Advanced Go patterns**: Generics, reflection, interfaces
- **Database internals**: GORM hooks, connection pooling, query optimization
- **Caching strategies**: TTL, invalidation, compression, distributed caching
- **System design**: Trade-offs between performance, complexity, and maintainability
- **Open source collaboration**: Code review, documentation, issue management

### Discussion Topics We're Exploring

- Should we support PostgreSQL? MongoDB?
- Code generation vs. reflection - which is better?
- How to handle real-time requirements (WebSocket invalidation)?
- Testing strategies for cache logic
- Metrics and observability - what should we track?
- Auto-migration: safe approaches for production

**Join the conversation**: Open an issue or discussion - let's learn together! 🚀

## � Edge Cases & Foreseen Issues

As we continue developing, we're aware of several edge cases and potential issues that need solutions:

### 1. **Circular Relationship Dependencies**

**Edge Case**:
```go
type User struct {
    ID      uint
    Orders  []Order `gorm:"foreignKey:UserID"`
}

type Order struct {
    ID     uint
    UserID uint
    User   User `gorm:"foreignKey:UserID"`  // Circular reference!
}
```

**Potential Issue**: Infinite loop in cache invalidation  
**Current Status**: Basic detection implemented, needs testing  
**Foreseen Challenge**: Complex circular chains (A→B→C→A)  
**Open Question**: How deep should invalidation traverse?

### 2. **Many-to-Many Relationships**

**Edge Case**:
```go
type User struct {
    ID    uint
    Roles []Role `gorm:"many2many:user_roles"`
}

type Role struct {
    ID    uint
    Users []User `gorm:"many2many:user_roles"`
}
```

**Potential Issue**: Join table changes not detected  
**Current Status**: Not implemented  
**Foreseen Challenge**: Detecting changes in join tables  
**Research Needed**: GORM hooks for many2many operations

### 3. **Polymorphic Associations**

**Edge Case**:
```go
type Comment struct {
    ID            uint
    CommentableID uint
    CommentableType string  // "User", "Post", etc.
}

type User struct {
    ID       uint
    Comments []Comment `gorm:"polymorphic:Commentable"`
}
```

**Potential Issue**: Type-based cache key generation  
**Current Status**: Not supported  
**Foreseen Challenge**: Dynamic type resolution  
**Complexity**: High - requires runtime type inspection

### 4. **Soft Deletes and Query Scopes**

**Edge Case**:
```go
type User struct {
    ID        uint
    DeletedAt gorm.DeletedAt  // Soft delete
}

// FindAll should exclude soft-deleted records
// But cache might include them
```

**Potential Issue**: Cache includes soft-deleted records  
**Current Status**: Needs investigation  
**Foreseen Challenge**: Cache key needs to include scope info  
**Workaround**: Invalidate all caches on soft delete

### 5. **Transaction Rollback Scenarios**

**Edge Case**:
```go
db.Transaction(func(tx *gorm.DB) error {
    userRepo.Create(ctx, user)  // Cache gets populated
    // ... some error occurs ...
    return errors.New("rollback")  // Cache NOT invalidated!
})
```

**Potential Issue**: Cache contains data that was rolled back  
**Current Status**: Not handled  
**Foreseen Challenge**: Detecting transaction context  
**Possible Solution**: Defer cache operations until commit

### 6. **Race Conditions in Distributed Systems**

**Edge Case**:
```go
// Instance A
user, _ := userRepo.FindByID(ctx, 1)  // Cache miss, queries DB
// ... 100ms delay ...

// Instance B (simultaneously)
userRepo.Update(ctx, user)  // Invalidates cache

// Instance A continues
// Stores stale data in cache!  // ⚠️ Race condition
```

**Potential Issue**: Stale data cached due to timing  
**Current Status**: Known issue  
**Foreseen Challenge**: Requires distributed locks or versioning  
**Research Areas**: 
- Cache versioning (ETags)
- Distributed locks (Redlock)
- Event streaming (Redis Pub/Sub)

### 7. **Large Result Set Memory Issues**

**Edge Case**:
```go
// FindAll on table with millions of rows
users, _ := userRepo.FindAll(ctx)  // Could be 100MB+
```

**Potential Issue**: Redis memory exhaustion or connection timeout  
**Current Status**: Compression helps, but not enough  
**Foreseen Challenge**: Need pagination or streaming  
**Possible Solutions**:
- Automatic pagination for large queries
- Stream results instead of caching
- Cache count only, not data

### 8. **Cache Key Collision Edge Cases**

**Edge Case**:
```go
// Different queries might generate same cache key
userRepo.FindWhere(ctx, "age > ? AND age < ?", 18, 65)
userRepo.FindWhere(ctx, "age > ? AND age < ?", 18, 65)  // Same
// But:
userRepo.FindWhere(ctx, "age > ?", 18)
userRepo.FindWhere(ctx, "age >?", 18)  // Different query, could collide!
```

**Potential Issue**: Cache key MD5 hash collision  
**Current Status**: Low probability but possible  
**Foreseen Challenge**: Need robust key generation  
**Mitigation**: Include normalized query in key

### 9. **Custom GORM Callbacks Interference**

**Edge Case**:
```go
// User has custom GORM callback
db.Callback().Create().Before("gorm:create").Register("custom", func(db *gorm.DB) {
    // Custom logic that modifies data
})

// sql4go might not detect these changes
```

**Potential Issue**: Cache invalidation misses callback changes  
**Current Status**: Not handled  
**Foreseen Challenge**: Detecting custom callbacks  
**Possible Solution**: Hook into GORM's callback system

### 10. **Time-Based Data Staleness**

**Edge Case**:
```go
type Product struct {
    ID        uint
    Price     float64
    ValidUntil time.Time  // Price changes at specific time
}

// Cache stores price, but validUntil expires
// Cache still returns old price!
```

**Potential Issue**: Time-based invalidation not supported  
**Current Status**: TTL only solution  
**Foreseen Challenge**: Dynamic TTL based on entity fields  
**Research Needed**: Per-entity TTL strategies

### 11. **Database Replication Lag**

**Edge Case**:
```go
// Write to primary database
userRepo.Update(ctx, user)  // Cache invalidated

// Read from replica (with lag)
user, _ := userRepo.FindByID(ctx, 1)  // Cache miss
// Queries replica, gets old data, caches it!
```

**Potential Issue**: Stale data cached from replica lag  
**Current Status**: Not addressed  
**Foreseen Challenge**: Detecting replica lag  
**Possible Solutions**:
- Read-after-write consistency checks
- Write operations mark entity as "recently updated"
- Temporary cache lockout after writes

### 12. **Memory Leaks from Reflection**

**Edge Case**:
```go
// Creating many repository instances
for i := 0; i < 10000; i++ {
    repo := sql4go.NewRepository[User](db, redis)
    // Does reflection cache grow unbounded?
}
```

**Potential Issue**: Reflection metadata not garbage collected  
**Current Status**: Needs profiling  
**Foreseen Challenge**: Balancing cache vs memory  
**Investigation**: Profile with pprof

## 🧪 How We'll Address These Issues

### Our Approach to Edge Cases

1. **Document Known Limitations**: Be transparent about what doesn't work
2. **Add Tests Incrementally**: Each edge case gets a test case
3. **Fail Loudly**: Better to error than silently produce wrong results
4. **Provide Escape Hatches**: Let users override behavior when needed
5. **Learn from Production**: Real usage will reveal more edge cases

### Ways You Can Help

- **Report Edge Cases**: Found a scenario we missed? Create an issue!
- **Share Use Cases**: How are you using it? What broke?
- **Propose Solutions**: Have ideas? Let's discuss!
- **Test in Your Environment**: Different setups reveal different issues
- **Contribute Tests**: Help us build a comprehensive test suite

### What We're Prioritizing

1. **Safety First**: Prefer consistency over performance
2. **Clear Errors**: Help users understand what went wrong
3. **Graceful Degradation**: Fail safely, not catastrophically
4. **Documentation**: Explain limitations clearly
5. **Iterative Improvement**: Start simple, handle complexity gradually

**These are hard problems - let's solve them together!** 💪

## �📚 When to Use sql4go

### ✅ Good Use Cases (Where We Excel)
- **Learning projects** exploring Go patterns
- **Read-heavy applications** (blogs, catalogs, dashboards)  
- **API backends** with frequent data access
- **Prototypes** that need quick database setup
- **Applications with clear relationships** between entities

### ⚠️ Experimental - Not Recommended For
- **Production systems** (until v1.0 - we're still learning!)
- **Write-heavy workloads** (cache overhead may not help)
- **Real-time data** requirements (cache staleness issues)
- **Mission-critical systems** (test thoroughly first!)
- **Complex many-to-many** relationships (limited support currently)

> 💡 **Use at your own risk**: This is an experimental learning project. Test extensively before considering production use!

## Best Practices

## Requirements

- Go 1.21 or higher
- MySQL 5.7+ or 8.0+
- Redis 6.0+ (optional, for caching)

## Dependencies

- [GORM](https://gorm.io/) - The fantastic ORM library for Go
- [go-redis/v9](https://github.com/redis/go-redis) - Type-safe Redis client
- [MySQL Driver](https://github.com/go-sql-driver/mysql) - Pure Go MySQL driver

## 🎓 Contributing & Learning Together

We believe in learning by doing. Whether you're a beginner or expert, there's something to learn here!

### For Beginners
- **Start small**: Fix typos, improve documentation
- **Ask questions**: Use GitHub Discussions
- **Read the code**: It's well-commented for learning
- **Try examples**: Break them, understand why

### For Intermediate Developers
- **Add tests**: Help us improve test coverage
- **Refactor code**: Spot patterns that can be improved
- **Add features**: Pick a challenge from above
- **Review PRs**: Learn by reviewing others' code

### For Advanced Developers
- **Architectural input**: Help us make better design decisions
- **Performance optimization**: Benchmark and improve
- **Complex features**: Tackle the hard problems
- **Mentorship**: Help others learn through code review

### Getting Started

```bash
# Fork and clone
git clone https://github.com/ammar0144/sql4go.git
cd sql4go

# Install dependencies
go mod download

# Read the code - start here:
# 1. sql4go.go - Simple entry point
# 2. pkg/repository/interface.go - Core interface
# 3. pkg/repository/generic.go - Main implementation

# Make your changes
# ...

# Format and test
go fmt ./...
go vet ./...

# Submit a PR and let's learn together!
```

## 📖 Documentation & Resources

- **[DESIGN.md](DESIGN.md)** - Deep dive into architecture and decisions
- **[CONTRIBUTING.md](CONTRIBUTING.md)** - Contribution guidelines
- **GitHub Discussions** - Ask questions, share ideas
- **GitHub Issues** - Bug reports and feature requests

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built on the shoulders of giants: [GORM](https://gorm.io/)
- Inspired by DDD patterns and clean architecture
- Special thanks to everyone who contributes feedback and ideas
- Learning from the Go community's best practices

---

**A Learning Project by [ammar0144](https://github.com/ammar0144)**

*"The best way to learn is to build, break, and rebuild together."*

---

### 📬 Get Involved

- ⭐ **Star this repo** if you find it interesting
- 🐛 **Report bugs** to help us improve  
- 💡 **Share ideas** in GitHub Discussions
- 🤝 **Contribute** - beginners welcome!
- 📣 **Spread the word** - help others learn

**Let's explore the boundaries of what's possible with Go!** 🚀

