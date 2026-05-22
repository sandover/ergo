#!/bin/bash
# Creates a realistic sample project fixture for testing ergo.
# Run from repo root: ./testdata/fixtures/create-sample-project.sh

set -e

FIXTURE_DIR="testdata/sample-project"
rm -rf "$FIXTURE_DIR"
mkdir -p "$FIXTURE_DIR"
cd "$FIXTURE_DIR"

ERGO="${ERGO:-../../ergo}"

# Initialize
$ERGO init

new_task() {
	$ERGO new task "$1"
}

set_task() {
	$ERGO set "$1" "$2"
}

# ============================================
# PHASE 1: Research & Design
# ============================================
DESIGN_EPIC=$(new_task '{"title":"Research & Design"}')

# Research tasks - some done, one in progress
REQ_TASK=$(new_task '{"title":"Define product requirements","epic":"'"$DESIGN_EPIC"'"}')
mkdir -p docs
cat > docs/prd.md << 'EOF'
# Product Requirements Document

## Problem Statement
Teams need a lightweight task tracker that works well with AI coding agents.

## Goals
1. Minimal footprint - single binary, no database
2. Agent-friendly - JSON output, clear task states
3. Human-friendly - readable CLI output, intuitive commands

## Non-Goals
- Real-time collaboration (v2)
- GUI interface (v2)

## Success Metrics
- <100ms for any command
- Zero external dependencies at runtime
EOF
set_task "$REQ_TASK" '{"claim":"maya","state":"done"}'
set_task "$REQ_TASK" '{"result":"docs/prd.md"}'

COMP_TASK=$(new_task '{"title":"Competitor analysis","epic":"'"$DESIGN_EPIC"'"}')
mkdir -p docs
cat > docs/competitor-analysis.md << 'EOF'
# Competitor Analysis

## Key Competitors
1. TaskFlow - Good UI but slow
2. PlanIt - Fast but complex  
3. DoThings - Simple but no deps

## Our Differentiation
- Event-sourced (auditable, recoverable)
- Agent-friendly JSON mode
- Minimal footprint
EOF
set_task "$COMP_TASK" '{"claim":"sonnet@agent-host","state":"done"}'
set_task "$COMP_TASK" '{"result":"docs/competitor-analysis.md"}'

INTERVIEW_TASK=$(new_task '{"title":"User interviews (3 customers)","epic":"'"$DESIGN_EPIC"'"}')
set_task "$INTERVIEW_TASK" '{"claim":"human@agent-host","state":"doing"}'

DESIGN_TASK=$(new_task '{"title":"Write technical design doc","epic":"'"$DESIGN_EPIC"'"}')
$ERGO sequence "$REQ_TASK" "$DESIGN_TASK"  # Design doc needs requirements first

# ============================================
# PHASE 2: Implementation (blocked by Design)
# ============================================
IMPL_EPIC=$(new_task '{"title":"Implementation"}')
$ERGO sequence "$DESIGN_EPIC" "$IMPL_EPIC"

# Backend tasks
SCAFFOLD_TASK=$(new_task '{"title":"Set up project scaffolding","epic":"'"$IMPL_EPIC"'"}')

MODEL_TASK=$(new_task '{"title":"Implement core data model","epic":"'"$IMPL_EPIC"'"}')
$ERGO sequence "$SCAFFOLD_TASK" "$MODEL_TASK"

API_TASK=$(new_task '{"title":"Build REST API endpoints","epic":"'"$IMPL_EPIC"'"}')
$ERGO sequence "$MODEL_TASK" "$API_TASK"

UI_TASK=$(new_task '{"title":"Build web frontend","epic":"'"$IMPL_EPIC"'"}')
$ERGO sequence "$API_TASK" "$UI_TASK"

TEST_TASK=$(new_task '{"title":"Write integration tests","epic":"'"$IMPL_EPIC"'"}')
$ERGO sequence "$API_TASK" "$TEST_TASK"

SEC_TASK=$(new_task '{"title":"Security review","epic":"'"$IMPL_EPIC"'"}')
$ERGO sequence "$API_TASK" "$SEC_TASK"

# ============================================
# PHASE 3: Launch (blocked by Implementation)
# ============================================
LAUNCH_EPIC=$(new_task '{"title":"Launch"}')
$ERGO sequence "$IMPL_EPIC" "$LAUNCH_EPIC"

STAGING_TASK=$(new_task '{"title":"Deploy to staging","epic":"'"$LAUNCH_EPIC"'"}')
$ERGO sequence "$UI_TASK" "$STAGING_TASK"    # Need frontend complete
$ERGO sequence "$TEST_TASK" "$STAGING_TASK"  # Need tests passing

QA_TASK=$(new_task '{"title":"QA sign-off","epic":"'"$LAUNCH_EPIC"'"}')
$ERGO sequence "$STAGING_TASK" "$QA_TASK"

NOTES_TASK=$(new_task '{"title":"Write release notes","epic":"'"$LAUNCH_EPIC"'"}')
$ERGO sequence "$UI_TASK" "$NOTES_TASK"  # Need to know what's shipping

PROD_TASK=$(new_task '{"title":"Production deploy","epic":"'"$LAUNCH_EPIC"'"}')
$ERGO sequence "$QA_TASK" "$PROD_TASK"
$ERGO sequence "$NOTES_TASK" "$PROD_TASK"

SOCIAL_TASK=$(new_task '{"title":"Announce on social media","epic":"'"$LAUNCH_EPIC"'"}')
$ERGO sequence "$PROD_TASK" "$SOCIAL_TASK"

# ============================================
# Standalone tasks (no epic)
# ============================================
README_TASK=$(new_task '{"title":"Update README with new features"}')
$ERGO sequence "$PROD_TASK" "$README_TASK"  # Doc the release after it ships

TYPO_TASK=$(new_task '{"title":"Fix typo in CLI help"}')
set_task "$TYPO_TASK" '{"claim":"maya","state":"done"}'

# A canceled task
DB_TASK=$(new_task '{"title":"Evaluate alternative database (decided against)"}')
set_task "$DB_TASK" '{"claim":"sonnet@agent-host","state":"canceled"}'

echo ""
echo "✓ Sample project created in $FIXTURE_DIR"
echo ""
$ERGO list
