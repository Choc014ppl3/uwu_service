BEGIN;

-- 1. Create enum types
CREATE TYPE learning_source_type_enum AS ENUM ('word', 'sentence');
CREATE TYPE user_stat_status_enum AS ENUM ('new', 'pass', 'recognize');
CREATE TYPE learning_item_action_type_enum AS ENUM ('quiz_passed', 'quiz_attempted', 'quiz_saved', 'dialogue_passed', 'dialogue_saved', 'chat_attempted', 'chat_passed', 'speech_attempted', 'speech_passed');

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

-- 4. Create learning_item_actions table
CREATE TABLE learning_item_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    learning_id UUID NOT NULL REFERENCES learning_items(id) ON DELETE CASCADE,
    action_type learning_item_action_type_enum NOT NULL,
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
CREATE INDEX idx_learning_item_actions_user_id ON learning_item_actions(user_id);

COMMIT;
