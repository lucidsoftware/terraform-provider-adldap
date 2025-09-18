package provider

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-ldap/ldap/v3"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/thoas/go-funk"
)

var _ resource.Resource = &LDAPObjectResource{}
var _ resource.ResourceWithImportState = &LDAPObjectResource{}
var _ resource.ResourceWithModifyPlan = &LDAPObjectResource{}
var _ resource.ResourceWithConfigure = &LDAPObjectResource{}

func NewLDAPObjectResource() resource.Resource {
	return &LDAPObjectResource{}
}

type LDAPObjectResource struct {
	providerData *LDAPProviderData
}

type LDAPObjectResourceModel struct {
	ID            types.String `tfsdk:"id"`
	DN            types.String `tfsdk:"dn"`
	ObjectClasses types.List   `tfsdk:"object_classes"`
	Attributes    types.Map    `tfsdk:"attributes"`
	IgnoreChanges types.List   `tfsdk:"ignore_changes"`
}

func (L *LDAPObjectResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_object"
}

func (L *LDAPObjectResource) Schema(_ context.Context, _ resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Generic LDAP object resource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resource identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"dn": schema.StringAttribute{
				MarkdownDescription: "DN of this ldap object",
				Required:            true,
			},
			"object_classes": schema.ListAttribute{
				MarkdownDescription: "A list of classes this object implements",
				ElementType:         types.StringType,
				Required:            true,
			},
			"attributes": schema.MapAttribute{
				MarkdownDescription: "The definition of an attribute, the name defines the type of the attribute",
				Optional:            true,
				ElementType:         types.SetType{ElemType: types.StringType},
				PlanModifiers: []planmodifier.Map{
					NewUnorderedMapListPlanModifier(),
				},
			},
			"ignore_changes": schema.ListAttribute{
				MarkdownDescription: "A list of types for which changes are ignored",
				Optional:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (L *LDAPObjectResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	if providerData, ok := request.ProviderData.(*LDAPProviderData); !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *LDAPProviderData, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)

		return
	} else {
		L.providerData = providerData
	}
}

