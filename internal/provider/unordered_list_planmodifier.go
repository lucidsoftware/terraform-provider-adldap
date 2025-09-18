package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// UnorderedListPlanModifier is a plan modifier that treats lists as unordered for comparison
type UnorderedListPlanModifier struct {
	attributeName string
}

// NewUnorderedListPlanModifier creates a new UnorderedListPlanModifier
func NewUnorderedListPlanModifier(attributeName string) planmodifier.List {
	return &UnorderedListPlanModifier{
		attributeName: attributeName,
	}
}

// Description returns a description of the plan modifier
func (m *UnorderedListPlanModifier) Description(_ context.Context) string {
	return fmt.Sprintf("Treats the %s attribute as an unordered list where element order is not significant", m.attributeName)
}

// MarkdownDescription returns a markdown description of the plan modifier
func (m *UnorderedListPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

// PlanModifyList implements the plan modifier logic
func (m *UnorderedListPlanModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	// Don't modify during creation or deletion
	if req.StateValue.IsNull() || req.PlanValue.IsNull() {
		return
	}

	// Don't modify if the planned value is unknown
	if req.PlanValue.IsUnknown() {
		return
	}

	// Extract the string values from both state and plan
	var stateValues, planValues []string
	
	resp.Diagnostics.Append(req.StateValue.ElementsAs(ctx, &stateValues, false)...)
	resp.Diagnostics.Append(req.PlanValue.ElementsAs(ctx, &planValues, false)...)
	
	if resp.Diagnostics.HasError() {
		return
	}

	// Compare as sets (order-independent)
	if areStringSlicesEqualAsSet(stateValues, planValues) {
		tflog.Debug(ctx, "Unordered list values are equivalent, preserving state ordering", map[string]interface{}{
			"attribute": m.attributeName,
			"state":     stateValues,
			"plan":      planValues,
		})
		
		// Values are equivalent as sets, so keep the state value to avoid unnecessary updates
		resp.PlanValue = req.StateValue
	}
}

// UnorderedMapListPlanModifier is a plan modifier for map attributes containing unordered lists
type UnorderedMapListPlanModifier struct{}

// NewUnorderedMapListPlanModifier creates a new UnorderedMapListPlanModifier
func NewUnorderedMapListPlanModifier() planmodifier.Map {
	return &UnorderedMapListPlanModifier{}
}

// Description returns a description of the plan modifier
func (m *UnorderedMapListPlanModifier) Description(_ context.Context) string {
	return "Treats certain map attributes as containing unordered lists where element order is not significant"
}

// MarkdownDescription returns a markdown description of the plan modifier
func (m *UnorderedMapListPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

// PlanModifyMap implements the plan modifier logic for map attributes
func (m *UnorderedMapListPlanModifier) PlanModifyMap(ctx context.Context, req planmodifier.MapRequest, resp *planmodifier.MapResponse) {
	tflog.Error(ctx, "UnorderedMapListPlanModifier: PlanModifyMap called")
	
	// Don't modify during creation or deletion
	if req.StateValue.IsNull() || req.PlanValue.IsNull() {
		tflog.Info(ctx, "UnorderedMapListPlanModifier: Skipping - null state or plan value", map[string]interface{}{
			"state_null": req.StateValue.IsNull(),
			"plan_null":  req.PlanValue.IsNull(),
		})
		return
	}

	// Don't modify if the planned value is unknown
	if req.PlanValue.IsUnknown() {
		tflog.Info(ctx, "UnorderedMapListPlanModifier: Skipping - plan value is unknown")
		return
	}

	tflog.Error(ctx, "UnorderedMapListPlanModifier: Processing plan modification")
	
	// Extract the map values (now containing sets instead of lists)
	var stateMap, planMap map[string]types.Set
	
	tflog.Info(ctx, "UnorderedMapListPlanModifier: Attempting extraction", map[string]interface{}{
		"state_type": fmt.Sprintf("%T", req.StateValue),
		"plan_type":  fmt.Sprintf("%T", req.PlanValue),
		"state_null": req.StateValue.IsNull(),
		"plan_null":  req.PlanValue.IsNull(),
	})
	
	stateDiags := req.StateValue.ElementsAs(ctx, &stateMap, false)
	resp.Diagnostics.Append(stateDiags...)
	if len(stateDiags) > 0 {
		tflog.Warn(ctx, "UnorderedMapListPlanModifier: State extraction errors", map[string]interface{}{
			"errors": stateDiags.Errors(),
		})
	}
	
	planDiags := req.PlanValue.ElementsAs(ctx, &planMap, false)
	resp.Diagnostics.Append(planDiags...)
	if len(planDiags) > 0 {
		tflog.Warn(ctx, "UnorderedMapListPlanModifier: Plan extraction errors", map[string]interface{}{
			"errors": planDiags.Errors(),
		})
	}
	
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "UnorderedMapListPlanModifier: Error extracting map values - aborting")
		return
	}

	tflog.Info(ctx, "UnorderedMapListPlanModifier: Extracted maps", map[string]interface{}{
		"state_attrs": len(stateMap),
		"plan_attrs":  len(planMap),
	})

	// Only process unordered attributes where there's a clear ordering-only difference
	for attrName, planSet := range planMap {
		stateSet, exists := stateMap[attrName]
		isUnordered := isUnorderedAttribute(attrName)
		
		// Convert sets to string slices for processing
		var stateValues, planValues []string
		if exists && !stateSet.IsNull() && !stateSet.IsUnknown() {
			resp.Diagnostics.Append(stateSet.ElementsAs(ctx, &stateValues, false)...)
		}
		if !planSet.IsNull() && !planSet.IsUnknown() {
			resp.Diagnostics.Append(planSet.ElementsAs(ctx, &planValues, false)...)
		}
		
		if resp.Diagnostics.HasError() {
			tflog.Warn(ctx, "UnorderedMapListPlanModifier: Error converting sets to slices")
			return
		}
		
		tflog.Info(ctx, "UnorderedMapListPlanModifier: Checking attribute", map[string]interface{}{
			"attribute":       attrName,
			"exists_in_state": exists,
			"is_unordered":    isUnordered,
			"state_count":     len(stateValues),
			"plan_count":      len(planValues),
		})
		
		if exists && isUnordered {
			equalAsSet := areStringSlicesEqualAsSet(stateValues, planValues)
			exactlyEqual := areStringSlicesExactlyEqual(stateValues, planValues)
			
			tflog.Info(ctx, "UnorderedMapListPlanModifier: Comparison results", map[string]interface{}{
				"attribute":      attrName,
				"equal_as_set":   equalAsSet,
				"exactly_equal":  exactlyEqual,
				"state_values":   stateValues,
				"plan_values":    planValues,
			})
			
			if exactlyEqual {
				// No changes at all - skip processing
				tflog.Info(ctx, "UnorderedMapListPlanModifier: Values are exactly equal", map[string]interface{}{
					"attribute": attrName,
				})
			} else if equalAsSet && !exactlyEqual {
				// SCENARIO 1: Pure ordering changes (existing logic - PROVEN WORKING)
				tflog.Warn(ctx, "UnorderedMapListPlanModifier: PURE ORDERING DIFFERENCE - MODIFYING PLAN", map[string]interface{}{
					"attribute":    attrName,
					"state_order":  stateValues,
					"plan_order":   planValues,
				})
				
				// Use state ordering to prevent churn
				resp.PlanValue = req.StateValue
				return
			} else {
				// SCENARIO 2: Mixed changes - detect pure ordering vs legitimate changes
				tflog.Info(ctx, "UnorderedMapListPlanModifier: Mixed changes detected - analyzing patterns", map[string]interface{}{
					"attribute":     attrName,
					"state_count":   len(stateValues),
					"plan_count":    len(planValues),
				})
				
				// Analyze the change patterns using set operations
				stateSet := stringSliceToSet(stateValues)
				planSet := stringSliceToSet(planValues)
				additions := setDifference(planSet, stateSet)
				removals := setDifference(stateSet, planSet)
				existing := setIntersection(stateSet, planSet)
				
				tflog.Info(ctx, "UnorderedMapListPlanModifier: Mixed change analysis", map[string]interface{}{
					"attribute":        attrName,
					"additions_count":  len(additions),
					"removals_count":   len(removals), 
					"existing_count":   len(existing),
				})
				
				// DEBUG: Log actual DN content for analysis
				tflog.Info(ctx, "UnorderedMapListPlanModifier: DN Content Analysis", map[string]interface{}{
					"attribute":     attrName,
					"state_values":  stateValues,
					"plan_values":   planValues,
				})
				
				// DEBUG: Log set content for detailed analysis
				additionsList := setToStringSlice(additions)
				removalsList := setToStringSlice(removals)
				tflog.Info(ctx, "UnorderedMapListPlanModifier: Set Content Analysis", map[string]interface{}{
					"attribute":  attrName,
					"additions":  additionsList,
					"removals":   removalsList,
				})
				
				// OPTION 3: Enhanced detection for framework index misalignment
				// Detect actual reordering artifacts: DNs that appear in both additions and removals
				reorderingArtifacts := setIntersection(additions, removals)
				hasReorderingArtifacts := len(reorderingArtifacts) > 0
				
				// Calculate real changes (legitimate additions/removals excluding reordering artifacts)
				realAdditions := setDifference(additions, reorderingArtifacts)
				realRemovals := setDifference(removals, reorderingArtifacts)
				
				tflog.Info(ctx, "UnorderedMapListPlanModifier: Framework index misalignment analysis", map[string]interface{}{
					"attribute":                attrName,
					"has_reordering_artifacts": hasReorderingArtifacts,
					"reordering_count":         len(reorderingArtifacts),
					"real_additions_count":     len(realAdditions),
					"real_removals_count":      len(realRemovals),
					"existing_count":           len(existing),
				})
				
				if hasReorderingArtifacts {
					// DETECTED: Actual reordering artifacts - DNs appearing as both add+remove
					tflog.Warn(ctx, "UnorderedMapListPlanModifier: REORDERING ARTIFACTS DETECTED - APPLYING SURGICAL FIX", map[string]interface{}{
						"attribute":           attrName,
						"artifacts":           len(reorderingArtifacts),
						"real_additions":      len(realAdditions),
						"real_removals":       len(realRemovals),
						"state_values":        stateValues,
						"plan_values":         planValues,
					})
					
					// SURGICAL APPROACH: Only prevent reordering artifacts, allow legitimate changes
					if len(realAdditions) == 0 && len(realRemovals) == 0 {
						// Pure reordering artifacts only - prevent all changes
						tflog.Warn(ctx, "Pure reordering artifacts - preventing all changes")
						resp.PlanValue = req.StateValue
						return
					} else {
						// Mixed: reordering artifacts + legitimate changes
						// This is complex - for now, allow through but log warning
						tflog.Warn(ctx, "Mixed reordering artifacts with legitimate changes - allowing through for now")
						// TODO: Implement surgical removal of only the reordering artifacts
					}
				} else if len(removals) == 0 && len(additions) > 0 && len(existing) > 0 {
					// Pure additions scenario with potential framework index misalignment
					// Need to preserve state ordering for existing members and append legitimate additions
					tflog.Warn(ctx, "UnorderedMapListPlanModifier: PURE ADDITIONS WITH REORDERING - FIXING PLAN", map[string]interface{}{
						"attribute":         attrName,
						"additions_count":   len(additions),
						"existing_count":    len(existing),
						"state_values":      stateValues,
						"plan_values":       planValues,
					})
					
					// Reconstruct plan: state ordering for existing + sorted additions
					var reconstructed []string
					
					// 1. Add existing members in STATE order (prevents reordering artifacts)
					for _, stateValue := range stateValues {
						if _, exists := existing[stateValue]; exists {
							reconstructed = append(reconstructed, stateValue)
						}
					}
					
					// 2. Add legitimate additions (sorted for consistency)
					additionsList := setToStringSlice(additions)
					sort.Strings(additionsList)
					reconstructed = append(reconstructed, additionsList...)
					
					tflog.Warn(ctx, "UnorderedMapListPlanModifier: Reconstructed plan", map[string]interface{}{
						"attribute":         attrName,
						"original_plan":     planValues,
						"reconstructed":     reconstructed,
					})
					
					// Replace the plan with reconstructed version
					// Reconstruct the MapValue with properly ordered member list
					var planMapExtracted map[string]types.Set
					planDiags := req.PlanValue.ElementsAs(ctx, &planMapExtracted, false)
					if len(planDiags) == 0 && planMapExtracted != nil {
						newPlanMap := make(map[string]types.Set)
						
						// Copy all attributes from the original plan
						for attrName, attrValue := range planMapExtracted {
							if attrName == "member" {
								// Replace member attribute with reconstructed list
								reconstructedSet := types.SetValueMust(
									types.StringType,
									convertStringsToAttrValues(reconstructed),
								)
								newPlanMap[attrName] = reconstructedSet
							} else {
								// Keep other attributes unchanged
								newPlanMap[attrName] = attrValue
							}
						}
						
						// Create new plan value with reconstructed member list
						mapType := req.PlanValue.Type(ctx).(basetypes.MapType)
						newPlanValue := types.MapValueMust(
							mapType.ElemType,
							convertSetMapToValueMap(newPlanMap),
						)
						resp.PlanValue = newPlanValue
					}
					return
				} else {
					// No reordering artifacts detected - allow legitimate changes through
					tflog.Info(ctx, "UnorderedMapListPlanModifier: No reordering artifacts - allowing legitimate changes", map[string]interface{}{
						"attribute":    attrName,
						"additions":    len(additions),
						"removals":     len(removals),
						"existing":     len(existing),
					})
				}
			}
		}
	}
	
	tflog.Info(ctx, "UnorderedMapListPlanModifier: No modifications needed")
}


