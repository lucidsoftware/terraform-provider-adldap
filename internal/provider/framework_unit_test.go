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
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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

func TestMaskAttributesFromArray(t *testing.T) {
	ctx := context.Background()

	t.Run("nil attributes", func(t *testing.T) {
		result := MaskAttributesFromArray(ctx, nil)
		if result == nil {
			t.Error("expected non-nil context")
		}
	})
}

func TestDecodeAttributeValues(t *testing.T) {
	t.Run("binary attribute with invalid base64", func(t *testing.T) {
		result := decodeAttributeValues("objectGUID", []string{"not-valid-base64!!!"})
		if len(result) != 1 || result[0] != "not-valid-base64!!!" {
			t.Errorf("expected original value on decode error: %v", result)
		}
	})
}

func TestConvertToUnorderedListValue(t *testing.T) {
	ctx := context.Background()

	t.Run("non-unordered attribute returns original", func(t *testing.T) {
		original := types.StringValue("test")
		result, diags := convertToUnorderedListValue(ctx, "cn", original)
		if diags.HasError() {
			t.Errorf("unexpected diagnostics: %v", diags.Errors())
		}
		if result != original {
			t.Error("expected original value for non-unordered attribute")
		}
	})

	t.Run("invalid type returns original with error", func(t *testing.T) {
		original := types.StringValue("test")
		result, diags := convertToUnorderedListValue(ctx, "member", original)
		if !diags.HasError() {
			t.Error("expected error for invalid type")
		}
		if result != original {
			t.Error("expected original value on error")
		}
	})
}

