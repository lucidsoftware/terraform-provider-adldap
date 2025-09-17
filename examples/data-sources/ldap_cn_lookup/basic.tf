data "ldap_cn_lookup" "user" {
  common_name = "John Doe"
  base_dn     = "OU=users,OU=people,DC=example,DC=com"
}

# Use in group membership  
resource "ldap_object" "managers" {
  dn             = "CN=managers,OU=groups,DC=example,DC=com"
  object_classes = ["top", "group"]
  
  attributes = {
    member = [data.ldap_cn_lookup.user.dn]
  }
}