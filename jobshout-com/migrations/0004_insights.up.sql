-- Insights: the news, articles, blogs, podcasts and videos hub at /insights.
-- One table with a `kind` discriminator rather than one system per format:
-- every kind shares the feed, moderation, topics and search.

CREATE TABLE IF NOT EXISTS insights_topics (
    slug         TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    position     INT  NOT NULL DEFAULT 0
);

INSERT INTO insights_topics (slug, name, description, position) VALUES
    ('ai-trends',           'AI Trends',            'Models, products and the shifts they cause.',         1),
    ('job-market',          'Job Market',           'Hiring demand, layoffs, salaries and where roles move.', 2),
    ('skills-careers',      'Skills & Careers',     'What to learn next and how careers are changing.',   3),
    ('hiring',              'Hiring',               'How teams recruit, assess and onboard with AI.',     4),
    ('tools-research',      'Tools & Research',     'Papers, benchmarks and tools worth knowing.',        5),
    ('policy-regulation',   'Policy & Regulation',  'Law, governance and workplace policy on AI.',        6)
ON CONFLICT (slug) DO NOTHING;

CREATE TABLE IF NOT EXISTS insights_items (
    id                   UUID PRIMARY KEY,
    kind                 TEXT NOT NULL,
    slug                 TEXT NOT NULL,
    title                TEXT NOT NULL,
    summary              TEXT NOT NULL DEFAULT '',
    body_md              TEXT NOT NULL DEFAULT '',
    -- Sanitised server-side from body_md; the web renders this and nothing else.
    body_html            TEXT NOT NULL DEFAULT '',
    cover_image_url      TEXT NOT NULL DEFAULT '',
    cover_image_alt      TEXT NOT NULL DEFAULT '',
    link_url             TEXT NOT NULL DEFAULT '',
    media_url            TEXT NOT NULL DEFAULT '',
    embed_provider       TEXT NOT NULL DEFAULT '',
    embed_url            TEXT NOT NULL DEFAULT '',
    duration_seconds     INT,
    transcript           TEXT NOT NULL DEFAULT '',
    author_email         TEXT NOT NULL,
    author_display_name  TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL DEFAULT 'draft',
    source               TEXT NOT NULL DEFAULT 'community',
    featured             BOOLEAN NOT NULL DEFAULT FALSE,
    review_note          TEXT NOT NULL DEFAULT '',
    reading_minutes      INT NOT NULL DEFAULT 1,
    published_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search               TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(summary, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(body_md, '')), 'C')
    ) STORED
);

CREATE UNIQUE INDEX IF NOT EXISTS insights_items_slug_idx ON insights_items (slug);

-- The public feed: published items, newest first, optionally by kind.
CREATE INDEX IF NOT EXISTS insights_items_feed_idx
    ON insights_items (status, published_at DESC);
CREATE INDEX IF NOT EXISTS insights_items_kind_idx
    ON insights_items (kind, status, published_at DESC);
CREATE INDEX IF NOT EXISTS insights_items_author_idx
    ON insights_items (lower(author_email), created_at DESC);
CREATE INDEX IF NOT EXISTS insights_items_search_idx
    ON insights_items USING GIN (search);

CREATE TABLE IF NOT EXISTS insights_item_topics (
    item_id     UUID NOT NULL REFERENCES insights_items(id) ON DELETE CASCADE,
    topic_slug  TEXT NOT NULL REFERENCES insights_topics(slug) ON DELETE CASCADE,
    PRIMARY KEY (item_id, topic_slug)
);

CREATE INDEX IF NOT EXISTS insights_item_topics_topic_idx
    ON insights_item_topics (topic_slug, item_id);

-- Double opt-in: a row starts pending and only a confirmed one gets mail.
CREATE TABLE IF NOT EXISTS insights_newsletter_subscribers (
    id              UUID PRIMARY KEY,
    email           TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    token           TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at    TIMESTAMPTZ,
    unsubscribed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS insights_newsletter_email_idx
    ON insights_newsletter_subscribers (lower(email));
CREATE UNIQUE INDEX IF NOT EXISTS insights_newsletter_token_idx
    ON insights_newsletter_subscribers (token);