func TestLDAPObjectResource_ResolveCNtoDN(t *testing.T) {
	ctx := context.Background()

	t.Run("missing users_ou", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		ri := &LDAPObjectResource{
			providerData: &LDAPProviderData{
				Conn:            backend.client(),
				UsersOU:         "",
				userLookupCache: map[string]UserLookupCacheEntry{},
			},
		}
		_, err := ri.resolveCNtoDN(ctx, "test")
		if err == nil {
			t.Error("expected error for missing users_ou")
		}
	})

	t.Run("search error continues to next base", func(t *testing.T) {
		ri := &LDAPObjectResource{
			providerData: &LDAPProviderData{
				Conn: stubLDAPClient{searchFunc: func(*ldap.SearchRequest) (*ldap.SearchResult, error) {
					return nil, errors.New("search error")
				}},
				UsersOU:         "ou=users,dc=example,dc=com",
				DisabledUsersOU: "ou=disabled,dc=example,dc=com",
				userLookupCache: map[string]UserLookupCacheEntry{},
			},
		}
		_, err := ri.resolveCNtoDN(ctx, "test")
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestLDAPObjectResource_ResolveSAMToDN(t *testing.T) {
	ctx := context.Background()

	t.Run("missing users_ou", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		ri := &LDAPObjectResource{
			providerData: &LDAPProviderData{
				Conn:            backend.client(),
				UsersOU:         "",
				userLookupCache: map[string]UserLookupCacheEntry{},
			},
		}
		_, err := ri.resolveSAMToDN(ctx, "test")
		if err == nil {
			t.Error("expected error for missing users_ou")
		}
	})

	t.Run("search error continues to next base", func(t *testing.T) {
		ri := &LDAPObjectResource{
			providerData: &LDAPProviderData{
				Conn: stubLDAPClient{searchFunc: func(*ldap.SearchRequest) (*ldap.SearchResult, error) {
					return nil, errors.New("search error")
				}},
				UsersOU:         "ou=users,dc=example,dc=com",
				DisabledUsersOU: "ou=disabled,dc=example,dc=com",
				userLookupCache: map[string]UserLookupCacheEntry{},
			},
		}
		_, err := ri.resolveSAMToDN(ctx, "test")
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestLDAPObjectResource_ResolveMemberCNs(t *testing.T) {
	ctx := context.Background()

	t.Run("empty list", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		resourceInstance := &LDAPObjectResource{
			providerData: &LDAPProviderData{
				Conn:            backend.client(),
				UsersOU:         "ou=users,dc=example,dc=com",
				DisabledUsersOU: "ou=disabled,dc=example,dc=com",
				userLookupCache: map[string]UserLookupCacheEntry{},
			},
		}
		dns, err := resourceInstance.resolveMemberCNs(ctx, []string{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(dns) != 0 {
			t.Errorf("expected empty result, got %v", dns)
		}
	})
}

func TestLDAPObjectResource_ResolveMemberSAMs(t *testing.T) {
	ctx := context.Background()

	t.Run("empty list", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		resourceInstance := &LDAPObjectResource{
			providerData: &LDAPProviderData{
				Conn:            backend.client(),
				UsersOU:         "ou=users,dc=example,dc=com",
				DisabledUsersOU: "ou=disabled,dc=example,dc=com",
				userLookupCache: map[string]UserLookupCacheEntry{},
			},
		}
		dns, err := resourceInstance.resolveMemberSAMs(ctx, []string{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(dns) != 0 {
			t.Errorf("expected empty result, got %v", dns)
		}
	})
}

func TestLDAPObjectResource_RefreshLDAPState(t *testing.T) {
	ctx := context.Background()

	t.Run("search error", func(t *testing.T) {
		ri := &LDAPObjectResource{
			providerData: &LDAPProviderData{
				Conn: stubLDAPClient{searchFunc: func(*ldap.SearchRequest) (*ldap.SearchResult, error) {
					return nil, errors.New("search error")
				}},
				userLookupCache: map[string]UserLookupCacheEntry{},
			},
		}
		_, err := ri.refreshLDAPState(ctx, "cn=test,dc=example,dc=com")
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestLDAPObjectResource_IsIgnored(t *testing.T) {
	ctx := context.Background()

	t.Run("diagnostics error", func(t *testing.T) {
		model := LDAPObjectResourceModel{
			IgnoreChanges: types.ListNull(types.StringType),
		}
		var diags diag.Diagnostics
		resourceInstance := &LDAPObjectResource{}
		result := resourceInstance.isIgnored(ctx, "cn", &model, diags)
		if result {
			t.Error("expected false on diagnostics error")
		}
	})
}

func TestLDAPObjectResource_AddLdapEntry(t *testing.T) {
	ctx := context.Background()

	t.Run("LDAP add failure", func(t *testing.T) {
		resourceInstance := &LDAPObjectResource{
			providerData: &LDAPProviderData{
				Conn: stubLDAPClient{addFunc: func(*ldap.AddRequest) error {
					return errors.New("add failed")
				}},
				UsersOU:         "ou=users,dc=example,dc=com",
				DisabledUsersOU: "ou=disabled,dc=example,dc=com",
				userLookupCache: map[string]UserLookupCacheEntry{},
			},
		}
		model := &LDAPObjectResourceModel{
			DN:            types.StringValue("cn=test,dc=example,dc=com"),
			ObjectClasses: stringListValue("person"),
			Attributes:    stringSetMapValue(map[string][]string{"cn": []string{"test"}}),
		}
		var diags diag.Diagnostics
		err := resourceInstance.addLdapEntry(ctx, model, &diags)
		if err == nil {
			t.Error("expected error from LDAP add failure")
		}
	})
}

func TestLDAPSAMLookupDataSource_Read(t *testing.T) {
	ctx := context.Background()

	t.Run("empty users_ou", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		pd := &LDAPProviderData{
			Conn:            backend.client(),
			UsersOU:         "",
			DisabledUsersOU: "",
			userLookupCache: map[string]UserLookupCacheEntry{},
		}
		ds := &LDAPSAMLookupDataSource{providerData: pd}
		var schemaResp datasource.SchemaResponse
		ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
		resp := datasource.ReadResponse{State: emptyDatasourceState(schemaResp.Schema)}
		ds.Read(ctx, datasource.ReadRequest{
			Config: datasourceConfig(t, schemaResp.Schema, LDAPSAMLookupDatasourceModel{
				SAMAccountName: types.StringValue("jdoe"),
				RequireFound:   types.BoolNull(),
			}),
		}, &resp)
	})

	t.Run("user found in disabled users OU", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		pd := &LDAPProviderData{
			Conn:            backend.client(),
			UsersOU:         "ou=users,dc=example,dc=com",
			DisabledUsersOU: "ou=disabled,dc=example,dc=com",
			userLookupCache: map[string]UserLookupCacheEntry{},
		}
		ds := &LDAPSAMLookupDataSource{providerData: pd}
		var schemaResp datasource.SchemaResponse
		ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
		resp := datasource.ReadResponse{State: emptyDatasourceState(schemaResp.Schema)}
		ds.Read(ctx, datasource.ReadRequest{
			Config: datasourceConfig(t, schemaResp.Schema, LDAPSAMLookupDatasourceModel{
				SAMAccountName: types.StringValue("duser"),
				RequireFound:   types.BoolNull(),
			}),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
	})

	t.Run("user not found with require_found", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		pd := &LDAPProviderData{
			Conn:            backend.client(),
			UsersOU:         "ou=users,dc=example,dc=com",
			DisabledUsersOU: "ou=disabled,dc=example,dc=com",
			userLookupCache: map[string]UserLookupCacheEntry{},
		}
		ds := &LDAPSAMLookupDataSource{providerData: pd}
		var schemaResp datasource.SchemaResponse
		ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
		resp := datasource.ReadResponse{State: emptyDatasourceState(schemaResp.Schema)}
		ds.Read(ctx, datasource.ReadRequest{
			Config: datasourceConfig(t, schemaResp.Schema, LDAPSAMLookupDatasourceModel{
				SAMAccountName: types.StringValue("nonexistent"),
				RequireFound:   types.BoolValue(true),
			}),
		}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Error("expected error for user not found with require_found")
		}
	})
}

func TestLDAPCNLookupDataSource_Read(t *testing.T) {
	ctx := context.Background()

	t.Run("search error returns empty", func(t *testing.T) {
		pd := &LDAPProviderData{
			Conn: stubLDAPClient{searchFunc: func(*ldap.SearchRequest) (*ldap.SearchResult, error) {
				return nil, errors.New("search error")
			}},
			UsersOU:         "ou=users,dc=example,dc=com",
			DisabledUsersOU: "",
			userLookupCache: map[string]UserLookupCacheEntry{},
		}
		ds := &LDAPCNLookupDataSource{providerData: pd}
		var schemaResp datasource.SchemaResponse
		ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
		resp := datasource.ReadResponse{State: emptyDatasourceState(schemaResp.Schema)}
		ds.Read(ctx, datasource.ReadRequest{
			Config: datasourceConfig(t, schemaResp.Schema, LDAPCNLookupDatasourceModel{
				CommonName:   types.StringValue("test"),
				BaseDN:       types.StringValue("ou=users,dc=example,dc=com"),
				RequireFound: types.BoolNull(),
			}),
		}, &resp)
	})

	t.Run("user found in disabled base DN", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		pd := &LDAPProviderData{
			Conn:            backend.client(),
			UsersOU:         "ou=users,dc=example,dc=com",
			DisabledUsersOU: "ou=disabled,dc=example,dc=com",
			userLookupCache: map[string]UserLookupCacheEntry{},
		}
		ds := &LDAPCNLookupDataSource{providerData: pd}
		var schemaResp datasource.SchemaResponse
		ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
		resp := datasource.ReadResponse{State: emptyDatasourceState(schemaResp.Schema)}
		ds.Read(ctx, datasource.ReadRequest{
			Config: datasourceConfig(t, schemaResp.Schema, LDAPCNLookupDatasourceModel{
				CommonName:     types.StringValue("Disabled User"),
				BaseDN:         types.StringValue("ou=users,dc=example,dc=com"),
				DisabledBaseDN: types.StringValue("ou=disabled,dc=example,dc=com"),
				RequireFound:   types.BoolNull(),
			}),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
	})
}

func TestUpdateWithSystemAttributeFiltering(t *testing.T) {
	ctx := context.Background()
	backend := newFakeLDAPBackend()
	backend.entries["cn=test-system,dc=example,dc=com"] = map[string][]string{
		"objectClass": []string{"person"},
		"cn":          []string{"test-system"},
		"sn":          []string{"Test"},
	}

	resourceInstance := &LDAPObjectResource{
		providerData: &LDAPProviderData{
			Conn:            backend.client(),
			UsersOU:         "ou=users,dc=example,dc=com",
			DisabledUsersOU: "ou=disabled,dc=example,dc=com",
			userLookupCache: map[string]UserLookupCacheEntry{},
		},
	}

	var schemaResp resource.SchemaResponse
	resourceInstance.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	stateModel := LDAPObjectResourceModel{
		ID:            types.StringValue("cn=test-system,dc=example,dc=com"),
		DN:            types.StringValue("cn=test-system,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes: stringSetMapValue(map[string][]string{
			"cn":                []string{"test-system"},
			"sn":                []string{"Test"},
			"objectGUID":        []string{"old-guid"},
			"distinguishedName": []string{"cn=test-system,dc=example,dc=com"},
		}),
		IgnoreChanges: types.ListNull(types.StringType),
	}

	planModel := LDAPObjectResourceModel{
		ID:            types.StringNull(),
		DN:            types.StringValue("cn=test-system,dc=example,dc=com"),
		ObjectClasses: stringListValue("person"),
		Attributes: stringSetMapValue(map[string][]string{
			"cn":                []string{"test-system"},
			"sn":                []string{"Updated"},
			"objectGUID":        []string{"new-guid"},
			"distinguishedName": []string{"cn=test-system,dc=example,dc=com"},
		}),
		IgnoreChanges: types.ListNull(types.StringType),
	}

	updateResp := resource.UpdateResponse{State: emptyResourceState(schemaResp.Schema)}
	resourceInstance.Update(ctx, resource.UpdateRequest{
		State: resourceState(t, schemaResp.Schema, stateModel),
		Plan:  resourcePlan(t, schemaResp.Schema, planModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", updateResp.Diagnostics.Errors())
	}

	entry := backend.entry("cn=test-system,dc=example,dc=com")
	if entry["sn"][0] != "Updated" {
		t.Errorf("expected sn=Updated, got %v", entry["sn"])
	}
}

func TestBinaryAttributeEncodeDecode(t *testing.T) {
	t.Run("objectGUID encoding", func(t *testing.T) {
		original := []string{"test-guid"}
		encoded := encodeAttributeValues("objectGUID", original)
		if len(encoded) != 1 || encoded[0] == original[0] {
			t.Error("expected base64 encoded value")
		}

		decoded := decodeAttributeValues("objectGUID", encoded)
		if len(decoded) != 1 || decoded[0] != original[0] {
			t.Errorf("expected decoded value to match original: %v vs %v", decoded, original)
		}
	})

	t.Run("objectSid encoding", func(t *testing.T) {
		original := []string{"test-sid"}
		encoded := encodeAttributeValues("objectSid", original)
		if len(encoded) != 1 || encoded[0] == original[0] {
			t.Error("expected base64 encoded value")
		}

		decoded := decodeAttributeValues("objectSid", encoded)
		if len(decoded) != 1 || decoded[0] != original[0] {
			t.Errorf("expected decoded value to match original: %v vs %v", decoded, original)
		}
	})

	t.Run("non-binary attribute passes through", func(t *testing.T) {
		original := []string{"plain", "values"}
		encoded := encodeAttributeValues("cn", original)
		if len(encoded) != 2 || encoded[0] != original[0] {
			t.Error("expected non-encoded values for non-binary attribute")
		}

		decoded := decodeAttributeValues("cn", encoded)
		if len(decoded) != 2 || decoded[0] != original[0] {
			t.Error("expected passthrough for non-binary attribute")
		}
	})
}

func TestProviderConfigureEnvVars(t *testing.T) {
	ctx := context.Background()
	providerInstance := New("test")().(*LDAPProvider)
	var schemaResp provider.SchemaResponse
	providerInstance.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

	t.Run("TLS insecure verify env var", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		withFakeDialer(t, func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
			return backend.client(), nil
		})
		t.Setenv("LDAP_TLS_INSECURE_VERIFY", "true")
		var resp provider.ConfigureResponse
		providerInstance.Configure(ctx, provider.ConfigureRequest{
			Config: providerConfig(t, schemaResp.Schema, LDAPProviderModel{
				LDAPURL:          types.StringValue("ldaps://fake.example.com:636"),
				LDAPBindDN:       types.StringValue("cn=admin,dc=example,dc=com"),
				LDAPBindPassword: types.StringValue("admin"),
			}),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
	})

	t.Run("TLS use starttls env var", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		withFakeDialer(t, func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
			return backend.client(), nil
		})
		t.Setenv("LDAP_TLS_USE_STARTTLS", "TRUE")
		var resp provider.ConfigureResponse
		providerInstance.Configure(ctx, provider.ConfigureRequest{
			Config: providerConfig(t, schemaResp.Schema, LDAPProviderModel{
				LDAPURL:          types.StringValue("ldap://fake.example.com:389"),
				LDAPBindDN:       types.StringValue("cn=admin,dc=example,dc=com"),
				LDAPBindPassword: types.StringValue("admin"),
			}),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
	})

	t.Run("config overrides env vars", func(t *testing.T) {
		backend := newFakeLDAPBackend()
		withFakeDialer(t, func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
			return backend.client(), nil
		})
		t.Setenv("LDAP_URL", "ldap://should-be-overridden.com")
		var resp provider.ConfigureResponse
		providerInstance.Configure(ctx, provider.ConfigureRequest{
			Config: providerConfig(t, schemaResp.Schema, LDAPProviderModel{
				LDAPURL:          types.StringValue("ldaps://config-value.com:636"),
				LDAPBindDN:       types.StringValue("cn=admin,dc=example,dc=com"),
				LDAPBindPassword: types.StringValue("admin"),
			}),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
	})

	t.Run("invalid LDAP URL parse", func(t *testing.T) {
		var resp provider.ConfigureResponse
		providerInstance.Configure(ctx, provider.ConfigureRequest{
			Config: providerConfig(t, schemaResp.Schema, LDAPProviderModel{
				LDAPURL:          types.StringValue("invalid:url"),
				LDAPBindDN:       types.StringValue("cn=admin"),
				LDAPBindPassword: types.StringValue("admin"),
			}),
		}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Error("expected error for invalid URL")
		}
	})
}

func TestUnorderedStringListTypeFunctions(t *testing.T) {
	ctx := context.Background()

	t.Run("ValueFromTerraform with valid list", func(t *testing.T) {
		tftVal := tftypes.NewValue(tftypes.List{
			ElementType: tftypes.String,
		}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "a"),
			tftypes.NewValue(tftypes.String, "b"),
		})
		listType := NewUnorderedStringListType()
		result, err := listType.ValueFromTerraform(ctx, tftVal)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("ValueFromTerraform with null list", func(t *testing.T) {
		tftVal := tftypes.NewValue(tftypes.List{
			ElementType: tftypes.String,
		}, nil)
		listType := NewUnorderedStringListType()
		result, err := listType.ValueFromTerraform(ctx, tftVal)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("NewUnorderedStringListValue with elements", func(t *testing.T) {
		elements := []attr.Value{basetypes.NewStringValue("a"), basetypes.NewStringValue("b")}
		val, diags := NewUnorderedStringListValue(elements)
		if diags.HasError() {
			t.Errorf("unexpected diagnostics: %v", diags.Errors())
		}
		if val.IsNull() {
			t.Error("expected non-null value")
		}
	})

	t.Run("NewUnorderedStringListValue with single element", func(t *testing.T) {
		elements := []attr.Value{basetypes.NewStringValue("only")}
		val, diags := NewUnorderedStringListValue(elements)
		if diags.HasError() {
			t.Errorf("unexpected diagnostics: %v", diags.Errors())
		}
		if val.IsNull() {
			t.Error("expected non-null value")
		}
	})

	t.Run("ListSemanticEquals with same values", func(t *testing.T) {
		val1, _ := NewUnorderedStringListValueFromStrings(ctx, []string{"a", "b", "c"})
		val2, _ := NewUnorderedStringListValueFromStrings(ctx, []string{"c", "b", "a"})
		equals, diags := val1.ListSemanticEquals(ctx, val2)
		if diags.HasError() {
			t.Errorf("unexpected diagnostics: %v", diags.Errors())
		}
		if !equals {
			t.Error("expected values to be semantically equal")
		}
	})

	t.Run("ListSemanticEquals with different values", func(t *testing.T) {
		val1, _ := NewUnorderedStringListValueFromStrings(ctx, []string{"a", "b"})
		val2, _ := NewUnorderedStringListValueFromStrings(ctx, []string{"a", "c"})
		equals, _ := val1.ListSemanticEquals(ctx, val2)
		if equals {
			t.Error("expected values to not be semantically equal")
		}
	})

	t.Run("ListSemanticEquals with different types", func(t *testing.T) {
		val := UnorderedStringListValue{}
		equals, diags := val.ListSemanticEquals(ctx, basetypes.NewListValueMust(types.StringType, []attr.Value{}))
		if diags.HasError() {
			t.Errorf("unexpected diagnostics: %v", diags.Errors())
		}
		if equals {
			t.Error("expected false for different types")
		}
	})

	t.Run("ListSemanticEquals with one null", func(t *testing.T) {
		val1, _ := NewUnorderedStringListValueFromStrings(ctx, nil)
		val2, _ := NewUnorderedStringListValueFromStrings(ctx, []string{"a"})
		equals, _ := val1.ListSemanticEquals(ctx, val2)
		if equals {
			t.Error("expected false when one is null")
		}
	})

	t.Run("ListSemanticEquals with one unknown", func(t *testing.T) {
		val1 := UnorderedStringListValue{ListValue: basetypes.NewListUnknown(types.StringType)}
		val2, _ := NewUnorderedStringListValueFromStrings(ctx, []string{"a"})
		equals, _ := val1.ListSemanticEquals(ctx, val2)
		if equals {
			t.Error("expected false when one is unknown")
		}
	})
}