func (L *LDAPObjectResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var data *LDAPObjectResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	if err := L.addLdapEntry(ctx, data, &response.Diagnostics); err != nil {
		response.Diagnostics.AddError(
			"Can not add resource",
			fmt.Sprintf("LDAP server reported: %s", err),
		)
		return
	}
	
	data.ID = data.DN
	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (L *LDAPObjectResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var data *LDAPObjectResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading entry", map[string]interface{}{"dn": data.DN.ValueString()})
	
	// Check for LDAP connection before attempting to read
	if L.providerData == nil || L.providerData.Conn == nil {
		response.Diagnostics.AddError(
			"Cannot read entry - no LDAP connection",
			"LDAP provider data or connection is nil",
		)
		return
	}
	
	if entry, err := GetEntry(L.providerData.Conn, data.DN.ValueString()); err != nil {
		response.Diagnostics.AddError(
			"Can not read entry",
			err.Error(),
		)
	} else {
		response.State.SetAttribute(ctx, path.Root("dn"), entry.DN)
		ctx = MaskAttributesFromArray(ctx, entry.Attributes)
		for _, attribute := range entry.Attributes {
			if attribute.Name == "objectClass" {
				response.State.SetAttribute(ctx, path.Root("object_classes"), attribute.Values)
			} else if !L.isIgnored(ctx, attribute.Name, data, response.Diagnostics) && !isSystemAttribute(attribute.Name) {
				encodedValues := encodeAttributeValues(attribute.Name, attribute.Values)
				// Convert string slice to set for new schema
				setValue, diags := types.SetValueFrom(ctx, types.StringType, encodedValues)
				response.Diagnostics.Append(diags...)
				if !response.Diagnostics.HasError() {
					response.State.SetAttribute(ctx, path.Root("attributes").AtMapKey(attribute.Name), setValue)
				}
			}
		}
		
		tflog.Debug(ctx, "Read entry", map[string]interface{}{"entry": ToLDIF(entry)})
	}
}

func (L *LDAPObjectResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var stateData *LDAPObjectResourceModel
	var planData *LDAPObjectResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &stateData)...)
	response.Diagnostics.Append(request.Plan.Get(ctx, &planData)...)
	if response.Diagnostics.HasError() {
		return
	}

	// Recreate object if DN changed
	if stateData.DN.ValueString() != planData.DN.ValueString() {
		tflog.Warn(ctx, "Recreating entry because the DN changed", map[string]interface{}{
			"oldDn": stateData.DN.ValueString(),
			"dn":    planData.DN.ValueString(),
		})

		if err := L.providerData.Conn.Del(ldap.NewDelRequest(stateData.DN.ValueString(), []ldap.Control{})); err != nil {
			response.Diagnostics.AddError(
				"Can not delete old DN entry",
				fmt.Sprintf("Trying to delete entry of old DN returned: %s", err),
			)
			return
		}
		if err := L.addLdapEntry(ctx, planData, &response.Diagnostics); err != nil {
			response.Diagnostics.AddError(
				"Can not add resource",
				fmt.Sprintf("LDAP server reported: %s", err),
			)
			return
		}
	} else {
		r := ldap.NewModifyRequest(planData.DN.ValueString(), []ldap.Control{})

		var stateObjectClasses []string
		response.Diagnostics.Append(stateData.ObjectClasses.ElementsAs(ctx, &stateObjectClasses, false)...)
		var planObjectClasses []string
		response.Diagnostics.Append(planData.ObjectClasses.ElementsAs(ctx, &planObjectClasses, false)...)

		var classesToAdd []string
		for _, class := range planObjectClasses {
			if funk.IndexOf(stateObjectClasses, class) == -1 {
				classesToAdd = append(classesToAdd, class)
			}
		}

		if len(classesToAdd) > 0 {
			r.Add("objectClass", classesToAdd)
		}

		var stateAttributes map[string][]string
		response.Diagnostics.Append(stateData.Attributes.ElementsAs(ctx, &stateAttributes, false)...)
		var planAttributes map[string][]string
		response.Diagnostics.Append(planData.Attributes.ElementsAs(ctx, &planAttributes, false)...)

		// No member attribute processing - handled at module level via data sources

		ctx = MaskAttributes(ctx, stateAttributes)
		for attributeType, stateValues := range stateAttributes {
			if L.isIgnored(ctx, attributeType, stateData, response.Diagnostics) || isSystemAttribute(attributeType) || isTerraformOnlyAttribute(attributeType) {
				continue
			}
			// state attribute is in the plan, compare the values
			if planValues, exists := planAttributes[attributeType]; exists {
				// For binary attributes, encode the plan values for comparison
				encodedPlanValues := encodeAttributeValues(attributeType, planValues)
				
				valuesChanged := false
				for _, stateValue := range stateValues {
					if !funk.ContainsString(encodedPlanValues, stateValue) {
						valuesChanged = true
					}
				}
				for _, encodedPlanValue := range encodedPlanValues {
					if !funk.ContainsString(stateValues, encodedPlanValue) {
						valuesChanged = true
					}
				}
				if valuesChanged {
					tflog.Debug(ctx, "Changing attribute", map[string]interface{}{
						"type":   attributeType,
						"values": planValues,
					})
					// Use decoded values for LDAP operations
					decodedValues := decodeAttributeValues(attributeType, encodedPlanValues)
					r.Replace(attributeType, decodedValues)
				}
			} else {
				tflog.Debug(ctx, "Removing attribute", map[string]interface{}{
					"type": attributeType,
				})
				r.Delete(attributeType, []string{})
			}
		}
		for attributeType, values := range planAttributes {
			if L.isIgnored(ctx, attributeType, planData, response.Diagnostics) || isSystemAttribute(attributeType) || isTerraformOnlyAttribute(attributeType) {
				continue
			}
			if _, exists := stateAttributes[attributeType]; !exists {
				tflog.Debug(ctx, "Adding attribute", map[string]interface{}{
					"type": attributeType,
				})
				// For binary attributes, decode the values before adding to LDAP
				decodedValues := decodeAttributeValues(attributeType, values)
				r.Add(attributeType, decodedValues)
			}
		}
		
		// DIAGNOSTIC: Log exactly what modifications are being attempted
		tflog.Warn(ctx, "DIAGNOSTIC: About to send LDAP ModifyRequest", map[string]interface{}{
			"dn": planData.DN.ValueString(),
			"modify_request_details": fmt.Sprintf("%+v", r),
		})
		
		// DIAGNOSTIC: Extract and log individual modify operations 
		var modifyOps []map[string]interface{}
		for _, change := range r.Changes {
			modifyOps = append(modifyOps, map[string]interface{}{
				"operation": change.Operation,
				"attribute": change.Modification.Type,
				"values": change.Modification.Vals,
			})
		}
		tflog.Warn(ctx, "DIAGNOSTIC: ModifyRequest operations breakdown", map[string]interface{}{
			"dn": planData.DN.ValueString(),
			"operations": modifyOps,
		})
		
		// DEFENSIVE: Final safety check - remove any system attributes from modify request
		var filteredChanges []ldap.Change
		for _, change := range r.Changes {
			if isSystemAttribute(change.Modification.Type) {
				tflog.Error(ctx, "CRITICAL: Attempted to modify system attribute - BLOCKING", map[string]interface{}{
					"dn": planData.DN.ValueString(),
					"system_attribute": change.Modification.Type,
					"operation": change.Operation,
					"values": change.Modification.Vals,
				})
				// Skip this change to prevent LDAP error
			} else {
				filteredChanges = append(filteredChanges, change)
			}
		}
		r.Changes = filteredChanges
		
		// DIAGNOSTIC: Log final filtered request
		if len(filteredChanges) != len(modifyOps) {
			tflog.Warn(ctx, "DIAGNOSTIC: Filtered out system attributes from ModifyRequest", map[string]interface{}{
				"dn": planData.DN.ValueString(),
				"original_operations": len(modifyOps),
				"filtered_operations": len(filteredChanges),
			})
		}
		
		// Fix for empty LDAP ModifyRequest - AD rejects empty modifications with Error 53
		if len(r.Changes) == 0 {
			tflog.Debug(ctx, "No LDAP changes needed - skipping modify operation", map[string]interface{}{
				"dn": planData.DN.ValueString(),
			})
			// Skip empty modifications that AD rejects with "Unwilling To Perform"
		} else {
			if err := L.providerData.Conn.Modify(r); err != nil {
				response.Diagnostics.AddError(
					"Can not modify entry",
					fmt.Sprintf("LDAP server reported: %s\nDN: %s\nOperations attempted: %+v", err, planData.DN.ValueString(), modifyOps),
				)
				return
			}
		}
	}
	
	planData.ID = planData.DN
	response.Diagnostics.Append(response.State.Set(ctx, &planData)...)
}

