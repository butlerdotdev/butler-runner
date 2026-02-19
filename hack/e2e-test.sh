#!/usr/bin/env bash
# Copyright 2026 The Butler Authors.
# SPDX-License-Identifier: Apache-2.0
#
# E2E test for butler-runner managed mode against a live butler-portal deployment.
#
# Usage:
#   PORTAL_KUBECONFIG=/path/to/kubeconfig bash hack/e2e-test.sh
#
# Environment variables:
#   PORTAL_KUBECONFIG   - Path to portal tenant kubeconfig (required)
#   BUTLER_PORTAL_URL   - Portal registry API URL (default: https://portal.butlerlabs.dev/api/registry)
#   BUTLER_RUNNER_BIN   - Path to butler-runner binary (default: ./butler-runner)
#   CNPG_NAMESPACE      - Namespace where CNPG cluster runs (default: butler-portal)
#   CNPG_CLUSTER        - CNPG cluster name (default: butler-portal-butler-portal-db)

set -euo pipefail

# --- Configuration ---

PORTAL_KUBECONFIG="${PORTAL_KUBECONFIG:?PORTAL_KUBECONFIG is required}"
BUTLER_PORTAL_URL="${BUTLER_PORTAL_URL:-https://portal.butlerlabs.dev/api/registry}"
BUTLER_RUNNER_BIN="${BUTLER_RUNNER_BIN:-./butler-runner}"
CNPG_NAMESPACE="${CNPG_NAMESPACE:-butler-portal}"
CNPG_CLUSTER="${CNPG_CLUSTER:-butler-portal-butler-portal-db}"
DB_NAME="backstage_plugin_registry"
DB_USER="butler_portal"

# --- Colors ---

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "${CYAN}[e2e]${NC} $*"; }
pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }

# --- Helpers ---

kc() {
  kubectl --kubeconfig="$PORTAL_KUBECONFIG" "$@"
}

get_cnpg_primary() {
  kc -n "$CNPG_NAMESPACE" get pods \
    -l "cnpg.io/cluster=$CNPG_CLUSTER,role=primary" \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null
}

get_db_password() {
  kc -n "$CNPG_NAMESPACE" get secret "${CNPG_CLUSTER}-app" \
    -o jsonpath='{.data.password}' | base64 -d
}

run_sql() {
  local sql="$1"
  kc -n "$CNPG_NAMESPACE" exec "$PRIMARY_POD" -- \
    bash -c "PGPASSWORD='$DB_PASSWORD' psql -U $DB_USER -d $DB_NAME -h localhost -t -A -c \"$sql\"" 2>/dev/null
}

run_sql_multi() {
  local sql="$1"
  kc -n "$CNPG_NAMESPACE" exec "$PRIMARY_POD" -i -- \
    bash -c "PGPASSWORD='$DB_PASSWORD' psql -U $DB_USER -d $DB_NAME -h localhost -t -A" <<< "$sql" 2>/dev/null
}

# --- Test data ---

gen_uuid() {
  if [ -f /proc/sys/kernel/random/uuid ]; then
    cat /proc/sys/kernel/random/uuid
  elif command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr '[:upper:]' '[:lower:]'
  else
    # Fallback: generate v4 UUID from random bytes
    local hex
    hex=$(od -An -tx1 -N16 /dev/urandom | tr -d ' \n')
    printf '%s-%s-4%s-%s-%s\n' \
      "${hex:0:8}" "${hex:8:4}" "${hex:13:3}" "${hex:16:4}" "${hex:20:12}"
  fi
}

ARTIFACT_ID=$(gen_uuid)
ENV_ID=$(gen_uuid)
MODULE_ID=$(gen_uuid)
RUN_ID=$(gen_uuid)
CALLBACK_TOKEN="brce_$(openssl rand -hex 32)"
TOKEN_HASH=$(echo -n "$CALLBACK_TOKEN" | sha256sum | awk '{print $1}')

# --- Cleanup trap ---

SEEDED=false

