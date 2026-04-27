#!/usr/bin/env bash
# Seed the JobShout dev environment with dummy data for every major section.
#
# Usage:
#   API_URL=https://dev-api.jobshout.co.uk \
#   EMAIL=you@example.com PASSWORD=secret \
#   ./scripts/seed_dev_data.sh
#
# Creates ~2-3 records per section (agents, projects, tasks, goals, workflows,
# knowledge files, sessions, scheduled tasks) scoped to the logged-in user's org.
# Idempotency: records use timestamped names, so re-runs produce fresh rows
# rather than clobbering or skipping. Clean up via the UI or direct DB if needed.

set -euo pipefail

API_URL="${API_URL:-https://dev-api.jobshout.co.uk}"
EMAIL="${EMAIL:?EMAIL env var required}"
PASSWORD="${PASSWORD:?PASSWORD env var required}"
STAMP="$(date +%Y%m%d-%H%M%S)"

step() { printf "\n\033[1;36m== %s ==\033[0m\n" "$*"; }
ok()   { printf "  \033[32m✓\033[0m %s\n" "$*"; }
warn() { printf "  \033[33m!\033[0m %s\n" "$*" >&2; }

post() {
  # POST to path with JSON body, return response body on stdout, fail on non-2xx
  local path="$1" body="$2"
  local resp http_code
  resp="$(curl -sS -w "\n%{http_code}" -X POST "$API_URL$path" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    --data "$body")"
  http_code="$(printf '%s' "$resp" | tail -n1)"
  body="$(printf '%s' "$resp" | sed '$d')"
  if [[ "$http_code" =~ ^2 ]]; then
    printf '%s' "$body"
  else
    warn "POST $path -> $http_code: $body"
    return 1
  fi
}

step "Login as $EMAIL"
login_resp="$(curl -sS -X POST "$API_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  --data "$(jq -n --arg e "$EMAIL" --arg p "$PASSWORD" '{email:$e,password:$p}')")"
TOKEN="$(printf '%s' "$login_resp" | jq -r '.access_token')"
ORG_ID="$(printf '%s' "$login_resp" | jq -r '.user.org_id')"
USER_ID="$(printf '%s' "$login_resp" | jq -r '.user.id')"
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo "Login failed: $login_resp" >&2
  exit 1
fi
ok "org_id=$ORG_ID user_id=$USER_ID"

# ---------------------------------------------------------------------------
step "Agents"
agent_ids=()
for spec in \
  'Research Analyst|analyst|Summarises web content and internal docs for decision briefs.' \
  'Code Reviewer|reviewer|Reviews pull requests for style, security, and correctness.' \
  'Planning Orchestrator|planner|Breaks high-level goals into actionable tasks for other agents.'
do
  name="${spec%%|*}"; rest="${spec#*|}"
  role="${rest%%|*}";  desc="${rest#*|}"
  body="$(jq -n --arg n "$name ($STAMP)" --arg r "$role" --arg d "$desc" \
    '{name:$n, role:$r, description:$d, engine_type:"go_native"}')"
  resp="$(post /api/v1/agents "$body")" || continue
  id="$(printf '%s' "$resp" | jq -r '.id // .agent.id // empty')"
  agent_ids+=("$id")
  ok "agent $name -> $id"
done
primary_agent="${agent_ids[0]:-}"
[[ -z "$primary_agent" ]] && { echo "No agents created; aborting downstream steps." >&2; exit 1; }

# ---------------------------------------------------------------------------
step "Projects"
project_ids=()
for spec in \
  'Q2 Website Refresh|high|Redesign marketing site for the Q2 launch.' \
  'Internal Knowledge Base|medium|Curate onboarding docs and runbooks into a searchable KB.' \
  'Support Automation Pilot|critical|Triage tier-1 support tickets with an AI workflow.'
do
  name="${spec%%|*}"; rest="${spec#*|}"
  prio="${rest%%|*}";  desc="${rest#*|}"
  body="$(jq -n --arg n "$name ($STAMP)" --arg p "$prio" --arg d "$desc" \
    '{name:$n, priority:$p, description:$d}')"
  resp="$(post /api/v1/projects "$body")" || continue
  id="$(printf '%s' "$resp" | jq -r '.id // .project.id // empty')"
  project_ids+=("$id")
  ok "project $name -> $id"
done
primary_project="${project_ids[0]:-}"

# ---------------------------------------------------------------------------
if [[ -n "$primary_project" ]]; then
  step "Tasks (under project $primary_project)"
  for spec in \
    'Draft new homepage copy|high|5' \
    'Audit existing design tokens|medium|3' \
    'Wire analytics events|low|2' \
    'Ship landing-page A/B test|critical|8'
  do
    title="${spec%%|*}"; rest="${spec#*|}"
    prio="${rest%%|*}";  pts="${rest#*|}"
    body="$(jq -n --arg pid "$primary_project" --arg t "$title" --arg p "$prio" --argjson sp "$pts" \
      '{project_id:$pid, title:$t, priority:$p, story_points:$sp}')"
    resp="$(post /api/v1/tasks "$body")" || continue
    id="$(printf '%s' "$resp" | jq -r '.id // .task.id // empty')"
    ok "task $title -> $id"
  done
