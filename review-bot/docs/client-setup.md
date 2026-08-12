# `/custom-review` client setup (~2 minutes)

Get an AI PR review from any machine — no Python, no Ollama, no checkout of
review-bot needed. The reviews run centrally on the Mac Studio; your editor
just talks to it over MCP.

## What you need

- **Server address**: `http://Balinders-Mac-Studio.local:8765/mcp`
  (LAN/mDNS name — see "If the .local name does not resolve" below).
- **Access token**: ask Balinder. It is the `REVIEW_BOT_TOKEN` value from the
  Mac Studio's `review-bot/.env`. Never commit this token to any repo.

## Claude Code

1. Register the server (user scope = all your projects):

   ```sh
   claude mcp add --transport http review-bot http://Balinders-Mac-Studio.local:8765/mcp \
     --header "Authorization: Bearer <token>" --scope user
   ```

2. Install the slash command — copy
   [`docs/clients/claude-code/custom-review.md`](clients/claude-code/custom-review.md)
   to `~/.claude/commands/custom-review.md`.

3. Check: `claude mcp list` shows `review-bot` ✔ connected, and a new session
   lists the tools `start_review`, `review_status`, `prime`, `list_repos`.

## Cursor

1. Register the server in `~/.cursor/mcp.json` (create the file if absent).
   Remote servers use `url` — there is **no** `type` key:

   ```json
   {
     "mcpServers": {
       "review-bot": {
         "url": "http://Balinders-Mac-Studio.local:8765/mcp",
         "headers": { "Authorization": "Bearer ${env:REVIEW_BOT_TOKEN}" }
       }
     }
   }
   ```

   Put the token in your shell profile so `${env:REVIEW_BOT_TOKEN}` resolves,
   then restart Cursor fully (env changes need a real restart):

   ```sh
   echo 'export REVIEW_BOT_TOKEN=<token>' >> ~/.zshrc
   ```

2. Install the command as a **Skill** (Cursor's Commands feature is
   deprecated) — copy
   [`docs/clients/cursor/custom-review/SKILL.md`](clients/cursor/custom-review/SKILL.md)
   to `~/.cursor/skills/custom-review/SKILL.md`.

3. Check: Cursor Settings → MCP shows `review-bot` with 4 tools.

## Using it

In a checkout of an allowlisted repo (the server's `list_repos` tool shows
which), type:

```
/custom-review 961 dry-run     # findings only, posts nothing to GitHub
/custom-review 961             # posts the review on the PR and links it
```

The agent queues the review and polls status roughly every 30 seconds; a
review takes a few minutes. In Cursor there is no `$ARGUMENTS` substitution —
whatever you type after `/custom-review` rides along as context, which the
skill text handles.

## Smoke test on a new machine

`/custom-review <small-open-pr-number> dry-run` — expect a verdict plus
findings and "nothing was posted". Then run it without `dry-run` to see the
posted review URL.

## Cursor caveats

- First tool call shows an **approval prompt** — approve `start_review` /
  `review_status` for the session.
- **No live progress display** (broken in Cursor 3.13.x); the polling loop is
  how you see progress. Each individual tool call returns in under a second,
  well inside Cursor's ~60s tool timeout.

## If the `.local` name does not resolve

Some networks block mDNS. Use the LAN IP instead — currently
`192.168.1.177` — in the URL (`http://192.168.1.177:8765/mcp`), or add a line
to `/etc/hosts`:

```
192.168.1.177  Balinders-Mac-Studio.local
```

(If the Mac Studio's IP changes, reserve it in the router's DHCP settings or
update this doc.)

## Troubleshooting

- **401 Unauthorized** — token missing/wrong. For Cursor check the env var is
  exported in the shell Cursor was launched from.
- **421 Misdirected Request** — the server's DNS-rebinding guard rejected the
  hostname you used. Add it to `REVIEW_BOT_ALLOWED_HOSTS` in the server's
  `.env` and restart the service.
- **"Job … not found (server restarted?)"** — jobs live in memory; submit the
  review again.
- **"Repo … is not on the allowlist"** — add the repo to `repos.json` on the
  Mac Studio, then restart the service (the allowlist is read at startup):
  `launchctl kickstart -k gui/$(id -u)/com.review-bot.mcp`.
