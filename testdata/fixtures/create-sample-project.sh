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

# Research tasks - some done, one in progress
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

## Non-Goals
- Real-time collaboration (v2)
- GUI interface (v2)

## Success Metrics
- <100ms for any command
- Zero external dependencies at runtime
EOF
$ERGO done "$REQ_TASK" --result docs/prd.md

COMP_TASK=$(new_task "Competitor analysis" "$DESIGN_EPIC")
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
$ERGO done "$COMP_TASK" --result docs/competitor-analysis.md

INTERVIEW_TASK=$(new_task "User interviews (3 customers)" "$DESIGN_EPIC")
$ERGO claim "$INTERVIEW_TASK" --agent human@agent-host

DESIGN_TASK=$(new_task "Write technical design doc" "$DESIGN_EPIC")
$ERGO sequence "$REQ_TASK" "$DESIGN_TASK"  # Design doc needs requirements first

# ============================================
# PHASE 2: Implementation (blocked by Design)
# ============================================
IMPL_EPIC=$(new_task "Implementation")
$ERGO sequence "$DESIGN_EPIC" "$IMPL_EPIC"

# Backend tasks
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

# ============================================
# PHASE 3: Launch (blocked by Implementation)
# ============================================
LAUNCH_EPIC=$(new_task "Launch")
$ERGO sequence "$IMPL_EPIC" "$LAUNCH_EPIC"

STAGING_TASK=$(new_task "Deploy to staging" "$LAUNCH_EPIC")
$ERGO sequence "$UI_TASK" "$STAGING_TASK"    # Need frontend complete
$ERGO sequence "$TEST_TASK" "$STAGING_TASK"  # Need tests passing

QA_TASK=$(new_task "QA sign-off" "$LAUNCH_EPIC")
$ERGO sequence "$STAGING_TASK" "$QA_TASK"

NOTES_TASK=$(new_task "Write release notes" "$LAUNCH_EPIC")
$ERGO sequence "$UI_TASK" "$NOTES_TASK"  # Need to know what's shipping

PROD_TASK=$(new_task "Production deploy" "$LAUNCH_EPIC")
$ERGO sequence "$QA_TASK" "$PROD_TASK"
$ERGO sequence "$NOTES_TASK" "$PROD_TASK"

SOCIAL_TASK=$(new_task "Announce on social media" "$LAUNCH_EPIC")
$ERGO sequence "$PROD_TASK" "$SOCIAL_TASK"

# ============================================
# Standalone tasks (no epic)
# ============================================
README_TASK=$(new_task "Update README with new features")
$ERGO sequence "$PROD_TASK" "$README_TASK"  # Doc the release after it ships

TYPO_TASK=$(new_task "Fix typo in CLI help")
$ERGO done "$TYPO_TASK"

# A canceled task
DB_TASK=$(new_task "Evaluate alternative database (decided against)")
$ERGO cancel "$DB_TASK"

echo ""
echo "✓ Sample project created in $FIXTURE_DIR"
echo ""
$ERGO list
