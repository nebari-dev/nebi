#!/bin/bash
# Generates a throwaway CA and a wildcard cert for *.${BASE_DOMAIN:-nebi.localhost}
# into docker/oidc/certs/. Trust certs/ca.crt in your browser/OS to avoid warnings.
set -euo pipefail
cd "$(dirname "$0")/certs"
base="${BASE_DOMAIN:-nebi.localhost}"

openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj "/CN=nebi spike CA" \
  -keyout ca.key -out ca.crt 2>/dev/null
openssl req -newkey rsa:2048 -nodes -subj "/CN=*.${base}" \
  -keyout local.key -out local.csr 2>/dev/null
openssl x509 -req -in local.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 30 \
  -extfile <(printf "subjectAltName=DNS:*.%s,DNS:%s\nextendedKeyUsage=serverAuth" "$base" "$base") \
  -out local.crt 2>/dev/null
rm -f local.csr ca.srl
echo "wrote $(pwd)/{ca.crt,local.crt,local.key} for *.${base}"
