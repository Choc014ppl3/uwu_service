BEGIN;

-- Drop newly created tables
DROP TABLE IF EXISTS video_actions CASCADE;
DROP TABLE IF EXISTS user_stats CASCADE;
DROP TABLE IF EXISTS learning_sources CASCADE;

-- Drop newly created ENUM types
DROP TYPE IF EXISTS video_action_type_enum CASCADE;
DROP TYPE IF EXISTS learning_source_type_enum CASCADE;
DROP TYPE IF EXISTS user_stat_status_enum CASCADE;

-- Recreate old user_stats table (from migration 000008)
CREATE TABLE user_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    dashboard_id UUID NOT NULL REFERENCES dashboard_items(id) ON DELETE CASCADE,
    action user_stat_action_enum NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_user_stats_user_id ON user_stats(user_id);
CREATE INDEX idx_user_stats_dashboard_id ON user_stats(dashboard_id);

COMMIT;