func (L *LDAPObjectResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var stateData *LDAPObjectResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &stateData)...)
	if response.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting entry", map[string]interface{}{"dn": stateData.DN.ValueString()})
	if err := L.providerData.Conn.Del(ldap.NewDelRequest(stateData.DN.ValueString(), []ldap.Control{})); err != nil {
		response.Diagnostics.AddError(
			"Can not delete entry",
			fmt.Sprintf("Trying to delete entry returned: %s", err),
		)
		return
	}
}

func (L *LDAPObjectResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	tflog.Info(ctx, "Importing entry", map[string]interface{}{"dn": request.ID})
	if entry, err := GetEntry(L.providerData.Conn, request.ID); err != nil {
		response.Diagnostics.AddError(
			"Can not read entry",
			err.Error(),
		)
	} else {
		resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
		ctx = MaskAttributesFromArray(ctx, entry.Attributes)
		response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("dn"), entry.DN)...)
		for _, attribute := range entry.Attributes {
			if attribute.Name == "objectClass" {
				response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("object_classes"), attribute.Values)...)
			} else if !isSystemAttribute(attribute.Name) {
				encodedValues := encodeAttributeValues(attribute.Name, attribute.Values)
				response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("attributes").AtMapKey(attribute.Name), encodedValues)...)
			}
		}
		
		tflog.Debug(ctx, "Imported entry", map[string]interface{}{
			"entry": ToLDIF(entry),
		})
	}
}

