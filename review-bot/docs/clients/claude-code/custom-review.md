---
description: AI-review a PR of the current repo via the review-bot server
argument-hint: [pr-number] [dry-run]
allowed-tools: Bash(git remote *), Bash(sleep *)
---
Determine the current repo's "owner/name" from `git remote get-url origin`.
Call the `start_review` tool on the `review-bot` MCP server with that repo and
pr_number=$ARGUMENTS (dry_run=true only if the arguments include "dry-run").
Then poll the `review_status` tool every 30 seconds (sleep between calls) until
state is done or failed. Summarize the verdict and findings; include the review URL.
