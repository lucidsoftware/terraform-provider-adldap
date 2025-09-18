package provider

import (
	"testing"
)

// TestAreStringSlicesEqualAsSet tests the set comparison utility function
func TestAreStringSlicesEqualAsSet(t *testing.T) {
	testCases := []struct {
		name     string
		slice1   []string
		slice2   []string
		expected bool
	}{
		{
			name:     "identical slices",
			slice1:   []string{"a", "b", "c"},
			slice2:   []string{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "same elements different order",
			slice1:   []string{"a", "b", "c"},
			slice2:   []string{"c", "a", "b"},
			expected: true,
		},
		{
			name:     "different elements",
			slice1:   []string{"a", "b", "c"},
			slice2:   []string{"a", "b", "d"},
			expected: false,
		},
		{
			name:     "different lengths",
			slice1:   []string{"a", "b"},
			slice2:   []string{"a", "b", "c"},
			expected: false,
		},
		{
			name:     "empty slices",
			slice1:   []string{},
			slice2:   []string{},
			expected: true,
		},
		{
			name:     "one empty one not",
			slice1:   []string{},
			slice2:   []string{"a"},
			expected: false,
		},
		{
			name:     "duplicates same order",
			slice1:   []string{"a", "b", "a"},
			slice2:   []string{"a", "b", "a"},
			expected: true,
		},
		{
			name:     "duplicates different order",
			slice1:   []string{"a", "b", "a"},
			slice2:   []string{"a", "a", "b"},
			expected: true,
		},
		{
			name:     "different duplicate counts",
			slice1:   []string{"a", "b", "a"},
			slice2:   []string{"a", "b", "b"},
			expected: false,
		},
		{
			name:     "LDAP DN example - same members different order",
			slice1:   []string{"CN=John Doe,OU=users,DC=example,DC=com", "CN=Jane Smith,OU=users,DC=example,DC=com"},
			slice2:   []string{"CN=Jane Smith,OU=users,DC=example,DC=com", "CN=John Doe,OU=users,DC=example,DC=com"},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := areStringSlicesEqualAsSet(tc.slice1, tc.slice2)
			if result != tc.expected {
				t.Errorf("areStringSlicesEqualAsSet(%v, %v) = %v, expected %v", tc.slice1, tc.slice2, result, tc.expected)
			}
		})
	}
}

// TestIsUnorderedAttribute tests the attribute classification function
func TestIsUnorderedAttribute(t *testing.T) {
	testCases := []struct {
		name      string
		attribute string
		expected  bool
	}{
		{
			name:      "member attribute",
			attribute: "member",
			expected:  true,
		},
		{
			name:      "objectClass attribute",
			attribute: "objectClass",
			expected:  true,
		},
		{
			name:      "cn attribute",
			attribute: "cn",
			expected:  false,
		},
		{
			name:      "description attribute",
			attribute: "description",
			expected:  false,
		},
		{
			name:      "uniqueMember attribute",
			attribute: "uniqueMember",
			expected:  true,
		},
		{
			name:      "case sensitive test",
			attribute: "MEMBER",
			expected:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isUnorderedAttribute(tc.attribute)
			if result != tc.expected {
				t.Errorf("isUnorderedAttribute(%s) = %v, expected %v", tc.attribute, result, tc.expected)
			}
		})
	}
}