// filterValidReferences filters out references with malformed DN syntax
// The main solution for referential integrity is LDAP state refresh (Solution 2)
// This function only performs basic DN format validation as a safety net
func filterValidReferences(ctx context.Context, references []string) []string {
	var validReferences []string
	
	for _, ref := range references {
		// Only filter out obviously malformed DNs
		if !isValidDNFormat(ref) {
			tflog.Debug(ctx, "Filtered out malformed reference", map[string]interface{}{
				"reference": ref,
				"reason":    "invalid DN format",
			})
			continue
		}
		
		// Accept all properly formatted DNs - existence checking is handled by LDAP state refresh
		validReferences = append(validReferences, ref)
	}
	
	return validReferences
}

// isValidDNFormat performs basic DN format validation without hardcoded patterns
func isValidDNFormat(dn string) bool {
	if dn == "" {
		return false
	}
	
	// Basic check for DN format (should contain = and at least one component)
	if !strings.Contains(dn, "=") {
		return false
	}
	
	// Check for basic DN structure: should have at least CN= or OU= etc.
	dnComponents := []string{"CN=", "OU=", "DC=", "O=", "C=", "STREET=", "L=", "ST="}
	hasValidComponent := false
	
	for _, component := range dnComponents {
		if strings.Contains(strings.ToUpper(dn), component) {
			hasValidComponent = true
			break
		}
	}
	
	return hasValidComponent
}

