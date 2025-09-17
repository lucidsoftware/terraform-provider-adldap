package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/thoas/go-funk"
)

// unorderedAttributes defines which LDAP attributes should be treated as unordered sets
var unorderedAttributes = []string{
	"member",          // Group membership
	"objectClass",     // Object classes
	"seeAlso",         // See also references
	"aliasedObjectName", // Alias references
	"uniqueMember",    // Unique group membership (alternative to member)
	"roleOccupant",    // Role occupants
	"owner",           // Owners
	"dnsRoot",         // DNS root references
	"wellKnownObjects", // Well-known objects
}

// isUnorderedAttribute determines if an LDAP attribute should be treated as unordered
func isUnorderedAttribute(attributeName string) bool {
	return funk.ContainsString(unorderedAttributes, attributeName)
}

// convertToUnorderedListValue converts a regular list value to an UnorderedStringListValue if the attribute is unordered
func convertToUnorderedListValue(ctx context.Context, attributeName string, value attr.Value) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics
	
	// Only convert if this is an unordered attribute
	if !isUnorderedAttribute(attributeName) {
		return value, diags
	}
	
	// Handle null and unknown values
	if value.IsNull() {
		return UnorderedStringListValue{
			ListValue: basetypes.NewListNull(basetypes.StringType{}),
		}, diags
	}
	
	if value.IsUnknown() {
		return UnorderedStringListValue{
			ListValue: basetypes.NewListUnknown(basetypes.StringType{}),
		}, diags
	}
	
	// Convert from regular list to unordered list
	listValue, ok := value.(basetypes.ListValue)
	if !ok {
		diags.AddError(
			"Invalid attribute type",
			"Expected list value for attribute: "+attributeName,
		)
		return value, diags
	}
	
	// Extract string values
	var stringValues []string
	diags.Append(listValue.ElementsAs(ctx, &stringValues, false)...)
	if diags.HasError() {
		return value, diags
	}
	
	// Create UnorderedStringListValue
	unorderedValue, createDiags := NewUnorderedStringListValueFromStrings(ctx, stringValues)
	diags.Append(createDiags...)
	
	return unorderedValue, diags
}

// convertFromUnorderedListValue converts an UnorderedStringListValue back to a regular list value
func convertFromUnorderedListValue(ctx context.Context, value attr.Value) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics
	
	unorderedValue, ok := value.(UnorderedStringListValue)
	if !ok {
		// Not an unordered value, return as-is
		return value, diags
	}
	
	// Return the underlying ListValue
	return unorderedValue.ListValue, diags
}

// getUnorderedAttributeType returns the appropriate attribute type for unordered attributes
func getUnorderedAttributeType(attributeName string) attr.Type {
	if isUnorderedAttribute(attributeName) {
		return NewUnorderedStringListType()
	}
	return basetypes.ListType{ElemType: basetypes.StringType{}}
}

// addUnorderedAttributeDescription adds a note about ordering to the description for unordered attributes
func addUnorderedAttributeDescription(attributeName, description string) string {
	if isUnorderedAttribute(attributeName) {
		suffix := " (Order is not significant for this attribute)"
		if !strings.HasSuffix(description, suffix) {
			return description + suffix
		}
	}
	return description
}