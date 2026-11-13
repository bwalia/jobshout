DROP TABLE IF EXISTS engagement_reviews;
DROP TABLE IF EXISTS engagements;
DROP TABLE IF EXISTS project_invitations;

ALTER TABLE bids
    DROP COLUMN IF EXISTS credits_spent,
    DROP COLUMN IF EXISTS viewed_at,
    DROP COLUMN IF EXISTS decline_reason;

ALTER TABLE projects
    DROP COLUMN IF EXISTS owner_account_id,
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS summary,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS visibility,
    DROP COLUMN IF EXISTS featured_until,
    DROP COLUMN IF EXISTS experience_level;
