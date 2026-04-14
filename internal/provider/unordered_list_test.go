package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

// TestAreStringSlicesExactlyEqual tests the exact equality function
func TestAreStringSlicesExactlyEqual(t *testing.T) {
	tests := []struct {
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
			name:     "different order",
			slice1:   []string{"a", "b", "c"},
			slice2:   []string{"c", "b", "a"},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := areStringSlicesExactlyEqual(tt.slice1, tt.slice2)
			if result != tt.expected {
				t.Errorf("areStringSlicesExactlyEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestUnorderedListPlanModifier tests the plan modifier
func TestUnorderedListPlanModifier(t *testing.T) {
	ctx := context.Background()
	modifier := NewUnorderedListPlanModifier("member")

	t.Run("null state returns early", func(t *testing.T) {
		planValue := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a")})
		resp := &planmodifier.ListResponse{PlanValue: planValue}
		modifier.PlanModifyList(ctx, planmodifier.ListRequest{
			StateValue: types.ListNull(types.StringType),
			PlanValue:  planValue,
		}, resp)
	})

	t.Run("null plan returns early", func(t *testing.T) {
		stateValue := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a")})
		resp := &planmodifier.ListResponse{PlanValue: stateValue}
		modifier.PlanModifyList(ctx, planmodifier.ListRequest{
			StateValue: stateValue,
			PlanValue:  types.ListNull(types.StringType),
		}, resp)
	})

	t.Run("unknown plan returns early", func(t *testing.T) {
		stateValue := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a")})
		resp := &planmodifier.ListResponse{PlanValue: stateValue}
		modifier.PlanModifyList(ctx, planmodifier.ListRequest{
			StateValue: stateValue,
			PlanValue:  types.ListUnknown(types.StringType),
		}, resp)
	})

	t.Run("equal as set preserves state", func(t *testing.T) {
		stateValue := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a"), types.StringValue("b")})
		planValue := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("b"), types.StringValue("a")})
		resp := &planmodifier.ListResponse{PlanValue: planValue}
		modifier.PlanModifyList(ctx, planmodifier.ListRequest{
			StateValue: stateValue,
			PlanValue:  planValue,
		}, resp)
		if !resp.PlanValue.Equal(stateValue) {
			t.Error("expected plan to equal state for equal sets")
		}
	})

	t.Run("not equal keeps plan", func(t *testing.T) {
		stateValue := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a")})
		planValue := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("b")})
		resp := &planmodifier.ListResponse{PlanValue: planValue}
		modifier.PlanModifyList(ctx, planmodifier.ListRequest{
			StateValue: stateValue,
			PlanValue:  planValue,
		}, resp)
	})
}

