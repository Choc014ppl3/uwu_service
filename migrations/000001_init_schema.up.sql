BEGIN;

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================
-- 1. Create Enum Types
-- ============================================================
CREATE TYPE learning_source_type_enum AS ENUM ('word', 'sentence');
CREATE TYPE user_stat_status_enum AS ENUM ('new', 'passed', 'recognized');
CREATE TYPE user_action_type_enum AS ENUM (
    'dialogue_saved', 'chat_attempted', 'speech_attempted', 'quiz_attempted', 'quiz_saved'
);

-- ============================================================
-- 2. Create Core Setup Tables
-- ============================================================
CREATE TABLE users (
    -- Primary Identity
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,

    -- Profile Information
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    avatar_url TEXT,
    bio TEXT,
    settings JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_users_email ON users(email);

CREATE TABLE features (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT
);

-- ============================================================
-- 3. Create Learning Content Tables
-- ============================================================
CREATE TABLE learning_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    feature_id INTEGER REFERENCES features(id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    language VARCHAR(20) NOT NULL,
    level VARCHAR(20),
    details JSONB DEFAULT '{}'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    tags JSONB DEFAULT '[]'::jsonb,
    is_active BOOLEAN DEFAULT true,
    created_by VARCHAR(50) DEFAULT 'system',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_learning_items_feature_id ON learning_items(feature_id);
CREATE INDEX idx_learning_items_language ON learning_items(language);
CREATE INDEX idx_learning_items_is_active ON learning_items(is_active);

CREATE TABLE learning_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    content TEXT NOT NULL,
    language VARCHAR(20) NOT NULL,
    type learning_source_type_enum NOT NULL,
    level VARCHAR(20),
    tags JSONB DEFAULT '[]'::jsonb,
    media JSONB DEFAULT '{}'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    translate JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (content, language)
);
CREATE INDEX idx_learning_sources_lang_type ON learning_sources(language, type);

-- ============================================================
-- 4. Create User Tracking Tables
-- ============================================================
CREATE TABLE user_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_id UUID NOT NULL REFERENCES learning_sources(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    language TEXT NOT NULL,
    type learning_source_type_enum NOT NULL,
    status user_stat_status_enum DEFAULT 'new',
    listen_count NUMERIC(10,2) DEFAULT 0,
    speak_count NUMERIC(10,2) DEFAULT 0,
    read_count NUMERIC(10,2) DEFAULT 0,
    write_count NUMERIC(10,2) DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (source_id, user_id)
);
CREATE INDEX idx_user_stats_user_id ON user_stats(user_id);
CREATE INDEX idx_user_stats_source_id ON user_stats(source_id);

CREATE TABLE user_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    learning_id UUID NOT NULL REFERENCES learning_items(id) ON DELETE CASCADE,
    action_type user_action_type_enum NOT NULL,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (learning_id, user_id)
);
CREATE INDEX idx_user_actions_user_id ON user_actions(user_id);

-- ============================================================
-- 5. Data Seeds
-- ============================================================
INSERT INTO features (name, description) VALUES
('Native Video', 'Experience the language exactly as locals use it'),
('Dialogue Guide', 'A complete roadmap to conversational success');

COMMIT;
