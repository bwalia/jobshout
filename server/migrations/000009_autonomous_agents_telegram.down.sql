-- 000009_autonomous_agents_telegram (rollback)

DROP TABLE IF EXISTS telegram_rate_limits;
DROP TABLE IF EXISTS telegram_link_tokens;
DROP TABLE IF EXISTS telegram_user_mappings;
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS chat_sessions;
DROP TABLE IF EXISTS multi_agent_jobs;
DROP TABLE IF EXISTS agent_goals;
DROP TABLE IF EXISTS agent_memory_long_term;
DROP TABLE IF EXISTS agent_memory_short_term;
