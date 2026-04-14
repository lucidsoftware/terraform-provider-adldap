package provider

import (
	"crypto/tls"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type fakeLDAPClient struct {
	backend *fakeLDAPBackend
	closed  bool
	debug   bool
}

type fakeLDAPBackend struct {
	mu             sync.Mutex
	entries        map[string]map[string][]string
	lastSearch     *ldap.SearchRequest
	lastAdd        *ldap.AddRequest
	lastModify     *ldap.ModifyRequest
	lastDelete     *ldap.DelRequest
	startTLSCalls  int
	bindCalls      int
	expectedBindDN string
	expectedPass   string
}

func newFakeLDAPBackend() *fakeLDAPBackend {
	return &fakeLDAPBackend{
		entries: map[string]map[string][]string{
			"dc=example,dc=com": {
				"objectClass":  []string{"top", "domain", "organization"},
				"dc":           []string{"example"},
				"creatorsName": []string{"cn=admin,dc=example,dc=com"},
			},
			"cn=John Doe,ou=users,dc=example,dc=com": {
				"objectClass":       []string{"top", "person", "user"},
				"cn":                []string{"John Doe"},
				"sn":                []string{"Doe"},
				"objectCategory":    []string{"Person"},
				"sAMAccountName":    []string{"jdoe"},
				"distinguishedName": []string{"cn=John Doe,ou=users,dc=example,dc=com"},
			},
			"cn=Disabled User,ou=disabled,dc=example,dc=com": {
				"objectClass":       []string{"top", "person", "user"},
				"cn":                []string{"Disabled User"},
				"sn":                []string{"User"},
				"objectCategory":    []string{"Person"},
				"sAMAccountName":    []string{"duser"},
				"distinguishedName": []string{"cn=Disabled User,ou=disabled,dc=example,dc=com"},
			},
			"cn=importtest,dc=example,dc=com": {
				"objectClass": []string{"person"},
				"cn":          []string{"importtest"},
				"sn":          []string{"test"},
			},
		},
		expectedBindDN: "cn=admin,dc=example,dc=com",
		expectedPass:   "admin",
	}
}

func (b *fakeLDAPBackend) cloneEntry(dn string, attributes map[string][]string, requested []string) *ldap.Entry {
	if len(requested) == 0 || containsString(requested, "*") {
		return ldap.NewEntry(dn, cloneStringMap(attributes))
	}

	selected := make(map[string][]string)
	for _, attribute := range requested {
		if attribute == "*" {
			return ldap.NewEntry(dn, cloneStringMap(attributes))
		}
		if attribute == "distinguishedName" {
			selected[attribute] = []string{dn}
			continue
		}
		if values, ok := attributes[attribute]; ok {
			selected[attribute] = append([]string(nil), values...)
		}
	}

	return ldap.NewEntry(dn, selected)
}

func (b *fakeLDAPBackend) entry(dn string) map[string][]string {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.entries[dn]
	if !ok {
		return nil
	}

	return cloneStringMap(entry)
}

func (b *fakeLDAPBackend) client() ldapClient {
	return &fakeLDAPClient{backend: b}
}

func (b *fakeLDAPBackend) setAttribute(dn, attribute string, values []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry := b.entries[dn]
	entry[attribute] = append([]string(nil), values...)
}

func (c *fakeLDAPClient) Add(req *ldap.AddRequest) error {
	c.backend.mu.Lock()
	defer c.backend.mu.Unlock()

	c.backend.lastAdd = req
	if _, exists := c.backend.entries[req.DN]; exists {
		return fmt.Errorf("entry already exists")
	}

	entry := make(map[string][]string)
	for _, attribute := range req.Attributes {
		entry[attribute.Type] = append([]string(nil), attribute.Vals...)
	}
	c.backend.entries[req.DN] = entry

	return nil
}

func (c *fakeLDAPClient) Bind(username, password string) error {
	c.backend.mu.Lock()
	defer c.backend.mu.Unlock()

	c.backend.bindCalls++
	if username != c.backend.expectedBindDN || password != c.backend.expectedPass {
		return fmt.Errorf("invalid credentials")
	}

	return nil
}

func (c *fakeLDAPClient) Close() error {
	c.closed = true
	return nil
}

func (c *fakeLDAPClient) Del(req *ldap.DelRequest) error {
	c.backend.mu.Lock()
	defer c.backend.mu.Unlock()

	c.backend.lastDelete = req
	delete(c.backend.entries, req.DN)
	return nil
}

func (c *fakeLDAPClient) Modify(req *ldap.ModifyRequest) error {
	c.backend.mu.Lock()
	defer c.backend.mu.Unlock()

	c.backend.lastModify = req
	entry, exists := c.backend.entries[req.DN]
	if !exists {
		return fmt.Errorf("entry not found")
	}

	for _, change := range req.Changes {
		switch change.Operation {
		case ldap.AddAttribute:
			entry[change.Modification.Type] = append(entry[change.Modification.Type], change.Modification.Vals...)
		case ldap.DeleteAttribute:
			delete(entry, change.Modification.Type)
		case ldap.ReplaceAttribute:
			entry[change.Modification.Type] = append([]string(nil), change.Modification.Vals...)
		default:
			return fmt.Errorf("unsupported modify operation %d", change.Operation)
		}
	}

	return nil
}

func (c *fakeLDAPClient) Search(req *ldap.SearchRequest) (*ldap.SearchResult, error) {
	c.backend.mu.Lock()
	defer c.backend.mu.Unlock()

	c.backend.lastSearch = req
	var dns []string
	for dn := range c.backend.entries {
		if !matchesScope(dn, req.BaseDN, req.Scope) {
			continue
		}
		if !matchesFilter(c.backend.entries[dn], req.Filter) {
			continue
		}
		dns = append(dns, dn)
	}
	sort.Strings(dns)

	result := &ldap.SearchResult{}
	for _, dn := range dns {
		result.Entries = append(result.Entries, c.backend.cloneEntry(dn, c.backend.entries[dn], req.Attributes))
	}

	return result, nil
}

func (c *fakeLDAPClient) SetDebug(enabled bool) {
	c.debug = enabled
}

func (c *fakeLDAPClient) StartTLS(_ *tls.Config) error {
	c.backend.mu.Lock()
	defer c.backend.mu.Unlock()

	c.backend.startTLSCalls++
	return nil
}

func cloneStringMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func decodeEscapedFilterValue(value string) string {
	replacer := strings.NewReplacer(
		`\\2a`, "*",
		`\\28`, "(",
		`\\29`, ")",
		`\\5c`, `\`,
		`\\00`, "",
	)
	return replacer.Replace(value)
}

func matchesFilter(entry map[string][]string, filter string) bool {
	if filter == "" || filter == "(&)" {
		return true
	}

	filter = strings.TrimPrefix(filter, "(&")
	filter = strings.TrimSuffix(filter, ")")
	filter = strings.TrimSuffix(filter, ")")
	parts := strings.Split(filter, ")(")

	for _, part := range parts {
		part = strings.TrimPrefix(part, "(")
		part = strings.TrimSuffix(part, ")")
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		attribute := kv[0]
		expected := decodeEscapedFilterValue(kv[1])
		values := entry[attribute]
		if !containsString(values, expected) {
			return false
		}
	}

	return true
}

func matchesScope(dn, baseDN string, scope int) bool {
	normalizedDN := strings.ToLower(dn)
	normalizedBase := strings.ToLower(baseDN)

	switch scope {
	case ldap.ScopeBaseObject:
		return normalizedDN == normalizedBase
	case ldap.ScopeWholeSubtree:
		return normalizedDN == normalizedBase || strings.HasSuffix(normalizedDN, ","+normalizedBase)
	default:
		return false
	}
}

func withFakeDialer(t *testing.T, dialer func(string, ...ldap.DialOpt) (ldapClient, error)) {
	t.Helper()

	original := ldapDialURL
	ldapDialURL = dialer
	t.Cleanup(func() {
		ldapDialURL = original
	})
}

func checkBackendAttribute(backend *fakeLDAPBackend, dn, attribute string, expected []string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		entry := backend.entry(dn)
		if entry == nil {
			return fmt.Errorf("entry %q not found", dn)
		}

		got := entry[attribute]
		if strings.Join(got, "|") != strings.Join(expected, "|") {
			return fmt.Errorf("attribute %s = %v, want %v", attribute, got, expected)
		}
		return nil
	}
}

func checkBackendEntryMissing(backend *fakeLDAPBackend, dn string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if entry := backend.entry(dn); entry != nil {
			return fmt.Errorf("entry %q still exists", dn)
		}
		return nil
	}
}

func TestLDAPProviderResourceLifecycleWithFakeBackend(t *testing.T) {
	backend := newFakeLDAPBackend()
	withFakeDialer(t, func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
		return backend.client(), nil
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig + fakeCreateObjectConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkBackendAttribute(backend, "cn=test,dc=example,dc=com", "sn", []string{"test"}),
					checkBackendAttribute(backend, "cn=test,dc=example,dc=com", "userPassword", []string{"password"}),
				),
			},
			{
				Config: fakeProviderConfig + fakeUpdateObjectConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkBackendAttribute(backend, "cn=test,dc=example,dc=com", "sn", []string{"test2"}),
					checkBackendAttribute(backend, "cn=test,dc=example,dc=com", "uid", []string{"test"}),
				),
			},
			{
				PreConfig: func() {
					backend.setAttribute("cn=test,dc=example,dc=com", "userPassword", []string{"rotated"})
				},
				Config: fakeProviderConfig + fakeUpdateObjectConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkBackendAttribute(backend, "cn=test,dc=example,dc=com", "userPassword", []string{"rotated"}),
				),
			},
			{
				Config: fakeProviderConfig + fakeRenameObjectConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkBackendEntryMissing(backend, "cn=test,dc=example,dc=com"),
					checkBackendAttribute(backend, "cn=test2,dc=example,dc=com", "cn", []string{"test2"}),
				),
			},
		},
	})
}

func TestLDAPProviderDataSourcesWithFakeBackend(t *testing.T) {
	backend := newFakeLDAPBackend()
	withFakeDialer(t, func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
		return backend.client(), nil
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig + fakeDataSourcesConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ldap_object.root", "dn", "dc=example,dc=com"),
					resource.TestCheckResourceAttr("data.ldap_search.root", "results.0.dc.0", "example"),
					resource.TestCheckResourceAttr("data.ldap_sam_lookup.user", "found", "true"),
					resource.TestCheckResourceAttr("data.ldap_cn_lookup.user", "found", "true"),
				),
			},
			{
				Config:      fakeProviderConfig + fakeSAMLookupMissingConfig,
				ExpectError: regexpMustCompile(`SAM account .* require_found is true`),
			},
			{
				Config:      fakeProviderConfig + fakeCNLookupMissingConfig,
				ExpectError: regexpMustCompile(`Common Name .* require_found is true`),
			},
		},
	})
}

func TestLDAPProviderConfigureSecurityValidation(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		expectExpr string
	}{
		{
			name:       "rejects plaintext ldap without starttls",
			config:     insecureProviderConfig + fakeSimpleDataSourceConfig,
			expectExpr: `Refusing to bind over ldap:// without STARTTLS`,
		},
		{
			name:       "rejects ldaps with starttls",
			config:     invalidStartTLSProviderConfig + fakeSimpleDataSourceConfig,
			expectExpr: `ldap_tls_use_starttls cannot be enabled`,
		},
		{
			name:       "rejects unsupported scheme",
			config:     unsupportedSchemeProviderConfig + fakeSimpleDataSourceConfig,
			expectExpr: `must use ldap:// or ldaps://`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newFakeLDAPBackend()
			withFakeDialer(t, func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
				return backend.client(), nil
			})

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      test.config,
						ExpectError: regexpMustCompile(test.expectExpr),
					},
				},
			})
		})
	}
}

