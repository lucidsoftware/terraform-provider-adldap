Refactor git@github.com:bakaut/terraform-provider-adldap.git with a strict security-first scope.

Goals:
- 100% Go test coverage for all handwritten production code
- full security audit
- fix all actionable vulnerabilities
- minimal diff
- no cosmetic refactoring
- no feature work
- no API changes unless required for security or testability

Rules:
- change code only for security, vulnerability remediation, bug fixes discovered during the audit, or to enable meaningful tests for 100% coverage
- preserve behavior wherever possible
- document every non-trivial change with justification
- explicitly report residual risks and anything that cannot be fixed safely

Required checks:
- go test ./... -coverpkg=./... -coverprofile=coverage.out
- go tool cover -func=coverage.out
- govulncheck ./...
- gosec ./...
- staticcheck ./...
- go vet ./...
- review dependencies in go.mod/go.sum
- inspect LDAP/TLS handling, secret leakage, Terraform Sensitive/state handling, diagnostics, LDAP filter injection, insecure defaults, timeout handling, error paths

Required outputs:
1. short plan
2. code + tests + CI updates
3. summary of findings and fixes
4. final coverage report proving 100%
5. final security report with:
   - findings
   - severity
   - remediation
   - residual risks
   - follow-up recommendations
6. exact commands to reproduce all checks locally

Acceptance criteria:
- 100% coverage proven
- all actionable vulnerabilities fixed
- scanners clean or remaining findings clearly justified
- no unnecessary refactoring

Start now.