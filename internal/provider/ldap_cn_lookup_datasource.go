package provider

import (
	"context"
	"fmt"
	"github.com/go-ldap/ldap/v3"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &LDAPCNLookupDataSource{}
var _ datasource.DataSourceWithConfigure = &LDAPCNLookupDataSource{}

func NewLDAPCNLookupDataSource() datasource.DataSource {
	return &LDAPCNLookupDataSource{}
}

type LDAPCNLookupDataSource struct {
	providerData *LDAPProviderData
}

type LDAPCNLookupDatasourceModel struct {
	Id             types.String `tfsdk:"id"`
	CommonName     types.String `tfsdk:"common_name"`
	BaseDN         types.String `tfsdk:"base_dn"`
	DisabledBaseDN types.String `tfsdk:"disabled_base_dn"`
	DN             types.String `tfsdk:"dn"`
	Found          types.Bool   `tfsdk:"found"`
	RequireFound   types.Bool   `tfsdk:"require_found"`
}

func (L *LDAPCNLookupDataSource) Metadata(_ context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_cn_lookup"
}

func (L *LDAPCNLookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "LDAP Common Name to DN lookup datasource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Datasource identifier",
			},
			"common_name": schema.StringAttribute{
				MarkdownDescription: "Common name (CN) to lookup",
				Required:            true,
			},
			"base_dn": schema.StringAttribute{
				MarkdownDescription: "Base DN to search for the user",
				Required:            true,
			},
			"disabled_base_dn": schema.StringAttribute{
				MarkdownDescription: "Optional base DN to search for disabled users",
				Optional:            true,
			},
			"dn": schema.StringAttribute{
				MarkdownDescription: "Distinguished name of the user (empty if not found)",
				Computed:            true,
			},
			"found": schema.BoolAttribute{
				MarkdownDescription: "Whether the user was found",
				Computed:            true,
			},
			"require_found": schema.BoolAttribute{
				MarkdownDescription: "Whether to return an error if the user is not found (default: false)",
				Optional:            true,
			},
		},
	}
}

func (L *LDAPCNLookupDataSource) Configure(_ context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (L *LDAPCNLookupDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data LDAPCNLookupDatasourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)

	if response.Diagnostics.HasError() {
		return
	}

	commonName := data.CommonName.ValueString()
	baseDN := data.BaseDN.ValueString()
	disabledBaseDN := ""
	if !data.DisabledBaseDN.IsNull() && !data.DisabledBaseDN.IsUnknown() {
		disabledBaseDN = data.DisabledBaseDN.ValueString()
	}
	
	// Set the ID for this datasource
	data.Id = types.StringValue(fmt.Sprintf("cn_lookup_%s", commonName))

	tflog.Debug(ctx, "Looking up Common Name", map[string]interface{}{
		"common_name":       commonName,
		"base_dn":          baseDN,
		"disabled_base_dn": disabledBaseDN,
	})

	// Create cache key for CN lookup (includes base DNs to prevent cache poisoning)
	rawKey := fmt.Sprintf("cn:%s:%s:%s", commonName, baseDN, disabledBaseDN)
	
	// Perform cached lookup
	dn, found := L.providerData.cachedUserLookup(ctx, rawKey, func() (string, bool) {
		// Search for user by CN
		filter := fmt.Sprintf("(&(objectCategory=Person)(cn=%s))", commonName)
		
		// Try primary base DN first
		if dn := L.searchForUser(ctx, baseDN, filter); dn != "" {
			return dn, true
		}

		// Try disabled base DN if configured
		if disabledBaseDN != "" {
			if dn := L.searchForUser(ctx, disabledBaseDN, filter); dn != "" {
				return dn, true
			}
		}

		// User not found
		return "", false
	})
	
	// Set the results
	data.DN = types.StringValue(dn)
	data.Found = types.BoolValue(found)
	
	// Check if require_found is enabled and user not found
	if !data.RequireFound.IsNull() && data.RequireFound.ValueBool() && !found {
		response.Diagnostics.AddError(
			"Common Name not found",
			fmt.Sprintf("Common Name '%s' was not found in the configured base DNs and require_found is true", commonName),
		)
		return
	}
	
	tflog.Debug(ctx, "Common Name not found", map[string]interface{}{
		"common_name": commonName,
		"base_dn":     baseDN,
	})

	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

// searchForUser performs LDAP search and returns DN if found, empty string if not found
func (L *LDAPCNLookupDataSource) searchForUser(ctx context.Context, baseDN, filter string) string {
	s := ldap.NewSearchRequest(baseDN, ldap.ScopeWholeSubtree, 0, 0, 0, false, filter, []string{}, []ldap.Control{})

	result, err := L.providerData.Conn.Search(s)
	if err != nil {
		tflog.Warn(ctx, "LDAP search failed", map[string]interface{}{
			"base_dn": baseDN,
			"filter":  filter,
			"error":   err.Error(),
		})
		return ""
	}

	if len(result.Entries) > 0 {
		dn := result.Entries[0].DN
		tflog.Debug(ctx, "User found", map[string]interface{}{
			"base_dn": baseDN,
			"filter":  filter,
			"dn":      dn,
		})
		return dn
	}

	return ""
}