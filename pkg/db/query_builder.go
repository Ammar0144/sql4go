package db

import (
	"fmt"
	"reflect"
	"strings"
)

// Production SQL Query Builder
// This package provides utilities for building complex SQL queries with full feature support.
// For cache-aware operations, use the smart query builder in the repository layer.
//
// SECURITY WARNING:
// This builder does NOT escape or validate table names, column names, or other SQL identifiers.
// All identifiers (table names, column names, etc.) MUST be validated before passing to this builder.
// DO NOT pass user input directly to any identifier parameters.
// User input should ONLY be passed as values through the Value field of Condition structs,
// which are properly parameterized in the final query.
//
// Safe Usage:
//   - Table and column names should be hardcoded or from trusted sources
//   - Use parameterized values for all user input (via Condition.Value)
//   - Validate/whitelist any dynamic identifier names before use
//
// Example - SAFE:
//   builder.NewBuilder("users").Select("id", "name").Where("email", Equal, userEmail)
//
// Example - UNSAFE (DO NOT DO THIS):
//   builder.NewBuilder(userInput).Select(userProvidedColumn)  // SQL INJECTION RISK!

// Operator represents SQL comparison operators
type Operator string

const (
	Equal              Operator = "="
	NotEqual           Operator = "!="
	GreaterThan        Operator = ">"
	GreaterThanOrEqual Operator = ">="
	LessThan           Operator = "<"
	LessThanOrEqual    Operator = "<="
	Like               Operator = "LIKE"
	NotLike            Operator = "NOT LIKE"
	In                 Operator = "IN"
	NotIn              Operator = "NOT IN"
	IsNull             Operator = "IS NULL"
	IsNotNull          Operator = "IS NOT NULL"
	Between            Operator = "BETWEEN"
	NotBetween         Operator = "NOT BETWEEN"
)

// JoinType represents SQL JOIN types
type JoinType string

const (
	InnerJoin JoinType = "INNER JOIN"
	LeftJoin  JoinType = "LEFT JOIN"
	RightJoin JoinType = "RIGHT JOIN"
	FullJoin  JoinType = "FULL OUTER JOIN"
	CrossJoin JoinType = "CROSS JOIN"
)

// LogicalOperator for combining conditions
type LogicalOperator string

const (
	And LogicalOperator = "AND"
	Or  LogicalOperator = "OR"
)

// Condition represents a WHERE/HAVING clause condition
type Condition struct {
	Field    string
	Operator Operator
	Value    interface{}
}

// ConditionGroup represents grouped conditions with logical operators
type ConditionGroup struct {
	Conditions []interface{} // Can be Condition or nested ConditionGroup
	Operator   LogicalOperator
}

// JoinClause represents a JOIN operation
type JoinClause struct {
	Type      JoinType
	Table     string
	Condition string
}

// Builder helps build complex SQL queries
type Builder struct {
	table      string
	selectCols []string
	distinct   bool
	joins      []JoinClause
	where      *ConditionGroup
	groupBy    []string
	having     *ConditionGroup
	orderBy    []string
	limit      int
	offset     int
	subqueries map[string]*Builder // Named subqueries
}

// NewBuilder creates a new query builder
// SECURITY: The table parameter must be a validated, trusted identifier.
// Do NOT pass user input directly - validate/whitelist table names first.
func NewBuilder(table string) *Builder {
	return &Builder{
		table:      table,
		selectCols: []string{"*"},
		distinct:   false,
		joins:      []JoinClause{},
		where:      &ConditionGroup{Operator: And},
		groupBy:    []string{},
		having:     &ConditionGroup{Operator: And},
		orderBy:    []string{},
		subqueries: make(map[string]*Builder),
	}
}

// Select sets the columns to select
// SECURITY: Column names are NOT escaped. Only pass validated, trusted identifiers.
// User input should NOT be passed to this method.
func (b *Builder) Select(cols ...string) *Builder {
	b.selectCols = cols
	return b
}

// Distinct enables DISTINCT selection
func (b *Builder) Distinct() *Builder {
	b.distinct = true
	return b
}

// Where adds a WHERE condition
// SECURITY: Field name is NOT escaped - must be a validated identifier.
// User input should be passed via the 'value' parameter, which is properly parameterized.
func (b *Builder) Where(field string, operator Operator, value interface{}) *Builder {
	b.where.Conditions = append(b.where.Conditions, Condition{
		Field:    field,
		Operator: operator,
		Value:    value,
	})
	return b
}

// WhereGroup adds a grouped WHERE condition
func (b *Builder) WhereGroup(operator LogicalOperator, fn func(*ConditionGroup)) *Builder {
	group := &ConditionGroup{Operator: operator}
	fn(group)
	b.where.Conditions = append(b.where.Conditions, group)
	return b
}

