INSERT INTO organisations (id, name, slug, created_at)
VALUES ('11111111-1111-1111-1111-111111111111', 'Acme Platform', 'acme-platform', NOW())
ON CONFLICT (id) DO NOTHING;

INSERT INTO jobs (
  id, organisation_id, title, summary, description, employment_type,
  location, compensation, requirements, status, created_at, updated_at, published_at
) VALUES
(
  '22222222-2222-2222-2222-222222222201',
  '11111111-1111-1111-1111-111111111111',
  'Senior Rust Engineer',
  'Build the AI-native employment marketplace core in Rust.',
  E'## About the role\n\nYou will design and ship Axum services, SQLx repositories, and agent tooling for JobShout.com.\n\n## Responsibilities\n- Own domain crates and API contracts\n- Write reliable Postgres migrations\n- Instrument services with OpenTelemetry\n',
  'permanent',
  '{"country":"GB","city":"London","remote":true}',
  '{"currency":"GBP","min_amount":90000,"max_amount":130000,"period":"annual"}',
  '["Rust","Tokio","Axum","PostgreSQL","OpenTelemetry"]',
  'published', NOW(), NOW(), NOW()
),
(
  '22222222-2222-2222-2222-222222222202',
  '11111111-1111-1111-1111-111111111111',
  'Platform Engineer (Kubernetes)',
  'Run JobShout.com on Kubernetes with strong observability.',
  E'We need someone who has operated multi-tenant SaaS on Kubernetes.\n\nStack: Helm, Prometheus, Grafana, Postgres, NATS.',
  'contract',
  '{"country":"GB","region":"England","city":"Remote","remote":true}',
  '{"currency":"GBP","min_amount":650,"max_amount":750,"period":"daily"}',
  '["Kubernetes","Helm","Prometheus","Terraform"]',
  'published', NOW(), NOW(), NOW()
),
(
  '22222222-2222-2222-2222-222222222203',
  '11111111-1111-1111-1111-111111111111',
  'Staff Frontend Engineer',
  'Ship the JobShout.com candidate and employer experiences in Next.js.',
  E'Build SSR job pages, AI chat streaming UI, and employer dashboards.\n\nTypeScript strict mode. No domain logic in the browser.',
  'permanent',
  '{"country":"US","city":"New York","remote":true}',
  '{"currency":"USD","min_amount":160000,"max_amount":210000,"period":"annual"}',
  '["TypeScript","Next.js","React","Tailwind"]',
  'published', NOW(), NOW(), NOW()
),
(
  '22222222-2222-2222-2222-222222222204',
  '11111111-1111-1111-1111-111111111111',
  'iOS Engineer (SwiftUI)',
  'Native candidate app with voice interviews.',
  E'Build JobShout iOS with SwiftUI, WebSockets, and AVFoundation for AI interviews.',
  'permanent',
  '{"country":"IE","city":"Dublin","remote":false}',
  '{"currency":"EUR","min_amount":85000,"max_amount":110000,"period":"annual"}',
  '["Swift","SwiftUI","Combine","WebSockets"]',
  'published', NOW(), NOW(), NOW()
),
(
  '22222222-2222-2222-2222-222222222205',
  '11111111-1111-1111-1111-111111111111',
  'DevOps / SRE',
  'Own reliability for the marketplace API and workers.',
  E'CI/CD, multi-arch containers, on-call, SLOs for API and MCP.',
  'permanent',
  '{"country":"DE","city":"Berlin","remote":true}',
  '{"currency":"EUR","min_amount":80000,"max_amount":105000,"period":"annual"}',
  '["SRE","Docker","GitHub Actions","Observability"]',
  'published', NOW(), NOW(), NOW()
),
(
  '22222222-2222-2222-2222-222222222206',
  '11111111-1111-1111-1111-111111111111',
  'ML Engineer — Matching',
  'Explainable job ↔ candidate matching.',
  E'Design hybrid keyword + vector matching. No protected characteristics.',
  'permanent',
  '{"country":"CA","city":"Toronto","remote":true}',
  '{"currency":"CAD","min_amount":140000,"max_amount":180000,"period":"annual"}',
  '["Python","embeddings","pgvector","evaluation"]',
  'published', NOW(), NOW(), NOW()
),
(
  '22222222-2222-2222-2222-222222222207',
  '11111111-1111-1111-1111-111111111111',
  'Product Designer',
  'Premium AI marketplace UX.',
  E'Design candidate discovery, employer hiring OS, and ChatGPT-style assistants.',
  'freelance',
  '{"country":"GB","city":"London","remote":true}',
  '{"currency":"GBP","min_amount":500,"max_amount":650,"period":"daily"}',
  '["Figma","Design systems","Accessibility"]',
  'published', NOW(), NOW(), NOW()
),
(
  '22222222-2222-2222-2222-222222222208',
  '11111111-1111-1111-1111-111111111111',
  'Security Engineer',
  'Tenant isolation, MCP scopes, agent policy.',
  E'Harden auth, RBAC, audit, and AI security (prompt injection).',
  'permanent',
  '{"country":"NL","city":"Amsterdam","remote":true}',
  '{"currency":"EUR","min_amount":90000,"max_amount":120000,"period":"annual"}',
  '["AppSec","OAuth","OIDC","Threat modelling"]',
  'published', NOW(), NOW(), NOW()
),
(
  '22222222-2222-2222-2222-222222222209',
  '11111111-1111-1111-1111-111111111111',
  'Solutions Engineer',
  'Help employers connect agents via MCP and API.',
  E'Partner with customers integrating Career and Hiring agents.',
  'permanent',
  '{"country":"SG","city":"Singapore","remote":false}',
  '{"currency":"SGD","min_amount":120000,"max_amount":160000,"period":"annual"}',
  '["APIs","MCP","Customer success"]',
  'published', NOW(), NOW(), NOW()
),
(
  '22222222-2222-2222-2222-222222222210',
  '11111111-1111-1111-1111-111111111111',
  'Technical Writer',
  'API, MCP, and agent documentation.',
  E'Write docs for developers.jobshout.com — clear, accurate, example-rich.',
  'part_time',
  '{"country":"AU","city":"Sydney","remote":true}',
  '{"currency":"AUD","min_amount":70,"max_amount":90,"period":"hourly"}',
  '["Technical writing","OpenAPI","Developer experience"]',
  'published', NOW(), NOW(), NOW()
)
ON CONFLICT (id) DO NOTHING;