// TestUnorderedMapListPlanModifier tests the map plan modifier
func TestUnorderedMapListPlanModifier(t *testing.T) {
	ctx := context.Background()
	modifier := NewUnorderedMapListPlanModifier()

	t.Run("null state returns early", func(t *testing.T) {
		planValue := types.MapValueMust(types.SetType{ElemType: types.StringType}, map[string]attr.Value{})
		resp := &planmodifier.MapResponse{PlanValue: planValue}
		modifier.PlanModifyMap(ctx, planmodifier.MapRequest{
			StateValue: types.MapNull(types.SetType{ElemType: types.StringType}),
			PlanValue:  planValue,
		}, resp)
	})

	t.Run("null plan returns early", func(t *testing.T) {
		stateValue := types.MapValueMust(types.SetType{ElemType: types.StringType}, map[string]attr.Value{})
		resp := &planmodifier.MapResponse{PlanValue: stateValue}
		modifier.PlanModifyMap(ctx, planmodifier.MapRequest{
			StateValue: stateValue,
			PlanValue:  types.MapNull(types.SetType{ElemType: types.StringType}),
		}, resp)
	})

	t.Run("unknown plan returns early", func(t *testing.T) {
		stateValue := types.MapValueMust(types.SetType{ElemType: types.StringType}, map[string]attr.Value{})
		resp := &planmodifier.MapResponse{PlanValue: stateValue}
		modifier.PlanModifyMap(ctx, planmodifier.MapRequest{
			StateValue: stateValue,
			PlanValue:  types.MapUnknown(types.SetType{ElemType: types.StringType}),
		}, resp)
	})

	t.Run("exactly equal unordered attribute", func(t *testing.T) {
		memberSet := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("cn=user1,dc=example,dc=com"), types.StringValue("cn=user2,dc=example,dc=com")})
		stateMap := map[string]attr.Value{"member": memberSet}
		planMap := map[string]attr.Value{"member": memberSet}
		resp := &planmodifier.MapResponse{PlanValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap)}
		modifier.PlanModifyMap(ctx, planmodifier.MapRequest{
			StateValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, stateMap),
			PlanValue:  types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap),
		}, resp)
	})

	t.Run("ordering difference preserves state", func(t *testing.T) {
		stateMemberSet := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("cn=user1"), types.StringValue("cn=user2")})
		planMemberSet := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("cn=user2"), types.StringValue("cn=user1")})
		stateMap := map[string]attr.Value{"member": stateMemberSet}
		planMap := map[string]attr.Value{"member": planMemberSet}
		resp := &planmodifier.MapResponse{PlanValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap)}
		modifier.PlanModifyMap(ctx, planmodifier.MapRequest{
			StateValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, stateMap),
			PlanValue:  types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap),
		}, resp)
	})

	t.Run("non-unordered attribute allows changes", func(t *testing.T) {
		stateDesc := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("old")})
		planDesc := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("new")})
		stateMap := map[string]attr.Value{"description": stateDesc}
		planMap := map[string]attr.Value{"description": planDesc}
		resp := &planmodifier.MapResponse{PlanValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap)}
		modifier.PlanModifyMap(ctx, planmodifier.MapRequest{
			StateValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, stateMap),
			PlanValue:  types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap),
		}, resp)
	})

	t.Run("pure additions scenario", func(t *testing.T) {
		stateMemberSet := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("cn=user1")})
		planMemberSet := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("cn=user1"), types.StringValue("cn=user2")})
		stateMap := map[string]attr.Value{"member": stateMemberSet}
		planMap := map[string]attr.Value{"member": planMemberSet}
		resp := &planmodifier.MapResponse{PlanValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap)}
		modifier.PlanModifyMap(ctx, planmodifier.MapRequest{
			StateValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, stateMap),
			PlanValue:  types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap),
		}, resp)
	})

	t.Run("removals allowed through", func(t *testing.T) {
		stateMemberSet := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("cn=user1"), types.StringValue("cn=user2")})
		planMemberSet := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("cn=user1")})
		stateMap := map[string]attr.Value{"member": stateMemberSet}
		planMap := map[string]attr.Value{"member": planMemberSet}
		resp := &planmodifier.MapResponse{PlanValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap)}
		modifier.PlanModifyMap(ctx, planmodifier.MapRequest{
			StateValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, stateMap),
			PlanValue:  types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap),
		}, resp)
	})

	t.Run("pure reordering artifacts detected", func(t *testing.T) {
		stateMemberSet := types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("cn=user1"),
			types.StringValue("cn=user2"),
		})
		planMemberSet := types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("cn=user2"),
			types.StringValue("cn=user1"),
		})
		stateMap := map[string]attr.Value{"member": stateMemberSet}
		planMap := map[string]attr.Value{"member": planMemberSet}
		resp := &planmodifier.MapResponse{PlanValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap)}
		modifier.PlanModifyMap(ctx, planmodifier.MapRequest{
			StateValue: types.MapValueMust(types.SetType{ElemType: types.StringType}, stateMap),
			PlanValue:  types.MapValueMust(types.SetType{ElemType: types.StringType}, planMap),
		}, resp)
	})
}
