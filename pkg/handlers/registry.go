package handlers

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Registry maps GVKs to resource handlers
type Registry struct {
	handlers map[schema.GroupVersionKind]ResourceHandler
	mutex    sync.RWMutex
}

// NewRegistry creates a new handler registry
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[schema.GroupVersionKind]ResourceHandler),
	}
}

// Register registers a handler for a specific GVK
func (r *Registry) Register(gvk schema.GroupVersionKind, handler ResourceHandler) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.handlers[gvk] = handler
}

// Get returns the handler for a specific GVK
func (r *Registry) Get(gvk schema.GroupVersionKind) (ResourceHandler, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	handler, exists := r.handlers[gvk]
	if !exists {
		return nil, fmt.Errorf("no handler registered for GVK %v", gvk)
	}

	return handler, nil
}

// Has checks if a handler is registered for a GVK
func (r *Registry) Has(gvk schema.GroupVersionKind) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	_, exists := r.handlers[gvk]
	return exists
}

// List returns all registered handlers
func (r *Registry) List() map[schema.GroupVersionKind]ResourceHandler {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[schema.GroupVersionKind]ResourceHandler)
	for gvk, handler := range r.handlers {
		result[gvk] = handler
	}

	return result
}

// Global registry instance
var globalRegistry = NewRegistry()

// Register registers a handler in the global registry
func Register(gvk schema.GroupVersionKind, handler ResourceHandler) {
	globalRegistry.Register(gvk, handler)
}

// Get returns a handler from the global registry
func Get(gvk schema.GroupVersionKind) (ResourceHandler, error) {
	return globalRegistry.Get(gvk)
}

// Has checks if a handler is registered in the global registry
func Has(gvk schema.GroupVersionKind) bool {
	return globalRegistry.Has(gvk)
}

// List returns all handlers from the global registry
func List() map[schema.GroupVersionKind]ResourceHandler {
	return globalRegistry.List()
}
