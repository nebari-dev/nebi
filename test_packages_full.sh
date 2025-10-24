#!/bin/bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password123"}' | jq -r '.token')

ENV_ID="5c2e88a4-b51d-4be7-8ab0-2736be5e714f"

# Check environment status
echo "Checking environment status..."
STATUS=$(curl -s "http://localhost:8080/api/v1/environments/$ENV_ID" \
  -H "Authorization: Bearer $TOKEN" | jq -r '.status')
echo "Status: $STATUS"

# List packages
echo "Listing packages..."
PACKAGES=$(curl -s "http://localhost:8080/api/v1/environments/$ENV_ID/packages" \
  -H "Authorization: Bearer $TOKEN")
echo "$PACKAGES" | jq .

# Check if it's an array
if echo "$PACKAGES" | jq -e 'type == "array"' > /dev/null; then
  echo "✅ Packages endpoint returns an array"
  COUNT=$(echo "$PACKAGES" | jq 'length')
  echo "Package count: $COUNT"
else
  echo "❌ Packages endpoint does not return an array"
fi
