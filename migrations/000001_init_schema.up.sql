-- 000001_init_schema.up.sql
-- Initial database schema for uwu_service

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create ENUMs
CREATE TYPE lang_code_enum AS ENUM ('en-US', 'zh-CN');
CREATE TYPE item_type_enum AS ENUM ('word', 'character', 'phrase', 'sentence');
CREATE TYPE interaction_type_enum AS ENUM ('speech', 'chat');

-- Table: learning_items
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

-- Table: conversation_scenarios
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

-- Indexes for common queries
CREATE INDEX idx_learning_items_lang_code ON learning_items(lang_code);
CREATE INDEX idx_learning_items_type ON learning_items(type);
CREATE INDEX idx_learning_items_is_active ON learning_items(is_active);
CREATE INDEX idx_conversation_scenarios_target_lang ON conversation_scenarios(target_lang);
CREATE INDEX idx_conversation_scenarios_is_active ON conversation_scenarios(is_active);