// OrWhere adds an OR WHERE condition
// For predictable behavior, this wraps the existing conditions in an OR group
func (b *Builder) OrWhere(field string, operator Operator, value interface{}) *Builder {
	// If no existing conditions, treat as regular Where
	if len(b.where.Conditions) == 0 {
		return b.Where(field, operator, value)
	}

	// Create new OR condition
	newCondition := Condition{
		Field:    field,
		Operator: operator,
		Value:    value,
	}

	// If the root is already an OR group, just append
	if b.where.Operator == Or {
		b.where.Conditions = append(b.where.Conditions, newCondition)
		return b
	}

	// Otherwise, wrap existing AND conditions in a group and create OR root
	// This preserves the AND semantics of existing conditions
	existingGroup := &ConditionGroup{
		Conditions: b.where.Conditions,
		Operator:   And,
	}

	b.where = &ConditionGroup{
		Conditions: []interface{}{existingGroup, newCondition},
		Operator:   Or,
	}

	return b
}

// Join adds a JOIN clause
func (b *Builder) Join(joinType JoinType, table, condition string) *Builder {
	b.joins = append(b.joins, JoinClause{
		Type:      joinType,
		Table:     table,
		Condition: condition,
	})
	return b
}

// InnerJoin adds an INNER JOIN
func (b *Builder) InnerJoin(table, condition string) *Builder {
	return b.Join(InnerJoin, table, condition)
}

// LeftJoin adds a LEFT JOIN
func (b *Builder) LeftJoin(table, condition string) *Builder {
	return b.Join(LeftJoin, table, condition)
}

// RightJoin adds a RIGHT JOIN
func (b *Builder) RightJoin(table, condition string) *Builder {
	return b.Join(RightJoin, table, condition)
}

// GroupBy adds GROUP BY columns
func (b *Builder) GroupBy(columns ...string) *Builder {
	b.groupBy = append(b.groupBy, columns...)
	return b
}

// Having adds a HAVING condition
func (b *Builder) Having(field string, operator Operator, value interface{}) *Builder {
	b.having.Conditions = append(b.having.Conditions, Condition{
		Field:    field,
		Operator: operator,
		Value:    value,
	})
	return b
}

// OrderBy adds an ORDER BY clause
func (b *Builder) OrderBy(field string, desc bool) *Builder {
	order := field
	if desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	b.orderBy = append(b.orderBy, order)
	return b
}

// Limit sets the LIMIT clause
// Negative values are normalized to 0
func (b *Builder) Limit(limit int) *Builder {
	if limit < 0 {
		limit = 0
	}
	b.limit = limit
	return b
}

// Offset sets the OFFSET clause
// Negative values are normalized to 0
func (b *Builder) Offset(offset int) *Builder {
	if offset < 0 {
		offset = 0
	}
	b.offset = offset
	return b
}

// AddSubquery adds a named subquery
func (b *Builder) AddSubquery(name string, subquery *Builder) *Builder {
	b.subqueries[name] = subquery
	return b
}

// Helper method to add conditions to a condition group
func (g *ConditionGroup) Where(field string, operator Operator, value interface{}) *ConditionGroup {
	g.Conditions = append(g.Conditions, Condition{
		Field:    field,
		Operator: operator,
		Value:    value,
	})
	return g
}

// Helper method to add nested condition groups
func (g *ConditionGroup) Group(operator LogicalOperator, fn func(*ConditionGroup)) *ConditionGroup {
	group := &ConditionGroup{Operator: operator}
	fn(group)
	g.Conditions = append(g.Conditions, group)
	return g
}

// BuildSelect builds a SELECT query
func (b *Builder) BuildSelect() (string, []interface{}) {
	var query strings.Builder
	var args []interface{}

	// SELECT clause
	query.WriteString("SELECT ")
	if b.distinct {
		query.WriteString("DISTINCT ")
	}
	query.WriteString(strings.Join(b.selectCols, ", "))
	query.WriteString(" FROM ")
	query.WriteString(b.table)

	// JOIN clauses
	if len(b.joins) > 0 {
		for _, join := range b.joins {
			query.WriteString(" ")
			query.WriteString(string(join.Type))
			query.WriteString(" ")
			query.WriteString(join.Table)
			query.WriteString(" ON ")
			query.WriteString(join.Condition)
		}
	}

	// WHERE clause
	if len(b.where.Conditions) > 0 {
		query.WriteString(" WHERE ")
		whereSQL, whereArgs := b.buildConditionGroup(b.where)
		query.WriteString(whereSQL)
		args = append(args, whereArgs...)
	}

	// GROUP BY clause
	if len(b.groupBy) > 0 {
		query.WriteString(" GROUP BY ")
		query.WriteString(strings.Join(b.groupBy, ", "))
	}

	// HAVING clause
	if len(b.having.Conditions) > 0 {
		query.WriteString(" HAVING ")
		havingSQL, havingArgs := b.buildConditionGroup(b.having)
		query.WriteString(havingSQL)
		args = append(args, havingArgs...)
	}

	// ORDER BY clause
	if len(b.orderBy) > 0 {
		query.WriteString(" ORDER BY ")
		query.WriteString(strings.Join(b.orderBy, ", "))
	}

	// LIMIT clause
	if b.limit > 0 {
		query.WriteString(fmt.Sprintf(" LIMIT %d", b.limit))
	}

	// OFFSET clause
	if b.offset > 0 {
		query.WriteString(fmt.Sprintf(" OFFSET %d", b.offset))
	}

	return query.String(), args
}

