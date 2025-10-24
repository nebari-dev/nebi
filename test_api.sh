#!/bin/bash

# Test Darb API end-to-end

echo "=== Testing Darb API ==="
echo

# 1. Login
echo "1. Logging in..."
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password123"}' | jq -r '.token')

if [ -z "$TOKEN" ]; then
  echo "❌ Failed to get token"
  exit 1
fi

echo "✅ Logged in successfully"
echo "Token: ${TOKEN:0:50}..."
echo

# 2. Create environment
echo "2. Creating environment..."
ENV_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/environments \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "test-env-1", "package_manager": "pixi"}')

echo "$ENV_RESPONSE" | jq .
ENV_ID=$(echo "$ENV_RESPONSE" | jq -r '.id')

if [ -z "$ENV_ID" ] || [ "$ENV_ID" = "null" ]; then
  echo "❌ Failed to create environment"
  exit 1
fi

echo "✅ Environment created with ID: $ENV_ID"
echo

# 3. List environments
echo "3. Listing environments..."
curl -s http://localhost:8080/api/v1/environments \
  -H "Authorization: Bearer $TOKEN" | jq .
echo

# 4. Get environment details
echo "4. Getting environment details..."
curl -s http://localhost:8080/api/v1/environments/$ENV_ID \
  -H "Authorization: Bearer $TOKEN" | jq .
echo

# 5. Wait for environment to be ready
echo "5. Waiting for environment to be ready..."
for i in {1..30}; do
  STATUS=$(curl -s http://localhost:8080/api/v1/environments/$ENV_ID \
    -H "Authorization: Bearer $TOKEN" | jq -r '.status')
  echo "  Status: $STATUS"

  if [ "$STATUS" = "ready" ]; then
    echo "✅ Environment is ready!"
    break
  elif [ "$STATUS" = "failed" ]; then
    echo "❌ Environment creation failed"
    exit 1
  fi

  sleep 1
done
echo

# 6. Install packages
echo "6. Installing packages..."
JOB_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/environments/$ENV_ID/packages \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"packages": ["python=3.11"]}')

echo "$JOB_RESPONSE" | jq .
JOB_ID=$(echo "$JOB_RESPONSE" | jq -r '.id')
echo "✅ Job created with ID: $JOB_ID"
echo

# 7. Check job status
echo "7. Checking job status..."
for i in {1..30}; do
  JOB_STATUS=$(curl -s http://localhost:8080/api/v1/jobs/$JOB_ID \
    -H "Authorization: Bearer $TOKEN")

  STATUS=$(echo "$JOB_STATUS" | jq -r '.status')
  echo "  Job status: $STATUS"

  if [ "$STATUS" = "completed" ]; then
    echo "✅ Job completed!"
    echo
    echo "Job logs:"
    echo "$JOB_STATUS" | jq -r '.logs'
    break
  elif [ "$STATUS" = "failed" ]; then
    echo "❌ Job failed"
    echo "$JOB_STATUS" | jq .
    exit 1
  fi

  sleep 1
done
echo

# 8. List packages
echo "8. Listing packages..."
curl -s http://localhost:8080/api/v1/environments/$ENV_ID/packages \
  -H "Authorization: Bearer $TOKEN" | jq .
echo

# 9. List all jobs
echo "9. Listing all jobs..."
curl -s http://localhost:8080/api/v1/jobs \
  -H "Authorization: Bearer $TOKEN" | jq '.[] | {id, type, status}'
echo

echo "=== All tests passed! ==="