func (L *LDAPObjectResource) ModifyPlan(ctx context.Context, request resource.ModifyPlanRequest, response *resource.ModifyPlanResponse) {
	tflog.Info(ctx, "ModifyPlan: Starting input transformation", map[string]interface{}{
		"operation": "input_transformation",
	})
	
	var stateData *LDAPObjectResourceModel
	var planData *LDAPObjectResourceModel

	response.Diagnostics.Append(request.State.Get(ctx, &stateData)...)
	response.Diagnostics.Append(request.Plan.Get(ctx, &planData)...)
	if response.Diagnostics.HasError() {
		return
	}

	// Handle create/delete operations
	if planData == nil {
		tflog.Info(ctx, "ModifyPlan: Delete operation - no plan modifications needed")
		return
	}

	// Handle DN changes (affects ID)
	if stateData != nil && stateData.DN.ValueString() != planData.DN.ValueString() {
		response.Diagnostics.Append(response.Plan.SetAttribute(ctx, path.Root("id"), types.StringUnknown())...)
		if response.Diagnostics.HasError() {
			return
		}
	}

	// No transformation in ModifyPlan - transformation happens in CREATE/UPDATE operations only
	// This avoids Terraform plan validation issues with Optional+Computed attributes

	// Handle system attributes and ignored attributes
	if stateData != nil {
		var stateAttributes map[string][]string
		response.Diagnostics.Append(stateData.Attributes.ElementsAs(ctx, &stateAttributes, false)...)
		if !response.Diagnostics.HasError() && stateAttributes != nil {
			// Get plan attributes for system attribute handling
			var planAttributes map[string][]string
			response.Diagnostics.Append(planData.Attributes.ElementsAs(ctx, &planAttributes, false)...)
			if !response.Diagnostics.HasError() && planAttributes != nil {
				for attributeType := range planAttributes {
					if L.isIgnored(ctx, attributeType, planData, response.Diagnostics) || isSystemAttribute(attributeType) {
						if stateValue, exists := stateAttributes[attributeType]; exists {
							response.Plan.SetAttribute(ctx, path.Root("attributes").AtMapKey(attributeType), stateValue)
						}
					}
				}
			}

			for attributeType := range stateAttributes {
				if L.isIgnored(ctx, attributeType, planData, response.Diagnostics) || isSystemAttribute(attributeType) {
					// Re-add attributes to the plan that were ignored and removed to not manage them
					response.Plan.SetAttribute(ctx, path.Root("attributes").AtMapKey(attributeType), stateAttributes[attributeType])
				}
			}
		}
	}
}

// isReferentialAttribute determines if an LDAP attribute contains references to other objects
func isReferentialAttribute(attributeName string) bool {
	referentialAttributes := []string{"member", "uniqueMember", "roleOccupant", "owner"}
	for _, attr := range referentialAttributes {
		if attr == attributeName {
			return true
		}
	}
	return false
}

// hasReferentialAttributeChanges checks if the plan contains changes to attributes that could be affected by LDAP referential integrity
func (L *LDAPObjectResource) hasReferentialAttributeChanges(ctx context.Context, stateAttributes, planAttributes map[string][]string) bool {
	for _, attrName := range []string{"member", "uniqueMember", "roleOccupant", "owner"} {
		stateValues := stateAttributes[attrName]
		planValues := planAttributes[attrName]
		
		// Check if there are differences in referential attributes
		if !areStringSlicesEqualAsSet(stateValues, planValues) {
			tflog.Debug(ctx, "Found referential attribute change", map[string]interface{}{
				"attribute": attrName,
				"state":     stateValues,
				"plan":      planValues,
			})
			return true
		}
	}
	
	return false
}

// refreshLDAPState refreshes the LDAP state for an object by reading directly from LDAP
func (L *LDAPObjectResource) refreshLDAPState(ctx context.Context, dn string) (map[string][]string, error) {
	entry, err := GetEntry(L.providerData.Conn, dn)
	if err != nil {
		return nil, fmt.Errorf("failed to read LDAP entry %s: %w", dn, err)
	}

	refreshedAttributes := make(map[string][]string)
	ctx = MaskAttributesFromArray(ctx, entry.Attributes)
	
	for _, attribute := range entry.Attributes {
		if attribute.Name != "objectClass" && !isSystemAttribute(attribute.Name) {
			encodedValues := encodeAttributeValues(attribute.Name, attribute.Values)
			refreshedAttributes[attribute.Name] = encodedValues
		}
	}
	
	tflog.Debug(ctx, "Refreshed LDAP state", map[string]interface{}{
		"dn":         dn,
		"attributes": refreshedAttributes,
	})
	
	return refreshedAttributes, nil
}

