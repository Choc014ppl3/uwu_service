BEGIN;

-- ============================================================
-- 1. Drop old tables (order matters for foreign keys)
-- ============================================================

-- Migration 007 tables (depend on videos, users)
DROP TABLE IF EXISTS user_retell_audio_logs CASCADE;
DROP TABLE IF EXISTS user_retell_sessions CASCADE;
DROP TABLE IF EXISTS retell_mission_points CASCADE;
DROP TABLE IF EXISTS user_quiz_logs CASCADE;
DROP TABLE IF EXISTS quiz_questions CASCADE;
DROP TABLE IF EXISTS lessons CASCADE;

-- Migration 001 tables
DROP TABLE IF EXISTS conversation_scenarios CASCADE;
DROP TABLE IF EXISTS learning_items CASCADE;

-- Migration 003-006 tables
DROP TABLE IF EXISTS videos CASCADE;

-- ============================================================
-- 2. Drop old ENUM types
-- ============================================================
DROP TYPE IF EXISTS quiz_question_type CASCADE;
DROP TYPE IF EXISTS retell_status_type CASCADE;
DROP TYPE IF EXISTS lang_code_enum CASCADE;
DROP TYPE IF EXISTS item_type_enum CASCADE;
DROP TYPE IF EXISTS interaction_type_enum CASCADE;

-- ============================================================
-- 3. Create ENUM types
-- ============================================================
CREATE TYPE attempt_status_enum AS ENUM ('completed', 'failed', 'skipped', 'in_progress');
CREATE TYPE user_action_type_enum AS ENUM ('comment', 'saved', 'done');
CREATE TYPE dashboard_item_type_enum AS ENUM ('word', 'sentence');
CREATE TYPE user_stat_action_enum AS ENUM ('listen', 'speak', 'read', 'write');

-- ============================================================
-- 4. Create new tables
-- ============================================================

-- Features: top-level grouping for learning content
CREATE TABLE features (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT
);

-- Learning Items: core content items linked to a feature
CREATE TABLE learning_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    feature_id INTEGER REFERENCES features(id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    lang_code VARCHAR(20) NOT NULL,
    estimated_level VARCHAR(20),
    details JSONB DEFAULT '{}'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    tags JSONB DEFAULT '[]'::jsonb,
    is_active BOOLEAN DEFAULT true,
    created_by VARCHAR(50) DEFAULT 'system',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Meaning Items: translations/meanings linked to a learning item
CREATE TABLE meaning_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    text TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    score JSONB DEFAULT '{}'::jsonb,
    meaning JSONB DEFAULT '[]'::jsonb,
    created_by VARCHAR(50) DEFAULT 'system',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Media Items: files (images, audio, video) linked to any system entity
CREATE TABLE media_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_path TEXT NOT NULL,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_by VARCHAR(50) DEFAULT 'system',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- User Attempts: tracks user practice attempts on learning items
CREATE TABLE user_attempts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    learning_id UUID NOT NULL REFERENCES learning_items(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL DEFAULT 1,
    attempt_duration INTEGER,
    attempt_status attempt_status_enum NOT NULL DEFAULT 'completed',
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- User Actions: tracks user interactions with learning items
CREATE TABLE user_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    learning_item_id UUID NOT NULL REFERENCES learning_items(id) ON DELETE CASCADE,
    type user_action_type_enum NOT NULL,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Dashboard Items: vocabulary and sentences shown on the dashboard
CREATE TABLE dashboard_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    learning_item_id UUID NOT NULL REFERENCES learning_items(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    type dashboard_item_type_enum NOT NULL,
    lang_code VARCHAR(20) NOT NULL,
    estimated_level VARCHAR(20),
    metadata JSONB DEFAULT '{}'::jsonb,
    created_by VARCHAR(50) DEFAULT 'system',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- User Stats: tracking of dashboard item interactions
CREATE TABLE user_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    dashboard_id UUID NOT NULL REFERENCES dashboard_items(id) ON DELETE CASCADE,
    action user_stat_action_enum NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- 4. Indexes
-- ============================================================
CREATE INDEX idx_learning_items_feature_id ON learning_items(feature_id);
CREATE INDEX idx_learning_items_lang_code ON learning_items(lang_code);
CREATE INDEX idx_learning_items_is_active ON learning_items(is_active);

CREATE INDEX idx_meaning_items_text ON meaning_items(text);

CREATE INDEX idx_user_attempts_user_id ON user_attempts(user_id);
CREATE INDEX idx_user_attempts_learning_id ON user_attempts(learning_id);
CREATE INDEX idx_user_attempts_user_learning ON user_attempts(user_id, learning_id);

CREATE INDEX idx_user_actions_user_id ON user_actions(user_id);
CREATE INDEX idx_user_actions_learning_item_id ON user_actions(learning_item_id);

CREATE INDEX idx_dashboard_items_learning_item_id ON dashboard_items(learning_item_id);
CREATE INDEX idx_dashboard_items_lang_code ON dashboard_items(lang_code);

CREATE INDEX idx_user_stats_user_id ON user_stats(user_id);
CREATE INDEX idx_user_stats_dashboard_id ON user_stats(dashboard_id);

-- ============================================================
-- 5. Data Seeds
-- ============================================================

INSERT INTO features (name, description) VALUES
('Native Immersion', 'Experience the language exactly as locals use it'),
('Gist Quiz', 'Catch the core message, skip the noise'),
('Retell Story', 'Absorb the narrative and make it your own'),
('Pocket Mission', 'Bite-sized challenges for learning on the go'),
('Rhythm & Flow', 'Master the melody of the language'),
('Vocabulary Reps', 'Build a rock-solid vocabulary through context'),
('Precision Check', 'Level up from "understood" to "unmistakable'),
('Structure Drill', 'Lock down your grammar effortlessly'),
('Sparring Mode', 'Think on your feet and build conversational reflexes'),
('Mission Guide', 'Your personal roadmap to conversational success');

COMMIT;
