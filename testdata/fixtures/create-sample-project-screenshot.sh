#!/bin/bash
# Creates a screenshot-safe sample project in /tmp for README captures.
# Run from repo root: ./testdata/fixtures/create-sample-project-screenshot.sh

set -e

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
ERGO="${ERGO:-$ROOT_DIR/ergo}"
FIXTURE_DIR="/tmp/ergo-screenshot"

rm -rf "$FIXTURE_DIR"
mkdir -p "$FIXTURE_DIR"
cd "$FIXTURE_DIR"

$ERGO init

new_task() {
	if [ "$#" -eq 2 ]; then
		$ERGO new task "$1" --epic "$2"
	else
		$ERGO new task "$1"
	fi
}

# ============================================
# PHASE 1: Research & Design
# ============================================
DESIGN_EPIC=$(new_task "Research & Design")

REQ_TASK=$(new_task "Define product requirements" "$DESIGN_EPIC")
mkdir -p docs
cat > docs/prd.md << 'EOF'
# Product Requirements Document

## Problem Statement
Teams need a lightweight task tracker that works well with AI coding agents.

## Goals
1. Minimal footprint - single binary, no database
2. Agent-friendly - clear output and task states
3. Human-friendly - readable CLI output, intuitive commands
EOF
$ERGO done "$REQ_TASK" --result docs/prd.md

COMP_TASK=$(new_task "Competitor analysis" "$DESIGN_EPIC")
$ERGO done "$COMP_TASK"

INTERVIEW_TASK=$(new_task "User interviews (3 customers)" "$DESIGN_EPIC")
$ERGO done "$INTERVIEW_TASK"

DESIGN_TASK=$(new_task "Write technical design doc" "$DESIGN_EPIC")
$ERGO sequence "$REQ_TASK" "$DESIGN_TASK"

# ============================================
# PHASE 2: Implementation (blocked by Design)
# ============================================
IMPL_EPIC=$(new_task "Implementation")
$ERGO sequence "$DESIGN_EPIC" "$IMPL_EPIC"

SCAFFOLD_TASK=$(new_task "Set up project scaffolding" "$IMPL_EPIC")

MODEL_TASK=$(new_task "Implement core data model" "$IMPL_EPIC")
$ERGO sequence "$SCAFFOLD_TASK" "$MODEL_TASK"

API_TASK=$(new_task "Build REST API endpoints" "$IMPL_EPIC")
$ERGO sequence "$MODEL_TASK" "$API_TASK"

UI_TASK=$(new_task "Build web frontend" "$IMPL_EPIC")
$ERGO sequence "$API_TASK" "$UI_TASK"

TEST_TASK=$(new_task "Write integration tests" "$IMPL_EPIC")
$ERGO sequence "$API_TASK" "$TEST_TASK"

SEC_TASK=$(new_task "Security review" "$IMPL_EPIC")
$ERGO sequence "$API_TASK" "$SEC_TASK"
cat <<'EOF' | $ERGO body "$SEC_TASK"
Goal: Perform a focused security review of the new REST API endpoints and data model, identifying risks and required fixes before launch.

Acceptance criteria:
- Review authn/authz for all endpoints; list any missing checks.
- Verify input validation on public-facing endpoints; note any gaps.
- Check data model invariants and ensure no sensitive fields are exposed in responses.
- Produce a short report with findings and severity tags.

Validation:
- Automated: run `go test ./...` and confirm all tests pass.
- Manual: spot-check at least 3 endpoints with malformed input and document behavior.

Consultation: If you find a critical security issue, pause and consult before proposing a fix.
EOF

# ============================================
# PHASE 3: Launch (blocked by Implementation)
# ============================================
LAUNCH_EPIC=$(new_task "Launch")
$ERGO sequence "$IMPL_EPIC" "$LAUNCH_EPIC"

STAGING_TASK=$(new_task "Deploy to staging" "$LAUNCH_EPIC")
$ERGO sequence "$UI_TASK" "$STAGING_TASK"
$ERGO sequence "$TEST_TASK" "$STAGING_TASK"

QA_TASK=$(new_task "QA sign-off" "$LAUNCH_EPIC")
$ERGO sequence "$STAGING_TASK" "$QA_TASK"

NOTES_TASK=$(new_task "Write release notes" "$LAUNCH_EPIC")
$ERGO sequence "$UI_TASK" "$NOTES_TASK"

PROD_TASK=$(new_task "Production deploy" "$LAUNCH_EPIC")
$ERGO sequence "$QA_TASK" "$PROD_TASK"
$ERGO sequence "$NOTES_TASK" "$PROD_TASK"

SOCIAL_TASK=$(new_task "Announce on social media" "$LAUNCH_EPIC")
$ERGO sequence "$PROD_TASK" "$SOCIAL_TASK"

# ============================================
# Standalone tasks (no epic)
# ============================================
README_TASK=$(new_task "Update README with new features")
$ERGO sequence "$PROD_TASK" "$README_TASK"

TYPO_TASK=$(new_task "Fix typo in CLI help")
$ERGO done "$TYPO_TASK"

DB_TASK=$(new_task "Evaluate alternative database (decided against)")
$ERGO cancel "$DB_TASK"

echo ""
echo "✓ Screenshot sample project created in $FIXTURE_DIR"
echo ""
$ERGO list
