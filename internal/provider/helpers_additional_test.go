package provider

import (
	"context"
	"crypto/tls"
	"net"
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestAttributeUtilsHelpers(t *testing.T) {
	ctx := context.Background()

	listValue := types.ListValueMust(types.StringType, []attr.Value{
		types.StringValue("b"),
		types.StringValue("a"),
	})

	unordered, diags := convertToUnorderedListValue(ctx, "member", listValue)
	if diags.HasError() {
		t.Fatalf("convertToUnorderedListValue diagnostics: %v", diags.Errors())
	}
	if _, ok := unordered.(UnorderedStringListValue); !ok {
		t.Fatalf("expected unordered value, got %T", unordered)
	}

	regular, diags := convertFromUnorderedListValue(ctx, unordered)
	if diags.HasError() {
		t.Fatalf("convertFromUnorderedListValue diagnostics: %v", diags.Errors())
	}
	if _, ok := regular.(basetypes.ListValue); !ok {
		t.Fatalf("expected list value, got %T", regular)
	}

	if _, diags := convertToUnorderedListValue(ctx, "member", types.StringValue("bad")); !diags.HasError() {
		t.Fatal("expected non-list conversion to fail")
	}
	if converted, diags := convertToUnorderedListValue(ctx, "cn", listValue); diags.HasError() || !converted.Equal(listValue) {
		t.Fatal("expected non-unordered attribute conversion passthrough")
	}
	if _, diags := convertToUnorderedListValue(ctx, "member", types.ListNull(types.StringType)); diags.HasError() {
		t.Fatal("expected null list conversion to succeed")
	}
	if _, diags := convertToUnorderedListValue(ctx, "member", types.ListUnknown(types.StringType)); diags.HasError() {
		t.Fatal("expected unknown list conversion to succeed")
	}
	if converted, diags := convertFromUnorderedListValue(ctx, listValue); diags.HasError() || !converted.Equal(listValue) {
		t.Fatal("expected regular list passthrough")
	}
	if getUnorderedAttributeType("member").String() == "" {
		t.Fatal("expected unordered type string")
	}
	if getUnorderedAttributeType("cn").String() == "" {
		t.Fatal("expected regular type string")
	}
	if addUnorderedAttributeDescription("member", "desc") == "desc" {
		t.Fatal("expected unordered attribute description suffix")
	}
	if addUnorderedAttributeDescription("member", "desc (Order is not significant for this attribute)") == "" {
		t.Fatal("expected existing suffix to be preserved")
	}
}

func TestUnorderedStringListTypeHelpers(t *testing.T) {
	ctx := context.Background()

	unorderedType := NewUnorderedStringListType()
	if unorderedType.String() == "" {
		t.Fatal("expected unordered type string")
	}

	value, diags := NewUnorderedStringListValue([]attr.Value{types.StringValue("a")})
	if diags.HasError() {
		t.Fatalf("NewUnorderedStringListValue diagnostics: %v", diags.Errors())
	}
	if !value.Equal(value) {
		t.Fatal("expected value to equal itself")
	}
	if value.Type(ctx).String() == "" {
		t.Fatal("expected value type")
	}
	if value.Equal(types.StringValue("nope")) {
		t.Fatal("expected unequal types to be false")
	}

	fromStrings, diags := NewUnorderedStringListValueFromStrings(ctx, []string{"b", "a"})
	if diags.HasError() {
		t.Fatalf("NewUnorderedStringListValueFromStrings diagnostics: %v", diags.Errors())
	}
	equal, diags := fromStrings.ListSemanticEquals(ctx, mustUnorderedListValue(ctx, []string{"a", "b"}))
	if diags.HasError() || !equal {
		t.Fatalf("ListSemanticEquals mismatch: equal=%v diags=%v", equal, diags.Errors())
	}
	if equal, diags := fromStrings.ListSemanticEquals(ctx, mustUnorderedListValue(ctx, []string{"a", "c"})); diags.HasError() || equal {
		t.Fatal("expected semantic inequality for different elements")
	}
	if equal, diags := fromStrings.ListSemanticEquals(ctx, UnorderedStringListValue{ListValue: basetypes.NewListNull(types.StringType)}); diags.HasError() || equal {
		t.Fatal("expected null semantic inequality")
	}
	if _, diags := NewUnorderedStringListValueFromStrings(ctx, nil); diags.HasError() {
		t.Fatal("expected nil string slice conversion to succeed")
	}
	if _, diags := NewUnorderedStringListValueFromStrings(ctx, []string{}); diags.HasError() {
		t.Fatal("expected empty string slice conversion to succeed")
	}

	tfValue := tftypes.NewValue(unorderedType.TerraformType(ctx), []tftypes.Value{
		tftypes.NewValue(tftypes.String, "a"),
	})
	converted, err := unorderedType.ValueFromTerraform(ctx, tfValue)
	if err != nil {
		t.Fatalf("ValueFromTerraform error: %v", err)
	}
	if converted.Type(ctx).String() == "" || unorderedType.ValueType(ctx).Type(context.Background()).String() == "" {
		t.Fatal("expected converted/value type metadata")
	}
}

func TestPlanModifierAndReferenceHelpers(t *testing.T) {
	ctx := context.Background()

	modifier := NewUnorderedListPlanModifier("member")
	if modifier.Description(ctx) == "" || modifier.MarkdownDescription(ctx) == "" {
		t.Fatal("expected list modifier descriptions")
	}

	listResp := planmodifier.ListResponse{}
	modifier.PlanModifyList(ctx, planmodifier.ListRequest{
		StateValue: mustListValue("a", "b"),
		PlanValue:  mustListValue("b", "a"),
	}, &listResp)
	if listResp.Diagnostics.HasError() {
		t.Fatalf("PlanModifyList diagnostics: %v", listResp.Diagnostics.Errors())
	}

	mapModifier := NewUnorderedMapListPlanModifier()
	if mapModifier.Description(ctx) == "" || mapModifier.MarkdownDescription(ctx) == "" {
		t.Fatal("expected map modifier descriptions")
	}
	mapResp := planmodifier.MapResponse{}
	mapModifier.PlanModifyMap(ctx, planmodifier.MapRequest{
		StateValue: mustMapValue(map[string][]string{"member": []string{"a", "b"}}),
		PlanValue:  mustMapValue(map[string][]string{"member": []string{"b", "a"}}),
	}, &mapResp)
	if mapResp.Diagnostics.HasError() {
		t.Fatalf("PlanModifyMap diagnostics: %v", mapResp.Diagnostics.Errors())
	}
	mapModifier.PlanModifyMap(ctx, planmodifier.MapRequest{
		StateValue: types.MapNull(types.SetType{ElemType: types.StringType}),
		PlanValue:  types.MapNull(types.SetType{ElemType: types.StringType}),
	}, &planmodifier.MapResponse{})

	if !isValidDNFormat("CN=John Doe,OU=users,DC=example,DC=com") || isValidDNFormat("not-a-dn") {
		t.Fatal("unexpected DN format validation")
	}
	if isValidDNFormat("") {
		t.Fatal("empty DN should be invalid")
	}
	if len(filterValidReferences(ctx, []string{"CN=John Doe,OU=users,DC=example,DC=com", "bad"})) != 1 {
		t.Fatal("expected invalid references to be filtered")
	}
	if len(getInvalidReferences([]string{"good", "bad"}, []string{"good"})) != 1 {
		t.Fatal("expected invalid references delta")
	}
	if !areStringSlicesExactlyEqual([]string{"a", "b"}, []string{"a", "b"}) {
		t.Fatal("expected exact equality")
	}
	if len(setDifference(stringSliceToSet([]string{"a", "b"}), stringSliceToSet([]string{"a"}))) != 1 {
		t.Fatal("expected set difference")
	}
	if len(setIntersection(stringSliceToSet([]string{"a", "b"}), stringSliceToSet([]string{"b"}))) != 1 {
		t.Fatal("expected set intersection")
	}
	if len(setToStringSlice(stringSliceToSet([]string{"a"}))) != 1 {
		t.Fatal("expected set to slice")
	}
	if len(convertStringsToAttrValues([]string{"a"})) != 1 || len(convertSetMapToValueMap(map[string]types.Set{"member": stringSetValue("a")})) != 1 {
		t.Fatal("expected conversion helpers to return values")
	}
}

func TestToolsAndLoggerHelpers(t *testing.T) {
	ctx := context.Background()

	if entry, err := GetEntry(&fakeLDAPClient{backend: newFakeLDAPBackend()}, "dc=example,dc=com", "*"); err != nil || entry.DN == "" {
		t.Fatalf("GetEntry failed: %v %v", err, entry)
	}
	if _, err := GetEntry(&fakeLDAPClient{backend: newFakeLDAPBackend()}, "missing", "*"); err == nil {
		t.Fatal("expected GetEntry missing entry error")
	}

	if encoded := encodeAttributeValues("objectGUID", []string{"binary"}); encoded[0] == "binary" {
		t.Fatal("expected binary attribute encoding")
	}
	if decoded := decodeAttributeValues("objectGUID", encodeAttributeValues("objectGUID", []string{"binary"})); decoded[0] != "binary" {
		t.Fatal("expected binary attribute decoding")
	}
	if unchanged := decodeAttributeValues("cn", []string{"value"}); unchanged[0] != "value" {
		t.Fatal("expected non-binary attribute decode passthrough")
	}
	if masked := MaskAttributes(ctx, map[string][]string{"unicodePwd": []string{"secret"}}); masked == nil {
		t.Fatal("expected masked context")
	}
	if masked := MaskAttributesFromArray(ctx, []*ldap.EntryAttribute{{Name: "userPassword", Values: []string{"secret"}}}); masked == nil {
		t.Fatal("expected masked array context")
	}

	t.Setenv("TF_LOG", "")
	if n, err := (TFLoggerAdapter{ctx: ctx}).Write([]byte("ignored")); err != nil || n != len("ignored") {
		t.Fatalf("logger adapter non-debug write mismatch: n=%d err=%v", n, err)
	}
	t.Setenv("TF_LOG", "DEBUG")
	if n, err := (TFLoggerAdapter{ctx: ctx}).Write([]byte("logged")); err != nil || n != len("logged") {
		t.Fatalf("logger adapter debug write mismatch: n=%d err=%v", n, err)
	}
}

func TestLDAPClientAdapterWrappers(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	conn := ldap.NewConn(clientConn, false)
	conn.Start()
	adapter := &ldapClientAdapter{conn: conn}

	_ = serverConn.Close()

	adapter.SetDebug(true)
	adapter.SetDebug(false)
	_ = adapter.Bind("cn=admin", "admin")
	_, _ = adapter.Search(ldap.NewSearchRequest("dc=example,dc=com", ldap.ScopeBaseObject, 0, 0, 0, false, "(&)", []string{"*"}, nil))
	_ = adapter.Add(ldap.NewAddRequest("cn=test,dc=example,dc=com", nil))
	_ = adapter.Modify(ldap.NewModifyRequest("cn=test,dc=example,dc=com", nil))
	_ = adapter.Del(ldap.NewDelRequest("cn=test,dc=example,dc=com", nil))
	_ = adapter.StartTLS(&tls.Config{InsecureSkipVerify: true})
	_ = adapter.Close()
}

func TestDatasourceMetadataMethods(t *testing.T) {
	ctx := context.Background()

	objectDS := NewLDAPObjectDataSource().(*LDAPObjectDataSource)
	objectMeta := datasource.MetadataResponse{}
	objectDS.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "ldap"}, &objectMeta)
	if objectMeta.TypeName != "ldap_object" {
		t.Fatal("unexpected object datasource metadata")
	}

	searchDS := NewLDAPSearchDataSource().(*LDAPSearchDataSource)
	searchMeta := datasource.MetadataResponse{}
	searchDS.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "ldap"}, &searchMeta)
	if searchMeta.TypeName != "ldap_search" {
		t.Fatal("unexpected search datasource metadata")
	}

	samDS := NewLDAPSAMLookupDataSource().(*LDAPSAMLookupDataSource)
	samMeta := datasource.MetadataResponse{}
	samDS.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "ldap"}, &samMeta)
	if samMeta.TypeName != "ldap_sam_lookup" {
		t.Fatal("unexpected sam datasource metadata")
	}

	cnDS := NewLDAPCNLookupDataSource().(*LDAPCNLookupDataSource)
	cnMeta := datasource.MetadataResponse{}
	cnDS.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "ldap"}, &cnMeta)
	if cnMeta.TypeName != "ldap_cn_lookup" {
		t.Fatal("unexpected cn datasource metadata")
	}

	resourceInstance := NewLDAPObjectResource().(*LDAPObjectResource)
	resourceMeta := resource.MetadataResponse{}
	resourceInstance.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "ldap"}, &resourceMeta)
	if resourceMeta.TypeName != "ldap_object" {
		t.Fatal("unexpected resource metadata")
	}
}

