BEGIN;

-- Drop new tables
DROP TABLE IF EXISTS user_stats CASCADE;
DROP TABLE IF EXISTS user_actions CASCADE;
DROP TABLE IF EXISTS dashboard_items CASCADE;
DROP TABLE IF EXISTS user_attempts CASCADE;
DROP TABLE IF EXISTS media_items CASCADE;
DROP TABLE IF EXISTS media_types CASCADE;
DROP TABLE IF EXISTS meaning_items CASCADE;
DROP TABLE IF EXISTS learning_items CASCADE;
DROP TABLE IF EXISTS features CASCADE;

-- Drop new ENUM types
DROP TYPE IF EXISTS user_stat_action_enum CASCADE;
DROP TYPE IF EXISTS dashboard_item_type_enum CASCADE;
DROP TYPE IF EXISTS user_action_type_enum CASCADE;
DROP TYPE IF EXISTS attempt_status_enum CASCADE;

-- Recreate old ENUM types
CREATE TYPE lang_code_enum AS ENUM ('en-US', 'zh-CN');
CREATE TYPE item_type_enum AS ENUM ('word', 'character', 'phrase', 'sentence');
CREATE TYPE interaction_type_enum AS ENUM ('speech', 'chat');
CREATE TYPE quiz_question_type AS ENUM ('multiple_choice', 'multiple_response', 'single_choice', 'ordering');
CREATE TYPE retell_status_type AS ENUM ('in_progress', 'completed', 'failed');

-- Recreate old learning_items
CREATE TABLE learning_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    content TEXT NOT NULL,
    lang_code lang_code_enum NOT NULL,
    meanings JSONB DEFAULT '{}'::jsonb NOT NULL,
    reading JSONB DEFAULT '{}'::jsonb,
    type item_type_enum NOT NULL,
    tags TEXT[],
    media JSONB DEFAULT '{}'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    is_active BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Recreate old conversation_scenarios
CREATE TABLE conversation_scenarios (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    topic TEXT NOT NULL,
    description TEXT,
    interaction_type interaction_type_enum NOT NULL,
    target_lang lang_code_enum NOT NULL DEFAULT 'en-US',
    estimated_turns VARCHAR(50),
    difficulty_level INTEGER DEFAULT 1,
    metadata JSONB DEFAULT '{}'::jsonb,
    is_active BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Recreate old videos
CREATE TABLE videos (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    video_url TEXT NOT NULL DEFAULT '',
    status VARCHAR(50) NOT NULL DEFAULT 'processing',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Recreate old lessons & related tables
CREATE TABLE lessons (
    id SERIAL PRIMARY KEY,
    video_id UUID REFERENCES videos(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    video_url TEXT,
    category VARCHAR(100),
    difficulty_level INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE quiz_questions (
    id SERIAL PRIMARY KEY,
    lesson_id INTEGER REFERENCES lessons(id) ON DELETE CASCADE,
    type quiz_question_type NOT NULL,
    skill_tag VARCHAR(50),
    question_data JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_quiz_logs (
    id SERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    lesson_id INTEGER REFERENCES lessons(id) ON DELETE CASCADE,
    score INTEGER NOT NULL,
    max_score INTEGER NOT NULL,
    answers_snapshot JSONB,
    time_taken_seconds INTEGER,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE retell_mission_points (
    id SERIAL PRIMARY KEY,
    lesson_id INTEGER REFERENCES lessons(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    keywords JSONB,
    weight INTEGER DEFAULT 1
);

CREATE TABLE user_retell_sessions (
    id SERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    lesson_id INTEGER REFERENCES lessons(id) ON DELETE CASCADE,
    status retell_status_type DEFAULT 'in_progress',
    attempt_count INTEGER DEFAULT 0,
    collected_point_ids JSONB DEFAULT '[]'::jsonb,
    current_score FLOAT DEFAULT 0.0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_retell_audio_logs (
    id SERIAL PRIMARY KEY,
    session_id INTEGER REFERENCES user_retell_sessions(id) ON DELETE CASCADE,
    audio_url TEXT,
    transcript TEXT,
    found_point_ids JSONB DEFAULT '[]'::jsonb,
    ai_feedback TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

COMMIT;
