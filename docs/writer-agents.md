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

### Covers (listing images)

Each article should get a unique cover from its angle and content when
`BLOG_COVER_IMAGES` is on (all rings). Publish maps it to OpsAPI
`featured_image_url`. If a live listing still shows the shared Workstation
hero, regenerate from content:

```http
POST /api/v1/blogs/articles/{articleID}/regenerate-cover
{"sync_cms": true}
```

Full procedure, backfill script, and skill: [blog-covers.md](./blog-covers.md),
`.claude/skills/blog-covers/`.

## Pick the website per article

One writer can file to several OpsAPI namespaces. **Write articles** has a
**Website** picker: the writer's own website (or this ring's CMS) by default,
then every saved `opsapi` integration as `Name — namespace`. **+ Add a
website…** saves a new one inline (org admins only), the same record as
Settings → Integrations, and selects it.

- **Over the API:** `POST /api/v1/blogs/generate` takes `"destination":
  "JobShout"` (an integration name). Task launch and chat take the same name
  as the Article Writer's `destination` field.
- **Precedence:** the launch's website, then the agent's `writer_destination`,
  then the ring's CMS (`OPSAPI_*` env). The choice is recorded on the run
  (`options.destination`), so Publish, auto-publish and Retry reach the same
  site even if the agent is repointed later.
- **Validation:** a named website that does not exist, is not active, or lacks
  its key, namespace or URL fails the launch with `400` before any writing
  starts. A website the agent names is still checked only at Publish, as before.
- **Keys:** sealed with AES-256-GCM (`SECRETS_ENCRYPTION_KEY`) in
  `integrations.credentials`. A server without that key **refuses** a new
  integration credential (`503`) rather than storing it in plaintext; rows
  saved earlier keep working, and a rename or status change still saves. The
  integrations API (org admins only) returns keys masked to the last four
  characters; `/blogs/destinations` returns only name, URL, namespace and
  `ready`. A run stores the name, never the key. A key this server cannot
  decrypt makes the website not ready instead of sending ciphertext.
- **Adding a website:** from the writer's settings (Website → **+ Add a
  website…**, saved as the writer's website) or from Write articles. The form
  asks for the **namespace first**; `GET /blogs/destinations/check` validates
  it (`400` for a bad slug or URL) and says whether the org already saved it
  on that OpsAPI. A saved namespace with a usable key is reused and no key is
  asked for; a saved one whose key is missing or unreadable asks for a new key
  (admins only); a new one asks for its key and saves it sealed. The check
  never returns a key. The server also refuses a second website for the same
  namespace and OpsAPI (`409`). The name defaults to the namespace and the URL
  to this ring's `OPSAPI_BASE_URL` (both under More options).

### Set up Workstation and JobShout (int)

1. **Mint one key per namespace**, as a namespace admin, scoped `cms`
   create/update/read only:

   ```
   OPSAPI_BASE_URL=https://int-opsapi.workstation.co.uk OPSAPI_NAMESPACE=workstation \
     OPSAPI_KEY_NAME=jobshout-blog-int ./scripts/opsapi-api-key.sh
   OPSAPI_BASE_URL=https://int-opsapi.workstation.co.uk OPSAPI_NAMESPACE=jobshout \
     OPSAPI_KEY_NAME=jobshout-blog-int ./scripts/opsapi-api-key.sh
   ```

   The key is printed once. Paste it straight into the next step; don't save
   it in a file or chat.

2. **Save each as a website**, from **Write articles → Website → + Add a
   website…** or Settings → Integrations → OpsAPI: `Workstation Website` /
   `workstation` and `JobShout` / `jobshout`, both at
   `https://int-opsapi.workstation.co.uk`. Set `SECRETS_ENCRYPTION_KEY` on the
   ring first (WSL Vault `jobshout/<ring>/core`, then restart `jobshout-api`);
   without it the form says so and the server refuses the key.

3. **Test.** Write one short article to each website, wait for it to finish,
   then **Send to Workstation Website** / **Send to JobShout**. The run should
   show *Drafted in workstation* / *Drafted in jobshout*, and the draft should
   appear in that namespace in the OpsAPI CMS. Publish the JobShout draft in
   the CMS and it appears on int jobshout.com `/insights`, which reads
   namespace `jobshout`.

Runs that pick no website behave exactly as before.

## Promote int → test → acc → prod

Export the tested agent on int (agent page → Export) and import the
`.jobshout-agent.json` on the next ring. Writer packages import **by name**:
onto the agent of that name, or as a new clone. The original Article Writer is
never overwritten by a clone. Settings travel; keys do not. The website
setting is a name, so it resolves to the importing ring's own integration.
Create that ring's integration (step 1) before the first run, and its schedule
(step 4).
