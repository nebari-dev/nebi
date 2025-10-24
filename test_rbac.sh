#!/bin/bash

# RBAC Test Script for Darb
# This script tests the complete RBAC implementation

set -e  # Exit on error

API_BASE="http://localhost:8080/api/v1"

echo "========================================="
echo "Darb RBAC Test Suite"
echo "========================================="

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
success() {
    echo -e "${GREEN}✓${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

info() {
    echo -e "${YELLOW}➜${NC} $1"
}

# Test 1: Login as admin
info "Test 1: Login as admin"
ADMIN_RESPONSE=$(curl -s -X POST $API_BASE/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password123"}')

ADMIN_TOKEN=$(echo $ADMIN_RESPONSE | jq -r '.token')

if [ "$ADMIN_TOKEN" = "null" ] || [ -z "$ADMIN_TOKEN" ]; then
    error "Failed to login as admin"
    echo "Response: $ADMIN_RESPONSE"
    exit 1
fi
success "Logged in as admin"

# Test 2: List users (admin only)
info "Test 2: List users (admin should succeed)"
USERS_RESPONSE=$(curl -s -X GET $API_BASE/admin/users \
  -H "Authorization: Bearer $ADMIN_TOKEN")

if echo "$USERS_RESPONSE" | jq -e '.error' > /dev/null; then
    error "Admin failed to list users"
    echo "Response: $USERS_RESPONSE"
    exit 1
fi
success "Admin can list users"

# Test 3: Create a regular user
info "Test 3: Create regular user"
USER1_RESPONSE=$(curl -s -X POST $API_BASE/admin/users \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "alice",
    "email": "alice@example.com",
    "password": "password123",
    "is_admin": false
  }')

USER1_ID=$(echo $USER1_RESPONSE | jq -r '.id')
if [ "$USER1_ID" = "null" ] || [ -z "$USER1_ID" ]; then
    error "Failed to create user alice"
    echo "Response: $USER1_RESPONSE"
    exit 1
fi
success "Created user: alice ($USER1_ID)"

# Test 4: Create another regular user
info "Test 4: Create second user"
USER2_RESPONSE=$(curl -s -X POST $API_BASE/admin/users \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "bob",
    "email": "bob@example.com",
    "password": "password123",
    "is_admin": false
  }')

USER2_ID=$(echo $USER2_RESPONSE | jq -r '.id')
if [ "$USER2_ID" = "null" ] || [ -z "$USER2_ID" ]; then
    error "Failed to create user bob"
    echo "Response: $USER2_RESPONSE"
    exit 1
fi
success "Created user: bob ($USER2_ID)"

# Test 5: Login as alice
info "Test 5: Login as alice"
ALICE_TOKEN=$(curl -s -X POST $API_BASE/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "alice", "password": "password123"}' | jq -r '.token')

if [ "$ALICE_TOKEN" = "null" ] || [ -z "$ALICE_TOKEN" ]; then
    error "Failed to login as alice"
    exit 1
fi
success "Logged in as alice"

# Test 6: Alice creates environment
info "Test 6: Alice creates environment"
ENV_RESPONSE=$(curl -s -X POST $API_BASE/environments \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "alice-env", "package_manager": "pixi"}')

ENV_ID=$(echo $ENV_RESPONSE | jq -r '.id')
if [ "$ENV_ID" = "null" ] || [ -z "$ENV_ID" ]; then
    error "Alice failed to create environment"
    echo "Response: $ENV_RESPONSE"
    exit 1
fi
success "Alice created environment: $ENV_ID"

# Test 7: Login as bob
info "Test 7: Login as bob"
BOB_TOKEN=$(curl -s -X POST $API_BASE/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "bob", "password": "password123"}' | jq -r '.token')

if [ "$BOB_TOKEN" = "null" ] || [ -z "$BOB_TOKEN" ]; then
    error "Failed to login as bob"
    exit 1
fi
success "Logged in as bob"

# Test 8: Bob tries to access Alice's environment (should fail)
info "Test 8: Bob tries to access Alice's environment (should fail)"
BOB_ACCESS=$(curl -s -X GET $API_BASE/environments/$ENV_ID \
  -H "Authorization: Bearer $BOB_TOKEN")

if echo "$BOB_ACCESS" | jq -e '.error' > /dev/null; then
    success "Bob correctly denied access to Alice's environment"
else
    error "Bob should not have access to Alice's environment"
    echo "Response: $BOB_ACCESS"
    exit 1
fi

# Test 9: Alice tries to access admin endpoint (should fail)
info "Test 9: Alice tries to access admin endpoint (should fail)"
ALICE_ADMIN=$(curl -s -X GET $API_BASE/admin/users \
  -H "Authorization: Bearer $ALICE_TOKEN")

if echo "$ALICE_ADMIN" | jq -e '.error' > /dev/null; then
    success "Alice correctly denied admin access"
else
    error "Alice should not have admin access"
    echo "Response: $ALICE_ADMIN"
    exit 1
fi

# Test 10: List roles
info "Test 10: List roles"
ROLES_RESPONSE=$(curl -s -X GET $API_BASE/admin/roles \
  -H "Authorization: Bearer $ADMIN_TOKEN")

VIEWER_ROLE_ID=$(echo $ROLES_RESPONSE | jq -r '.[] | select(.name=="viewer") | .id')
if [ "$VIEWER_ROLE_ID" = "null" ] || [ -z "$VIEWER_ROLE_ID" ]; then
    error "Failed to find viewer role"
    echo "Response: $ROLES_RESPONSE"
    exit 1
fi
success "Found viewer role: $VIEWER_ROLE_ID"

# Test 11: Admin grants Bob viewer access to Alice's environment
info "Test 11: Grant Bob viewer access to Alice's environment"
PERM_RESPONSE=$(curl -s -X POST $API_BASE/admin/permissions \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$USER2_ID\",
    \"environment_id\": \"$ENV_ID\",
    \"role_id\": $VIEWER_ROLE_ID
  }")

PERM_ID=$(echo $PERM_RESPONSE | jq -r '.id')
if [ "$PERM_ID" = "null" ] || [ -z "$PERM_ID" ]; then
    error "Failed to grant permission"
    echo "Response: $PERM_RESPONSE"
    exit 1
fi
success "Granted Bob viewer access to Alice's environment"

# Test 12: Bob can now read Alice's environment
info "Test 12: Bob can now read Alice's environment"
BOB_READ=$(curl -s -X GET $API_BASE/environments/$ENV_ID \
  -H "Authorization: Bearer $BOB_TOKEN")

if echo "$BOB_READ" | jq -e '.error' > /dev/null; then
    error "Bob should now be able to read the environment"
    echo "Response: $BOB_READ"
    exit 1
fi
success "Bob can read Alice's environment"

# Test 13: Bob tries to delete Alice's environment (should fail - viewer can't write)
info "Test 13: Bob tries to delete environment (should fail - viewer only)"
BOB_DELETE=$(curl -s -X DELETE $API_BASE/environments/$ENV_ID \
  -H "Authorization: Bearer $BOB_TOKEN")

if echo "$BOB_DELETE" | jq -e '.error' > /dev/null; then
    success "Bob correctly denied write access (viewer role)"
else
    error "Bob should not be able to delete (viewer role)"
    echo "Response: $BOB_DELETE"
    exit 1
fi

# Test 14: List permissions
info "Test 14: List permissions"
PERMS_LIST=$(curl -s -X GET $API_BASE/admin/permissions \
  -H "Authorization: Bearer $ADMIN_TOKEN")

if echo "$PERMS_LIST" | jq -e '.error' > /dev/null; then
    error "Failed to list permissions"
    echo "Response: $PERMS_LIST"
    exit 1
fi
success "Listed permissions successfully"

# Test 15: View audit logs
info "Test 15: View audit logs"
AUDIT_LOGS=$(curl -s -X GET $API_BASE/admin/audit-logs \
  -H "Authorization: Bearer $ADMIN_TOKEN")

if echo "$AUDIT_LOGS" | jq -e '.error' > /dev/null; then
    error "Failed to fetch audit logs"
    echo "Response: $AUDIT_LOGS"
    exit 1
fi

LOG_COUNT=$(echo "$AUDIT_LOGS" | jq 'length')
success "Retrieved $LOG_COUNT audit log entries"

# Test 16: Toggle admin status
info "Test 16: Toggle Bob to admin"
TOGGLE_RESPONSE=$(curl -s -X POST $API_BASE/admin/users/$USER2_ID/toggle-admin \
  -H "Authorization: Bearer $ADMIN_TOKEN")

IS_ADMIN=$(echo $TOGGLE_RESPONSE | jq -r '.is_admin')
if [ "$IS_ADMIN" != "true" ]; then
    error "Failed to make Bob admin"
    echo "Response: $TOGGLE_RESPONSE"
    exit 1
fi
success "Bob is now an admin"

# Test 17: Bob can now access admin endpoints
info "Test 17: Bob (now admin) can access admin endpoints"
BOB_ADMIN_ACCESS=$(curl -s -X GET $API_BASE/admin/users \
  -H "Authorization: Bearer $BOB_TOKEN")

if echo "$BOB_ADMIN_ACCESS" | jq -e '.error' > /dev/null; then
    error "Bob should now have admin access"
    echo "Response: $BOB_ADMIN_ACCESS"
    exit 1
fi
success "Bob can access admin endpoints"

# Test 18: Toggle Bob back to regular user
info "Test 18: Toggle Bob back to regular user"
TOGGLE_BACK=$(curl -s -X POST $API_BASE/admin/users/$USER2_ID/toggle-admin \
  -H "Authorization: Bearer $ADMIN_TOKEN")

IS_ADMIN_BACK=$(echo $TOGGLE_BACK | jq -r '.is_admin')
if [ "$IS_ADMIN_BACK" != "false" ]; then
    error "Failed to revoke Bob's admin status"
    echo "Response: $TOGGLE_BACK"
    exit 1
fi
success "Bob's admin status revoked"

echo ""
echo "========================================="
echo -e "${GREEN}All RBAC tests passed!${NC}"
echo "========================================="
echo ""
echo "Summary:"
echo "  ✓ Admin authentication"
echo "  ✓ User management"
echo "  ✓ Environment ownership"
echo "  ✓ Permission enforcement"
echo "  ✓ Role-based access control"
echo "  ✓ Audit logging"
echo "  ✓ Admin toggle functionality"
echo ""
