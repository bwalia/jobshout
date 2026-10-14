# Writer agents per website — configure, don't code

One Article Writer pipeline serves every website. A new site or style is a
**clone** of the Article Writer with its own settings. No Go code, no deploy.

## Set one up (on int first)

1. **Website.** Add an `opsapi` integration to the org, named for the site —
   in **Settings → Integrations** (org admins), or over the API:

   ```json
   POST /api/v1/integrations
   {
     "name": "Workstation Website",
     "provider": "opsapi",
     "base_url": "https://int-opsapi.workstation.co.uk",
     "credentials": { "api_key": "opsk_…" },
     "config": { "namespace_slug": "workstation" }
   }
   ```

   Mint the key with `scripts/opsapi-api-key.sh` (scope `cms` create/update/read
   on that namespace). Each ring has its own integration of the same name,
   pointing at that ring's CMS with that ring's key.

2. **Clone.** Open the Article Writer → Workspace tab → *Writer settings* →
   *Clone for another website or project*, e.g. "Workstation Blog Writer".

3. **Settings** on the clone (*Writer settings*):

   | Setting | engine_config key | Example |
   |---|---|---|
   | Writing style | `writer_style` | Business + tech mix, benefits first, tips and tricks, 700–1,000 words, end with "Try this". |
   | Keywords | `writer_focus` | Agentic AI, Generative AI, FinTech, DevOps, SRE, Observability |
   | Industry | `writer_industry` | Banking, Education, HealthTech, FinTech, Tax Returns |
   | Written for | `writer_audience` | `developer`, `technote` or `business` |
   | Website | `writer_destination` | Workstation Website |

   The model is the agent's own model (Models tab), as before.

4. **Schedule.** Create a `blog` schedule with `agent_id` set to the clone and
   `{"trending": true, "trending_count": 1, "auto_publish": true}`. Keywords,
   industry and reader come from the agent; a schedule that names its own wins,
   so one agent can serve several schedules.

A writer with a website files there only. It is never sent to the jobshout.com
Insights queue, which only the original Article Writer's runs reach.

## Promote int → test → acc → prod

Export the tested agent on int (agent page → Export) and import the
`.jobshout-agent.json` on the next ring. Writer packages import **by name**:
onto the agent of that name, or as a new clone. The original Article Writer is
never overwritten by a clone. Settings travel; keys do not. The website
setting is a name, so it resolves to the importing ring's own integration.
Create that ring's integration (step 1) before the first run, and its schedule
(step 4).