func TestLDAPProviderConfigureDialFailures(t *testing.T) {
	withFakeDialer(t, func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
		return nil, fmt.Errorf("dial failed")
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fakeProviderConfig + fakeSimpleDataSourceConfig,
				ExpectError: regexpMustCompile(`Error connecting to LDAP server: dial failed`),
			},
		},
	})
}

func TestLDAPObjectImportStateWithFakeBackend(t *testing.T) {
	backend := newFakeLDAPBackend()
	withFakeDialer(t, func(_ string, _ ...ldap.DialOpt) (ldapClient, error) {
		return backend.client(), nil
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig + fakeImportObjectConfig,
			},
			{
				ResourceName:      "ldap_object.importtest",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "cn=importtest,dc=example,dc=com",
			},
		},
	})
}

func regexpMustCompile(expr string) *regexp.Regexp {
	return regexp.MustCompile(expr)
}

const fakeProviderConfig = `
provider "ldap" {
  ldap_url = "ldaps://fake.example.com:636"
  ldap_bind_dn = "cn=admin,dc=example,dc=com"
  ldap_bind_password = "admin"
  users_ou = "ou=users,dc=example,dc=com"
  disabled_users_ou = "ou=disabled,dc=example,dc=com"
}
`

const insecureProviderConfig = `
provider "ldap" {
  ldap_url = "ldap://fake.example.com:389"
  ldap_bind_dn = "cn=admin,dc=example,dc=com"
  ldap_bind_password = "admin"
}
`

