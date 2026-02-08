#!/usr/bin/env bash
# Copyright (C) 2025-2026 Kraklabs. All rights reserved.
# MIE MCP Integration Test - sends real JSON-RPC requests to the compiled binary
set -euo pipefail

BINARY="./bin/mie"
TMPDIR=$(mktemp -d)
CONFIG_DIR="$TMPDIR/.mie"
mkdir -p "$CONFIG_DIR"

# Create minimal config
cat > "$CONFIG_DIR/config.yaml" << 'YAML'
version: "1"
storage:
  engine: mem
embedding:
  enabled: false
YAML

PASS=0
FAIL=0
TOTAL=0

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

# Start MCP server in background with pipes
FIFO_IN="$TMPDIR/mcp_in"
FIFO_OUT="$TMPDIR/mcp_out"
mkfifo "$FIFO_IN" "$FIFO_OUT"

MIE_CONFIG_PATH="$CONFIG_DIR/config.yaml" MIE_STORAGE_ENGINE=mem \
  "$BINARY" --mcp < "$FIFO_IN" > "$FIFO_OUT" 2>/dev/null &
MCP_PID=$!

# Open file descriptors
exec 3>"$FIFO_IN"  # write to server
exec 4<"$FIFO_OUT" # read from server

cleanup() {
  exec 3>&- 2>/dev/null || true
  exec 4<&- 2>/dev/null || true
  kill $MCP_PID 2>/dev/null || true
  wait $MCP_PID 2>/dev/null || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

# Send a JSON-RPC request and read response
REQ_ID=0
send_request() {
  local method="$1"
  local params="$2"
  REQ_ID=$((REQ_ID + 1))

  local req
  if [ "$params" = "null" ]; then
    req="{\"jsonrpc\":\"2.0\",\"id\":$REQ_ID,\"method\":\"$method\"}"
  else
    req="{\"jsonrpc\":\"2.0\",\"id\":$REQ_ID,\"method\":\"$method\",\"params\":$params}"
  fi

  echo "$req" >&3
  read -r -t 10 RESPONSE <&4
  echo "$RESPONSE"
}

send_notification() {
  local method="$1"
  local params="${2:-null}"

  local req
  if [ "$params" = "null" ]; then
    req="{\"jsonrpc\":\"2.0\",\"method\":\"$method\"}"
  else
    req="{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params}"
  fi

  echo "$req" >&3
}

call_tool() {
  local name="$1"
  local args="$2"
  send_request "tools/call" "{\"name\":\"$name\",\"arguments\":$args}"
}

# Extract text from tool response
get_text() {
  echo "$1" | python3 -c "
import sys, json
r = json.load(sys.stdin)
if 'result' in r and 'content' in r['result']:
    print(r['result']['content'][0]['text'])
elif 'error' in r:
    print('ERROR: ' + r['error'].get('message', str(r['error'])))
else:
    print(json.dumps(r))
" 2>/dev/null
}

is_error() {
  echo "$1" | python3 -c "
import sys, json
r = json.load(sys.stdin)
if 'result' in r:
    print(str(r['result'].get('isError', False)).lower())
else:
    print('true')
" 2>/dev/null
}

assert_contains() {
  local text="$1"
  local needle="$2"
  local msg="$3"
  TOTAL=$((TOTAL + 1))
  if printf '%s' "$text" | grep -qF -- "$needle"; then
    PASS=$((PASS + 1))
    echo -e "  ${GREEN}PASS${NC} $msg"
  else
    FAIL=$((FAIL + 1))
    echo -e "  ${RED}FAIL${NC} $msg"
    echo -e "       Expected to contain: ${YELLOW}$needle${NC}"
    echo -e "       Got: $(printf '%s' "$text" | head -c 200)"
  fi
}

assert_not_contains() {
  local text="$1"
  local needle="$2"
  local msg="$3"
  TOTAL=$((TOTAL + 1))
  if printf '%s' "$text" | grep -qF -- "$needle"; then
    FAIL=$((FAIL + 1))
    echo -e "  ${RED}FAIL${NC} $msg"
    echo -e "       Should NOT contain: ${YELLOW}$needle${NC}"
  else
    PASS=$((PASS + 1))
    echo -e "  ${GREEN}PASS${NC} $msg"
  fi
}

assert_is_error() {
  local resp="$1"
  local msg="$2"
  TOTAL=$((TOTAL + 1))
  if [ "$(is_error "$resp")" = "true" ]; then
    PASS=$((PASS + 1))
    echo -e "  ${GREEN}PASS${NC} $msg"
  else
    FAIL=$((FAIL + 1))
    echo -e "  ${RED}FAIL${NC} $msg (expected isError=true)"
  fi
}

assert_not_error() {
  local resp="$1"
  local msg="$2"
  TOTAL=$((TOTAL + 1))
  if [ "$(is_error "$resp")" = "false" ]; then
    PASS=$((PASS + 1))
    echo -e "  ${GREEN}PASS${NC} $msg"
  else
    FAIL=$((FAIL + 1))
    echo -e "  ${RED}FAIL${NC} $msg (expected isError=false)"
  fi
}

extract_id() {
  local text="$1"
  local prefix="$2"
  echo "$text" | python3 -c "
import sys, re
text = sys.stdin.read()
m = re.search(r'\[($prefix[^\]]+)\]', text)
if m: print(m.group(1))
else: print('')
" 2>/dev/null
}

echo "============================================"
echo "  MIE MCP Integration Test"
echo "  Binary: $BINARY"
echo "  Storage: in-memory"
echo "============================================"
echo ""

# ============================================================
# 1. INITIALIZE SESSION
# ============================================================
echo "--- 1. Initialize MCP Session ---"

RESP=$(send_request "initialize" '{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}')
assert_contains "$RESP" '"protocolVersion"' "Initialize returns protocolVersion"
assert_contains "$RESP" '"mie"' "Server identifies as mie"

send_notification "notifications/initialized"
echo ""

# ============================================================
# 2. LIST TOOLS
# ============================================================
echo "--- 2. List Tools ---"

RESP=$(send_request "tools/list" "null")
TEXT=$(get_text "$RESP")
for tool in mie_store mie_bulk_store mie_query mie_get mie_update mie_delete mie_list mie_conflicts mie_analyze mie_export mie_status; do
  assert_contains "$RESP" "\"$tool\"" "Tool $tool is listed"
done
echo ""

# ============================================================
# 3. STATUS (empty DB)
# ============================================================
echo "--- 3. Status (empty DB) ---"

RESP=$(call_tool "mie_status" '{}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Facts: 0" "Zero facts"
assert_contains "$TEXT" "Entities: 0" "Zero entities"
assert_contains "$TEXT" "empty graph" "Empty graph message"
assert_contains "$TEXT" "not configured" "Embeddings not configured (not misleading)"
assert_not_contains "$TEXT" "Embeddings enabled (provider not configured)" "No misleading embeddings message"
echo ""

# ============================================================
# 4. STORE - All 5 types
# ============================================================
echo "--- 4. Store All Node Types ---"

# Fact
RESP=$(call_tool "mie_store" '{"type":"fact","content":"Go is a compiled language","category":"technical","confidence":0.95,"source_agent":"test-script"}')
assert_not_error "$RESP" "Store fact succeeds"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Stored fact" "Fact stored"
FACT_ID=$(extract_id "$TEXT" "fact:")
echo "       Fact ID: $FACT_ID"

# Entity
RESP=$(call_tool "mie_store" '{"type":"entity","name":"Golang","kind":"technology","description":"A compiled programming language by Google","source_agent":"test-script"}')
assert_not_error "$RESP" "Store entity succeeds"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Stored entity" "Entity stored"
ENT_ID=$(extract_id "$TEXT" "ent:")
echo "       Entity ID: $ENT_ID"

# Decision
RESP=$(call_tool "mie_store" '{"type":"decision","title":"Use CozoDB for storage","rationale":"Graph + vector in one engine","alternatives":"[\"PostgreSQL\",\"Neo4j\"]","context":"MIE architecture","source_agent":"test-script"}')
assert_not_error "$RESP" "Store decision succeeds"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Stored decision" "Decision stored"
DEC_ID=$(extract_id "$TEXT" "dec:")
echo "       Decision ID: $DEC_ID"

# Event
RESP=$(call_tool "mie_store" '{"type":"event","title":"MIE v0.1.0 Release","description":"First public release","event_date":"2026-01-15","source_agent":"test-script"}')
assert_not_error "$RESP" "Store event succeeds"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Stored event" "Event stored"
EVT_ID=$(extract_id "$TEXT" "evt:")
echo "       Event ID: $EVT_ID"

# Topic
RESP=$(call_tool "mie_store" '{"type":"topic","name":"backend-architecture","description":"Backend design and architecture decisions"}')
assert_not_error "$RESP" "Store topic succeeds"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Stored topic" "Topic stored"
TOP_ID=$(extract_id "$TEXT" "top:")
echo "       Topic ID: $TOP_ID"
echo ""

# ============================================================
# 5. STORE - Validation errors
# ============================================================
echo "--- 5. Store Validation Errors ---"

RESP=$(call_tool "mie_store" '{"type":"fact","category":"general"}')
assert_is_error "$RESP" "Fact without content errors"
assert_contains "$(get_text "$RESP")" "content is required" "Error mentions content"

RESP=$(call_tool "mie_store" '{"type":"entity","kind":"person"}')
assert_is_error "$RESP" "Entity without name errors"

RESP=$(call_tool "mie_store" '{"type":"entity","name":"Test","kind":"invalid_kind"}')
assert_is_error "$RESP" "Entity with invalid kind errors"
assert_contains "$(get_text "$RESP")" "invalid entity kind" "Error mentions invalid kind"

RESP=$(call_tool "mie_store" '{"type":"fact","content":"test","category":"nonexistent","source_agent":"test"}')
assert_is_error "$RESP" "Fact with invalid category errors"
assert_contains "$(get_text "$RESP")" "invalid category" "Error mentions invalid category"

RESP=$(call_tool "mie_store" '{"type":"fact","content":"test","category":"general","confidence":2.5,"source_agent":"test"}')
assert_is_error "$RESP" "Fact with confidence > 1.0 errors"
assert_contains "$(get_text "$RESP")" "confidence must be between" "Error mentions confidence range"

RESP=$(call_tool "mie_store" '{"type":"decision","title":"Test"}')
assert_is_error "$RESP" "Decision without rationale errors"

RESP=$(call_tool "mie_store" '{"type":"event","title":"Test"}')
assert_is_error "$RESP" "Event without date errors"

RESP=$(call_tool "mie_store" '{}')
assert_is_error "$RESP" "Store without type errors"
echo ""

# ============================================================
# 6. STORE - Relationships & dangling edge validation
# ============================================================
echo "--- 6. Relationships & Edge Validation ---"

# Valid relationship
RESP=$(call_tool "mie_store" "{\"type\":\"fact\",\"content\":\"Golang was created at Google\",\"category\":\"technical\",\"source_agent\":\"test\",\"relationships\":[{\"edge\":\"fact_entity\",\"target_id\":\"$ENT_ID\"}]}")
assert_not_error "$RESP" "Store with valid relationship succeeds"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "fact_entity" "Relationship created"
FACT2_ID=$(extract_id "$TEXT" "fact:")

# Dangling edge - target doesn't exist
RESP=$(call_tool "mie_store" '{"type":"fact","content":"Dangling test","category":"general","source_agent":"test","relationships":[{"edge":"fact_entity","target_id":"ent:nonexistent000000"}]}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Stored fact" "Fact itself stored"
assert_contains "$TEXT" "not found" "Warning about nonexistent target"
echo ""

# ============================================================
# 7. GET - Retrieve nodes by ID
# ============================================================
echo "--- 7. Get Nodes By ID ---"

RESP=$(call_tool "mie_get" "{\"node_id\":\"$FACT_ID\"}")
assert_not_error "$RESP" "Get fact by ID"
assert_contains "$(get_text "$RESP")" "Go is a compiled language" "Fact content returned"

RESP=$(call_tool "mie_get" "{\"node_id\":\"$ENT_ID\"}")
assert_not_error "$RESP" "Get entity by ID"
assert_contains "$(get_text "$RESP")" "Golang" "Entity name returned"

RESP=$(call_tool "mie_get" "{\"node_id\":\"$DEC_ID\"}")
assert_not_error "$RESP" "Get decision by ID"
assert_contains "$(get_text "$RESP")" "CozoDB" "Decision title returned"

RESP=$(call_tool "mie_get" "{\"node_id\":\"$EVT_ID\"}")
assert_not_error "$RESP" "Get event by ID"
assert_contains "$(get_text "$RESP")" "2026-01-15" "Event date returned"

RESP=$(call_tool "mie_get" "{\"node_id\":\"$TOP_ID\"}")
assert_not_error "$RESP" "Get topic by ID"
assert_contains "$(get_text "$RESP")" "backend-architecture" "Topic name returned"

# Nonexistent
RESP=$(call_tool "mie_get" '{"node_id":"fact:doesnotexist12345"}')
assert_is_error "$RESP" "Get nonexistent node errors"
echo ""

# ============================================================
# 8. QUERY - Exact mode
# ============================================================
echo "--- 8. Query - Exact Mode ---"

RESP=$(call_tool "mie_query" '{"query":"compiled language","mode":"exact","node_types":["fact"]}')
assert_not_error "$RESP" "Exact search finds fact"
assert_contains "$(get_text "$RESP")" "compiled language" "Content returned"

RESP=$(call_tool "mie_query" '{"query":"Golang","mode":"exact","node_types":["entity"]}')
assert_not_error "$RESP" "Exact search finds entity"
assert_contains "$(get_text "$RESP")" "Golang" "Entity found"

RESP=$(call_tool "mie_query" '{"query":"xyznonexistent123","mode":"exact"}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "No results found" "No results for nonexistent query"
echo ""

# ============================================================
# 9. QUERY - valid_only=false (BUG FIX #1)
# ============================================================
echo "--- 9. Query valid_only=false (Bugfix #1) ---"

# Store a fact, then invalidate it
RESP=$(call_tool "mie_store" '{"type":"fact","content":"Python is version 3.11","category":"technical","source_agent":"test"}')
OLD_FACT_ID=$(extract_id "$(get_text "$RESP")" "fact:")

RESP=$(call_tool "mie_store" '{"type":"fact","content":"Python is version 3.12","category":"technical","source_agent":"test"}')
NEW_FACT_ID=$(extract_id "$(get_text "$RESP")" "fact:")

call_tool "mie_update" "{\"node_id\":\"$OLD_FACT_ID\",\"action\":\"invalidate\",\"reason\":\"Version updated\",\"replacement_id\":\"$NEW_FACT_ID\"}" > /dev/null

# valid_only=false should find the invalidated fact
RESP=$(call_tool "mie_query" '{"query":"Python is version 3.11","mode":"exact","valid_only":false}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Python is version 3.11" "valid_only=false returns invalidated fact"

# valid_only=true should NOT find it
RESP=$(call_tool "mie_query" '{"query":"Python is version 3.11","mode":"exact","valid_only":true}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "No results found" "valid_only=true hides invalidated fact"
echo ""

# ============================================================
# 10. QUERY - Date filters (BUG FIX #2)
# ============================================================
echo "--- 10. Query Date Filters (Bugfix #2) ---"

# created_after=1 (epoch) should find recently created facts
RESP=$(call_tool "mie_query" '{"query":"compiled language","mode":"exact","created_after":1}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "compiled language" "created_after=1 finds recent fact"

# created_after=9999999999 should find nothing
RESP=$(call_tool "mie_query" '{"query":"compiled language","mode":"exact","created_after":9999999999}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "No results found" "created_after far future finds nothing"

# created_before=1 should find nothing
RESP=$(call_tool "mie_query" '{"query":"compiled language","mode":"exact","created_before":1}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "No results found" "created_before=1 finds nothing"
echo ""

# ============================================================
# 11. QUERY - Graph mode
# ============================================================
echo "--- 11. Query - Graph Traversals ---"

# related_entities from fact
RESP=$(call_tool "mie_query" "{\"query\":\"x\",\"mode\":\"graph\",\"node_id\":\"$FACT2_ID\",\"traversal\":\"related_entities\"}")
assert_not_error "$RESP" "related_entities traversal"
assert_contains "$(get_text "$RESP")" "Golang" "Finds related entity"

# facts_about_entity
RESP=$(call_tool "mie_query" "{\"query\":\"x\",\"mode\":\"graph\",\"node_id\":\"$ENT_ID\",\"traversal\":\"facts_about_entity\"}")
assert_not_error "$RESP" "facts_about_entity traversal"
assert_contains "$(get_text "$RESP")" "Google" "Finds fact about entity"

# invalidation_chain
RESP=$(call_tool "mie_query" "{\"query\":\"x\",\"mode\":\"graph\",\"node_id\":\"$NEW_FACT_ID\",\"traversal\":\"invalidation_chain\"}")
assert_not_error "$RESP" "invalidation_chain traversal"
assert_contains "$(get_text "$RESP")" "Version updated" "Shows invalidation reason"

# Validation: graph mode without node_id
RESP=$(call_tool "mie_query" '{"query":"x","mode":"graph","traversal":"related_entities"}')
assert_is_error "$RESP" "Graph mode without node_id errors"

# Validation: graph mode without traversal
RESP=$(call_tool "mie_query" "{\"query\":\"x\",\"mode\":\"graph\",\"node_id\":\"$FACT_ID\"}")
assert_is_error "$RESP" "Graph mode without traversal errors"
echo ""

# ============================================================
# 12. LIST - All types + filters
# ============================================================
echo "--- 12. List - All Types & Filters ---"

RESP=$(call_tool "mie_list" '{"node_type":"fact"}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Facts" "List facts header"
assert_contains "$TEXT" "Valid" "Facts table has Valid column (Bugfix)"

RESP=$(call_tool "mie_list" '{"node_type":"entity"}')
assert_contains "$(get_text "$RESP")" "Golang" "List entities shows Golang"

RESP=$(call_tool "mie_list" '{"node_type":"decision"}')
assert_contains "$(get_text "$RESP")" "CozoDB" "List decisions shows CozoDB"

RESP=$(call_tool "mie_list" '{"node_type":"event"}')
assert_contains "$(get_text "$RESP")" "MIE v0.1.0" "List events shows release"

RESP=$(call_tool "mie_list" '{"node_type":"topic"}')
assert_contains "$(get_text "$RESP")" "backend-architecture" "List topics"

# Category filter
RESP=$(call_tool "mie_list" '{"node_type":"fact","category":"technical"}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "compiled language" "Category filter works"

# Kind filter
RESP=$(call_tool "mie_list" '{"node_type":"entity","kind":"technology"}')
assert_contains "$(get_text "$RESP")" "Golang" "Kind filter works"

# Pagination
RESP=$(call_tool "mie_list" '{"node_type":"fact","limit":1,"offset":0}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "next page" "Pagination hint shown"

# Date filter
RESP=$(call_tool "mie_list" '{"node_type":"fact","created_after":1}')
assert_not_error "$RESP" "List with created_after"

RESP=$(call_tool "mie_list" '{"node_type":"fact","created_after":9999999999}')
assert_contains "$(get_text "$RESP")" "0 total" "created_after future returns 0"
echo ""

# ============================================================
# 13. LIST - Topic filter (BUG FIX #4)
# ============================================================
echo "--- 13. List Topic Filter (Bugfix #4) ---"

# Link fact to topic
call_tool "mie_store" "{\"type\":\"fact\",\"content\":\"Architecture uses microservices\",\"category\":\"technical\",\"source_agent\":\"test\",\"relationships\":[{\"edge\":\"fact_topic\",\"target_id\":\"$TOP_ID\"}]}" > /dev/null

# List filtered by topic
RESP=$(call_tool "mie_list" '{"node_type":"fact","topic":"backend-architecture"}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "microservices" "Topic-filtered fact found"
assert_contains "$TEXT" "1 total" "Only 1 fact linked to topic"
assert_not_contains "$TEXT" "compiled language" "Unlinked facts excluded"
echo ""

# ============================================================
# 14. BULK STORE
# ============================================================
echo "--- 14. Bulk Store ---"

# Normal bulk store with cross-refs
RESP=$(call_tool "mie_bulk_store" '{"items":[{"type":"entity","name":"Redis","kind":"technology","description":"In-memory cache","source_agent":"test"},{"type":"fact","content":"Redis is used for caching","category":"technical","source_agent":"test","relationships":[{"edge":"fact_entity","target_ref":0}]}]}')
assert_not_error "$RESP" "Bulk store with cross-refs succeeds"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Stored 2 items" "2 items stored"
assert_contains "$TEXT" "fact_entity" "Cross-ref relationship created"

# Pluralization fix
RESP=$(call_tool "mie_bulk_store" '{"items":[{"type":"entity","name":"Docker","kind":"technology","source_agent":"test"}]}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "1 entity" "Correct singular: 1 entity"
assert_not_contains "$TEXT" "entitys" "No incorrect pluralization"

# Out-of-bounds target_ref
RESP=$(call_tool "mie_bulk_store" '{"items":[{"type":"fact","content":"OOB ref test","category":"general","source_agent":"test","relationships":[{"edge":"fact_entity","target_ref":99}]}]}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "out of bounds" "Out-of-bounds target_ref reported"

# Empty items
RESP=$(call_tool "mie_bulk_store" '{"items":[]}')
assert_is_error "$RESP" "Empty items array errors"
echo ""

# ============================================================
# 15. UPDATE
# ============================================================
echo "--- 15. Update ---"

# Update entity description
RESP=$(call_tool "mie_update" "{\"node_id\":\"$ENT_ID\",\"action\":\"update_description\",\"new_value\":\"A statically typed, compiled programming language\"}")
assert_not_error "$RESP" "Update description succeeds"

# Verify update
RESP=$(call_tool "mie_get" "{\"node_id\":\"$ENT_ID\"}")
assert_contains "$(get_text "$RESP")" "statically typed" "Description updated"

# Update decision status
RESP=$(call_tool "mie_update" "{\"node_id\":\"$DEC_ID\",\"action\":\"update_status\",\"new_value\":\"superseded\"}")
assert_not_error "$RESP" "Update status succeeds"

# Invalid status
RESP=$(call_tool "mie_update" "{\"node_id\":\"$DEC_ID\",\"action\":\"update_status\",\"new_value\":\"banana\"}")
assert_is_error "$RESP" "Invalid status errors"
assert_contains "$(get_text "$RESP")" "Must be one of" "Lists valid statuses"

# Wrong action for type
RESP=$(call_tool "mie_update" "{\"node_id\":\"$FACT_ID\",\"action\":\"update_description\",\"new_value\":\"test\"}")
assert_is_error "$RESP" "update_description on fact errors"
echo ""

# ============================================================
# 16. CONFLICTS
# ============================================================
echo "--- 16. Conflicts (embeddings disabled) ---"

RESP=$(call_tool "mie_conflicts" '{}')
assert_is_error "$RESP" "Conflicts requires embeddings"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "embeddings" "Error mentions embeddings"

RESP=$(call_tool "mie_conflicts" '{"threshold":0.99}')
assert_is_error "$RESP" "Conflicts with threshold also requires embeddings"

RESP=$(call_tool "mie_conflicts" '{"category":"technical"}')
assert_is_error "$RESP" "Conflicts with category also requires embeddings"
echo ""

# ============================================================
# 17. ANALYZE
# ============================================================
echo "--- 17. Analyze ---"

RESP=$(call_tool "mie_analyze" '{"content":"We decided to migrate from PostgreSQL to CozoDB for the storage layer because it supports both graph queries and vector search natively."}')
assert_not_error "$RESP" "Analyze succeeds"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" "Existing Memory Context" "Has context section"
assert_contains "$TEXT" "Evaluation Guide" "Has evaluation guide"
assert_contains "$TEXT" "mie_store" "Has store reference"
echo ""

# ============================================================
# 18. EXPORT - JSON
# ============================================================
echo "--- 18. Export - JSON ---"

RESP=$(call_tool "mie_export" '{"format":"json"}')
assert_not_error "$RESP" "JSON export succeeds"
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" '"version"' "Has version"
assert_contains "$TEXT" '"facts"' "Has facts"
assert_contains "$TEXT" '"entities"' "Has entities"
assert_contains "$TEXT" '"decisions"' "Has decisions"
assert_contains "$TEXT" '"edges"' "Has edges"

# Filtered export
RESP=$(call_tool "mie_export" '{"format":"json","node_types":["topic"]}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" '"topics"' "Has topics when filtered"
assert_not_contains "$TEXT" '"facts"' "No facts when filtering to topics only"
echo ""

# ============================================================
# 19. EXPORT - Datalog (BUG FIX #12)
# ============================================================
echo "--- 19. Export - Datalog Escaping (Bugfix #12) ---"

# Store a fact with special chars
call_tool "mie_store" '{"type":"fact","content":"User'\''s preference is '\''dark mode'\''","category":"preference","source_agent":"test"}' > /dev/null

RESP=$(call_tool "mie_export" '{"format":"datalog"}')
TEXT=$(get_text "$RESP")
assert_contains "$TEXT" ":put mie_fact" "Datalog has :put statements"
assert_not_contains "$TEXT" 'id: "fact:' "No Go double-quoted strings (uses single quotes)"
echo ""

# ============================================================
# 20. EXPORT - Relationship filtering (BUG FIX #13)
# ============================================================
echo "--- 20. Export Relationship Filtering (Bugfix #13) ---"

RESP=$(call_tool "mie_export" '{"format":"json","node_types":["entity"]}')
TEXT=$(get_text "$RESP")

# Should not include fact_topic edges when only exporting entities
RESULT=$(echo "$TEXT" | python3 -c "
import sys, json
d = json.load(sys.stdin)
edges = d.get('edges', {})
has_ft = 'fact_topic' in edges
has_fi = 'invalidates' in edges
print('fact_topic' if has_ft else 'ok')
" 2>/dev/null)
TOTAL=$((TOTAL + 1))
if [ "$RESULT" = "ok" ]; then
  PASS=$((PASS + 1))
  echo -e "  ${GREEN}PASS${NC} No fact_topic edges when exporting only entities"
else
  FAIL=$((FAIL + 1))
  echo -e "  ${RED}FAIL${NC} Should not have fact_topic edges when exporting only entities"
fi
echo ""

# ============================================================
# 21. DELETE
# ============================================================
echo "--- 21. Delete ---"

# Store something to delete
RESP=$(call_tool "mie_store" '{"type":"fact","content":"Temporary fact to delete","category":"general","source_agent":"test"}')
DEL_FACT_ID=$(extract_id "$(get_text "$RESP")" "fact:")

# Delete it
RESP=$(call_tool "mie_delete" "{\"action\":\"delete_node\",\"node_id\":\"$DEL_FACT_ID\"}")
assert_not_error "$RESP" "Delete node succeeds"

# Verify it's gone
RESP=$(call_tool "mie_get" "{\"node_id\":\"$DEL_FACT_ID\"}")
assert_is_error "$RESP" "Deleted node is gone"
echo ""

# ============================================================
# 22. ERROR HANDLING
# ============================================================
echo "--- 22. Error Handling ---"

# Unknown method
RESP=$(send_request "unknown/method" "null")
assert_contains "$RESP" '"error"' "Unknown method returns error"
assert_contains "$RESP" "-32601" "Method not found code"

# Invalid tool name
RESP=$(call_tool "mie_nonexistent" '{}')
assert_is_error "$RESP" "Unknown tool returns error"

# Missing required params
RESP=$(call_tool "mie_query" '{}')
assert_is_error "$RESP" "Query without query param errors"

RESP=$(call_tool "mie_list" '{}')
assert_is_error "$RESP" "List without node_type errors"

RESP=$(call_tool "mie_delete" '{"action":"delete_node"}')
assert_is_error "$RESP" "Delete without node_id errors"
echo ""

# ============================================================
# FINAL STATUS
# ============================================================
echo "--- Final Status ---"

RESP=$(call_tool "mie_status" '{}')
TEXT=$(get_text "$RESP")
echo "  $TEXT" | head -10
echo ""

# ============================================================
# SUMMARY
# ============================================================
echo "============================================"
echo "  TEST RESULTS"
echo "============================================"
echo -e "  Total:  $TOTAL"
echo -e "  ${GREEN}Passed: $PASS${NC}"
if [ $FAIL -gt 0 ]; then
  echo -e "  ${RED}Failed: $FAIL${NC}"
else
  echo -e "  Failed: 0"
fi
echo "============================================"

if [ $FAIL -gt 0 ]; then
  exit 1
fi