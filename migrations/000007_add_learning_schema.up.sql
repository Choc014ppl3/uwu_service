BEGIN;

-- 1. Create Custom ENUM Types
CREATE TYPE quiz_question_type AS ENUM ('multiple_choice', 'multiple_response', 'ordering');
CREATE TYPE retell_status_type AS ENUM ('in_progress', 'completed', 'failed');

-- 2. Lessons Table
CREATE TABLE lessons (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    video_url TEXT,
    category VARCHAR(100),
    difficulty_level INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- 3. Gist Quiz Tables
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
    user_id UUID REFERENCES users(id) ON DELETE CASCADE, -- Changed from INTEGER to UUID
    lesson_id INTEGER REFERENCES lessons(id) ON DELETE CASCADE,
    score INTEGER NOT NULL,
    max_score INTEGER NOT NULL,
    answers_snapshot JSONB,
    time_taken_seconds INTEGER,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- 4. Retell Check Tables
CREATE TABLE retell_mission_points (
    id SERIAL PRIMARY KEY,
    lesson_id INTEGER REFERENCES lessons(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    keywords JSONB,
    weight INTEGER DEFAULT 1
);

CREATE TABLE user_retell_sessions (
    id SERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE, -- Changed from INTEGER to UUID
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

-- 5. Indexes
CREATE INDEX idx_quiz_logs_user_lesson ON user_quiz_logs(user_id, lesson_id);
CREATE INDEX idx_retell_sessions_user_lesson ON user_retell_sessions(user_id, lesson_id);
CREATE INDEX idx_quiz_questions_lesson ON quiz_questions(lesson_id);
CREATE INDEX idx_mission_points_lesson ON retell_mission_points(lesson_id);

COMMIT;
