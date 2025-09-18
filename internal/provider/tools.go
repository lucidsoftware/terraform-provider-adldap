package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/go-ldap/ldap/v3"
	"github.com/go-ldap/ldif"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/thoas/go-funk"
)

// GetEntry returns a specific entry and is a shortcut around the search function.
func GetEntry(conn *ldap.Conn, dn string, attrs ...string) (ldap.Entry, error) {
	s := ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, 0, 0, 0, false, "(&)", attrs, []ldap.Control{})

	if result, err := conn.Search(s); err != nil {
		return ldap.Entry{}, err
	} else {
		if len(result.Entries) != 1 {
			return ldap.Entry{}, fmt.Errorf("search returned %d results", len(result.Entries))
		}
		return *result.Entries[0], nil
	}
}

// ToLDIF converts the given ldap entry into an LDIF representation.
func ToLDIF(entry interface{}) string {
	if l, err := ldif.ToLDIF(entry); err == nil {
		if m, err := ldif.Marshal(l); err == nil {
			return m
		}
	}
	return ""
}

// MaskAttributes searches attributes of an LDAP entry for sensitive data and masks the values.
func MaskAttributes(ctx context.Context, attributes map[string][]string) context.Context {
	for attributeType, values := range attributes {
		if attributeType == "userPassword" {
			funk.ForEach(values, func(value string) {
				ctx = tflog.MaskLogStrings(ctx, value)
			})
		}
	}
	return ctx
}

// MaskAttributesFromArray is a MaskAttributes adapter for ldap.EntryAttribute-Arrays.
func MaskAttributesFromArray(ctx context.Context, attributes []*ldap.EntryAttribute) context.Context {
	var attributesHash = funk.Reduce(
		attributes,
		func(acc map[string][]string, a *ldap.EntryAttribute) map[string][]string {
			acc[a.Name] = a.Values
			return acc
		},
		make(map[string][]string),
	)
	if h, ok := attributesHash.(map[string][]string); !ok {
		return ctx
	} else {
		return MaskAttributes(ctx, h)
	}
}

// isBinaryAttribute checks if an attribute should be stored as base64
func isBinaryAttribute(name string) bool {
	return name == "objectGUID" || name == "objectSid"
}

// isSystemAttribute checks if an attribute should be excluded from modifications
func isSystemAttribute(name string) bool {
	return name == "objectGUID" || 
		   name == "objectSid" ||
		   // Temporarily allow distinguishedName for DN resolution
		   // name == "distinguishedName" ||
		   name == "dSCorePropagationData" ||
		   name == "instanceType" ||
		   name == "whenCreated" ||
		   name == "whenChanged" ||
		   name == "uSNCreated" ||
		   name == "uSNChanged" ||
		   name == "memberOf"
}

// isTerraformOnlyAttribute returns true if the attribute is a Terraform-only attribute
// that should never be sent to LDAP operations (it gets resolved to other attributes)
func isTerraformOnlyAttribute(name string) bool {
	return name == "member_cn" || name == "member_sam"
}

// encodeAttributeValues encodes binary attribute values to base64
func encodeAttributeValues(attributeName string, values []string) []string {
	if !isBinaryAttribute(attributeName) {
		return values
	}
	
	encodedValues := make([]string, len(values))
	for i, value := range values {
		encodedValues[i] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	return encodedValues
}

// decodeAttributeValues decodes base64 attribute values back to binary
func decodeAttributeValues(attributeName string, values []string) []string {
	if !isBinaryAttribute(attributeName) {
		return values
	}
	
	decodedValues := make([]string, len(values))
	for i, value := range values {
		if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
			decodedValues[i] = string(decoded)
		} else {
			decodedValues[i] = value
		}
	}
	return decodedValues
}
