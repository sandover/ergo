#!/bin/bash
# This script owns the disposable backlog used for README and extension screenshots.
# It writes only to /tmp/ergo-screenshot and uses the installed CLI as the source of truth.
# Keep the project compact enough for a screenshot while covering the visible lifecycle states.

set -e

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
ERGO="${ERGO:-$ROOT_DIR/bin/ergo}"
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
# Lantern beta launch
# ============================================
LAUNCH_EPIC=$(new_task "Lantern public beta")

STORY_TASK=$(new_task "Write the 90-second launch story" "$LAUNCH_EPIC")
mkdir -p notes
cat > notes/launch-story.md << 'EOF'
# Lantern public beta

Lantern gives small teams a calm, shared place to see what needs attention next.

The beta story focuses on three promises: a clear first step, visible ownership,
and a work history that does not disappear when the meeting ends.
EOF
$ERGO done "$STORY_TASK"
$ERGO result "$STORY_TASK" "Drafted the beta launch story" --file notes/launch-story.md

TIMELINE_TASK=$(new_task "Polish the shared activity timeline" "$LAUNCH_EPIC")
$ERGO claim "$TIMELINE_TASK" --agent "pixel@lantern"
cat << 'EOF' | $ERGO body "$TIMELINE_TASK"
## Goal
- Make the activity timeline feel calm and useful during a busy launch week.

## Acceptance criteria
- Keep the latest action visible without hiding older context.
- Make ownership and next steps scannable at a glance.
- Preserve keyboard navigation in the compact layout.
EOF

A11Y_TASK=$(new_task "Run the keyboard and screen-reader pass" "$LAUNCH_EPIC")
$ERGO sequence "$TIMELINE_TASK" "$A11Y_TASK"

BETA_TASK=$(new_task "Cut the beta release candidate" "$LAUNCH_EPIC")
cat << 'EOF' | $ERGO body "$BETA_TASK"
## Goal
- Package the first invite-only beta for the five launch teams.

## Release checklist
- Confirm the onboarding copy.
- Verify the export path.
- Attach the short rollback note.
EOF

ONBOARDING_TASK=$(new_task "Test first-run onboarding with five teams" "$LAUNCH_EPIC")
$ERGO claim "$ONBOARDING_TASK" --agent "mara@lantern"
$ERGO fail "$ONBOARDING_TASK" -m "The invite flow still loses the workspace name after the second step."

# ============================================
# Sustainable hosting
# ============================================
OPS_EPIC=$(new_task "Sustainable hosting")

RETENTION_TASK=$(new_task "Choose the default retention window" "$OPS_EPIC")
$ERGO done "$RETENTION_TASK"

BACKUP_TASK=$(new_task "Exercise restore from a cold backup" "$OPS_EPIC")
$ERGO done "$BACKUP_TASK"

DOCS_TASK=$(new_task "Publish the internal cost playbook" "$OPS_EPIC")
$ERGO cancel "$DOCS_TASK" -m "Superseded by the shorter launch checklist."

BUDGET_TASK=$(new_task "Set a weekly beta budget alert" "$OPS_EPIC")
$ERGO claim "$BUDGET_TASK" --agent "jo@lantern"

# ============================================
# Standalone work around the launch
# ============================================
README_TASK=$(new_task "Capture the README tour")

SECURITY_TASK=$(new_task "Resolve the staging secret rotation warning")
$ERGO block "$SECURITY_TASK" -m "Waiting for the hosting provider's rotation window."

DEMO_TASK=$(new_task "Record the two-minute product tour")
$ERGO sequence "$LAUNCH_EPIC" "$DEMO_TASK"

printf '%s\n' "✓ Screenshot sample project created in $FIXTURE_DIR"
printf '%s\n' "  Open $FIXTURE_DIR/.ergo/backlog.jsonl in VS Code or run:"
printf '%s\n' "  $ERGO --dir $FIXTURE_DIR list"
$ERGO --dir "$FIXTURE_DIR" list
