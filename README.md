# Terraform LDAP provider

Terraform provider to manage and read entries in an LDAP directory with advanced Active Directory support.

Inspired by [elastic-infra/ldap](https://registry.terraform.io/providers/elastic-infra/ldap/latest)
Forked from [doddevops/ldap](https://github.com/dodevops/terraform-provider-ldap)
Updated to Terraform Plugin Framework v2 with comprehensive LDAP features including:

- **Active Directory memberOf Compliance**: Complete elimination of memberOf processing, respecting AD's read-only constraint
- **Active Directory Compliant**: Respects AD's read-only memberOf constraint - use direct member relationships
- **User Lifecycle Management**: Automatically resolves user CNs and sAMAccountNames to current DNs
- **LDAP Reverse Reference Integrity**: Handles automatic LDAP reference updates gracefully
- **Unordered Attribute Handling**: Proper handling of LDAP multi-valued attributes where order is not significant

## Using the provider

Add the following Terraform code to start using the provider:

```terraform
terraform {
  required_providers {
    ldap = {
      source  = "registry.terraform.io/lucidsoftware/adldap"
      version = "~> 0.8.0"
    }
  }
}

provider "ldap" {
  ldap_url             = "ldaps://ldap.example.com:636"
  ldap_bind_dn         = "cn=service-account,ou=service-accounts,dc=example,dc=com"
  ldap_bind_password   = var.ldap_password
  users_ou             = "ou=users,ou=people,dc=example,dc=com"
  disabled_users_ou    = "ou=disabled,ou=people,dc=example,dc=com"
}
```

### Advanced Features

#### User Lifecycle Management with member_cn and member_sam

The provider supports automatic resolution of user Common Names and sAMAccountNames to current Distinguished Names, handling user lifecycle changes transparently:

```terraform
resource "ldap_object" "ai_users_group" {
  dn             = "cn=ai_users,ou=groups,ou=department,dc=example,dc=com"
  object_classes = ["top", "group"]
  
  # Traditional DN-based membership (brittle to user moves)
  attributes = {
    cn                = ["ai_users"]
    name              = ["ai_users"]
    sAMAccountName    = ["ai_users"]
    distinguishedName = ["cn=ai_users,ou=groups,ou=department,dc=example,dc=com"]
    
    # Use either member_cn or member_sam for resilient membership
    member = [
      "cn=John Smith,ou=users,ou=people,dc=example,dc=com",  # Breaks if user moves OUs
    ]
  }
  
  # Resilient membership using Common Names (recommended)
  member_cn = [
    "John Smith",    # Automatically resolves to current DN
    "Jane Doe",      # Handles user moves between OUs seamlessly
    "Bob Wilson"
  ]
  
  # Alternative: Resilient membership using sAMAccountNames
  member_sam = [
    "jsmith",        # Automatically resolves to current DN
    "jdoe",          # Handles user lifecycle changes
    "bwilson"
  ]
}
```

#### Active Directory Group Membership Management

**Important**: Active Directory treats `memberOf` as read-only. Manage group relationships using direct `member` attributes in parent groups:

```terraform
# ✅ CORRECT: Manage from parent group
resource "ldap_object" "administrators" {
  dn             = "CN=Administrators,OU=groups,DC=example,DC=com"
  object_classes = ["top", "group"]
  
  attributes = {
    member = [ldap_object.user.dn]
  }
}

# ✅ Active Directory automatically maintains memberOf for you
resource "ldap_object" "user" {
  dn             = "CN=jsmith,OU=users,DC=example,DC=com"
  object_classes = ["top", "person", "user"]
  
  attributes = {
    cn              = ["jsmith"]
    sAMAccountName  = ["jsmith"]
    # memberOf is automatically maintained by AD - don't specify it
  }
}
```

#### LDAP Reverse Reference Integrity Support

The provider automatically handles LDAP reference integrity issues that occur when:
- Objects are deleted and LDAP auto-removes references
- Users move between OUs and DNs change  
- Group hierarchies are modified

**Example scenario handled gracefully:**
1. Delete an object that other groups reference in `member` attributes
2. LDAP automatically removes stale references through referential integrity
3. Provider detects the change and updates Terraform state accordingly
4. No "Provider produced invalid plan" errors

## System Attributes

The provider automatically excludes certain system-managed attributes from LDAP operations to prevent "Unwilling To Perform" errors when working with directories like Active Directory. These attributes include:

- `objectGUID`, `objectSid`  
- `distinguishedName`
- `dSCorePropagationData`, `instanceType`
- `whenCreated`, `whenChanged`
- `uSNCreated`, `uSNChanged`
- `memberOf` (read-only in Active Directory)

These attributes can be read using the `ldap_object` data source if needed.

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, first make sure to have a running LDAP server. We've included a 
docker-compose file to quickly start a matching test server.

    cd contrib/test-ldap-server
    docker-compose up -d

Then you can set the following environment variables:

- LDAP_NONTLS_URL: The non-TLS enabled URL to the LDAP server
- LDAP_BIND_DN: The bind DN to access the LDAP server
- LDAP_BIND_PASSWORD: The bind password to access the LDAP server
- LDAP_TLS_URL: The TLS enabled URL to access the LDAP server

The URL variables are used to test the non-tls, TLS and STARTTLS features of the provider.

If you use the provided test server, the variables are already set for you.

Afterwards run `make testacc`.
