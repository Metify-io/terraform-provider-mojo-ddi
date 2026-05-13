terraform {
  required_providers {
    mojo = {
      source  = "metify/mojo-ddi"
      version = "~> 0.1"
    }
  }
}

provider "mojo" {
  endpoint = "https://mojo.local:8443/mcp"
  # api_key from MOJO_API_KEY env var
}

# Create a VRF for the staging environment
resource "mojo_vrf" "staging" {
  name           = "staging"
  description    = "Staging environment VRF"
  enforce_unique = true
}
