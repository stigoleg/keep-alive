package keepalive

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// CleanupManager manages cleanup operations with timeout and error tracking
type CleanupManager struct {
	mu          sync.Mutex
	resources   []CleanupResource
	timeout     time.Duration
	cleanupOnce sync.Once
}

// CleanupResource represents a resource that needs cleanup
type CleanupResource interface {
	Cleanup() error
	Name() string
}

// CleanupFunc is a function-based cleanup resource
type CleanupFunc struct {
	name string
	fn   func() error
}

func (c *CleanupFunc) Cleanup() error {
	return c.fn()
}

func (c *CleanupFunc) Name() string {
	return c.name
}

// NewCleanupManager creates a new cleanup manager with the specified timeout
func NewCleanupManager(timeout time.Duration) *CleanupManager {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &CleanupManager{
		resources: make([]CleanupResource, 0),
		timeout:   timeout,
	}
}

// Register adds a resource to be cleaned up
func (cm *CleanupManager) Register(resource CleanupResource) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.resources = append(cm.resources, resource)
}

// RegisterFunc registers a cleanup function
func (cm *CleanupManager) RegisterFunc(name string, fn func() error) {
	cm.Register(&CleanupFunc{name: name, fn: fn})
}

// Execute performs cleanup of all registered resources with timeout
func (cm *CleanupManager) Execute() []error {
	var errors []error
	cm.cleanupOnce.Do(func() {
		errors = cm.executeWithTimeout()
	})
	return errors
}

func (cm *CleanupManager) executeWithTimeout() []error {
	cm.mu.Lock()
	resources := make([]CleanupResource, len(cm.resources))
	copy(resources, cm.resources)
	cm.mu.Unlock()

	if len(resources) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), cm.timeout)
	defer cancel()

	done := make(chan struct{})
	var cleanupErrors []error
	var mu sync.Mutex

	go func() {
		defer close(done)
		for _, resource := range resources {
			func() {
				defer func() {
					if r := recover(); r != nil {
						mu.Lock()
						cleanupErrors = append(cleanupErrors, errors.New("panic during cleanup"))
						mu.Unlock()
						log.Printf("cleanup: panic cleaning up %s: %v", resource.Name(), r)
					}
				}()

				if err := resource.Cleanup(); err != nil {
					mu.Lock()
					cleanupErrors = append(cleanupErrors, err)
					mu.Unlock()
					log.Printf("cleanup: error cleaning up %s: %v", resource.Name(), err)
				} else {
					log.Printf("cleanup: successfully cleaned up %s", resource.Name())
				}
			}()
		}
	}()

	select {
	case <-done:
		return cleanupErrors
	case <-ctx.Done():
		log.Printf("cleanup: timeout after %v, some resources may not have been cleaned up", cm.timeout)
		mu.Lock()
		cleanupErrors = append(cleanupErrors, errors.New("cleanup timeout exceeded"))
		mu.Unlock()
		return cleanupErrors
	}
}

// Clear removes all registered resources without executing cleanup
func (cm *CleanupManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.resources = cm.resources[:0]
}
