#!/bin/bash
# Creates a screenshot-safe sample project in /tmp for README captures.
# Run from repo root: ./testdata/fixtures/create-sample-project-screenshot.sh

set -e

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
ERGO="$ROOT_DIR/ergo"
FIXTURE_DIR="/tmp/ergo-screenshot"

rm -rf "$FIXTURE_DIR"
mkdir -p "$FIXTURE_DIR"
cd "$FIXTURE_DIR"

$ERGO init

# ============================================
# PHASE 1: Research & Design
# ============================================
DESIGN_EPIC=$(printf '%s' '{"title":"Research & Design"}' | $ERGO new epic)

REQ_TASK=$(printf '%s' '{"title":"Define product requirements","epic":"'"$DESIGN_EPIC"'"}' | $ERGO new task)
mkdir -p docs
cat > docs/prd.md << 'EOF'
# Product Requirements Document

## Problem Statement
Teams need a lightweight task tracker that works well with AI coding agents.

## Goals
1. Minimal footprint - single binary, no database
2. Agent-friendly - JSON output, clear task states
3. Human-friendly - readable CLI output, intuitive commands
EOF
printf '%s' '{"claim":"maya","state":"done"}' | $ERGO set "$REQ_TASK"
printf '%s' '{"result_path":"docs/prd.md","result_summary":"PRD complete"}' | $ERGO set "$REQ_TASK"

COMP_TASK=$(printf '%s' '{"title":"Competitor analysis","epic":"'"$DESIGN_EPIC"'"}' | $ERGO new task)
printf '%s' '{"claim":"sonnet@agent-host","state":"done"}' | $ERGO set "$COMP_TASK"

INTERVIEW_TASK=$(printf '%s' '{"title":"User interviews (3 customers)","epic":"'"$DESIGN_EPIC"'"}' | $ERGO new task)
printf '%s' '{"claim":"human@agent-host","state":"done"}' | $ERGO set "$INTERVIEW_TASK"

DESIGN_TASK=$(printf '%s' '{"title":"Write technical design doc","epic":"'"$DESIGN_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$REQ_TASK" "$DESIGN_TASK"

# ============================================
# PHASE 2: Implementation (blocked by Design)
# ============================================
IMPL_EPIC=$(printf '%s' '{"title":"Implementation"}' | $ERGO new epic)
$ERGO sequence "$DESIGN_EPIC" "$IMPL_EPIC"

SCAFFOLD_TASK=$(printf '%s' '{"title":"Set up project scaffolding","epic":"'"$IMPL_EPIC"'"}' | $ERGO new task)

MODEL_TASK=$(printf '%s' '{"title":"Implement core data model","epic":"'"$IMPL_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$SCAFFOLD_TASK" "$MODEL_TASK"

API_TASK=$(printf '%s' '{"title":"Build REST API endpoints","epic":"'"$IMPL_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$MODEL_TASK" "$API_TASK"

UI_TASK=$(printf '%s' '{"title":"Build web frontend","epic":"'"$IMPL_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$API_TASK" "$UI_TASK"

TEST_TASK=$(printf '%s' '{"title":"Write integration tests","epic":"'"$IMPL_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$API_TASK" "$TEST_TASK"

SEC_TASK=$(printf '%s' '{"title":"Security review","epic":"'"$IMPL_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$API_TASK" "$SEC_TASK"
printf '%s' '{"body":"Goal: Perform a focused security review of the new REST API endpoints and data model, identifying risks and required fixes before launch.\n\nAcceptance criteria:\n- Review authn/authz for all endpoints; list any missing checks.\n- Verify input validation on public-facing endpoints; note any gaps.\n- Check data model invariants and ensure no sensitive fields are exposed in responses.\n- Produce a short report with findings and severity tags.\n\nValidation:\n- Automated: run `go test ./...` and confirm all tests pass.\n- Manual: spot-check at least 3 endpoints with malformed input and document behavior.\n\nConsultation: If you find a critical security issue, pause and consult before proposing a fix."}' | $ERGO set "$SEC_TASK"

# ============================================
# PHASE 3: Launch (blocked by Implementation)
# ============================================
LAUNCH_EPIC=$(printf '%s' '{"title":"Launch"}' | $ERGO new epic)
$ERGO sequence "$IMPL_EPIC" "$LAUNCH_EPIC"

STAGING_TASK=$(printf '%s' '{"title":"Deploy to staging","epic":"'"$LAUNCH_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$UI_TASK" "$STAGING_TASK"
$ERGO sequence "$TEST_TASK" "$STAGING_TASK"

QA_TASK=$(printf '%s' '{"title":"QA sign-off","epic":"'"$LAUNCH_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$STAGING_TASK" "$QA_TASK"

NOTES_TASK=$(printf '%s' '{"title":"Write release notes","epic":"'"$LAUNCH_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$UI_TASK" "$NOTES_TASK"

PROD_TASK=$(printf '%s' '{"title":"Production deploy","epic":"'"$LAUNCH_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$QA_TASK" "$PROD_TASK"
$ERGO sequence "$NOTES_TASK" "$PROD_TASK"

SOCIAL_TASK=$(printf '%s' '{"title":"Announce on social media","epic":"'"$LAUNCH_EPIC"'"}' | $ERGO new task)
$ERGO sequence "$PROD_TASK" "$SOCIAL_TASK"

# ============================================
# Standalone tasks (no epic)
# ============================================
README_TASK=$(printf '%s' '{"title":"Update README with new features"}' | $ERGO new task)
$ERGO sequence "$PROD_TASK" "$README_TASK"

TYPO_TASK=$(printf '%s' '{"title":"Fix typo in CLI help"}' | $ERGO new task)
printf '%s' '{"claim":"maya","state":"done"}' | $ERGO set "$TYPO_TASK"

DB_TASK=$(printf '%s' '{"title":"Evaluate alternative database (decided against)"}' | $ERGO new task)
printf '%s' '{"claim":"sonnet@agent-host","state":"canceled"}' | $ERGO set "$DB_TASK"

echo ""
echo "✓ Screenshot sample project created in $FIXTURE_DIR"
echo ""
$ERGO list