else
  warn "Skipping tasks (no project created)"
fi

# ---------------------------------------------------------------------------
step "Goals (on primary agent $primary_agent)"
for goal in \
  'Compile a two-paragraph briefing on the latest TypeScript 5.7 features.' \
  'List three competitors and summarise their pricing pages.' \
  'Draft a weekly status update based on last week'"'"'s completed tasks.'
do
  body="$(jq -n --arg g "$goal" '{goal_text:$g, max_iter:5}')"
  if resp="$(post "/api/v1/agents/$primary_agent/goals" "$body")"; then
    id="$(printf '%s' "$resp" | jq -r '.id // .goal.id // empty')"
    ok "goal queued -> $id"
  fi
done

# ---------------------------------------------------------------------------
step "Knowledge files (on primary agent $primary_agent)"
for spec in \
  'company-overview.md|# Company Overview\n\nJobshout helps teams coordinate AI agents across projects.' \
  'style-guide.md|# Style Guide\n\n- Prefer plain English.\n- Use bullet lists for more than three items.' \
  'escalation-playbook.md|# Escalation Playbook\n\n1. Triage\n2. Notify on-call\n3. Open incident channel'
do
  filename="${spec%%|*}"; content="${spec#*|}"
  body="$(jq -n --arg f "$filename" --arg c "$(printf '%b' "$content")" '{filename:$f, content:$c}')"
  if resp="$(post "/api/v1/agents/$primary_agent/knowledge" "$body")"; then
    id="$(printf '%s' "$resp" | jq -r '.id // empty')"
    ok "knowledge $filename -> $id"
  fi
done

# ---------------------------------------------------------------------------
step "Workflows"
if [[ "${#agent_ids[@]}" -ge 2 ]]; then
  # Two-step DAG: analyst -> reviewer
  body="$(jq -n \
    --arg name "Research-and-review pipeline ($STAMP)" \
    --arg desc "Analyst gathers context, reviewer critiques the draft." \
    --arg a1 "${agent_ids[0]}" \
    --arg a2 "${agent_ids[1]}" \
    '{name:$name, description:$desc, steps:[
       {name:"gather", agent_id:$a1, input_template:"Summarise {{input.topic}} in three bullet points.", position:0, depends_on:[], engine_type:"go_native"},
       {name:"review", agent_id:$a2, input_template:"Review the following draft: {{steps.gather.output}}", position:1, depends_on:["gather"], engine_type:"go_native"}
     ]}')"
  if resp="$(post /api/v1/workflows "$body")"; then
    id="$(printf '%s' "$resp" | jq -r '.id // .workflow.id // empty')"
    ok "workflow research-and-review -> $id"
  fi
else
  warn "Skipping workflow (need at least 2 agents)"
fi

# ---------------------------------------------------------------------------
step "Sessions"
for spec in \
  'Launch planning ($STAMP)|Collaborative planning for the Q2 launch.' \
  'Customer research digest ($STAMP)|Shared scratchpad for customer interview summaries.'
do
  name="${spec%%|*}"; desc="${spec#*|}"
  name_expanded="$(eval "printf '%s' \"$name\"")"
  body="$(jq -n --arg n "$name_expanded" --arg d "$desc" \
    '{name:$n, description:$d, tags:["dev-seed"]}')"
  if resp="$(post /api/v1/sessions "$body")"; then
    id="$(printf '%s' "$resp" | jq -r '.id // empty')"
    ok "session $name_expanded -> $id"
  fi
done

# ---------------------------------------------------------------------------
step "Scheduled tasks"
if [[ -n "$primary_agent" ]]; then
  body="$(jq -n --arg n "Daily briefing ($STAMP)" --arg a "$primary_agent" \
    '{name:$n, task_type:"agent", agent_id:$a, input_prompt:"Produce a one-paragraph summary of yesterday'"'"'s completed work.", schedule_type:"cron", cron_expression:"0 9 * * *", priority:"medium", tags:["dev-seed"]}')"
  if resp="$(post /api/v1/scheduled-tasks "$body")"; then
    id="$(printf '%s' "$resp" | jq -r '.id // empty')"
    ok "scheduled daily-briefing -> $id"
  fi

  body="$(jq -n --arg n "Hourly heartbeat ($STAMP)" --arg a "$primary_agent" \
    '{name:$n, task_type:"agent", agent_id:$a, input_prompt:"Log a heartbeat message.", schedule_type:"interval", interval_seconds:3600, priority:"low", tags:["dev-seed"]}')"
  if resp="$(post /api/v1/scheduled-tasks "$body")"; then
    id="$(printf '%s' "$resp" | jq -r '.id // empty')"
    ok "scheduled hourly-heartbeat -> $id"
  fi
fi

step "Done"
echo "Seeded with stamp: $STAMP"
echo "Login to https://dev.jobshout.co.uk to explore the new records."