// buildConditionGroup builds SQL for a condition group with proper logical operators
func (b *Builder) buildConditionGroup(group *ConditionGroup) (string, []interface{}) {
	if len(group.Conditions) == 0 {
		return "", nil
	}

	var conditions []string
	var args []interface{}

	for _, item := range group.Conditions {
		switch cond := item.(type) {
		case Condition:
			condSQL, condArgs := b.buildCondition(cond)
			conditions = append(conditions, condSQL)
			args = append(args, condArgs...)
		case *ConditionGroup:
			if len(cond.Conditions) > 0 {
				groupSQL, groupArgs := b.buildConditionGroup(cond)
				conditions = append(conditions, "("+groupSQL+")")
				args = append(args, groupArgs...)
			}
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}

	operator := " " + string(group.Operator) + " "
	return strings.Join(conditions, operator), args
}

// buildCondition builds SQL for a single condition
func (b *Builder) buildCondition(cond Condition) (string, []interface{}) {
	switch cond.Operator {
	case IsNull, IsNotNull:
		return fmt.Sprintf("%s %s", cond.Field, cond.Operator), nil
	case In, NotIn:
		return b.buildInCondition(cond)
	case Between, NotBetween:
		return b.buildBetweenCondition(cond)
	default:
		return fmt.Sprintf("%s %s ?", cond.Field, cond.Operator), []interface{}{cond.Value}
	}
}

// buildInCondition builds IN/NOT IN conditions with proper placeholder expansion
func (b *Builder) buildInCondition(cond Condition) (string, []interface{}) {
	if cond.Value == nil {
		if cond.Operator == In {
			return "1 = 0", nil
		}
		return "1 = 1", nil
	}

	v := reflect.ValueOf(cond.Value)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		// Single value, treat as regular condition
		return fmt.Sprintf("%s %s (?)", cond.Field, cond.Operator), []interface{}{cond.Value}
	}

	length := v.Len()
	if length == 0 {
		// Empty slice - return condition that never matches
		if cond.Operator == In {
			return "1 = 0", nil
		}
		return "1 = 1", nil
	}

	placeholders := make([]string, length)
	args := make([]interface{}, length)
	for i := 0; i < length; i++ {
		placeholders[i] = "?"
		args[i] = v.Index(i).Interface()
	}

	sql := fmt.Sprintf("%s %s (%s)", cond.Field, cond.Operator, strings.Join(placeholders, ", "))
	return sql, args
}

// buildBetweenCondition builds BETWEEN/NOT BETWEEN conditions
func (b *Builder) buildBetweenCondition(cond Condition) (string, []interface{}) {
	// Expect value to be a slice/array with exactly 2 elements
	if cond.Value == nil {
		return "1 = 0", nil
	}

	v := reflect.ValueOf(cond.Value)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		// Return error condition - non-slice values are invalid for BETWEEN
		return "1 = 0", nil // Invalid condition that never matches
	}

	if v.Len() != 2 {
		// Return error condition - BETWEEN requires exactly 2 values
		return "1 = 0", nil // Invalid condition that never matches
	}

	sql := fmt.Sprintf("%s %s ? AND ?", cond.Field, cond.Operator)
	args := []interface{}{v.Index(0).Interface(), v.Index(1).Interface()}
	return sql, args
}

// BuildInsert builds an INSERT query
func (b *Builder) BuildInsert(columns []string) (string, int) {
	var query strings.Builder
	query.WriteString("INSERT INTO ")
	query.WriteString(b.table)
	query.WriteString(" (")
	query.WriteString(strings.Join(columns, ", "))
	query.WriteString(") VALUES (")

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	query.WriteString(strings.Join(placeholders, ", "))
	query.WriteString(")")

	return query.String(), len(columns)
}

// BuildUpdate builds an UPDATE query
func (b *Builder) BuildUpdate(columns []string, whereField string) (string, int) {
	var query strings.Builder
	query.WriteString("UPDATE ")
	query.WriteString(b.table)
	query.WriteString(" SET ")

	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = col + " = ?"
	}
	query.WriteString(strings.Join(setClauses, ", "))

	if whereField != "" {
		query.WriteString(" WHERE ")
		query.WriteString(whereField)
		query.WriteString(" = ?")
		return query.String(), len(columns) + 1
	}

	return query.String(), len(columns)
}

// BuildDelete builds a DELETE query
func (b *Builder) BuildDelete(whereField string) string {
	query := fmt.Sprintf("DELETE FROM %s", b.table)
	if whereField != "" {
		query += fmt.Sprintf(" WHERE %s = ?", whereField)
	}
	return query
}
