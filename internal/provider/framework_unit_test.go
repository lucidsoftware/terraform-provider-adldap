package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type stubLDAPClient struct {
	addFunc      func(*ldap.AddRequest) error
	bindFunc     func(string, string) error
	closeFunc    func() error
	delFunc      func(*ldap.DelRequest) error
	modifyFunc   func(*ldap.ModifyRequest) error
	searchFunc   func(*ldap.SearchRequest) (*ldap.SearchResult, error)
	setDebugFunc func(bool)
	startTLSFunc func(*tls.Config) error
}

func (s stubLDAPClient) Add(req *ldap.AddRequest) error {
	if s.addFunc != nil {
		return s.addFunc(req)
	}
	return nil
}

func (s stubLDAPClient) Bind(username, password string) error {
	if s.bindFunc != nil {
		return s.bindFunc(username, password)
	}
	return nil
}

func (s stubLDAPClient) Close() error {
	if s.closeFunc != nil {
		return s.closeFunc()
	}
	return nil
}

func (s stubLDAPClient) Del(req *ldap.DelRequest) error {
	if s.delFunc != nil {
		return s.delFunc(req)
	}
	return nil
}

func (s stubLDAPClient) Modify(req *ldap.ModifyRequest) error {
	if s.modifyFunc != nil {
		return s.modifyFunc(req)
	}
	return nil
}

func (s stubLDAPClient) Search(req *ldap.SearchRequest) (*ldap.SearchResult, error) {
	if s.searchFunc != nil {
		return s.searchFunc(req)
	}
	return &ldap.SearchResult{}, nil
}

func (s stubLDAPClient) SetDebug(enabled bool) {
	if s.setDebugFunc != nil {
		s.setDebugFunc(enabled)
	}
}

func (s stubLDAPClient) StartTLS(config *tls.Config) error {
	if s.startTLSFunc != nil {
		return s.startTLSFunc(config)
	}
	return nil
}

func providerConfig(t *testing.T, schema pschema.Schema, model LDAPProviderModel) tfsdk.Config {
	t.Helper()

	state := tfsdk.State{Schema: schema}
	diags := state.Set(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("provider config diagnostics: %v", diags.Errors())
	}

	return tfsdk.Config{Schema: schema, Raw: state.Raw}
}

func resourceConfig(t *testing.T, schema rschema.Schema, model LDAPObjectResourceModel) tfsdk.Config {
	t.Helper()

	state := tfsdk.State{Schema: schema}
	diags := state.Set(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("resource config diagnostics: %v", diags.Errors())
	}

	return tfsdk.Config{Schema: schema, Raw: state.Raw}
}

func resourcePlan(t *testing.T, schema rschema.Schema, model LDAPObjectResourceModel) tfsdk.Plan {
	t.Helper()

	plan := tfsdk.Plan{Schema: schema}
	diags := plan.Set(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("resource plan diagnostics: %v", diags.Errors())
	}

	return plan
}

func resourceState(t *testing.T, schema rschema.Schema, model LDAPObjectResourceModel) tfsdk.State {
	t.Helper()

	state := tfsdk.State{Schema: schema}
	diags := state.Set(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("resource state diagnostics: %v", diags.Errors())
	}

	return state
}

func datasourceConfig[T any](t *testing.T, schema dsschema.Schema, model T) tfsdk.Config {
	t.Helper()

	state := tfsdk.State{Schema: schema}
	diags := state.Set(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("datasource config diagnostics: %v", diags.Errors())
	}

	return tfsdk.Config{Schema: schema, Raw: state.Raw}
}

func emptyResourceState(schema rschema.Schema) tfsdk.State {
	return tfsdk.State{
		Schema: schema,
		Raw:    tftypes.NewValue(schema.Type().TerraformType(context.Background()), nil),
	}
}

func emptyDatasourceState(schema dsschema.Schema) tfsdk.State {
	return tfsdk.State{
		Schema: schema,
		Raw:    tftypes.NewValue(schema.Type().TerraformType(context.Background()), nil),
	}
}

func stringListValue(values ...string) types.List {
	if len(values) == 0 {
		return types.ListNull(types.StringType)
	}

	attrValues := make([]attr.Value, 0, len(values))
	for _, value := range values {
		attrValues = append(attrValues, basetypes.NewStringValue(value))
	}

	return types.ListValueMust(types.StringType, attrValues)
}

func stringSetValue(values ...string) types.Set {
	attrValues := make([]attr.Value, 0, len(values))
	for _, value := range values {
		attrValues = append(attrValues, basetypes.NewStringValue(value))
	}

	return types.SetValueMust(types.StringType, attrValues)
}