cleanup() {
  if [ "$SEEDED" = "true" ]; then
    log "Cleaning up test data..."
    run_sql_multi "
      DELETE FROM module_run_logs WHERE run_id = '$RUN_ID';
      DELETE FROM module_run_outputs WHERE run_id = '$RUN_ID';
      DELETE FROM module_runs WHERE id = '$RUN_ID';
      DELETE FROM environment_modules WHERE id = '$MODULE_ID';
      DELETE FROM environments WHERE id = '$ENV_ID';
      DELETE FROM artifacts WHERE id = '$ARTIFACT_ID';
    " || warn "Cleanup failed (may need manual cleanup)"
    log "Cleanup complete"
  fi
}

trap cleanup EXIT

# --- Preflight checks ---

log "=== Butler Runner E2E Test ==="
log ""
log "Portal URL:  $BUTLER_PORTAL_URL"
log "Runner bin:  $BUTLER_RUNNER_BIN"
log "Run ID:      $RUN_ID"
log ""

# Check butler-runner binary exists
if [ ! -x "$BUTLER_RUNNER_BIN" ]; then
  fail "butler-runner binary not found at $BUTLER_RUNNER_BIN"
  exit 1
fi

# Check portal kubeconfig works
log "Checking portal cluster access..."
if ! kc -n "$CNPG_NAMESPACE" get pods >/dev/null 2>&1; then
  fail "Cannot access portal cluster via kubeconfig"
  exit 1
fi
pass "Portal cluster accessible"

# Find CNPG primary pod
log "Finding CNPG primary pod..."
PRIMARY_POD=$(get_cnpg_primary)
if [ -z "$PRIMARY_POD" ]; then
  fail "Cannot find CNPG primary pod (cluster=$CNPG_CLUSTER)"
  exit 1
fi
pass "CNPG primary: $PRIMARY_POD"

# Get DB password
log "Retrieving database credentials..."
DB_PASSWORD=$(get_db_password)
if [ -z "$DB_PASSWORD" ]; then
  fail "Cannot retrieve DB password from secret"
  exit 1
fi
pass "Database credentials retrieved"

