## 0.1.0 (Unreleased)

BUG FIXES:

- harden provider configuration by rejecting insecure plaintext LDAP binds without STARTTLS
- mark `ldap_bind_password` as sensitive and remove password-bearing debug logging
- escape CN and sAMAccountName lookup filter values to prevent LDAP filter injection
- restore `distinguishedName` handling as a system-managed attribute excluded from write operations
- improve Terraform framework diagnostic propagation in provider, datasource, and resource code paths
- update vulnerable transitive dependencies reported by `govulncheck`
- update direct Go module dependencies to their latest available releases
- require tag-based releases to pass verification and acceptance checks before publishing artifacts

TESTS:

- expand direct unit coverage for provider configuration, resource lifecycle, datasource behavior, and helper functions
- gate acceptance tests behind `TF_ACC=1`
- add CI verification for coverage, `go vet`, `staticcheck`, `gosec`, and `govulncheck`
- update workflows to current maintained GitHub Action versions and replace the third-party compose action with direct `docker compose`
