# Fail-fast validation with lifecycle support
data "ldap_cn_lookup" "manager" {
  common_name      = "Jane Smith"
  base_dn          = "OU=users,OU=people,DC=example,DC=com"
  disabled_base_dn = "OU=disabled,OU=people,DC=example,DC=com"
  require_found    = true # Will fail if user doesn't exist in either OU
}

# Validate multiple users exist before creating group
data "ldap_cn_lookup" "team_lead" {
  common_name   = "Bob Wilson"
  base_dn       = "OU=users,OU=people,DC=example,DC=com"
  require_found = true
}

resource "ldap_object" "leadership_team" {
  dn             = "CN=leadership,OU=groups,DC=example,DC=com"
  object_classes = ["top", "group"]

  attributes = {
    description = ["Leadership team - all members validated"]
    member = [
      data.ldap_cn_lookup.manager.dn,
      data.ldap_cn_lookup.team_lead.dn
    ]
  }
}