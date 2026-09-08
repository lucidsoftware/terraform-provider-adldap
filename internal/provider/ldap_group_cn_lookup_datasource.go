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

var _ datasource.DataSource = &LDAPGroupCNLookupDataSource{}
var _ datasource.DataSourceWithConfigure = &LDAPGroupCNLookupDataSource{}

func NewLDAPGroupCNLookupDataSource() datasource.DataSource {
	return &LDAPGroupCNLookupDataSource{}
}

type LDAPGroupCNLookupDataSource struct {
	providerData *LDAPProviderData
}

type LDAPGroupCNLookupDatasourceModel struct {
	ID           types.String `tfsdk:"id"`
	CommonName   types.String `tfsdk:"common_name"`
	BaseDN       types.String `tfsdk:"base_dn"`
	DN           types.String `tfsdk:"dn"`
	Found        types.Bool   `tfsdk:"found"`
	RequireFound types.Bool   `tfsdk:"require_found"`
}

func (L *LDAPGroupCNLookupDataSource) Metadata(_ context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_group_cn_lookup"
}

func (L *LDAPGroupCNLookupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "LDAP group Common Name to DN lookup datasource. The lookup must match exactly one group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Datasource identifier",
			},
			"common_name": schema.StringAttribute{
				MarkdownDescription: "Group Common Name (CN) to look up",
				Required:            true,
			},
			"base_dn": schema.StringAttribute{
				MarkdownDescription: "Base DN to search for the group",
				Required:            true,
			},
			"dn": schema.StringAttribute{
				MarkdownDescription: "Distinguished name of the group (empty if not found)",
				Computed:            true,
			},
			"found": schema.BoolAttribute{
				MarkdownDescription: "Whether the group was found",
				Computed:            true,
			},
			"require_found": schema.BoolAttribute{
				MarkdownDescription: "Whether to return an error if the group is not found (default: false)",
				Optional:            true,
			},
		},
	}
}

func (L *LDAPGroupCNLookupDataSource) Configure(_ context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	providerData, ok := request.ProviderData.(*LDAPProviderData)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Datasource Configure Type",
			fmt.Sprintf("Expected *LDAPProviderData, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)
		return
	}

	L.providerData = providerData
}

func (L *LDAPGroupCNLookupDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data LDAPGroupCNLookupDatasourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	commonName := data.CommonName.ValueString()
	baseDN := data.BaseDN.ValueString()
	data.ID = types.StringValue(fmt.Sprintf("group_cn_lookup_%s", commonName))

	tflog.Debug(ctx, "Looking up group Common Name", map[string]interface{}{
		"common_name": commonName,
		"base_dn":     baseDN,
	})

	// Include the group object class and base DN in the key so group and user
	// lookups cannot share a cached result for the same CN.
	rawKey := fmt.Sprintf("group-cn:%s:%s", commonName, baseDN)
	dn, found, err := L.providerData.cachedLookup(ctx, "Group", rawKey, func() (string, bool, error) {
		filter := fmt.Sprintf("(&(objectCategory=group)(cn=%s))", ldap.EscapeFilter(commonName))
		return L.searchForGroup(ctx, commonName, baseDN, filter)
	})
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to look up group Common Name",
			fmt.Sprintf("Unable to look up group Common Name '%s' in base DN '%s': %s", commonName, baseDN, err),
		)
		return
	}

	data.DN = types.StringValue(dn)
	data.Found = types.BoolValue(found)

	if !data.RequireFound.IsNull() && data.RequireFound.ValueBool() && !found {
		response.Diagnostics.AddError(
			"Group Common Name not found",
			fmt.Sprintf("Group Common Name '%s' was not found in base DN '%s' and require_found is true", commonName, baseDN),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

// searchForGroup performs an LDAP group search and returns its DN if exactly
// one group matches the common name.
func (L *LDAPGroupCNLookupDataSource) searchForGroup(ctx context.Context, commonName, baseDN, filter string) (string, bool, error) {
	search := ldap.NewSearchRequest(baseDN, ldap.ScopeWholeSubtree, 0, 0, 0, false, filter, []string{}, []ldap.Control{})

	result, err := L.providerData.Conn.Search(search)
	if err != nil {
		tflog.Warn(ctx, "LDAP group search failed", map[string]interface{}{
			"base_dn": baseDN,
			"filter":  filter,
			"error":   err.Error(),
		})
		return "", false, fmt.Errorf("LDAP search failed: %w", err)
	}

	dn, found, err := uniqueGroupDN(commonName, baseDN, result.Entries)
	if err != nil || !found {
		return dn, found, err
	}

	tflog.Debug(ctx, "Group found", map[string]interface{}{
		"base_dn": baseDN,
		"filter":  filter,
		"dn":      dn,
	})
	return dn, true, nil
}

// uniqueGroupDN returns a result only when the CN search matches exactly one group.
func uniqueGroupDN(commonName, baseDN string, entries []*ldap.Entry) (string, bool, error) {
	switch len(entries) {
	case 0:
		return "", false, nil
	case 1:
		return entries[0].DN, true, nil
	default:
		return "", false, fmt.Errorf("multiple groups found with Common Name '%s' in base DN '%s'", commonName, baseDN)
	}
}
