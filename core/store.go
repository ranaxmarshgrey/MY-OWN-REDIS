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

// Delete removes a key from the store.
// It returns true if the key existed and was deleted, or false if it did not exist or was expired.
func Delete(key string) bool {
	if obj, exists := store[key]; exists {
		delete(store, key)
		if obj.ExpiresAt != -1 && obj.ExpiresAt <= time.Now().UnixMilli() {
			return false
		}
		return true
	}
	return false
}

// Get retrieves the Object pointer for a key, or nil if the key does not exist or has expired.
// If the key has expired, it is deleted from the store (passive deletion) and nil is returned.
func Get(key string) *Object {
	obj, exists := store[key]
	if !exists {
		return nil
	}

	if obj.ExpiresAt != -1 && obj.ExpiresAt <= time.Now().UnixMilli() {
		Delete(key)
		return nil
	}

	return obj
}

func DeleteExpiredKeys() {
	for {
		nowMs := time.Now().UnixMilli()

		sampleSize := 0
		expiredCount := 0

		for key, obj := range store {
			// We only sample keys that have an expiration.
			if obj.ExpiresAt == -1 {
				continue
			}

			sampleSize++

			if obj.ExpiresAt <= nowMs {
				delete(store, key)
				expiredCount++
			}

			// Maximum 20 keys per sampling cycle.
			if sampleSize >= 20 {
				break
			}
		}

		// Nothing with an expiration was sampled.
		if sampleSize == 0 {
			return
		}

		expiredFraction := float64(expiredCount) / float64(sampleSize)

		// If <= 25% of the sample was expired,
		// stop this cleanup cycle.
		if expiredFraction <= 0.25 {
			return
		}
	}
}