func stringSetMapValue(entries map[string][]string) types.Map {
	valueMap := make(map[string]attr.Value, len(entries))
	for key, values := range entries {
		valueMap[key] = stringSetValue(values...)
	}

	return types.MapValueMust(types.SetType{ElemType: types.StringType}, valueMap)
}

func TestProviderConfigureAndMetadata(t *testing.T) {
	ctx := context.Background()
	providerInstance := New("test")().(*LDAPProvider)

	var metadataResp provider.MetadataResponse
	providerInstance.Metadata(ctx, provider.MetadataRequest{}, &metadataResp)
	if metadataResp.TypeName != "ldap" || metadataResp.Version != "test" {
		t.Fatalf("unexpected metadata: %+v", metadataResp)
	}

	var schemaResp provider.SchemaResponse
	providerInstance.Schema(ctx, provider.SchemaRequest{}, &schemaResp)
	passwordAttr := schemaResp.Schema.Attributes["ldap_bind_password"].(pschema.StringAttribute)
	if !passwordAttr.Sensitive {
		t.Fatal("ldap_bind_password must be sensitive")
	}

	backend := newFakeLDAPBackend()
	withFakeDialer(t, func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
		return backend.client(), nil
	})

	cfg := providerConfig(t, schemaResp.Schema, LDAPProviderModel{
		LDAPURL:          types.StringValue("ldaps://fake.example.com:636"),
		LDAPBindDN:       types.StringValue("cn=admin,dc=example,dc=com"),
		LDAPBindPassword: types.StringValue("admin"),
		UsersOU:          types.StringValue("ou=users,dc=example,dc=com"),
		DisabledUsersOU:  types.StringValue("ou=disabled,dc=example,dc=com"),
	})

	var configureResp provider.ConfigureResponse
	providerInstance.Configure(ctx, provider.ConfigureRequest{Config: cfg}, &configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("unexpected configure diagnostics: %v", configureResp.Diagnostics.Errors())
	}

	configuredData, ok := configureResp.ResourceData.(*LDAPProviderData)
	if !ok || configuredData.Conn == nil {
		t.Fatalf("unexpected configure data: %#v", configureResp.ResourceData)
	}
}

func TestProviderConfigureFailures(t *testing.T) {
	ctx := context.Background()
	providerInstance := New("test")().(*LDAPProvider)
	var schemaResp provider.SchemaResponse
	providerInstance.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

	tests := []struct {
		name   string
		model  LDAPProviderModel
		dialer func(string, ...ldap.DialOpt) (ldapClient, error)
	}{
		{
			name:  "missing url",
			model: LDAPProviderModel{LDAPBindDN: types.StringValue("cn=admin"), LDAPBindPassword: types.StringValue("admin")},
		},
		{
			name:  "missing bind dn",
			model: LDAPProviderModel{LDAPURL: types.StringValue("ldaps://fake.example.com:636"), LDAPBindPassword: types.StringValue("admin")},
		},
		{
			name:  "missing password",
			model: LDAPProviderModel{LDAPURL: types.StringValue("ldaps://fake.example.com:636"), LDAPBindDN: types.StringValue("cn=admin")},
		},
		{
			name:  "invalid url",
			model: LDAPProviderModel{LDAPURL: types.StringValue("://bad"), LDAPBindDN: types.StringValue("cn=admin"), LDAPBindPassword: types.StringValue("admin")},
		},
		{
			name:  "insecure ldap",
			model: LDAPProviderModel{LDAPURL: types.StringValue("ldap://fake.example.com:389"), LDAPBindDN: types.StringValue("cn=admin"), LDAPBindPassword: types.StringValue("admin")},
		},
		{
			name:  "unsupported scheme",
			model: LDAPProviderModel{LDAPURL: types.StringValue("http://fake.example.com"), LDAPBindDN: types.StringValue("cn=admin"), LDAPBindPassword: types.StringValue("admin")},
		},
		{
			name:  "ldaps with starttls",
			model: LDAPProviderModel{LDAPURL: types.StringValue("ldaps://fake.example.com:636"), LDAPBindDN: types.StringValue("cn=admin"), LDAPBindPassword: types.StringValue("admin"), LDAPTLSUseStartTLS: types.BoolValue(true)},
		},
		{
			name:  "dial error",
			model: LDAPProviderModel{LDAPURL: types.StringValue("ldaps://fake.example.com:636"), LDAPBindDN: types.StringValue("cn=admin"), LDAPBindPassword: types.StringValue("admin")},
			dialer: func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
				return nil, errors.New("dial failed")
			},
		},
		{
			name:  "starttls error",
			model: LDAPProviderModel{LDAPURL: types.StringValue("ldap://fake.example.com:389"), LDAPBindDN: types.StringValue("cn=admin"), LDAPBindPassword: types.StringValue("admin"), LDAPTLSUseStartTLS: types.BoolValue(true)},
			dialer: func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
				return stubLDAPClient{startTLSFunc: func(*tls.Config) error { return errors.New("starttls failed") }}, nil
			},
		},
		{
			name:  "bind error",
			model: LDAPProviderModel{LDAPURL: types.StringValue("ldaps://fake.example.com:636"), LDAPBindDN: types.StringValue("cn=admin"), LDAPBindPassword: types.StringValue("admin")},
			dialer: func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
				return stubLDAPClient{bindFunc: func(string, string) error { return errors.New("bind failed") }}, nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withFakeDialer(t, func(target string, opts ...ldap.DialOpt) (ldapClient, error) {
				if test.dialer != nil {
					return test.dialer(target, opts...)
				}
				return newFakeLDAPBackend().client(), nil
			})

			var resp provider.ConfigureResponse
			providerInstance.Configure(ctx, provider.ConfigureRequest{
				Config: providerConfig(t, schemaResp.Schema, test.model),
			}, &resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected configure error for %s", test.name)
			}
		})
	}
}

