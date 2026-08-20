package provider

import (
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func TestUniqueGroupDN(t *testing.T) {
	const (
		commonName = "engineering"
		baseDN     = "OU=groups,DC=example,DC=com"
		groupDN    = "CN=engineering,OU=groups,DC=example,DC=com"
	)

	tests := []struct {
		name        string
		entries     []*ldap.Entry
		wantDN      string
		wantFound   bool
		wantErrText string
	}{
		{
			name:      "not found",
			wantFound: false,
		},
		{
			name:      "unique match",
			entries:   []*ldap.Entry{{DN: groupDN}},
			wantDN:    groupDN,
			wantFound: true,
		},
		{
			name: "ambiguous match",
			entries: []*ldap.Entry{
				{DN: groupDN},
				{DN: "CN=engineering,OU=archive,OU=groups,DC=example,DC=com"},
			},
			wantErrText: "multiple groups found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dn, found, err := uniqueGroupDN(commonName, baseDN, tt.entries)
			if dn != tt.wantDN || found != tt.wantFound {
				t.Errorf("uniqueGroupDN() = (%q, %t), want (%q, %t)", dn, found, tt.wantDN, tt.wantFound)
			}
			if tt.wantErrText == "" && err != nil {
				t.Errorf("uniqueGroupDN() returned unexpected error: %s", err)
			}
			if tt.wantErrText != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrText)) {
				t.Errorf("uniqueGroupDN() error = %v, want text %q", err, tt.wantErrText)
			}
		})
	}
}
