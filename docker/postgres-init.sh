#!/bin/bash
# Creates one database + owner role each for keycloak and nebi.
# Runs once, on an empty data directory (docker-entrypoint-initdb.d).
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" \
  -v kc_pw="$KEYCLOAK_DB_PASSWORD" -v nebi_pw="$NEBI_DB_PASSWORD" <<'SQL'
CREATE ROLE keycloak LOGIN PASSWORD :'kc_pw';
CREATE DATABASE keycloak OWNER keycloak;
CREATE ROLE nebi LOGIN PASSWORD :'nebi_pw';
CREATE DATABASE nebi OWNER nebi;
SQL