func TestLDAPObjectResourceLifecycleDirect(t *testing.T) {
	ctx := context.Background()
	backend := newFakeLDAPBackend()
	resourceInstance := NewLDAPObjectResource().(*LDAPObjectResource)
	resourceInstance.providerData = &LDAPProviderData{
		Conn:            backend.client(),
		UsersOU:         "ou=users,dc=example,dc=com",
		DisabledUsersOU: "ou=disabled,dc=example,dc=com",
		userLookupCache: map[string]UserLookupCacheEntry{},
	}

	var schemaResp resource.SchemaResponse
	resourceInstance.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	createModel := LDAPObjectResourceModel{
		ID:            types.StringNull(),
		DN:            types.StringValue("cn=test,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes: stringSetMapValue(map[string][]string{
			"cn":                []string{"test"},
			"sn":                []string{"test"},
			"userPassword":      []string{"password"},
			"distinguishedName": []string{"cn=test,dc=example,dc=com"},
		}),
		IgnoreChanges: stringListValue("userPassword"),
	}

	createReq := resource.CreateRequest{
		Config: resourceConfig(t, schemaResp.Schema, createModel),
		Plan:   resourcePlan(t, schemaResp.Schema, createModel),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	createResp.State = emptyResourceState(schemaResp.Schema)
	resourceInstance.Create(ctx, createReq, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics.Errors())
	}

	if backend.entry("cn=test,dc=example,dc=com")["distinguishedName"] != nil {
		t.Fatal("system attributes must not be added to LDAP")
	}

	readResp := resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw}}
	resourceInstance.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics.Errors())
	}

	updateModel := LDAPObjectResourceModel{
		ID:            types.StringNull(),
		DN:            types.StringValue("cn=test,dc=example,dc=com"),
		ObjectClasses: stringListValue("person", "uidObject"),
		Attributes: stringSetMapValue(map[string][]string{
			"cn":           []string{"test"},
			"sn":           []string{"test2"},
			"uid":          []string{"test"},
			"userPassword": []string{"password"},
		}),
		IgnoreChanges: stringListValue("userPassword"),
	}

	updateResp := resource.UpdateResponse{State: emptyResourceState(schemaResp.Schema)}
	resourceInstance.Update(ctx, resource.UpdateRequest{
		State: createResp.State,
		Plan:  resourcePlan(t, schemaResp.Schema, updateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", updateResp.Diagnostics.Errors())
	}

	if got := backend.entry("cn=test,dc=example,dc=com")["sn"]; len(got) != 1 || got[0] != "test2" {
		t.Fatalf("updated sn mismatch: %v", got)
	}
	postUpdateState := updateResp.State

	importResp := resource.ImportStateResponse{State: emptyResourceState(schemaResp.Schema)}
	resourceInstance.ImportState(ctx, resource.ImportStateRequest{ID: "cn=test,dc=example,dc=com"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", importResp.Diagnostics.Errors())
	}

	renameModel := LDAPObjectResourceModel{
		ID:            types.StringNull(),
		DN:            types.StringValue("cn=test2,dc=example,dc=com"),
		ObjectClasses: stringListValue("person", "uidObject"),
		Attributes: stringSetMapValue(map[string][]string{
			"cn":  []string{"test2"},
			"sn":  []string{"test2"},
			"uid": []string{"test"},
		}),
		IgnoreChanges: stringListValue("userPassword"),
	}

	modifyPlanResp := resource.ModifyPlanResponse{Plan: resourcePlan(t, schemaResp.Schema, renameModel)}
	resourceInstance.ModifyPlan(ctx, resource.ModifyPlanRequest{
		State: postUpdateState,
		Plan:  resourcePlan(t, schemaResp.Schema, renameModel),
	}, &modifyPlanResp)
	if modifyPlanResp.Diagnostics.HasError() {
		t.Fatalf("modify plan diagnostics: %v", modifyPlanResp.Diagnostics.Errors())
	}

	updateResp = resource.UpdateResponse{State: emptyResourceState(schemaResp.Schema)}
	resourceInstance.Update(ctx, resource.UpdateRequest{
		State: postUpdateState,
		Plan:  resourcePlan(t, schemaResp.Schema, renameModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("rename diagnostics: %v", updateResp.Diagnostics.Errors())
	}

	if backend.entry("cn=test,dc=example,dc=com") != nil {
		t.Fatal("old DN should be deleted after rename")
	}
	if backend.entry("cn=test2,dc=example,dc=com") == nil {
		t.Fatal("new DN should exist after rename")
	}

	deleteResp := resource.DeleteResponse{}
	resourceInstance.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics.Errors())
	}
	if backend.entry("cn=test2,dc=example,dc=com") != nil {
		t.Fatal("entry should be deleted")
	}
}

func TestLDAPDataSourcesDirect(t *testing.T) {
	ctx := context.Background()
	backend := newFakeLDAPBackend()
	providerData := &LDAPProviderData{
		Conn:            backend.client(),
		UsersOU:         "ou=users,dc=example,dc=com",
		DisabledUsersOU: "ou=disabled,dc=example,dc=com",
		userLookupCache: map[string]UserLookupCacheEntry{},
	}

	objectDS := NewLDAPObjectDataSource().(*LDAPObjectDataSource)
	objectDS.providerData = providerData
	var objectSchemaResp datasource.SchemaResponse
	objectDS.Schema(ctx, datasource.SchemaRequest{}, &objectSchemaResp)
	objectResp := datasource.ReadResponse{State: emptyDatasourceState(objectSchemaResp.Schema)}
	objectDS.Read(ctx, datasource.ReadRequest{
		Config: datasourceConfig(t, objectSchemaResp.Schema, LDAPObjectDatasourceModel{
			Id:                   types.StringNull(),
			DN:                   types.StringValue("dc=example,dc=com"),
			ObjectClasses:        types.ListNull(types.StringType),
			Attributes:           types.MapNull(types.SetType{ElemType: types.StringType}),
			AdditionalAttributes: types.SetValueMust(types.StringType, []attr.Value{basetypes.NewStringValue("creatorsName")}),
		}),
	}, &objectResp)
	if objectResp.Diagnostics.HasError() {
		t.Fatalf("object datasource diagnostics: %v", objectResp.Diagnostics.Errors())
	}

	searchDS := NewLDAPSearchDataSource().(*LDAPSearchDataSource)
	searchDS.providerData = providerData
	var searchSchemaResp datasource.SchemaResponse
	searchDS.Schema(ctx, datasource.SchemaRequest{}, &searchSchemaResp)
	searchResp := datasource.ReadResponse{State: emptyDatasourceState(searchSchemaResp.Schema)}
	searchDS.Read(ctx, datasource.ReadRequest{
		Config: datasourceConfig(t, searchSchemaResp.Schema, LDAPSearchDatasourceModel{
			Id:                   types.StringNull(),
			BaseDN:               types.StringValue("dc=example,dc=com"),
			Scope:                types.StringValue("baseObject"),
			AdditionalAttributes: types.SetValueMust(types.StringType, []attr.Value{basetypes.NewStringValue("creatorsName")}),
			Results:              types.ListNull(types.MapType{ElemType: types.ListType{ElemType: types.StringType}}),
		}),
	}, &searchResp)
	if searchResp.Diagnostics.HasError() {
		t.Fatalf("search datasource diagnostics: %v", searchResp.Diagnostics.Errors())
	}

	samDS := NewLDAPSAMLookupDataSource().(*LDAPSAMLookupDataSource)
	samDS.providerData = providerData
	var samSchemaResp datasource.SchemaResponse
	samDS.Schema(ctx, datasource.SchemaRequest{}, &samSchemaResp)
	samResp := datasource.ReadResponse{State: emptyDatasourceState(samSchemaResp.Schema)}
	samDS.Read(ctx, datasource.ReadRequest{
		Config: datasourceConfig(t, samSchemaResp.Schema, LDAPSAMLookupDatasourceModel{
			Id:             types.StringNull(),
			SAMAccountName: types.StringValue("jdoe"),
			DN:             types.StringNull(),
			Found:          types.BoolNull(),
			RequireFound:   types.BoolNull(),
		}),
	}, &samResp)
	if samResp.Diagnostics.HasError() {
		t.Fatalf("sam datasource diagnostics: %v", samResp.Diagnostics.Errors())
	}

	cnDS := NewLDAPCNLookupDataSource().(*LDAPCNLookupDataSource)
	cnDS.providerData = providerData
	var cnSchemaResp datasource.SchemaResponse
	cnDS.Schema(ctx, datasource.SchemaRequest{}, &cnSchemaResp)
	cnResp := datasource.ReadResponse{State: emptyDatasourceState(cnSchemaResp.Schema)}
	cnDS.Read(ctx, datasource.ReadRequest{
		Config: datasourceConfig(t, cnSchemaResp.Schema, LDAPCNLookupDatasourceModel{
			Id:             types.StringNull(),
			CommonName:     types.StringValue("John Doe"),
			BaseDN:         types.StringValue("ou=users,dc=example,dc=com"),
			DisabledBaseDN: types.StringValue("ou=disabled,dc=example,dc=com"),
			DN:             types.StringNull(),
			Found:          types.BoolNull(),
			RequireFound:   types.BoolNull(),
		}),
	}, &cnResp)
	if cnResp.Diagnostics.HasError() {
		t.Fatalf("cn datasource diagnostics: %v", cnResp.Diagnostics.Errors())
	}
}

func TestLDAPResourceResolutionHelpers(t *testing.T) {
	ctx := context.Background()
	backend := newFakeLDAPBackend()
	resourceInstance := &LDAPObjectResource{
		providerData: &LDAPProviderData{
			Conn:            backend.client(),
			UsersOU:         "ou=users,dc=example,dc=com",
			DisabledUsersOU: "ou=disabled,dc=example,dc=com",
			userLookupCache: map[string]UserLookupCacheEntry{},
		},
	}

	if dn, err := resourceInstance.resolveCNtoDN(ctx, "John Doe"); err != nil || dn == "" {
		t.Fatalf("resolveCNtoDN failed: %v %q", err, dn)
	}
	if dns, err := resourceInstance.resolveMemberCNs(ctx, []string{"John Doe"}); err != nil || len(dns) != 1 {
		t.Fatalf("resolveMemberCNs failed: %v %v", err, dns)
	}
	if dn, err := resourceInstance.resolveSAMToDN(ctx, "jdoe"); err != nil || dn == "" {
		t.Fatalf("resolveSAMToDN failed: %v %q", err, dn)
	}
	if dns, err := resourceInstance.resolveMemberSAMs(ctx, []string{"jdoe"}); err != nil || len(dns) != 1 {
		t.Fatalf("resolveMemberSAMs failed: %v %v", err, dns)
	}
	if !resourceInstance.hasReferentialAttributeChanges(ctx, map[string][]string{"member": []string{"a"}}, map[string][]string{"member": []string{"b"}}) {
		t.Fatal("expected referential attribute changes")
	}
	if !isReferentialAttribute("member") || isReferentialAttribute("cn") {
		t.Fatal("unexpected referential attribute classification")
	}
	if refreshed, err := resourceInstance.refreshLDAPState(ctx, "cn=John Doe,ou=users,dc=example,dc=com"); err != nil || refreshed["cn"][0] != "John Doe" {
		t.Fatalf("refreshLDAPState failed: %v %v", err, refreshed)
	}
}

func TestDatasourceAndResourceConfigureValidation(t *testing.T) {
	ctx := context.Background()
	providerData := &LDAPProviderData{userLookupCache: map[string]UserLookupCacheEntry{}}

	resourceInstance := NewLDAPObjectResource().(*LDAPObjectResource)
	resourceConfigureResp := resource.ConfigureResponse{}
	resourceInstance.Configure(ctx, resource.ConfigureRequest{ProviderData: providerData}, &resourceConfigureResp)
	if resourceInstance.providerData == nil {
		t.Fatal("resource provider data should be configured")
	}

	objectDS := NewLDAPObjectDataSource().(*LDAPObjectDataSource)
	objectDS.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})
	if objectDS.providerData == nil {
		t.Fatal("object datasource provider data should be configured")
	}

	searchDS := NewLDAPSearchDataSource().(*LDAPSearchDataSource)
	searchDS.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})
	if searchDS.providerData == nil {
		t.Fatal("search datasource provider data should be configured")
	}

	samDS := NewLDAPSAMLookupDataSource().(*LDAPSAMLookupDataSource)
	samDS.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})
	if samDS.providerData == nil {
		t.Fatal("sam datasource provider data should be configured")
	}

	cnDS := NewLDAPCNLookupDataSource().(*LDAPCNLookupDataSource)
	cnDS.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})
	if cnDS.providerData == nil {
		t.Fatal("cn datasource provider data should be configured")
	}
}