func (L *LDAPObjectResource) addLdapEntry(ctx context.Context, data *LDAPObjectResourceModel, diagnostics *diag.Diagnostics) error {
	var objectClasses []string
	diagnostics.Append(data.ObjectClasses.ElementsAs(ctx, &objectClasses, false)...)
	if diagnostics.HasError() {
		return errors.New("error converting data")
	}

	// Extract attributes as sets, then convert to string slices for LDAP operations
	var attributeSets map[string]types.Set
	diagnostics.Append(data.Attributes.ElementsAs(ctx, &attributeSets, false)...)
	if diagnostics.HasError() {
		return errors.New("error converting attribute sets")
	}
	
	// Convert sets to string slices for LDAP library compatibility
	attributes := make(map[string][]string)
	for attrName, attrSet := range attributeSets {
		if !attrSet.IsNull() && !attrSet.IsUnknown() {
			var stringValues []string
			diagnostics.Append(attrSet.ElementsAs(ctx, &stringValues, false)...)
			if diagnostics.HasError() {
				return errors.New("error converting set to string slice for attribute: " + attrName)
			}
			attributes[attrName] = stringValues
		}
	}

	// No member attribute processing - handled at module level via data sources

	tflog.Info(ctx, "Adding new item", map[string]interface{}{
		"dn":          data.DN.ValueString(),
		"objectClass": objectClasses,
		"attributes":  attributes,
	})
	a := ldap.NewAddRequest(data.DN.ValueString(), []ldap.Control{})
	a.Attribute("objectClass", objectClasses)

	ctx = MaskAttributes(ctx, attributes)

	for attributeType, values := range attributes {
		if isSystemAttribute(attributeType) || isTerraformOnlyAttribute(attributeType) {
			continue
		}
		// For binary attributes, decode the values before adding to LDAP
		decodedValues := decodeAttributeValues(attributeType, values)
		a.Attribute(attributeType, decodedValues)
	}

	tflog.Debug(ctx, "Adding LDAP entry", map[string]interface{}{
		"entry": ToLDIF(a),
	})

	return L.providerData.Conn.Add(a)
}

func (L *LDAPObjectResource) isIgnored(ctx context.Context, attributeType string, data *LDAPObjectResourceModel, diagnostics diag.Diagnostics) bool {
	var ignoredAttributes []string
	diagnostics.Append(data.IgnoreChanges.ElementsAs(ctx, &ignoredAttributes, false)...)

	if diagnostics.HasError() {
		return false
	}
	return funk.ContainsString(ignoredAttributes, attributeType)
}

// resolveCNtoDN searches for a user by CN and returns their current DN
func (L *LDAPObjectResource) resolveCNtoDN(ctx context.Context, cn string) (string, error) {
	if L.providerData.UsersOU == "" {
		return "", fmt.Errorf("users_ou not configured in provider - cannot resolve CN '%s'", cn)
	}

	searchBases := []string{L.providerData.UsersOU}
	
	// Add disabled users OU if it's different from users OU
	if L.providerData.DisabledUsersOU != "" && L.providerData.DisabledUsersOU != L.providerData.UsersOU {
		searchBases = append(searchBases, L.providerData.DisabledUsersOU)
	}

	for _, baseDN := range searchBases {
		tflog.Debug(ctx, "Searching for user by CN", map[string]interface{}{
			"cn": cn,
			"baseDN": baseDN,
		})

		searchRequest := ldap.NewSearchRequest(
			baseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			fmt.Sprintf("(&(objectClass=user)(cn=%s))", ldap.EscapeFilter(cn)),
			[]string{"distinguishedName"},
			nil,
		)

		sr, err := L.providerData.Conn.Search(searchRequest)
		if err != nil {
			tflog.Warn(ctx, "Error searching for user", map[string]interface{}{
				"cn": cn,
				"baseDN": baseDN,
				"error": err.Error(),
			})
			continue
		}

		if len(sr.Entries) == 1 {
			dn := sr.Entries[0].DN
			tflog.Debug(ctx, "Resolved CN to DN", map[string]interface{}{
				"cn": cn,
				"dn": dn,
			})
			return dn, nil
		} else if len(sr.Entries) > 1 {
			return "", fmt.Errorf("multiple users found with CN '%s' in %s", cn, baseDN)
		}
	}

	return "", fmt.Errorf("user with CN '%s' not found in configured search bases", cn)
}

// resolveMemberCNs converts a list of CNs to their current DNs
func (L *LDAPObjectResource) resolveMemberCNs(ctx context.Context, memberCNs []string) ([]string, error) {
	var resolvedDNs []string
	
	for _, cn := range memberCNs {
		dn, err := L.resolveCNtoDN(ctx, cn)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve CN '%s': %w", cn, err)
		}
		resolvedDNs = append(resolvedDNs, dn)
	}
	
	return resolvedDNs, nil
}

