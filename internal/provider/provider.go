package provider

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"github.com/go-ldap/ldap/v3"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// UserLookupCacheEntry represents a cached user lookup result
type UserLookupCacheEntry struct {
	DN    string // Distinguished name if found, empty if not found
	Found bool   // Whether the user was found
}

// LDAPProviderData contains the LDAP connection and configuration
type LDAPProviderData struct {
	Conn            ldapClient
	UsersOU         string
	DisabledUsersOU string
	// User lookup cache to prevent redundant LDAP queries within a single Terraform run
	userLookupCache map[string]UserLookupCacheEntry
	cacheMutex      sync.RWMutex
}

// Ensure LDAPProvider satisfies various provider interfaces.
var _ provider.Provider = &LDAPProvider{}

// LDAPProvider defines the provider implementation.
type LDAPProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// LDAPProviderModel describes the provider data model.
type LDAPProviderModel struct {
	LDAPURL               types.String `tfsdk:"ldap_url"`
	LDAPBindDN            types.String `tfsdk:"ldap_bind_dn"`
	LDAPBindPassword      types.String `tfsdk:"ldap_bind_password"`
	LDAPTLSInsecureVerify types.Bool   `tfsdk:"ldap_tls_insecure_verify"`
	LDAPTLSUseStartTLS    types.Bool   `tfsdk:"ldap_tls_use_starttls"`
	UsersOU               types.String `tfsdk:"users_ou"`
	DisabledUsersOU       types.String `tfsdk:"disabled_users_ou"`
}

func (p *LDAPProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ldap"
	resp.Version = p.version
}

