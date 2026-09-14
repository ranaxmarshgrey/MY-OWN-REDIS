package core

import "time"

type Object struct {
	Value     interface{}
	ExpiresAt int64 // Absolute Unix timestamp in milliseconds; -1 means no expiration
}

var store = make(map[string]*Object)

// NewObject constructs a new Object with either an absolute expiration timestamp or -1.
func NewObject(value interface{}, durationMs int64) *Object {
	var expiresAt int64 = -1

	if durationMs > 0 {
		nowMs := time.Now().UnixMilli()
		expiresAt = nowMs + durationMs
	}

	return &Object{
		Value:     value,
		ExpiresAt: expiresAt,
	}
}

// Put associates a key with an Object in the store map.
func Put(key string, obj *Object) {
	store[key] = obj
}

// Get retrieves the Object pointer for a key, or nil if the key does not exist.
// Note: Get does not perform expiration checks.
func Get(key string) *Object {
	return store[key]
}
