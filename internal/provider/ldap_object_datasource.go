package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &LDAPObjectDataSource{}
var _ datasource.DataSourceWithConfigure = &LDAPObjectDataSource{}

func NewLDAPObjectDataSource() datasource.DataSource {
	return &LDAPObjectDataSource{}
}

type LDAPObjectDataSource struct {
	providerData *LDAPProviderData
}

type LDAPObjectDatasourceModel struct {
	Id                   types.String `tfsdk:"id"`
	DN                   types.String `tfsdk:"dn"`
	ObjectClasses        types.List   `tfsdk:"object_classes"`
	Attributes           types.Map    `tfsdk:"attributes"`
	AdditionalAttributes types.Set    `tfsdk:"additional_attributes"`
}

func (L *LDAPObjectDataSource) Metadata(_ context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_object"
}

func (L *LDAPObjectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Generic LDAP object datasource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Datasource identifier",
			},
			"dn": schema.StringAttribute{
				MarkdownDescription: "DN of this ldap object",
				Required:            true,
			},
			"additional_attributes": schema.SetAttribute{
				MarkdownDescription: "Any additional attributes to request, such as constructed attributes",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"object_classes": schema.ListAttribute{
				MarkdownDescription: "A list of classes this object implements",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"attributes": schema.MapAttribute{
				MarkdownDescription: "The definition of an attribute, the name defines the type of the attribute",
				Computed:            true,
				ElementType:         types.SetType{ElemType: types.StringType},
			},
		},
	}
}

func (L *LDAPObjectDataSource) Configure(_ context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (L *LDAPObjectDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data LDAPObjectDatasourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	// Note: For data source, attributes will be populated from LDAP query results

	var objectClasses []string
	response.Diagnostics.Append(data.ObjectClasses.ElementsAs(ctx, &objectClasses, false)...)
	if response.Diagnostics.HasError() {
		return
	}

	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("id"), data.DN)...)
	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("dn"), data.DN)...)

	var additionalAttributes []string
	response.Diagnostics.Append(data.AdditionalAttributes.ElementsAs(ctx, &additionalAttributes, false)...)
	if response.Diagnostics.HasError() {
		return
	}

	if entry, err := GetEntry(L.providerData.Conn, data.DN.ValueString(), append(additionalAttributes, "*")...); err != nil {
		response.Diagnostics.AddError(
			"Can not read entry",
			err.Error(),
		)
	} else {
		response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("dn"), entry.DN)...)
		ctx = MaskAttributesFromArray(ctx, entry.Attributes)
		for _, attribute := range entry.Attributes {
			if attribute.Name == "objectClass" {
				response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("object_classes"), attribute.Values)...)
			} else if !isSystemAttribute(attribute.Name) {
				encodedValues := encodeAttributeValues(attribute.Name, attribute.Values)
				// Convert string slice to set for new schema
				setValue, diags := types.SetValueFrom(ctx, types.StringType, encodedValues)
				response.Diagnostics.Append(diags...)
				if !response.Diagnostics.HasError() {
					response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("attributes").AtMapKey(attribute.Name), setValue)...)
				}
			}
		}
		tflog.Debug(ctx, "Read entry", map[string]interface{}{
			"entry": ToLDIF(entry),
		})
	}
}
