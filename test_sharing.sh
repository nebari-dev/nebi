#!/bin/bash

# Environment Sharing Test Script

set -e
API_BASE="http://localhost:8080/api/v1"

echo "========================================="
echo "Environment Sharing Test"
echo "========================================="

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

success() { echo -e "${GREEN}✓${NC} $1"; }
error() { echo -e "${RED}✗${NC} $1"; exit 1; }

# 1. Login as admin and create alice
echo "➜ Creating alice..."
ADMIN_TOKEN=$(curl -s -X POST $API_BASE/auth/login -H "Content-Type: application/json" -d '{"username": "admin", "password": "password123"}' | jq -r '.token')

ALICE_RESPONSE=$(curl -s -X POST $API_BASE/admin/users -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" -d '{"username": "alice_share", "email": "alice_share@example.com", "password": "password123", "is_admin": false}')
ALICE_ID=$(echo $ALICE_RESPONSE | jq -r '.id')
[[ "$ALICE_ID" != "null" ]] && success "Created alice: $ALICE_ID" || error "Failed to create alice"

# 2. Create bob
echo "➜ Creating bob..."
BOB_RESPONSE=$(curl -s -X POST $API_BASE/admin/users -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" -d '{"username": "bob_share", "email": "bob_share@example.com", "password": "password123", "is_admin": false}')
BOB_ID=$(echo $BOB_RESPONSE | jq -r '.id')
[[ "$BOB_ID" != "null" ]] && success "Created bob: $BOB_ID" || error "Failed to create bob"

# 3. Alice logs in and creates environment
echo "➜ Alice creates environment..."
ALICE_TOKEN=$(curl -s -X POST $API_BASE/auth/login -H "Content-Type: application/json" -d '{"username": "alice_share", "password": "password123"}' | jq -r '.token')
ENV_RESPONSE=$(curl -s -X POST $API_BASE/environments -H "Authorization: Bearer $ALICE_TOKEN" -H "Content-Type: application/json" -d '{"name": "alice-shared-env", "package_manager": "pixi"}')
ENV_ID=$(echo $ENV_RESPONSE | jq -r '.id')
[[ "$ENV_ID" != "null" ]] && success "Alice created environment: $ENV_ID" || error "Failed to create environment"

# 4. Bob tries to access (should fail)
echo "➜ Bob tries to access before sharing..."
BOB_TOKEN=$(curl -s -X POST $API_BASE/auth/login -H "Content-Type: application/json" -d '{"username": "bob_share", "password": "password123"}' | jq -r '.token')
BOB_ACCESS=$(curl -s -X GET $API_BASE/environments/$ENV_ID -H "Authorization: Bearer $BOB_TOKEN")
if echo "$BOB_ACCESS" | jq -e '.error' > /dev/null; then
    success "Bob correctly denied access"
else
    error "Bob should not have access yet"
fi

# 5. Alice shares with Bob (viewer)
echo "➜ Alice shares environment with Bob (viewer)..."
SHARE_RESPONSE=$(curl -s -X POST $API_BASE/environments/$ENV_ID/share -H "Authorization: Bearer $ALICE_TOKEN" -H "Content-Type: application/json" -d "{\"user_id\": \"$BOB_ID\", \"role\": \"viewer\"}")
if echo "$SHARE_RESPONSE" | jq -e '.id' > /dev/null; then
    success "Alice shared environment with Bob"
else
    error "Failed to share: $SHARE_RESPONSE"
fi

# 6. Bob can now read
echo "➜ Bob can now read environment..."
BOB_READ=$(curl -s -X GET $API_BASE/environments/$ENV_ID -H "Authorization: Bearer $BOB_TOKEN")
if echo "$BOB_READ" | jq -e '.id' > /dev/null; then
    success "Bob can read the environment"
else
    error "Bob should be able to read: $BOB_READ"
fi

# 7. Bob tries to delete (should fail - viewer only)
echo "➜ Bob tries to delete (should fail - viewer only)..."
BOB_DELETE=$(curl -s -X DELETE $API_BASE/environments/$ENV_ID -H "Authorization: Bearer $BOB_TOKEN")
if echo "$BOB_DELETE" | jq -e '.error' > /dev/null; then
    success "Bob correctly denied write access (viewer role)"
else
    error "Bob should not be able to delete"
fi

# 8. List collaborators
echo "➜ List collaborators..."
COLLABORATORS=$(curl -s -X GET $API_BASE/environments/$ENV_ID/collaborators -H "Authorization: Bearer $ALICE_TOKEN")
COLLAB_COUNT=$(echo "$COLLABORATORS" | jq 'length')
[[ "$COLLAB_COUNT" == "2" ]] && success "Found 2 collaborators (alice + bob)" || error "Expected 2 collaborators, got $COLLAB_COUNT"

# 9. Alice unshares
echo "➜ Alice revokes Bob's access..."
UNSHARE_RESPONSE=$(curl -s -X DELETE $API_BASE/environments/$ENV_ID/share/$BOB_ID -H "Authorization: Bearer $ALICE_TOKEN")
success "Alice revoked Bob's access"

# 10. Bob can no longer access
echo "➜ Bob tries to access after unshare (should fail)..."
BOB_ACCESS_AFTER=$(curl -s -X GET $API_BASE/environments/$ENV_ID -H "Authorization: Bearer $BOB_TOKEN")
if echo "$BOB_ACCESS_AFTER" | jq -e '.error' > /dev/null; then
    success "Bob correctly denied access after unshare"
else
    error "Bob should not have access after unshare"
fi

echo ""
echo "========================================="
echo -e "${GREEN}All sharing tests passed!${NC}"
echo "========================================="
echo ""
echo "Summary:"
echo "  ✓ Environment owner can share"
echo "  ✓ Viewer role grants read-only access"
echo "  ✓ Viewer cannot perform write operations"
echo "  ✓ List collaborators works"
echo "  ✓ Owner can revoke access"
echo ""
