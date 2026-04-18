// Package cache provides a simple in-memory key/value cache for testing.
// This file is a large synthetic fixture used by TestTokenReduction in ast_test.go.
package cache

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Status represents the operational status of the cache.
type Status int

// Entry holds a cached value together with its expiration time.
type Entry struct {
	Value     interface{}
	ExpiresAt time.Time
}

// Cache is a thread-safe in-memory key/value store with optional TTL support.
type Cache struct {
	mu      sync.RWMutex
	data    map[string]Entry
	maxSize int
}

const (
	// StatusOK indicates the cache is operating normally.
	StatusOK Status = iota
	// StatusFull indicates the cache has reached its maximum capacity.
	StatusFull
	// StatusEvicting indicates the cache is currently evicting stale entries.
	StatusEvicting
	// DefaultMaxSize is the default maximum number of entries.
	DefaultMaxSize = 1024
	// DefaultTTL is the default time-to-live for cache entries.
	DefaultTTL = 5 * time.Minute
)

// New creates a new Cache with the given maximum size.
// If maxSize is 0 or negative, DefaultMaxSize is used.
func New(maxSize int) *Cache {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	return &Cache{
		data:    make(map[string]Entry, maxSize),
		maxSize: maxSize,
	}
}

// Set stores a value under the given key with the specified TTL.
// A zero TTL means the entry never expires.
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) error {
	if key == "" {
		return errors.New("cache: key must not be empty")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.data) >= c.maxSize {
		c.evictExpired()
	}
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	c.data[key] = Entry{Value: value, ExpiresAt: exp}
	return nil
}

// Get retrieves the value for the given key.
// Returns false if the key does not exist or the entry has expired.
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.data[key]
	if !ok {
		return nil, false
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	return entry.Value, true
}

// Delete removes the entry with the given key.
// Returns true if the key existed, false otherwise.
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.data[key]
	if ok {
		delete(c.data, key)
	}
	return ok
}

// Len returns the number of entries currently in the cache, including expired ones.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]Entry, c.maxSize)
}

// Stats returns a human-readable summary of the cache's current state.
func (c *Cache) Stats() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return fmt.Sprintf("Cache{entries=%d, maxSize=%d}", len(c.data), c.maxSize)
}

// evictExpired removes all expired entries from the cache.
// Must be called with c.mu held for writing.
func (c *Cache) evictExpired() {
	now := time.Now()
	for k, v := range c.data {
		if !v.ExpiresAt.IsZero() && now.After(v.ExpiresAt) {
			delete(c.data, k)
		}
	}
}

// normalizeKey converts a raw key to a canonical form.
func normalizeKey(key string) string {
	if len(key) == 0 {
		return key
	}
	return fmt.Sprintf("cache:%s", key)
}
