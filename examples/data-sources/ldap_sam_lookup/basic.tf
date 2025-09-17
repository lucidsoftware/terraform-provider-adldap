data "ldap_sam_lookup" "user" {
  sam_account_name = "jdoe"
}

# Use in group membership
resource "ldap_object" "group" {
  dn             = "CN=developers,OU=groups,DC=example,DC=com"
  object_classes = ["top", "group"]
  
  attributes = {
    member = [data.ldap_sam_lookup.user.dn]
  }
}