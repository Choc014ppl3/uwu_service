BEGIN;

-- 1a. Idempotent cleanup for enums
DROP TYPE IF EXISTS learning_source_type_enum CASCADE;
DROP TYPE IF EXISTS user_stat_status_enum CASCADE;
DROP TYPE IF EXISTS user_action_type_enum CASCADE;
DROP TYPE IF EXISTS learning_item_action_type_enum CASCADE;

-- 1. Create enum types
CREATE TYPE learning_source_type_enum AS ENUM ('word', 'sentence');
CREATE TYPE user_stat_status_enum AS ENUM ('new', 'pass', 'recognize');
CREATE TYPE user_action_type_enum AS ENUM ('quiz_passed', 'quiz_attempted', 'quiz_saved', 'dialogue_passed', 'dialogue_saved', 'chat_attempted', 'chat_passed', 'speech_attempted', 'speech_passed');

-- 1b. Drop deprecated tables from old schema
DROP TABLE IF EXISTS meaning_items CASCADE;
DROP TABLE IF EXISTS media_items CASCADE;
DROP TABLE IF EXISTS user_attempts CASCADE;
DROP TABLE IF EXISTS user_actions CASCADE;
DROP TABLE IF EXISTS dashboard_items CASCADE;
DROP TABLE IF EXISTS learning_item_actions CASCADE;

-- 2. Create learning_sources table
CREATE TABLE learning_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    content TEXT NOT NULL UNIQUE,
    language VARCHAR(20) NOT NULL,
    type learning_source_type_enum NOT NULL,
    level VARCHAR(20),
    tags JSONB DEFAULT '[]'::jsonb,
    media JSONB DEFAULT '{}'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    translate JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 3. Update user_stats table
-- We drop the old one and recreate it to match the new radical schema request
DROP TABLE IF EXISTS user_stats CASCADE;

CREATE TABLE user_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_id UUID NOT NULL REFERENCES learning_sources(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    language TEXT NOT NULL,
    type learning_source_type_enum NOT NULL,
    status user_stat_status_enum DEFAULT 'new',
    listen_count INTEGER DEFAULT 0,
    speak_count INTEGER DEFAULT 0,
    read_count INTEGER DEFAULT 0,
    write_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 4. Create user_actions table (replaces the old one and learning_item_actions)
CREATE TABLE user_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    learning_id UUID NOT NULL REFERENCES learning_items(id) ON DELETE CASCADE,
    action_type user_action_type_enum NOT NULL,
    attempt_count INTEGER DEFAULT 0,
    pass_count INTEGER DEFAULT 0,
    fail_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (learning_id, user_id)
);

-- 5. Indexes
CREATE INDEX idx_learning_sources_lang_type ON learning_sources(language, type);
CREATE INDEX idx_user_stats_user_id ON user_stats(user_id);
CREATE INDEX idx_user_stats_source_id ON user_stats(source_id);
CREATE INDEX idx_user_actions_user_id ON user_actions(user_id);

COMMIT;

-- 6. Rename columns in learning_items
ALTER TABLE learning_items RENAME COLUMN lang_code TO language;
ALTER TABLE learning_items RENAME COLUMN estimated_level TO level;
