package provider

import (
	"crypto/tls"

	"github.com/go-ldap/ldap/v3"
)

type ldapClient interface {
	Add(*ldap.AddRequest) error
	Bind(string, string) error
	Close() error
	Del(*ldap.DelRequest) error
	Modify(*ldap.ModifyRequest) error
	Search(*ldap.SearchRequest) (*ldap.SearchResult, error)
	SetDebug(bool)
	StartTLS(*tls.Config) error
}

type ldapClientAdapter struct {
	conn *ldap.Conn
}

func (a *ldapClientAdapter) Add(req *ldap.AddRequest) error {
	return a.conn.Add(req)
}

func (a *ldapClientAdapter) Bind(username, password string) error {
	return a.conn.Bind(username, password)
}

func (a *ldapClientAdapter) Close() error {
	return a.conn.Close()
}

func (a *ldapClientAdapter) Del(req *ldap.DelRequest) error {
	return a.conn.Del(req)
}

func (a *ldapClientAdapter) Modify(req *ldap.ModifyRequest) error {
	return a.conn.Modify(req)
}

func (a *ldapClientAdapter) Search(req *ldap.SearchRequest) (*ldap.SearchResult, error) {
	return a.conn.Search(req)
}

func (a *ldapClientAdapter) SetDebug(enabled bool) {
	if enabled {
		a.conn.Debug = true
		return
	}

	a.conn.Debug = false
}

func (a *ldapClientAdapter) StartTLS(config *tls.Config) error {
	return a.conn.StartTLS(config)
}

var ldapDialURL = func(target string, opts ...ldap.DialOpt) (ldapClient, error) {
	conn, err := ldap.DialURL(target, opts...)
	if err != nil {
		return nil, err
	}

	return &ldapClientAdapter{conn: conn}, nil
}
