#!/bin/bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password123"}' | jq -r '.token')

echo "Token: ${TOKEN:0:50}..."

# Create environment
echo "Creating environment..."
ENV_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/environments \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "test-env", "package_manager": "pixi"}')

echo "$ENV_RESPONSE" | jq .
ENV_ID=$(echo "$ENV_RESPONSE" | jq -r '.id')
echo "Environment ID: $ENV_ID"

# Wait for environment to be ready
sleep 3

# List packages
echo "Listing packages..."
curl -s "http://localhost:8080/api/v1/environments/$ENV_ID/packages" \
  -H "Authorization: Bearer $TOKEN" | jq .
