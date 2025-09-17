package provider

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// UnorderedStringListType is a custom type that represents a list of strings where order doesn't matter
type UnorderedStringListType struct {
	basetypes.ListType
}

// UnorderedStringListValue is a custom value that implements semantic equality for unordered string lists
type UnorderedStringListValue struct {
	basetypes.ListValue
}

// NewUnorderedStringListType creates a new UnorderedStringListType
func NewUnorderedStringListType() UnorderedStringListType {
	return UnorderedStringListType{
		ListType: basetypes.ListType{
			ElemType: basetypes.StringType{},
		},
	}
}

// NewUnorderedStringListValue creates a new UnorderedStringListValue
func NewUnorderedStringListValue(elements []attr.Value) (UnorderedStringListValue, diag.Diagnostics) {
	listValue, diags := basetypes.NewListValue(basetypes.StringType{}, elements)
	if diags.HasError() {
		return UnorderedStringListValue{}, diags
	}
	
	return UnorderedStringListValue{
		ListValue: listValue,
	}, diags
}

// NewUnorderedStringListValueFromStrings creates a new UnorderedStringListValue from a slice of strings
func NewUnorderedStringListValueFromStrings(ctx context.Context, strings []string) (UnorderedStringListValue, diag.Diagnostics) {
	var diags diag.Diagnostics
	
	if strings == nil {
		return UnorderedStringListValue{
			ListValue: basetypes.NewListNull(basetypes.StringType{}),
		}, diags
	}
	
	if len(strings) == 0 {
		listValue, listDiags := basetypes.NewListValue(basetypes.StringType{}, []attr.Value{})
		diags.Append(listDiags...)
		return UnorderedStringListValue{
			ListValue: listValue,
		}, diags
	}
	
	elements := make([]attr.Value, len(strings))
	for i, s := range strings {
		elements[i] = basetypes.NewStringValue(s)
	}
	
	listValue, listDiags := basetypes.NewListValue(basetypes.StringType{}, elements)
	diags.Append(listDiags...)
	
	return UnorderedStringListValue{
		ListValue: listValue,
	}, diags
}

// Type returns the type of the UnorderedStringListValue
func (v UnorderedStringListValue) Type(ctx context.Context) attr.Type {
	return NewUnorderedStringListType()
}

// Equal returns true if the two UnorderedStringListValues are equal
func (v UnorderedStringListValue) Equal(o attr.Value) bool {
	other, ok := o.(UnorderedStringListValue)
	if !ok {
		return false
	}
	
	return v.ListValue.Equal(other.ListValue)
}

// ListSemanticEquals implements semantic equality for unordered lists
func (v UnorderedStringListValue) ListSemanticEquals(ctx context.Context, newValuable basetypes.ListValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	
	// Check if the new value is also an UnorderedStringListValue
	newValue, ok := newValuable.(UnorderedStringListValue)
	if !ok {
		return false, diags
	}
	
	// Handle null and unknown values
	if v.IsNull() && newValue.IsNull() {
		return true, diags
	}
	
	if v.IsNull() || newValue.IsNull() {
		return false, diags
	}
	
	if v.IsUnknown() && newValue.IsUnknown() {
		return true, diags
	}
	
	if v.IsUnknown() || newValue.IsUnknown() {
		return false, diags
	}
	
	// Convert both lists to string slices
	var priorValues, newValues []string
	diags.Append(v.ElementsAs(ctx, &priorValues, false)...)
	diags.Append(newValue.ElementsAs(ctx, &newValues, false)...)
	
	if diags.HasError() {
		return false, diags
	}
	
	// Compare as sets (order-independent)
	return areStringSlicesEqualAsSet(priorValues, newValues), diags
}

// ValueFromTerraform converts a Terraform value to an UnorderedStringListValue
func (t UnorderedStringListType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	listValue, err := t.ListType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	
	listVal, ok := listValue.(basetypes.ListValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", listValue)
	}
	
	return UnorderedStringListValue{
		ListValue: listVal,
	}, nil
}

// ValueType returns the type of values this type can hold
func (t UnorderedStringListType) ValueType(ctx context.Context) attr.Value {
	return UnorderedStringListValue{}
}

// String returns a string representation of the type
func (t UnorderedStringListType) String() string {
	return "UnorderedStringListType"
}

// areStringSlicesEqualAsSet compares two string slices as sets (order-independent)
func areStringSlicesEqualAsSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	
	// Create sorted copies to compare
	aCopy := make([]string, len(a))
	bCopy := make([]string, len(b))
	copy(aCopy, a)
	copy(bCopy, b)
	
	sort.Strings(aCopy)
	sort.Strings(bCopy)
	
	return reflect.DeepEqual(aCopy, bCopy)
}