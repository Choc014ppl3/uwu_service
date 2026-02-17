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
    text TEXT NOT NULL,
    lang_code VARCHAR(20) NOT NULL,
    estimated_level VARCHAR(20),
    details JSONB DEFAULT '{}'::jsonb,
    tags JSONB DEFAULT '[]'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Meaning Items: translations/meanings linked to a learning item
CREATE TABLE meaning_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    content_id UUID NOT NULL REFERENCES learning_items(id) ON DELETE CASCADE,
    native_code VARCHAR(20) NOT NULL,
    created_by VARCHAR(50) DEFAULT 'system',
    meaning JSONB DEFAULT '{}'::jsonb NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Media Types: lookup table for media categories
CREATE TABLE media_types (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT
);

-- Media Items: files (images, audio, video) linked to any system entity
CREATE TABLE media_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    system_id UUID NOT NULL,
    type_id INTEGER NOT NULL REFERENCES media_types(id) ON DELETE RESTRICT,
    file_path TEXT NOT NULL,
    created_by VARCHAR(50) DEFAULT 'system',
    metadata JSONB DEFAULT '{}'::jsonb,
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

-- ============================================================
-- 4. Indexes
-- ============================================================
CREATE INDEX idx_learning_items_feature_id ON learning_items(feature_id);
CREATE INDEX idx_learning_items_lang_code ON learning_items(lang_code);
CREATE INDEX idx_learning_items_is_active ON learning_items(is_active);
CREATE INDEX idx_learning_items_metadata_batch ON learning_items USING gin (metadata jsonb_path_ops);

CREATE INDEX idx_meaning_items_content_id ON meaning_items(content_id);
CREATE INDEX idx_meaning_items_native_code ON meaning_items(native_code);

CREATE INDEX idx_media_items_system_id ON media_items(system_id);
CREATE INDEX idx_media_items_type_id ON media_items(type_id);

CREATE INDEX idx_user_attempts_user_id ON user_attempts(user_id);
CREATE INDEX idx_user_attempts_learning_id ON user_attempts(learning_id);
CREATE INDEX idx_user_attempts_user_learning ON user_attempts(user_id, learning_id);

COMMIT;
