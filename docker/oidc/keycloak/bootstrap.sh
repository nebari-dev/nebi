#!/bin/bash
# Post-import Keycloak configuration that a realm import cannot express.
#
# nebi always requests scope "openid profile email groups". Keycloak rejects
# unknown scopes (invalid_scope), so the realm needs a "groups" client scope.
# Declaring clientScopes in nebi-realm.json would stop Keycloak from creating
# its built-in scopes (profile, email, roles, ...), so create it here instead.
# Idempotent: safe on every `docker compose up`.
set -euo pipefail
K=/opt/keycloak/bin/kcadm.sh
R=nebi

$K config credentials --server http://keycloak:8080 --realm master \
  --user "$KEYCLOAK_ADMIN_USERNAME" --password "$KEYCLOAK_ADMIN_PASSWORD"

scope_id=$($K get client-scopes -r $R --fields id,name --format csv --noquotes | grep ',groups$' | cut -d, -f1 || true)
if [ -z "$scope_id" ]; then
  $K create client-scopes -r $R -f - <<'JSON'
{
  "name": "groups",
  "protocol": "openid-connect",
  "attributes": {"include.in.token.scope": "true", "display.on.consent.screen": "false"},
  "protocolMappers": [{
    "name": "groups",
    "protocol": "openid-connect",
    "protocolMapper": "oidc-group-membership-mapper",
    "config": {
      "claim.name": "groups",
      "full.path": "false",
      "id.token.claim": "true",
      "access.token.claim": "true",
      "userinfo.token.claim": "true"
    }
  }]
}
JSON
  scope_id=$($K get client-scopes -r $R --fields id,name --format csv --noquotes | grep ',groups$' | cut -d, -f1 || true)
fi

for client in nebi nebi-cli; do
  cid=$($K get clients -r $R -q clientId=$client --fields id --format csv --noquotes)
  $K update clients/$cid/default-client-scopes/$scope_id -r $R
done
echo "keycloak bootstrap complete"