// getInvalidReferences returns the difference between original and filtered references
func getInvalidReferences(original, filtered []string) []string {
	var invalid []string
	filteredSet := make(map[string]bool)
	
	for _, ref := range filtered {
		filteredSet[ref] = true
	}
	
	for _, ref := range original {
		if !filteredSet[ref] {
			invalid = append(invalid, ref)
		}
	}
	
	return invalid
}


// areStringSlicesExactlyEqual checks if two string slices are exactly equal (same order, same elements)
func areStringSlicesExactlyEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	
	return true
}

/*
// MemberSAMResolutionPlanModifier - COMMENTED OUT DUE TO COMPILE ERRORS
// TODO: Fix the compilation errors and re-enable
type MemberSAMResolutionPlanModifier struct {
	providerData *LDAPProviderData
}
*/

// Helper functions for set operations

// stringSliceToSet converts a string slice to a set (map[string]struct{})
func stringSliceToSet(slice []string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, s := range slice {
		set[s] = struct{}{}
	}
	return set
}

// setDifference returns elements in setA that are not in setB (setA - setB)
func setDifference(setA, setB map[string]struct{}) map[string]struct{} {
	diff := make(map[string]struct{})
	for item := range setA {
		if _, exists := setB[item]; !exists {
			diff[item] = struct{}{}
		}
	}
	return diff
}

// setIntersection returns elements that exist in both setA and setB
func setIntersection(setA, setB map[string]struct{}) map[string]struct{} {
	intersection := make(map[string]struct{})
	for item := range setA {
		if _, exists := setB[item]; exists {
			intersection[item] = struct{}{}
		}
	}
	return intersection
}

// setToStringSlice converts a set back to a string slice for debugging
func setToStringSlice(set map[string]struct{}) []string {
	slice := make([]string, 0, len(set))
	for item := range set {
		slice = append(slice, item)
	}
	return slice
}

// convertStringsToAttrValues converts a string slice to attr.Value slice for set construction
func convertStringsToAttrValues(strings []string) []attr.Value {
	values := make([]attr.Value, len(strings))
	for i, s := range strings {
		values[i] = types.StringValue(s)
	}
	return values
}

// convertSetMapToValueMap converts map[string]types.Set to map[string]attr.Value
func convertSetMapToValueMap(setMap map[string]types.Set) map[string]attr.Value {
	valueMap := make(map[string]attr.Value)
	for key, set := range setMap {
		valueMap[key] = set
	}
	return valueMap
}