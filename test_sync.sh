#!/bin/bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password123"}' | jq -r '.token')

PIXI_TOML=$(cat example-pixi.toml)

# Create environment with pixi.toml
echo "Creating environment with pixi.toml..."
ENV_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/environments \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"test-with-packages\", \"package_manager\": \"pixi\", \"pixi_toml\": $(echo "$PIXI_TOML" | jq -Rs .)}")

echo "$ENV_RESPONSE" | jq .
ENV_ID=$(echo "$ENV_RESPONSE" | jq -r '.id')
echo "Environment ID: $ENV_ID"

# Wait for environment to be ready
echo "Waiting for environment..."
for i in {1..30}; do
  STATUS=$(curl -s "http://localhost:8080/api/v1/environments/$ENV_ID" \
    -H "Authorization: Bearer $TOKEN" | jq -r '.status')
  echo "  Status: $STATUS"
  
  if [ "$STATUS" = "ready" ]; then
    break
  fi
  sleep 2
done

# List packages
echo ""
echo "Listing packages..."
curl -s "http://localhost:8080/api/v1/environments/$ENV_ID/packages" \
  -H "Authorization: Bearer $TOKEN" | jq '. | length'
