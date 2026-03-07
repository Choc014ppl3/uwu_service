BEGIN;

-- Drop newly created tables
DROP TABLE IF EXISTS user_actions CASCADE;
DROP TABLE IF EXISTS user_stats CASCADE;
DROP TABLE IF EXISTS learning_sources CASCADE;

-- Drop newly created ENUM types
DROP TYPE IF EXISTS user_action_type_enum CASCADE;
DROP TYPE IF EXISTS learning_source_type_enum CASCADE;
DROP TYPE IF EXISTS user_stat_status_enum CASCADE;

-- Recreate old user_stats table is skipped because 
-- it depends on deprecated tables (like dashboard_items) which
-- may have been permanently dropped.

-- Note: We do not restore the five deprecated tables here (meaning_items, 
-- media_items, user_attempts, user_actions, dashboard_items) because down 
-- migrations are generally best-effort rollback tools and restoring structural
-- drops with data is complex. If needed, the previous migration 000008 provides 
-- the schema.

-- Rename columns in learning_items back to original
ALTER TABLE learning_items RENAME COLUMN language TO lang_code;
ALTER TABLE learning_items RENAME COLUMN level TO estimated_level;

COMMIT;
