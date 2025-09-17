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

var _ datasource.DataSource = &LDAPSAMLookupDataSource{}
var _ datasource.DataSourceWithConfigure = &LDAPSAMLookupDataSource{}

func NewLDAPSAMLookupDataSource() datasource.DataSource {
	return &LDAPSAMLookupDataSource{}
}

type LDAPSAMLookupDataSource struct {
	providerData *LDAPProviderData
}

type LDAPSAMLookupDatasourceModel struct {
	Id             types.String `tfsdk:"id"`
	SAMAccountName types.String `tfsdk:"sam_account_name"`
	DN             types.String `tfsdk:"dn"`
	Found          types.Bool   `tfsdk:"found"`
	RequireFound   types.Bool   `tfsdk:"require_found"`
}

func (L *LDAPSAMLookupDataSource) Metadata(_ context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_sam_lookup"
}

func (L *LDAPSAMLookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "LDAP SAM account name to DN lookup datasource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Datasource identifier",
			},
			"sam_account_name": schema.StringAttribute{
				MarkdownDescription: "SAM account name to lookup",
				Required:            true,
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

func (L *LDAPSAMLookupDataSource) Configure(_ context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (L *LDAPSAMLookupDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data LDAPSAMLookupDatasourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)

	if response.Diagnostics.HasError() {
		return
	}

	samAccountName := data.SAMAccountName.ValueString()
	
	// Set the ID for this datasource
	data.Id = types.StringValue(fmt.Sprintf("sam_lookup_%s", samAccountName))

	tflog.Debug(ctx, "Looking up SAM account", map[string]interface{}{
		"sam_account_name": samAccountName,
		"users_ou":         L.providerData.UsersOU,
		"disabled_users_ou": L.providerData.DisabledUsersOU,
	})

	// Create cache key for SAM lookup
	rawKey := fmt.Sprintf("sam:%s", samAccountName)
	
	// Perform cached lookup
	dn, found := L.providerData.cachedUserLookup(ctx, rawKey, func() (string, bool) {
		tflog.Info(ctx, "LDAP lookup function called", map[string]interface{}{
			"sam_account_name": samAccountName,
			"users_ou":         L.providerData.UsersOU,
			"disabled_users_ou": L.providerData.DisabledUsersOU,
		})
		
		// Search for user by sAMAccountName
		filter := fmt.Sprintf("(&(objectCategory=Person)(sAMAccountName=%s))", samAccountName)
		
		// Try active users first
		if L.providerData.UsersOU != "" {
			tflog.Info(ctx, "Searching in users OU", map[string]interface{}{
				"base_dn": L.providerData.UsersOU,
				"filter":  filter,
			})
			if dn := L.searchForUser(ctx, L.providerData.UsersOU, filter); dn != "" {
				tflog.Info(ctx, "User found in users OU", map[string]interface{}{"dn": dn})
				return dn, true
			}
		}

		// Try disabled users if configured
		if L.providerData.DisabledUsersOU != "" {
			tflog.Info(ctx, "Searching in disabled users OU", map[string]interface{}{
				"base_dn": L.providerData.DisabledUsersOU,
				"filter":  filter,
			})
			if dn := L.searchForUser(ctx, L.providerData.DisabledUsersOU, filter); dn != "" {
				tflog.Info(ctx, "User found in disabled users OU", map[string]interface{}{"dn": dn})
				return dn, true
			}
		}

		// User not found
		tflog.Info(ctx, "User not found in any OU", map[string]interface{}{
			"sam_account_name": samAccountName,
		})
		return "", false
	})
	
	// Set the results
	data.DN = types.StringValue(dn)
	data.Found = types.BoolValue(found)
	
	// Check if require_found is enabled and user not found
	if !data.RequireFound.IsNull() && data.RequireFound.ValueBool() && !found {
		response.Diagnostics.AddError(
			"SAM account not found",
			fmt.Sprintf("SAM account '%s' was not found in the configured OUs and require_found is true", samAccountName),
		)
		return
	}
	
	tflog.Debug(ctx, "SAM account not found", map[string]interface{}{
		"sam_account_name": samAccountName,
	})

	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

// searchForUser performs LDAP search and returns DN if found, empty string if not found
func (L *LDAPSAMLookupDataSource) searchForUser(ctx context.Context, baseDN, filter string) string {
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