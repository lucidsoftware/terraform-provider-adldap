package provider

import (
	"context"
	"testing"
)

func TestGenerateCacheKey(t *testing.T) {
	tests := []struct {
		name    string
		rawKey  string
		wantLen int
	}{
		{
			name:    "SAM lookup key",
			rawKey:  "sam:testuser",
			wantLen: 64, // SHA-256 hex string length
		},
		{
			name:    "CN lookup key with base DNs",
			rawKey:  "cn:John Doe:OU=users,DC=example,DC=com:OU=disabled,DC=example,DC=com",
			wantLen: 64, // SHA-256 hex string length
		},
		{
			name:    "Empty string",
			rawKey:  "",
			wantLen: 64, // SHA-256 hex string length
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateCacheKey(tt.rawKey)
			if len(result) != tt.wantLen {
				t.Errorf("generateCacheKey() length = %d, want %d", len(result), tt.wantLen)
			}
			// Verify it's a hex string
			for _, char := range result {
				if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
					t.Errorf("generateCacheKey() result contains non-hex character: %c", char)
				}
			}
		})
	}
}

func TestCachedUserLookup(t *testing.T) {
	// Create provider data with cache
	providerData := &LDAPProviderData{
		userLookupCache: make(map[string]UserLookupCacheEntry),
	}

	ctx := context.Background()
	rawKey := "sam:testuser"
	callCount := 0

	// Mock lookup function
	lookupFunc := func() (string, bool) {
		callCount++
		return "CN=testuser,OU=users,DC=example,DC=com", true
	}

	// First call - should execute lookup function
	dn1, found1 := providerData.cachedUserLookup(ctx, rawKey, lookupFunc)
	if callCount != 1 {
		t.Errorf("First call should execute lookup function, callCount = %d, want 1", callCount)
	}
	if !found1 {
		t.Errorf("First call found = %t, want true", found1)
	}
	expectedDN := "CN=testuser,OU=users,DC=example,DC=com"
	if dn1 != expectedDN {
		t.Errorf("First call dn = %s, want %s", dn1, expectedDN)
	}

	// Second call - should use cache
	dn2, found2 := providerData.cachedUserLookup(ctx, rawKey, lookupFunc)
	if callCount != 1 {
		t.Errorf("Second call should use cache, callCount = %d, want 1", callCount)
	}
	if !found2 {
		t.Errorf("Second call found = %t, want true", found2)
	}
	if dn2 != expectedDN {
		t.Errorf("Second call dn = %s, want %s", dn2, expectedDN)
	}

	// Verify cache contents
	cacheKey := generateCacheKey(rawKey)
	entry, exists := providerData.userLookupCache[cacheKey]
	if !exists {
		t.Errorf("Cache entry should exist for key %s", cacheKey)
	}
	if entry.DN != expectedDN {
		t.Errorf("Cache entry DN = %s, want %s", entry.DN, expectedDN)
	}
	if !entry.Found {
		t.Errorf("Cache entry Found = %t, want true", entry.Found)
	}
}

func TestCachedUserLookupNotFound(t *testing.T) {
	// Create provider data with cache
	providerData := &LDAPProviderData{
		userLookupCache: make(map[string]UserLookupCacheEntry),
	}

	ctx := context.Background()
	rawKey := "sam:nonexistentuser"
	callCount := 0

	// Mock lookup function that returns not found
	lookupFunc := func() (string, bool) {
		callCount++
		return "", false
	}

	// First call - should execute lookup function
	dn1, found1 := providerData.cachedUserLookup(ctx, rawKey, lookupFunc)
	if callCount != 1 {
		t.Errorf("First call should execute lookup function, callCount = %d, want 1", callCount)
	}
	if found1 {
		t.Errorf("First call found = %t, want false", found1)
	}
	if dn1 != "" {
		t.Errorf("First call dn = %s, want empty string", dn1)
	}

	// Second call - should use cache
	dn2, found2 := providerData.cachedUserLookup(ctx, rawKey, lookupFunc)
	if callCount != 1 {
		t.Errorf("Second call should use cache, callCount = %d, want 1", callCount)
	}
	if found2 {
		t.Errorf("Second call found = %t, want false", found2)
	}
	if dn2 != "" {
		t.Errorf("Second call dn = %s, want empty string", dn2)
	}

	// Verify cache contents
	cacheKey := generateCacheKey(rawKey)
	entry, exists := providerData.userLookupCache[cacheKey]
	if !exists {
		t.Errorf("Cache entry should exist for key %s", cacheKey)
	}
	if entry.DN != "" {
		t.Errorf("Cache entry DN = %s, want empty string", entry.DN)
	}
	if entry.Found {
		t.Errorf("Cache entry Found = %t, want false", entry.Found)
	}
}

func TestCacheKeyUniqueness(t *testing.T) {
	// Test that different raw keys produce different cache keys
	keys := []string{
		"sam:testuser",
		"sam:john",
		"cn:John Doe:OU=users,DC=example,DC=com:",
		"cn:John Doe:OU=contractors,DC=example,DC=com:",
		"cn:Jane Smith:OU=users,DC=example,DC=com:",
	}

	cacheKeys := make(map[string]string)
	for _, rawKey := range keys {
		cacheKey := generateCacheKey(rawKey)
		if existing, exists := cacheKeys[cacheKey]; exists {
			t.Errorf("Cache key collision: %s and %s both produce %s", rawKey, existing, cacheKey)
		}
		cacheKeys[cacheKey] = rawKey
	}
}