func (p *LDAPProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Terraform provider to manage and read entries in an LDAP directory.

Inspired by [elastic-infra/ldap](https://registry.terraform.io/providers/elastic-infra/ldap/latest), but updated to
Terraform Framework and including ignoring attributes and a data source.

All provider options can be set by the respective environment variables as well.
`,
		Attributes: map[string]schema.Attribute{
			"ldap_url": schema.StringAttribute{
				MarkdownDescription: "LDAP URL to managed server (`LDAP_URL`)",
				Optional:            true,
			},
			"ldap_bind_dn": schema.StringAttribute{
				MarkdownDescription: "Bind DN used to manage directory (`LDAP_BIND_DN`)",
				Optional:            true,
			},
			"ldap_bind_password": schema.StringAttribute{
				MarkdownDescription: "Bind password (`LDAP_BIND_PASSWORD`)",
				Optional:            true,
				Sensitive:           true,
			},
			"ldap_tls_insecure_verify": schema.BoolAttribute{
				MarkdownDescription: "Whether to skip certificate verification (`LDAP_TLS_INSECURE_VERIFY`)",
				Optional:            true,
			},
			"ldap_tls_use_starttls": schema.BoolAttribute{
				MarkdownDescription: "Whether to connect using STARTTLS (`LDAP_TLS_USE_STARTTLS`)",
				Optional:            true,
			},
			"users_ou": schema.StringAttribute{
				MarkdownDescription: "Base DN for searching active users when resolving member_cn attributes (`LDAP_USERS_OU`)",
				Optional:            true,
			},
			"disabled_users_ou": schema.StringAttribute{
				MarkdownDescription: "Base DN for searching disabled users when resolving member_cn attributes. Defaults to users_ou if not specified (`LDAP_DISABLED_USERS_OU`)",
				Optional:            true,
			},
		},
	}
}

func (p *LDAPProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Debug(ctx, "Checking configuration")
	ldapUrl := os.Getenv("LDAP_URL")
	ldapBindDN := os.Getenv("LDAP_BIND_DN")
	ldapBindPassword := os.Getenv("LDAP_BIND_PASSWORD")
	ldapTLSInsecureVerify := false
	if v := os.Getenv("LDAP_TLS_INSECURE_VERIFY"); v != "" {
		ldapTLSInsecureVerify = strings.ToUpper(v) == "TRUE"
	}

	ldapTLSUseStartTLS := false
	if v := os.Getenv("LDAP_TLS_USE_STARTTLS"); v != "" {
		ldapTLSUseStartTLS = strings.ToUpper(v) == "TRUE"
	}

	ldapUsersOU := os.Getenv("LDAP_USERS_OU")
	ldapDisabledUsersOU := os.Getenv("LDAP_DISABLED_USERS_OU")

	var data LDAPProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.LDAPURL.ValueString() != "" {
		ldapUrl = data.LDAPURL.ValueString()
	}

	if data.LDAPBindDN.ValueString() != "" {
		ldapBindDN = data.LDAPBindDN.ValueString()
	}

	if data.LDAPBindPassword.ValueString() != "" {
		ldapBindPassword = data.LDAPBindPassword.ValueString()
	}

	if !data.LDAPTLSInsecureVerify.IsNull() {
		ldapTLSInsecureVerify = data.LDAPTLSInsecureVerify.ValueBool()
	}

	if !data.LDAPTLSUseStartTLS.IsNull() {
		ldapTLSUseStartTLS = data.LDAPTLSUseStartTLS.ValueBool()
	}

	if data.UsersOU.ValueString() != "" {
		ldapUsersOU = data.UsersOU.ValueString()
	}

	if data.DisabledUsersOU.ValueString() != "" {
		ldapDisabledUsersOU = data.DisabledUsersOU.ValueString()
	}

	if ldapUrl == "" {
		resp.Diagnostics.AddError(
			"No LDAP url specified",
			"Configure the ldap_url attribute or LDAP_URL environment variable for the provider",
		)
		return
	}

	if ldapBindDN == "" {
		resp.Diagnostics.AddError(
			"No LDAP bind dn specified",
			"Configure the ldap_bind_dn attribute or LDAP_BIND_DN environment variable for the provider",
		)
		return
	}

	if ldapBindPassword == "" {
		resp.Diagnostics.AddError(
			"No LDAP bind password specified",
			"Configure the ldap_bind_password attribute or LDAP_BIND_PASSWORD environment variable for the provider",
		)
		return
	}

	ctx = tflog.MaskLogStrings(ctx, ldapBindPassword)

	parsedLDAPURL, err := url.Parse(ldapUrl)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid LDAP URL",
			fmt.Sprintf("Could not parse LDAP URL %q: %s", ldapUrl, err),
		)
		return
	}

	switch parsedLDAPURL.Scheme {
	case "ldaps":
		if ldapTLSUseStartTLS {
			resp.Diagnostics.AddError(
				"Invalid LDAP TLS configuration",
				"ldap_tls_use_starttls cannot be enabled when ldap_url already uses the ldaps:// scheme",
			)
			return
		}
	case "ldap":
		if !ldapTLSUseStartTLS {
			resp.Diagnostics.AddError(
				"Insecure LDAP configuration",
				"Refusing to bind over ldap:// without STARTTLS. Use ldaps:// or set ldap_tls_use_starttls = true.",
			)
			return
		}
	default:
		resp.Diagnostics.AddError(
			"Unsupported LDAP URL scheme",
			fmt.Sprintf("LDAP URL %q must use ldap:// or ldaps://", ldapUrl),
		)
		return
	}

	loggerAdapter := TFLoggerAdapter{ctx: ctx}
	logger := log.New(loggerAdapter, "", log.LstdFlags)
	ldap.Logger(logger)

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: ldapTLSInsecureVerify,
	}
	if host := parsedLDAPURL.Hostname(); host != "" {
		tlsConfig.ServerName = host
	}

	o := []ldap.DialOpt{
		ldap.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}),
	}

	if ldapTLSInsecureVerify {
		tflog.Debug(ctx, "Connecting insecurely to the LDAP server")
	}

	if parsedLDAPURL.Scheme == "ldaps" {
		o = append(o, ldap.DialWithTLSConfig(tlsConfig))
	}

	tflog.Debug(ctx, "Connecting to LDAP server", map[string]interface{}{"url": ldapUrl})
	if conn, err := ldapDialURL(ldapUrl, o...); err != nil {
		resp.Diagnostics.AddError(
			"Can't connect to LDAP server",
			fmt.Sprintf("Error connecting to LDAP server: %s", err),
		)
		return
	} else {
		conn.SetDebug(os.Getenv("TF_LOG") == "DEBUG")
		if ldapTLSUseStartTLS {
			tflog.Debug(ctx, "Connecting using StartTLS")
			if err := conn.StartTLS(tlsConfig); err != nil {
				_ = conn.Close()
				resp.Diagnostics.AddError(
					"Can't start TLS",
					fmt.Sprintf("Error starting TLS: %s", err),
				)
				return
			}
		}
		tflog.Debug(ctx, "Binding to LDAP server", map[string]interface{}{"bindDN": ldapBindDN})
		if err := conn.Bind(ldapBindDN, ldapBindPassword); err != nil {
			_ = conn.Close()
			resp.Diagnostics.AddError(
				"Can't bind to LDAP server",
				fmt.Sprintf("Error binding to LDAP server: %s", err),
			)
			return
		}

		// Default disabled_users_ou to users_ou if not specified
		if ldapDisabledUsersOU == "" {
			ldapDisabledUsersOU = ldapUsersOU
		}

		providerData := &LDAPProviderData{
			Conn:            conn,
			UsersOU:         ldapUsersOU,
			DisabledUsersOU: ldapDisabledUsersOU,
			userLookupCache: make(map[string]UserLookupCacheEntry),
		}

		resp.DataSourceData = providerData
		resp.ResourceData = providerData
	}
}

func (p *LDAPProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewLDAPObjectResource,
	}
}

func (p *LDAPProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewLDAPObjectDataSource,
		NewLDAPSearchDataSource,
		NewLDAPSAMLookupDataSource,
		NewLDAPCNLookupDataSource,
	}
}

// generateCacheKey creates a FIPS 140-3 compliant checksummed cache key from the raw key components
func generateCacheKey(rawKey string) string {
	hasher := sha256.New()
	hasher.Write([]byte(rawKey))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// cachedUserLookup performs a user lookup with caching support
func (p *LDAPProviderData) cachedUserLookup(ctx context.Context, rawKey string, lookupFunc func() (string, bool)) (string, bool) {
	// Generate checksummed cache key
	cacheKey := generateCacheKey(rawKey)
	
	// Check cache first (read lock)
	p.cacheMutex.RLock()
	if entry, exists := p.userLookupCache[cacheKey]; exists {
		p.cacheMutex.RUnlock()
		tflog.Debug(ctx, "User lookup cache hit", map[string]interface{}{
			"cache_key_raw":  rawKey,
			"cache_key_hash": cacheKey,
			"found":         entry.Found,
		})
		return entry.DN, entry.Found
	}
	p.cacheMutex.RUnlock()
	
	// Cache miss - perform LDAP lookup
	tflog.Debug(ctx, "User lookup cache miss - performing LDAP query", map[string]interface{}{
		"cache_key_raw":  rawKey,
		"cache_key_hash": cacheKey,
	})
	
	dn, found := lookupFunc()
	
	// Store result in cache (write lock)
	p.cacheMutex.Lock()
	p.userLookupCache[cacheKey] = UserLookupCacheEntry{
		DN:    dn,
		Found: found,
	}
	p.cacheMutex.Unlock()
	
	tflog.Debug(ctx, "User lookup result cached", map[string]interface{}{
		"cache_key_raw":  rawKey,
		"cache_key_hash": cacheKey,
		"found":         found,
	})
	
	return dn, found
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &LDAPProvider{
			version: version,
		}
	}
}
