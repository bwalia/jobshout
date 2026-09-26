# SEO site structure skills

Use this checklist when a website git repo is provided (and CMS APIs are unavailable).

## Locate layout

1. Read `README.md`, `STRUCTURE.md`, or `docs/seo.md` for content / public roots.
2. Prefer these public roots for `sitemap.xml` and `robots.txt`: `public/`, `static/`, `dist/`, `docs/`, `_site/`, `out/`.
3. Prefer these content roots for pages: `content/`, `src/pages/`, `pages/`, `app/`, `_posts/`, `posts/`.

## CMS hints

| Platform | Meta / slug | Sitemap |
| --- | --- | --- |
| WordPress | Theme `header.php` / Yoast / RankMath; WP REST slug + excerpt | `/wp-sitemap.xml` or Yoast XML |
| OpsAPI contents | `seo_title`, `seo_description` on posts | Public host `/sitemap.xml` |
| Hugo | Front matter + `layouts/` | `static/sitemap.xml` or Hugo config |
| Jekyll | Front matter + `_layouts/` | Sitemap plugin |
| Next.js | `metadata` / `next-seo` | `public/sitemap.xml` |
| Static HTML | Regenerate common templates; patch `<title>` / meta | Add `sitemap.xml` under public root |

## Publish fallback

1. Try WordPress REST or OpsAPI when credentials exist.
2. If API publish fails (or platform is static), open a GitHub PR with:
   - `sitemap.xml` (under public root when known)
   - `robots.txt` with a `Sitemap:` line
   - `SEO_PATCH.md` describing meta, slug, and keyword proposals for other agents
