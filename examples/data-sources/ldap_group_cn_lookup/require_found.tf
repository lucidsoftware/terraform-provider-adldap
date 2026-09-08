# Fail fast if this nested group does not exist.
data "ldap_group_cn_lookup" "engineering" {
  common_name   = "engineering"
  base_dn       = "OU=groups,DC=example,DC=com"
  require_found = true
}