const invalidStartTLSProviderConfig = `
provider "ldap" {
  ldap_url = "ldaps://fake.example.com:636"
  ldap_bind_dn = "cn=admin,dc=example,dc=com"
  ldap_bind_password = "admin"
  ldap_tls_use_starttls = true
}
`

const unsupportedSchemeProviderConfig = `
provider "ldap" {
  ldap_url = "http://fake.example.com"
  ldap_bind_dn = "cn=admin,dc=example,dc=com"
  ldap_bind_password = "admin"
}
`

const fakeSimpleDataSourceConfig = `
data "ldap_object" "root" {
  dn = "dc=example,dc=com"
}
`

const fakeCreateObjectConfig = `
resource "ldap_object" "test" {
  dn = "cn=test,dc=example,dc=com"
  object_classes = ["person"]
  attributes = {
    cn = ["test"]
    sn = ["test"]
    userPassword = ["password"]
    distinguishedName = ["cn=test,dc=example,dc=com"]
  }
  ignore_changes = ["userPassword"]
}
`

const fakeUpdateObjectConfig = `
resource "ldap_object" "test" {
  dn = "cn=test,dc=example,dc=com"
  object_classes = ["person", "uidObject"]
  attributes = {
    cn = ["test"]
    sn = ["test2"]
    uid = ["test"]
    userPassword = ["password"]
  }
  ignore_changes = ["userPassword"]
}
`