# Verify DB connectivity
log "Verifying database connectivity..."
TABLE_COUNT=$(run_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'")
if [ -z "$TABLE_COUNT" ] || [ "$TABLE_COUNT" -lt 10 ]; then
  fail "Database check failed (found $TABLE_COUNT tables, expected 30+)"
  exit 1
fi
pass "Database has $TABLE_COUNT tables"

# Check network connectivity to portal
log "Checking portal connectivity..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 "$BUTLER_PORTAL_URL/v1/health" 2>/dev/null || echo "000")
if [ "$HTTP_CODE" = "000" ]; then
  warn "Cannot reach $BUTLER_PORTAL_URL (HTTP code: $HTTP_CODE)"
  warn "butler-runner may still work if running in a pod with network access"
else
  pass "Portal reachable (HTTP $HTTP_CODE)"
fi

# --- Seed test data ---

log ""
log "=== Seeding test data ==="

log "Inserting artifact, environment, module, and run..."
run_sql_multi "
INSERT INTO artifacts (id, namespace, name, type, status, storage_config, source_config, created_by, created_at, updated_at)
VALUES (
  '$ARTIFACT_ID', 'e2e-test', 'null-module', 'terraform-module', 'active',
  '{}',
  '{\"repositoryUrl\": \"https://github.com/butlerdotdev/butler-runner.git\", \"branch\": \"main\", \"path\": \"testdata/e2e\"}',
  'e2e-test', NOW(), NOW()
);

INSERT INTO environments (id, name, team, status, created_by, created_at, updated_at)
VALUES ('$ENV_ID', 'e2e-test-env', 'e2e-test', 'active', 'e2e-test', NOW(), NOW());

INSERT INTO environment_modules (id, environment_id, artifact_id, name, artifact_namespace, artifact_name, execution_mode, working_directory, status, created_at, updated_at)
VALUES (
  '$MODULE_ID', '$ENV_ID', '$ARTIFACT_ID', 'null-module',
  'e2e-test', 'null-module', 'byoc', 'testdata/e2e', 'active', NOW(), NOW()
);

INSERT INTO module_runs (id, module_id, environment_id, module_name, artifact_namespace, artifact_name, operation, mode, status, callback_token_hash, triggered_by, trigger_source, tf_version, created_at, updated_at)
VALUES (
  '$RUN_ID', '$MODULE_ID', '$ENV_ID', 'null-module',
  'e2e-test', 'null-module', 'plan', 'byoc', 'pending',
  '$TOKEN_HASH', 'e2e-test', 'api', '1.9.8', NOW(), NOW()
);
"
SEEDED=true

# Verify seed
RUN_STATUS=$(run_sql "SELECT status FROM module_runs WHERE id = '$RUN_ID'")
if [ "$RUN_STATUS" != "pending" ]; then
  fail "Seed verification failed: run status is '$RUN_STATUS', expected 'pending'"
  exit 1
fi
pass "Test data seeded (run status: pending)"

# --- Execute butler-runner ---

log ""
log "=== Running butler-runner ==="
log "BUTLER_URL=$BUTLER_PORTAL_URL"
log "BUTLER_RUN_ID=$RUN_ID"
log "BUTLER_TOKEN=brce_****"
log ""

RUNNER_EXIT=0
BUTLER_URL="$BUTLER_PORTAL_URL" \
  BUTLER_RUN_ID="$RUN_ID" \
  BUTLER_TOKEN="$CALLBACK_TOKEN" \
  "$BUTLER_RUNNER_BIN" exec 2>&1 || RUNNER_EXIT=$?

log ""
log "butler-runner exit code: $RUNNER_EXIT"

# --- Verify results ---

log ""
log "=== Verifying results ==="

# Check run status
FINAL_STATUS=$(run_sql "SELECT status FROM module_runs WHERE id = '$RUN_ID'")
log "Run status: $FINAL_STATUS"

if [ "$FINAL_STATUS" = "succeeded" ]; then
  pass "Run completed successfully"
elif [ "$FINAL_STATUS" = "failed" ]; then
  fail "Run failed"
  # Show error output if available
  ERROR_OUTPUT=$(run_sql "SELECT content FROM module_run_outputs WHERE run_id = '$RUN_ID' AND output_type = 'error_output' LIMIT 1")
  if [ -n "$ERROR_OUTPUT" ]; then
    log "Error output: $ERROR_OUTPUT"
  fi
else
  fail "Unexpected run status: $FINAL_STATUS (expected 'succeeded')"
fi

# Check if outputs were recorded
OUTPUT_COUNT=$(run_sql "SELECT COUNT(*) FROM module_run_outputs WHERE run_id = '$RUN_ID'")
log "Run outputs recorded: $OUTPUT_COUNT"
if [ "$OUTPUT_COUNT" -gt 0 ]; then
  pass "Run outputs present ($OUTPUT_COUNT records)"
else
  warn "No run outputs recorded"
fi

# Check exit code in DB
DB_EXIT_CODE=$(run_sql "SELECT COALESCE(exit_code::text, 'null') FROM module_runs WHERE id = '$RUN_ID'")
log "DB exit code: $DB_EXIT_CODE"

# Check resource counts
RESOURCES=$(run_sql "SELECT COALESCE(resources_to_add, 0) || '/' || COALESCE(resources_to_change, 0) || '/' || COALESCE(resources_to_destroy, 0) FROM module_runs WHERE id = '$RUN_ID'")
log "Resources (add/change/destroy): $RESOURCES"

# --- Final result ---

log ""
if [ "$FINAL_STATUS" = "succeeded" ] && [ "$RUNNER_EXIT" -eq 0 ]; then
  log "=== ${GREEN}E2E TEST PASSED${NC} ==="
  exit 0
else
  log "=== ${RED}E2E TEST FAILED${NC} ==="
  log "Runner exit: $RUNNER_EXIT, DB status: $FINAL_STATUS"
  exit 1
fi