func TestCRUDAndDatasourceErrorPaths(t *testing.T) {
	ctx := context.Background()
	failingClient := stubLDAPClient{
		addFunc:    func(*ldap.AddRequest) error { return errors.New("add failed") },
		delFunc:    func(*ldap.DelRequest) error { return errors.New("delete failed") },
		modifyFunc: func(*ldap.ModifyRequest) error { return errors.New("modify failed") },
		searchFunc: func(*ldap.SearchRequest) (*ldap.SearchResult, error) { return nil, errors.New("search failed") },
	}

	providerData := &LDAPProviderData{
		Conn:            failingClient,
		UsersOU:         "ou=users,dc=example,dc=com",
		DisabledUsersOU: "ou=disabled,dc=example,dc=com",
		userLookupCache: map[string]UserLookupCacheEntry{},
	}

	resourceInstance := &LDAPObjectResource{providerData: providerData}
	var resourceSchemaResp resource.SchemaResponse
	resourceInstance.Schema(ctx, resource.SchemaRequest{}, &resourceSchemaResp)
	model := LDAPObjectResourceModel{
		ID:            types.StringNull(),
		DN:            types.StringValue("cn=test,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes:    stringSetMapValue(map[string][]string{"cn": []string{"test"}}),
		IgnoreChanges: types.ListNull(types.StringType),
	}

	createResp := resource.CreateResponse{State: emptyResourceState(resourceSchemaResp.Schema)}
	resourceInstance.Create(ctx, resource.CreateRequest{
		Config: resourceConfig(t, resourceSchemaResp.Schema, model),
		Plan:   resourcePlan(t, resourceSchemaResp.Schema, model),
	}, &createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected create failure")
	}

	deleteResp := resource.DeleteResponse{}
	resourceInstance.Delete(ctx, resource.DeleteRequest{State: resourceState(t, resourceSchemaResp.Schema, model)}, &deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected delete failure")
	}

	updateResp := resource.UpdateResponse{State: emptyResourceState(resourceSchemaResp.Schema)}
	updateModel := LDAPObjectResourceModel{
		ID:            types.StringNull(),
		DN:            types.StringValue("cn=test,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes:    stringSetMapValue(map[string][]string{"cn": []string{"changed"}}),
		IgnoreChanges: types.ListNull(types.StringType),
	}
	resourceInstance.Update(ctx, resource.UpdateRequest{
		State: resourceState(t, resourceSchemaResp.Schema, model),
		Plan:  resourcePlan(t, resourceSchemaResp.Schema, updateModel),
	}, &updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected update failure")
	}

	objectDS := &LDAPObjectDataSource{providerData: providerData}
	var objectSchemaResp datasource.SchemaResponse
	objectDS.Schema(ctx, datasource.SchemaRequest{}, &objectSchemaResp)
	objectResp := datasource.ReadResponse{State: emptyDatasourceState(objectSchemaResp.Schema)}
	objectDS.Read(ctx, datasource.ReadRequest{
		Config: datasourceConfig(t, objectSchemaResp.Schema, LDAPObjectDatasourceModel{
			Id:                   types.StringNull(),
			DN:                   types.StringValue("dc=example,dc=com"),
			ObjectClasses:        types.ListNull(types.StringType),
			Attributes:           types.MapNull(types.SetType{ElemType: types.StringType}),
			AdditionalAttributes: types.SetNull(types.StringType),
		}),
	}, &objectResp)
	if !objectResp.Diagnostics.HasError() {
		t.Fatal("expected object datasource failure")
	}

	searchDS := &LDAPSearchDataSource{providerData: providerData}
	var searchSchemaResp datasource.SchemaResponse
	searchDS.Schema(ctx, datasource.SchemaRequest{}, &searchSchemaResp)
	searchResp := datasource.ReadResponse{State: emptyDatasourceState(searchSchemaResp.Schema)}
	searchDS.Read(ctx, datasource.ReadRequest{
		Config: datasourceConfig(t, searchSchemaResp.Schema, LDAPSearchDatasourceModel{
			Id:                   types.StringNull(),
			BaseDN:               types.StringValue("dc=example,dc=com"),
			Scope:                types.StringValue("wholeSubtree"),
			Filter:               types.StringValue("(&)"),
			AdditionalAttributes: types.SetNull(types.StringType),
			Results:              types.ListNull(types.MapType{ElemType: types.ListType{ElemType: types.StringType}}),
		}),
	}, &searchResp)
	if !searchResp.Diagnostics.HasError() {
		t.Fatal("expected search datasource failure")
	}

	samDS := &LDAPSAMLookupDataSource{providerData: providerData}
	if dn := samDS.searchForUser(ctx, "ou=users,dc=example,dc=com", "(&)"); dn != "" {
		t.Fatal("expected empty DN on failed SAM search")
	}

	cnDS := &LDAPCNLookupDataSource{providerData: providerData}
	if dn := cnDS.searchForUser(ctx, "ou=users,dc=example,dc=com", "(&)"); dn != "" {
		t.Fatal("expected empty DN on failed CN search")
	}
}

func TestSearchAndLookupHelpersEscapeFilters(t *testing.T) {
	ctx := context.Background()
	backend := newFakeLDAPBackend()
	resourceInstance := &LDAPObjectResource{
		providerData: &LDAPProviderData{
			Conn:            backend.client(),
			UsersOU:         "ou=users,dc=example,dc=com",
			DisabledUsersOU: "ou=disabled,dc=example,dc=com",
			userLookupCache: map[string]UserLookupCacheEntry{},
		},
	}

	_, _ = resourceInstance.resolveSAMToDN(ctx, "john*)(cn=*")
	if backend.lastSearch == nil || backend.lastSearch.Filter == "" {
		t.Fatal("expected resolveSAMToDN to issue a search")
	}

	samDS := &LDAPSAMLookupDataSource{providerData: resourceInstance.providerData}
	_ = samDS.searchForUser(ctx, "ou=users,dc=example,dc=com", fmt.Sprintf("(&(objectCategory=Person)(sAMAccountName=%s))", ldap.EscapeFilter("john*)(cn=*")))
	if backend.lastSearch == nil || backend.lastSearch.Filter == "" {
		t.Fatal("expected searchForUser to issue a search")
	}
}

func TestModifyPlanPreservesIgnoredAttributes(t *testing.T) {
	ctx := context.Background()
	resourceInstance := NewLDAPObjectResource().(*LDAPObjectResource)
	var schemaResp resource.SchemaResponse
	resourceInstance.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	stateModel := LDAPObjectResourceModel{
		ID:            types.StringValue("cn=test,dc=example,dc=com"),
		DN:            types.StringValue("cn=test,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes: stringSetMapValue(map[string][]string{
			"description": []string{"from-state"},
		}),
		IgnoreChanges: stringListValue("description"),
	}
	planModel := LDAPObjectResourceModel{
		ID:            types.StringNull(),
		DN:            types.StringValue("cn=test,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes:    stringSetMapValue(map[string][]string{}),
		IgnoreChanges: stringListValue("description"),
	}

	resp := resource.ModifyPlanResponse{
		Plan: resourcePlan(t, schemaResp.Schema, planModel),
	}
	resourceInstance.ModifyPlan(ctx, resource.ModifyPlanRequest{
		State: resourceState(t, schemaResp.Schema, stateModel),
		Plan:  resourcePlan(t, schemaResp.Schema, planModel),
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("modify plan diagnostics: %v", resp.Diagnostics.Errors())
	}

	var result LDAPObjectResourceModel
	diags := resp.Plan.Get(ctx, &result)
	if diags.HasError() {
		t.Fatalf("modify plan read diagnostics: %v", diags.Errors())
	}

	var descriptionValues map[string][]string
	diags = result.Attributes.ElementsAs(ctx, &descriptionValues, false)
	if diags.HasError() || descriptionValues["description"][0] != "from-state" {
		t.Fatalf("ignored attribute was not preserved: %v %v", diags.Errors(), descriptionValues)
	}
}

func TestLDAPProviderInterfaces(t *testing.T) {
	ctx := context.Background()
	providerInstance := New("test")().(*LDAPProvider)

	if len(providerInstance.Resources(ctx)) != 1 {
		t.Fatal("expected one resource factory")
	}
	if len(providerInstance.DataSources(ctx)) != 4 {
		t.Fatal("expected four datasource factories")
	}
}

func TestNilConnectionReadErrors(t *testing.T) {
	ctx := context.Background()
	resourceInstance := &LDAPObjectResource{}
	var schemaResp resource.SchemaResponse
	resourceInstance.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	model := LDAPObjectResourceModel{
		ID:            types.StringNull(),
		DN:            types.StringValue("cn=test,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes:    stringSetMapValue(map[string][]string{"cn": []string{"test"}}),
		IgnoreChanges: types.ListNull(types.StringType),
	}

	resp := resource.ReadResponse{State: emptyResourceState(schemaResp.Schema)}
	resourceInstance.Read(ctx, resource.ReadRequest{
		State: resourceState(t, schemaResp.Schema, model),
	}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected read to fail without LDAP connection")
	}
}

func TestConfigureTypeMismatchDiagnostics(t *testing.T) {
	ctx := context.Background()
	resourceInstance := NewLDAPObjectResource().(*LDAPObjectResource)
	resourceResp := resource.ConfigureResponse{}
	resourceInstance.Configure(ctx, resource.ConfigureRequest{ProviderData: "bad"}, &resourceResp)
	if !resourceResp.Diagnostics.HasError() {
		t.Fatal("expected resource configure type mismatch error")
	}

	objectDS := NewLDAPObjectDataSource().(*LDAPObjectDataSource)
	objectResp := datasource.ConfigureResponse{}
	objectDS.Configure(ctx, datasource.ConfigureRequest{ProviderData: "bad"}, &objectResp)
	if !objectResp.Diagnostics.HasError() {
		t.Fatal("expected object datasource configure type mismatch error")
	}

	searchDS := NewLDAPSearchDataSource().(*LDAPSearchDataSource)
	searchResp := datasource.ConfigureResponse{}
	searchDS.Configure(ctx, datasource.ConfigureRequest{ProviderData: "bad"}, &searchResp)
	if !searchResp.Diagnostics.HasError() {
		t.Fatal("expected search datasource configure type mismatch error")
	}

	samDS := NewLDAPSAMLookupDataSource().(*LDAPSAMLookupDataSource)
	samResp := datasource.ConfigureResponse{}
	samDS.Configure(ctx, datasource.ConfigureRequest{ProviderData: "bad"}, &samResp)
	if !samResp.Diagnostics.HasError() {
		t.Fatal("expected sam datasource configure type mismatch error")
	}

	cnDS := NewLDAPCNLookupDataSource().(*LDAPCNLookupDataSource)
	cnResp := datasource.ConfigureResponse{}
	cnDS.Configure(ctx, datasource.ConfigureRequest{ProviderData: "bad"}, &cnResp)
	if !cnResp.Diagnostics.HasError() {
		t.Fatal("expected cn datasource configure type mismatch error")
	}
}

func TestImportStateWithMissingEntryReturnsDiagnostic(t *testing.T) {
	ctx := context.Background()
	backend := newFakeLDAPBackend()
	resourceInstance := &LDAPObjectResource{
		providerData: &LDAPProviderData{
			Conn:            backend.client(),
			userLookupCache: map[string]UserLookupCacheEntry{},
		},
	}

	var schemaResp resource.SchemaResponse
	resourceInstance.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	resp := resource.ImportStateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	resourceInstance.ImportState(ctx, resource.ImportStateRequest{ID: "cn=missing,dc=example,dc=com"}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected import of missing entry to fail")
	}
}

func TestStateContainsExpectedIDAfterModifyPlanDNChange(t *testing.T) {
	ctx := context.Background()
	resourceInstance := NewLDAPObjectResource().(*LDAPObjectResource)
	var schemaResp resource.SchemaResponse
	resourceInstance.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	stateModel := LDAPObjectResourceModel{
		ID:            types.StringValue("cn=test,dc=example,dc=com"),
		DN:            types.StringValue("cn=test,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes:    stringSetMapValue(map[string][]string{"cn": []string{"test"}}),
		IgnoreChanges: types.ListNull(types.StringType),
	}
	planModel := LDAPObjectResourceModel{
		ID:            types.StringNull(),
		DN:            types.StringValue("cn=test2,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes:    stringSetMapValue(map[string][]string{"cn": []string{"test2"}}),
		IgnoreChanges: types.ListNull(types.StringType),
	}

	resp := resource.ModifyPlanResponse{Plan: resourcePlan(t, schemaResp.Schema, planModel)}
	resourceInstance.ModifyPlan(ctx, resource.ModifyPlanRequest{
		State: resourceState(t, schemaResp.Schema, stateModel),
		Plan:  resourcePlan(t, schemaResp.Schema, planModel),
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("modify plan diagnostics: %v", resp.Diagnostics.Errors())
	}

	var id types.String
	diags := resp.Plan.GetAttribute(ctx, path.Root("id"), &id)
	if diags.HasError() || !id.IsUnknown() {
		t.Fatalf("expected id to be unknown after DN change: %v %v", diags.Errors(), id)
	}
}
