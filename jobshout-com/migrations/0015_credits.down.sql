ALTER TABLE projects DROP COLUMN IF EXISTS featured_listing_until;
DROP TABLE IF EXISTS bid_daily_caps;
DROP TABLE IF EXISTS stripe_webhook_events;
DROP TABLE IF EXISTS account_subscriptions;
DROP TABLE IF EXISTS subscription_plans;
DROP TABLE IF EXISTS credit_packs;
DROP TABLE IF EXISTS credit_ledger;
