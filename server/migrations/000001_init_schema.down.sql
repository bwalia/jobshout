-- Drop all tables in reverse dependency order
DROP TABLE IF EXISTS ws_events;
DROP TABLE IF EXISTS agent_metrics;
DROP TABLE IF EXISTS marketplace_bundle_agents;
DROP TABLE IF EXISTS marketplace_agents;
DROP TABLE IF EXISTS knowledge_files;
DROP TABLE IF EXISTS task_status_history;
DROP TABLE IF EXISTS task_comments;
DROP TABLE IF EXISTS task_labels;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS project_agents;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS refresh_tokens;
ALTER TABLE IF EXISTS organizations DROP CONSTRAINT IF EXISTS fk_organizations_owner;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