// resolveSAMToDN searches for a user by sAMAccountName and returns their current DN
func (L *LDAPObjectResource) resolveSAMToDN(ctx context.Context, samAccountName string) (string, error) {
	tflog.Info(ctx, "=== resolveSAMToDN ENTRY ===", map[string]interface{}{
		"samAccountName": samAccountName,
		"users_ou": L.providerData.UsersOU,
		"disabled_users_ou": L.providerData.DisabledUsersOU,
	})
	
	if L.providerData.UsersOU == "" {
		tflog.Error(ctx, "users_ou not configured in provider", map[string]interface{}{
			"samAccountName": samAccountName,
		})
		return "", fmt.Errorf("users_ou not configured in provider - cannot resolve sAMAccountName '%s'", samAccountName)
	}

	searchBases := []string{L.providerData.UsersOU}
	
	// Add disabled users OU if it's different from users OU
	if L.providerData.DisabledUsersOU != "" && L.providerData.DisabledUsersOU != L.providerData.UsersOU {
		searchBases = append(searchBases, L.providerData.DisabledUsersOU)
	}
	
	tflog.Info(ctx, "Will search these bases for SAM", map[string]interface{}{
		"samAccountName": samAccountName,
		"searchBases": searchBases,
	})

	for _, baseDN := range searchBases {
		tflog.Debug(ctx, "Searching for user by sAMAccountName", map[string]interface{}{
			"sAMAccountName": samAccountName,
			"baseDN": baseDN,
		})

		// Use more efficient objectCategory=Person instead of objectClass=user
		searchRequest := ldap.NewSearchRequest(
			baseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			fmt.Sprintf("(&(objectCategory=Person)(sAMAccountName=%s))", ldap.EscapeFilter(samAccountName)),
			[]string{"distinguishedName"},
			nil,
		)

		sr, err := L.providerData.Conn.Search(searchRequest)
		if err != nil {
			tflog.Warn(ctx, "Error searching for user by sAMAccountName", map[string]interface{}{
				"sAMAccountName": samAccountName,
				"baseDN": baseDN,
				"error": err.Error(),
			})
			continue
		}

		tflog.Info(ctx, "LDAP search completed", map[string]interface{}{
			"sAMAccountName": samAccountName,
			"baseDN": baseDN,
			"entries_found": len(sr.Entries),
		})

		if len(sr.Entries) == 1 {
			dn := sr.Entries[0].DN
			tflog.Info(ctx, "Successfully resolved sAMAccountName to DN", map[string]interface{}{
				"sAMAccountName": samAccountName,
				"dn": dn,
				"baseDN": baseDN,
			})
			return dn, nil
		} else if len(sr.Entries) > 1 {
			tflog.Error(ctx, "Multiple users found with same sAMAccountName", map[string]interface{}{
				"sAMAccountName": samAccountName,
				"baseDN": baseDN,
				"count": len(sr.Entries),
			})
			return "", fmt.Errorf("multiple users found with sAMAccountName '%s' in %s", samAccountName, baseDN)
		} else {
			tflog.Info(ctx, "No user found in this search base", map[string]interface{}{
				"sAMAccountName": samAccountName,
				"baseDN": baseDN,
			})
		}
	}

	return "", fmt.Errorf("user with sAMAccountName '%s' not found in configured search bases", samAccountName)
}

// resolveMemberSAMs converts a list of sAMAccountNames to their current DNs
func (L *LDAPObjectResource) resolveMemberSAMs(ctx context.Context, memberSAMs []string) ([]string, error) {
	var resolvedDNs []string
	
	tflog.Info(ctx, "=== resolveMemberSAMs ENTRY ===", map[string]interface{}{
		"memberSAMs": memberSAMs,
		"count": len(memberSAMs),
	})
	
	for i, sam := range memberSAMs {
		tflog.Info(ctx, "Resolving individual SAM", map[string]interface{}{
			"sam": sam,
			"index": i,
		})
		
		dn, err := L.resolveSAMToDN(ctx, sam)
		
		tflog.Info(ctx, "Individual SAM resolution result", map[string]interface{}{
			"sam": sam,
			"dn": dn,
			"error": err,
		})
		
		if err != nil {
			tflog.Error(ctx, "Failed to resolve SAM to DN", map[string]interface{}{
				"sam": sam,
				"error": err.Error(),
			})
			return nil, fmt.Errorf("failed to resolve sAMAccountName '%s': %w", sam, err)
		}
		resolvedDNs = append(resolvedDNs, dn)
	}
	
	tflog.Info(ctx, "=== resolveMemberSAMs EXIT ===", map[string]interface{}{
		"input_count": len(memberSAMs),
		"resolved_count": len(resolvedDNs),
		"resolved_dns": resolvedDNs,
	})
	
	return resolvedDNs, nil
}
