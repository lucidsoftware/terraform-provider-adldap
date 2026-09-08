data "ldap_group_cn_lookup" "engineering" {
  common_name = "engineering"
  base_dn     = "OU=groups,DC=example,DC=com"
}

# Use a group as a member of another group.
resource "ldap_object" "all_engineers" {
  dn             = "CN=all-engineers,OU=groups,DC=example,DC=com"
  object_classes = ["top", "group"]

  attributes = {
    member = [data.ldap_group_cn_lookup.engineering.dn]
  }
}
