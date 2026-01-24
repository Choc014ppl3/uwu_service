package repository

import (
	"context"
	"time"
)

// Entity is a base interface for all entities.
type Entity interface {
	GetID() string
}

// Repository is a generic repository interface.
type Repository[T Entity] interface {
	GetByID(ctx context.Context, id string) (T, error)
	GetAll(ctx context.Context) ([]T, error)
	Create(ctx context.Context, entity T) error
	Update(ctx context.Context, entity T) error
	Delete(ctx context.Context, id string) error
}

// BaseEntity provides common fields for entities.
type BaseEntity struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetID returns the entity ID.
func (e *BaseEntity) GetID() string {
	return e.ID
}

// InMemoryRepository is a simple in-memory repository implementation.
type InMemoryRepository[T Entity] struct {
	data map[string]T
}

// NewInMemoryRepository creates a new in-memory repository.
func NewInMemoryRepository[T Entity]() *InMemoryRepository[T] {
	return &InMemoryRepository[T]{
		data: make(map[string]T),
	}
}

// GetByID retrieves an entity by ID.
func (r *InMemoryRepository[T]) GetByID(ctx context.Context, id string) (T, error) {
	var zero T
	if entity, ok := r.data[id]; ok {
		return entity, nil
	}
	return zero, ErrNotFound
}

// GetAll retrieves all entities.
func (r *InMemoryRepository[T]) GetAll(ctx context.Context) ([]T, error) {
	entities := make([]T, 0, len(r.data))
	for _, entity := range r.data {
		entities = append(entities, entity)
	}
	return entities, nil
}

// Create creates a new entity.
func (r *InMemoryRepository[T]) Create(ctx context.Context, entity T) error {
	if _, ok := r.data[entity.GetID()]; ok {
		return ErrAlreadyExists
	}
	r.data[entity.GetID()] = entity
	return nil
}

// Update updates an existing entity.
func (r *InMemoryRepository[T]) Update(ctx context.Context, entity T) error {
	if _, ok := r.data[entity.GetID()]; !ok {
		return ErrNotFound
	}
	r.data[entity.GetID()] = entity
	return nil
}

// Delete deletes an entity by ID.
func (r *InMemoryRepository[T]) Delete(ctx context.Context, id string) error {
	if _, ok := r.data[id]; !ok {
		return ErrNotFound
	}
	delete(r.data, id)
	return nil
}

// Common repository errors
var (
	ErrNotFound      = &RepositoryError{Code: "NOT_FOUND", Message: "entity not found"}
	ErrAlreadyExists = &RepositoryError{Code: "ALREADY_EXISTS", Message: "entity already exists"}
)

// RepositoryError represents a repository error.
type RepositoryError struct {
	Code    string
	Message string
}

func (e *RepositoryError) Error() string {
	return e.Code + ": " + e.Message
}