const fakeRenameObjectConfig = `
resource "ldap_object" "test" {
  dn = "cn=test2,dc=example,dc=com"
  object_classes = ["person", "uidObject"]
  attributes = {
    cn = ["test2"]
    sn = ["test2"]
    uid = ["test"]
    userPassword = ["password"]
  }
  ignore_changes = ["userPassword"]
}
`

const fakeImportObjectConfig = `
resource "ldap_object" "importtest" {
  dn = "cn=importtest,dc=example,dc=com"
  object_classes = ["person"]
  attributes = {
    cn = ["importtest"]
    sn = ["test"]
  }
}
`

const fakeDataSourcesConfig = `
data "ldap_object" "root" {
  dn = "dc=example,dc=com"
  additional_attributes = ["creatorsName"]
}

data "ldap_search" "root" {
  base_dn = "dc=example,dc=com"
  additional_attributes = ["creatorsName"]
}

data "ldap_sam_lookup" "user" {
  sam_account_name = "jdoe"
}

data "ldap_cn_lookup" "user" {
  common_name = "John Doe"
  base_dn = "ou=users,dc=example,dc=com"
  disabled_base_dn = "ou=disabled,dc=example,dc=com"
}
`

const fakeSAMLookupMissingConfig = `
data "ldap_sam_lookup" "missing" {
  sam_account_name = "missing"
  require_found = true
}
`

const fakeCNLookupMissingConfig = `
data "ldap_cn_lookup" "missing" {
  common_name = "Missing"
  base_dn = "ou=users,dc=example,dc=com"
  disabled_base_dn = "ou=disabled,dc=example,dc=com"
  require_found = true
}
`
