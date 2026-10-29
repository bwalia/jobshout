-- Down migration for Upwork-style freelancer marketplace
DROP TABLE IF EXISTS project_categories_mapping;
DROP TABLE IF EXISTS project_categories;
DROP TABLE IF EXISTS freelancer_profiles;
DROP TABLE IF EXISTS bids;
DROP TABLE IF EXISTS projects;