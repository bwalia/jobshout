---
name: custom-review
description: AI-review a pull request of the current repo via the review-bot MCP server
disable-model-invocation: true
---
Determine the current repo's "owner/name" from `git remote get-url origin`.
The PR number is whatever the user typed after the command; if they also typed
"dry-run", pass dry_run=true.

Call the `start_review` tool from the `review-bot` MCP server with that repo
and PR number. It returns a job_id immediately. Then poll the `review_status`
tool from the same server every 30 seconds until the state is done or failed —
a review takes a few minutes, so keep polling patiently.

When it finishes, summarize the verdict and the findings (blocking ones first)
and include the review URL if one is present.
