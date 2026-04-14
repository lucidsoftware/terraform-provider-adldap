package provider

import (
	"context"
	"fmt"
	"github.com/go-ldap/ldap/v3"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &LDAPSearchDataSource{}
var _ datasource.DataSourceWithConfigure = &LDAPSearchDataSource{}

func NewLDAPSearchDataSource() datasource.DataSource {
	return &LDAPSearchDataSource{}
}

type LDAPSearchDataSource struct {
	providerData *LDAPProviderData
}

type LDAPSearchDatasourceModel struct {
	Id                   types.String `tfsdk:"id"`
	BaseDN               types.String `tfsdk:"base_dn"`
	Scope                types.String `tfsdk:"scope"`
	Filter               types.String `tfsdk:"filter"`
	Results              types.List   `tfsdk:"results"`
	AdditionalAttributes types.Set    `tfsdk:"additional_attributes"`
}

func (L *LDAPSearchDataSource) Metadata(_ context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_search"
}

func (L *LDAPSearchDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Generic LDAP search datasource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Datasource identifier",
			},
			"base_dn": schema.StringAttribute{
				MarkdownDescription: "Base DN to use to search for LDAP objects",
				Optional:            true,
			},
			"scope": schema.StringAttribute{
				MarkdownDescription: "Scope to use to search for LDAP objects",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("baseObject", "singleLevel", "wholeSubtree"),
				},
			},
			"filter": schema.StringAttribute{
				MarkdownDescription: "Filter to search for LDAP objects with",
				Optional:            true,
			},
			"additional_attributes": schema.SetAttribute{
				MarkdownDescription: "Any additional attributes to request, such as constructed or operational attributes",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"results": schema.ListAttribute{
				MarkdownDescription: "List of LDAP objects returned from the search",
				Computed:            true,
				ElementType: types.MapType{
					ElemType: types.ListType{ElemType: types.StringType},
				},
			},
		},
	}
}

func (L *LDAPSearchDataSource) Configure(_ context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	if providerData, ok := request.ProviderData.(*LDAPProviderData); !ok {
		response.Diagnostics.AddError(
			"Unexpected Datasource Configure Type",
			fmt.Sprintf("Expected *LDAPProviderData, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)

		return
	} else {
		L.providerData = providerData
	}
}

func (L *LDAPSearchDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data LDAPSearchDatasourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)

	var additionalAttributes []string
	response.Diagnostics.Append(data.AdditionalAttributes.ElementsAs(ctx, &additionalAttributes, false)...)

	var scope int

	if data.Scope.IsUnknown() || data.Scope.IsNull() {
		scope = ldap.ScopeBaseObject
	} else {
		switch data.Scope.ValueString() {
		case "baseObject":
			scope = ldap.ScopeBaseObject
		case "singleLevel":
			scope = ldap.ScopeSingleLevel
		case "wholeSubtree":
			scope = ldap.ScopeWholeSubtree
		}
	}

	var filter string

	if data.Filter.IsUnknown() || data.Filter.IsNull() {
		filter = "(&)"
	} else {
		filter = data.Filter.ValueString()
	}

	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("id"), fmt.Sprintf("%s/%s/%s", data.BaseDN.ValueString(), data.Scope.ValueString(), filter))...)
	if response.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Searching for ldap entries", map[string]interface{}{
		"baseDN":               data.BaseDN.ValueString(),
		"scope":                scope,
		"filter":               filter,
		"additionalAttributes": additionalAttributes,
	})

	s := ldap.NewSearchRequest(data.BaseDN.ValueString(), scope, 0, 0, 0, false, filter, append(additionalAttributes, "*"), []ldap.Control{})

	if result, err := L.providerData.Conn.Search(s); err != nil {
		response.Diagnostics.AddError(
			"Can not read entry",
			err.Error(),
		)
	} else {
		// Build complete results structure first, then set atomically
		var results []map[string][]string
		
		for _, entry := range result.Entries {
			ctx := MaskAttributesFromArray(ctx, entry.Attributes)
			tflog.Debug(ctx, "Found entry", map[string]interface{}{
				"entry": ToLDIF(entry),
			})
			
			// Build complete entry map
			entryMap := make(map[string][]string)
			
			// Add DN first - with debug logging
			entryMap["dn"] = []string{entry.DN}
			tflog.Debug(ctx, "Setting DN field", map[string]interface{}{
				"dn_value": entry.DN,
				"entry_map_keys_after_dn": len(entryMap),
			})
			
			// Add all other attributes
			for _, attribute := range entry.Attributes {
				if !isSystemAttribute(attribute.Name) {
					encodedValues := encodeAttributeValues(attribute.Name, attribute.Values)
					entryMap[attribute.Name] = encodedValues
				}
			}
			
			tflog.Debug(ctx, "Complete entry map built", map[string]interface{}{
				"total_keys": len(entryMap),
				"has_dn_key": entryMap["dn"] != nil,
				"dn_values": entryMap["dn"],
			})
			
			results = append(results, entryMap)
		}
		
		// Set complete results structure once
		response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("results"), results)...)
	}
}