func TestConfigureNilProviderData(t *testing.T) {
	ctx := context.Background()

	resourceInstance := NewLDAPObjectResource().(*LDAPObjectResource)
	resourceResp := resource.ConfigureResponse{}
	resourceInstance.Configure(ctx, resource.ConfigureRequest{}, &resourceResp)
	if resourceResp.Diagnostics.HasError() {
		t.Fatal("nil provider data should be a no-op for resource configure")
	}

	objectDS := NewLDAPObjectDataSource().(*LDAPObjectDataSource)
	objectResp := datasource.ConfigureResponse{}
	objectDS.Configure(ctx, datasource.ConfigureRequest{}, &objectResp)
	if objectResp.Diagnostics.HasError() {
		t.Fatal("nil provider data should be a no-op for object datasource configure")
	}

	searchDS := NewLDAPSearchDataSource().(*LDAPSearchDataSource)
	searchResp := datasource.ConfigureResponse{}
	searchDS.Configure(ctx, datasource.ConfigureRequest{}, &searchResp)
	if searchResp.Diagnostics.HasError() {
		t.Fatal("nil provider data should be a no-op for search datasource configure")
	}

	samDS := NewLDAPSAMLookupDataSource().(*LDAPSAMLookupDataSource)
	samResp := datasource.ConfigureResponse{}
	samDS.Configure(ctx, datasource.ConfigureRequest{}, &samResp)
	if samResp.Diagnostics.HasError() {
		t.Fatal("nil provider data should be a no-op for sam datasource configure")
	}

	cnDS := NewLDAPCNLookupDataSource().(*LDAPCNLookupDataSource)
	cnResp := datasource.ConfigureResponse{}
	cnDS.Configure(ctx, datasource.ConfigureRequest{}, &cnResp)
	if cnResp.Diagnostics.HasError() {
		t.Fatal("nil provider data should be a no-op for cn datasource configure")
	}
}

func mustUnorderedListValue(ctx context.Context, values []string) UnorderedStringListValue {
	result, diags := NewUnorderedStringListValueFromStrings(ctx, values)
	if diags.HasError() {
		panic(diags.Errors())
	}
	return result
}

func mustListValue(values ...string) basetypes.ListValue {
	attrValues := make([]attr.Value, 0, len(values))
	for _, value := range values {
		attrValues = append(attrValues, types.StringValue(value))
	}
	return types.ListValueMust(types.StringType, attrValues)
}

func mustMapValue(entries map[string][]string) basetypes.MapValue {
	values := make(map[string]attr.Value, len(entries))
	for key, entryValues := range entries {
		values[key] = stringSetValue(entryValues...)
	}
	return types.MapValueMust(types.SetType{ElemType: types.StringType}, values)
}
