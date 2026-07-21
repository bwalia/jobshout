-- Phase 4 (cont.): seed a starter set of built-in skills.
--
-- Built-in skills have org_id = NULL, so they are visible to every org in the
-- enable picker. The tool-kind skills reference tools registered in the Go tool
-- registry at startup (http_request, shell_command); the prompt-kind skills
-- contribute a system-prompt fragment.
--
-- NOTE: the UNIQUE (org_id, slug) index does NOT dedupe these rows, because
-- Postgres treats NULL org_id values as distinct — so ON CONFLICT would never
-- fire. We instead insert only slugs that are not already present among the
-- built-ins, which keeps the seed idempotent if it is ever re-applied.

INSERT INTO skills (org_id, slug, name, description, kind, config_json, version, status)
SELECT NULL, v.slug, v.name, v.description, v.kind, v.config_json::jsonb, v.version, 'published'
FROM (VALUES
    ('web-fetch', 'Web Fetch',
     'Fetch and read content from public URLs to answer questions with live data.',
     'tool', '{"tool":"http_request"}', '1.0.0'),

    ('shell-runner', 'Shell Runner',
     'Run allow-listed shell commands to inspect the local environment.',
     'tool', '{"tool":"shell_command"}', '1.0.0'),

    ('concise-writer', 'Concise Writer',
     'Answer as briefly as correctness allows — lead with the conclusion, cut filler, prefer lists over prose.',
     'prompt', '{"prompt":"Be concise. Lead with the direct answer, then supporting detail only if it changes the decision. Prefer short sentences and lists. Never pad."}', '1.0.0'),

    ('cite-sources', 'Cite Sources',
     'Attach the source (URL, tool output, or file) behind every factual claim.',
     'prompt', '{"prompt":"For every factual claim, cite where it came from — the URL you fetched, the tool output, or the document. If you cannot cite it, say so explicitly rather than asserting it."}', '1.0.0')
) AS v(slug, name, description, kind, config_json, version)
WHERE NOT EXISTS (
    SELECT 1 FROM skills s WHERE s.org_id IS NULL AND s.slug = v.slug
);
