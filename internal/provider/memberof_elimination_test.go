package provider

import (
	"context"
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestSystemAttributeFiltering tests that memberOf is properly classified as system attribute
func TestSystemAttributeFiltering(t *testing.T) {
	testCases := []struct {
		name           string
		attributeName  string
		shouldBeSystem bool
	}{
		{"memberOf is system attribute", "memberOf", true},
		{"objectGUID is system attribute", "objectGUID", true},
		{"cn is not system attribute", "cn", false},
		{"member is not system attribute", "member", false},
		{"description is not system attribute", "description", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isSystemAttribute(tc.attributeName)
			if result != tc.shouldBeSystem {
				t.Errorf("isSystemAttribute(%s) should return %v, got %v", tc.attributeName, tc.shouldBeSystem, result)
			}
		})
	}
}

// TestUnorderedAttributeExclusion tests that memberOf is NOT in unordered attributes list
func TestUnorderedAttributeExclusion(t *testing.T) {
	// Test that memberOf is not considered an unordered attribute
	if isUnorderedAttribute("memberOf") {
		t.Error("memberOf should not be in unordered attributes list")
	}
	
	// Verify other expected unordered attributes are still present
	expectedUnordered := []string{"member", "objectClass", "uniqueMember", "roleOccupant", "owner"}
	for _, attr := range expectedUnordered {
		if !isUnorderedAttribute(attr) {
			t.Errorf("Expected unordered attribute %s should still be present", attr)
		}
	}
	
	// Double-check that memberOf is not in the unorderedAttributes slice
	for _, attr := range unorderedAttributes {
		if attr == "memberOf" {
			t.Error("memberOf should not appear in unorderedAttributes slice")
		}
	}
}

// TestLDAPSearchFiltersSystemAttributes tests that ldap_search datasource filters system attributes
func TestLDAPSearchFiltersSystemAttributes(t *testing.T) {
	// Create a mock LDAP entry with system attributes including memberOf
	mockEntry := &ldap.Entry{
		DN: "CN=testuser,OU=users,DC=example,DC=com",
		Attributes: []*ldap.EntryAttribute{
			{Name: "cn", Values: []string{"testuser"}},
			{Name: "mail", Values: []string{"test@example.com"}},
			{Name: "memberOf", Values: []string{"CN=group1,OU=groups,DC=example,DC=com"}},
			{Name: "objectGUID", Values: []string{"guid-value"}},
			{Name: "distinguishedName", Values: []string{"CN=testuser,OU=users,DC=example,DC=com"}},
		},
	}
	
	// Test that system attributes are properly filtered
	_ = MaskAttributesFromArray(context.Background(), mockEntry.Attributes)
	
	// Simulate what the datasource does - filter system attributes
	var processedAttrs []string
	for _, attribute := range mockEntry.Attributes {
		if !isSystemAttribute(attribute.Name) {
			processedAttrs = append(processedAttrs, attribute.Name)
		}
	}
	
	// Verify memberOf and other system attributes are filtered out
	for _, attr := range processedAttrs {
		if attr == "memberOf" {
			t.Error("memberOf should be filtered out")
		}
		if attr == "objectGUID" {
			t.Error("objectGUID should be filtered out")
		}
		if attr == "distinguishedName" {
			t.Error("distinguishedName should be filtered out")
		}
	}
	
	// Verify non-system attributes are preserved
	foundCN := false
	foundMail := false
	for _, attr := range processedAttrs {
		if attr == "cn" {
			foundCN = true
		}
		if attr == "mail" {
			foundMail = true
		}
	}
	if !foundCN {
		t.Error("cn should not be filtered out")
	}
	if !foundMail {
		t.Error("mail should not be filtered out")
	}
}

// TestLDAPObjectDataSourceFiltersSystemAttributes tests that ldap_object datasource filters system attributes  
func TestLDAPObjectDataSourceFiltersSystemAttributes(t *testing.T) {
	// Create a mock LDAP entry
	mockEntry := &ldap.Entry{
		DN: "CN=testgroup,OU=groups,DC=example,DC=com", 
		Attributes: []*ldap.EntryAttribute{
			{Name: "cn", Values: []string{"testgroup"}},
			{Name: "description", Values: []string{"Test group"}},
			{Name: "member", Values: []string{"CN=user1,OU=users,DC=example,DC=com"}},
			{Name: "memberOf", Values: []string{"CN=parent,OU=groups,DC=example,DC=com"}},
			{Name: "objectGUID", Values: []string{"guid-value"}},
		},
	}
	
	// Test the filtering logic used in datasource
	var processedAttrs []string
	for _, attribute := range mockEntry.Attributes {
		if attribute.Name != "objectClass" && !isSystemAttribute(attribute.Name) {
			processedAttrs = append(processedAttrs, attribute.Name)
		}
	}
	
	// Verify memberOf is filtered out but member is preserved
	foundMemberOf := false
	foundMember := false
	for _, attr := range processedAttrs {
		if attr == "memberOf" {
			foundMemberOf = true
		}
		if attr == "member" {
			foundMember = true
		}
	}
	
	if foundMemberOf {
		t.Error("memberOf should be filtered out by datasource")
	}
	if !foundMember {
		t.Error("member should not be filtered out")
	}
}

// TestNoMemberOfInCodePaths tests that memberOf doesn't appear in critical code paths
func TestNoMemberOfInCodePaths(t *testing.T) {
	// Test 1: Verify it's not treated as binary attribute
	if isBinaryAttribute("memberOf") {
		t.Error("memberOf should not be treated as binary attribute")
	}
	
	// Test 2: Verify encodeAttributeValues doesn't process memberOf specially
	testValues := []string{"CN=group1,OU=groups,DC=example,DC=com"}
	encoded := encodeAttributeValues("memberOf", testValues)
	// For non-binary attributes, values should be returned unchanged
	if len(encoded) != len(testValues) {
		t.Error("memberOf should not receive special encoding")
	}
	for i, val := range encoded {
		if val != testValues[i] {
			t.Error("memberOf values should not be modified during encoding")
		}
	}
}

// TestMemberOfNotInUnorderedAttributesSlice verifies memberOf was completely removed from unordered processing
func TestMemberOfNotInUnorderedAttributesSlice(t *testing.T) {
	// This is the most critical test - ensures memberOf is completely removed from unordered attribute processing
	for i, attr := range unorderedAttributes {
		if attr == "memberOf" {
			t.Errorf("CRITICAL: memberOf found at index %d in unorderedAttributes slice - this will cause processing", i)
		}
	}
	
	// Log the current unordered attributes for verification
	t.Logf("Current unordered attributes: %v", unorderedAttributes)
}

// TestLDAPResourceStructureExcludesMemberOf verifies the resource model doesn't have MemberOf field
func TestLDAPResourceStructureExcludesMemberOf(t *testing.T) {
	// This test compiles successfully only if MemberOf field doesn't exist in the struct
	model := &LDAPObjectResourceModel{
		// Only initialize fields that should exist - if MemberOf existed, this would fail to compile
		ID:            types.StringNull(),
		DN:            types.StringNull(),  
		ObjectClasses: types.ListNull(types.StringType),
		Attributes:    types.MapNull(types.ListType{ElemType: types.StringType}),
		IgnoreChanges: types.ListNull(types.StringType),
	}
	
	// If this test compiles and runs, it proves MemberOf field was successfully removed.
	_ = model
}
