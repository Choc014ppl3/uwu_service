-- 000001_init_schema.down.sql
-- Rollback initial schema

-- Drop indexes
DROP INDEX IF EXISTS idx_conversation_scenarios_is_active;
DROP INDEX IF EXISTS idx_conversation_scenarios_target_lang;
DROP INDEX IF EXISTS idx_learning_items_is_active;
DROP INDEX IF EXISTS idx_learning_items_type;
DROP INDEX IF EXISTS idx_learning_items_lang_code;

-- Drop tables
DROP TABLE IF EXISTS conversation_scenarios;
DROP TABLE IF EXISTS learning_items;

-- Drop ENUMs
DROP TYPE IF EXISTS interaction_type_enum;
DROP TYPE IF EXISTS item_type_enum;
DROP TYPE IF EXISTS lang_code_enum;

-- Note: We don't drop uuid-ossp extension as it might be used by other schemas
