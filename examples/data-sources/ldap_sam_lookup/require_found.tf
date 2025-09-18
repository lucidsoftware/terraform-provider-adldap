# Fail-fast validation - errors if user doesn't exist
data "ldap_sam_lookup" "required_user" {
  sam_account_name = "admin"
  require_found    = true  # Will fail if admin user doesn't exist
}

# Conditional logic based on user existence
data "ldap_sam_lookup" "optional_user" {
  sam_account_name = "contractor1"
  require_found    = false  # Default behavior
}

resource "ldap_object" "conditional_group" {
  dn             = "CN=project-team,OU=groups,DC=example,DC=com"
  object_classes = ["top", "group"]
  
  attributes = {
    # Only add contractor if they exist
    member = data.ldap_sam_lookup.optional_user.found ? [
      data.ldap_sam_lookup.required_user.dn,
      data.ldap_sam_lookup.optional_user.dn
    ] : [
      data.ldap_sam_lookup.required_user.dn
    ]
  }
}