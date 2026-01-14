#!/bin/bash
# Creates a realistic sample project fixture for testing ergo.
# Run from repo root: ./testdata/fixtures/create-sample-project.sh

set -e

FIXTURE_DIR="testdata/sample-project"
rm -rf "$FIXTURE_DIR"
mkdir -p "$FIXTURE_DIR"
cd "$FIXTURE_DIR"

ERGO="../../ergo"

# Initialize
$ERGO init

# ============================================
# PHASE 1: Research & Design
# ============================================
DESIGN_EPIC=$($ERGO new epic "Research & Design")

# Research tasks - some done, one in progress
REQ_TASK=$($ERGO new task "Define product requirements" --epic "$DESIGN_EPIC")
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
$ERGO set "$REQ_TASK" claim=maya state=done
$ERGO set "$REQ_TASK" result.path=docs/prd.md result.summary="PRD complete"

COMP_TASK=$($ERGO new task "Competitor analysis" --epic "$DESIGN_EPIC")
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
$ERGO set "$COMP_TASK" claim=brandon state=done
$ERGO set "$COMP_TASK" result.path=docs/competitor-analysis.md result.summary="Competitor landscape documented"

INTERVIEW_TASK=$($ERGO new task "User interviews (3 customers)" --epic "$DESIGN_EPIC")
$ERGO set "$INTERVIEW_TASK" worker=human  # Human only - requires customer calls
$ERGO set "$INTERVIEW_TASK" claim=brandon state=doing

DESIGN_TASK=$($ERGO new task "Write technical design doc" --epic "$DESIGN_EPIC")
$ERGO dep "$DESIGN_TASK" "$REQ_TASK"  # Design doc needs requirements first

# ============================================
# PHASE 2: Implementation (blocked by Design)
# ============================================
IMPL_EPIC=$($ERGO new epic "Implementation")
$ERGO dep "$IMPL_EPIC" "$DESIGN_EPIC"

# Backend tasks
SCAFFOLD_TASK=$($ERGO new task "Set up project scaffolding" --epic "$IMPL_EPIC")

MODEL_TASK=$($ERGO new task "Implement core data model" --epic "$IMPL_EPIC")
$ERGO dep "$MODEL_TASK" "$SCAFFOLD_TASK"

API_TASK=$($ERGO new task "Build REST API endpoints" --epic "$IMPL_EPIC")
$ERGO dep "$API_TASK" "$MODEL_TASK"

UI_TASK=$($ERGO new task "Build web frontend" --epic "$IMPL_EPIC")
$ERGO dep "$UI_TASK" "$API_TASK"

TEST_TASK=$($ERGO new task "Write integration tests" --epic "$IMPL_EPIC")
$ERGO dep "$TEST_TASK" "$API_TASK"

SEC_TASK=$($ERGO new task "Security review" --epic "$IMPL_EPIC")
$ERGO set "$SEC_TASK" worker=human  # Human only - requires expertise
$ERGO dep "$SEC_TASK" "$API_TASK"

# ============================================
# PHASE 3: Launch (blocked by Implementation)
# ============================================
LAUNCH_EPIC=$($ERGO new epic "Launch")
$ERGO dep "$LAUNCH_EPIC" "$IMPL_EPIC"

STAGING_TASK=$($ERGO new task "Deploy to staging" --epic "$LAUNCH_EPIC")
$ERGO dep "$STAGING_TASK" "$UI_TASK"    # Need frontend complete
$ERGO dep "$STAGING_TASK" "$TEST_TASK"  # Need tests passing

QA_TASK=$($ERGO new task "QA sign-off" --epic "$LAUNCH_EPIC")
$ERGO set "$QA_TASK" worker=human  # Human only - manual testing
$ERGO dep "$QA_TASK" "$STAGING_TASK"

NOTES_TASK=$($ERGO new task "Write release notes" --epic "$LAUNCH_EPIC")
$ERGO dep "$NOTES_TASK" "$UI_TASK"  # Need to know what's shipping

PROD_TASK=$($ERGO new task "Production deploy" --epic "$LAUNCH_EPIC")
$ERGO dep "$PROD_TASK" "$QA_TASK"
$ERGO dep "$PROD_TASK" "$NOTES_TASK"

SOCIAL_TASK=$($ERGO new task "Announce on social media" --epic "$LAUNCH_EPIC")
$ERGO set "$SOCIAL_TASK" worker=human  # Human only - brand voice
$ERGO dep "$SOCIAL_TASK" "$PROD_TASK"

# ============================================
# Standalone tasks (no epic)
# ============================================
README_TASK=$($ERGO new task "Update README with new features")
$ERGO dep "$README_TASK" "$PROD_TASK"  # Doc the release after it ships

TYPO_TASK=$($ERGO new task "Fix typo in CLI help")
$ERGO set "$TYPO_TASK" claim=maya state=done

# A canceled task
DB_TASK=$($ERGO new task "Evaluate alternative database (decided against)")
$ERGO set "$DB_TASK" claim=brandon state=canceled

echo ""
echo "✓ Sample project created in $FIXTURE_DIR"
echo ""
$ERGO list
