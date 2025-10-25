package repository

// Entity interface defines the minimal contract for repository entities
// GORM models should implement this for optimal caching and relationship detection
// If not implemented, the repository will use reflection as fallback
type Entity interface {
	// TableName returns the database table name for this entity
	// This should match GORM's table naming convention
	// If not implemented, GORM's default naming will be used
	TableName() string

	// GetPrimaryKeyValue returns the actual value of the primary key
	// Used for cache invalidation and dependency tracking
	// If not implemented, reflection will be used to find 'ID' field
	GetPrimaryKeyValue() interface{}
}

// RelationshipAware allows entities to define their relationships for cache invalidation
// When implemented, the repository can automatically invalidate related caches
type RelationshipAware interface {
	Entity

	// GetRelationships returns a map of relationship types to related entity info
	// Format: map[relationType][]RelatedEntity
	// Example: {"belongs_to": [{"customer", customerID}], "has_many": [{"orders", nil}]}
	GetRelationships() map[string][]RelatedEntity
}

// RelatedEntity represents a relationship to another entity
type RelatedEntity struct {
	EntityType string      // The related entity type (table name)
	EntityID   interface{} // The related entity ID (nil for has_many without specific ID)
}
