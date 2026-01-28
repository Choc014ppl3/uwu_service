-- 1. Enable UUID extension (if not exists)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 2. Create ENUM types for Data Consistency
CREATE TYPE item_type_enum AS ENUM ('word', 'character', 'phrase', 'sentence');
CREATE TYPE interaction_type_enum AS ENUM ('speech', 'chat');

-- 3. Table learning_items (for vocabulary/grammar)
CREATE TABLE IF NOT EXISTS learning_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Content Core
    content TEXT NOT NULL,                  -- The word/sentence (e.g., "Hello", "猫")
    meaning TEXT NOT NULL,                  -- Meaning in native language
    reading_simple TEXT,                    -- Simple reading (e.g., "Neko")
    reading_ipa TEXT,                       -- IPA reading (e.g., "/neko/")
    
    -- Classification
    type item_type_enum NOT NULL,           -- Type (word, character, etc.)
    tags TEXT[],                            -- Tags for categorization (e.g., ['HSK1', 'Food'])
    
    -- Rich Data (JSONB for flexibility)
    media_assets JSONB DEFAULT '{}',        -- Images, audio (Structure: {image_url: "", audio_url: ""})
    metadata JSONB DEFAULT '{}',            -- Grammar, POS, components (Structure: {pos: "noun", tone: 3, components: []})
    
    -- System fields
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexing for JSONB (Important for searching in metadata)
CREATE INDEX IF NOT EXISTS idx_learning_items_metadata ON learning_items USING GIN (metadata);
CREATE INDEX IF NOT EXISTS idx_learning_items_content ON learning_items (content);

-- 4. Table conversation_scenarios (for conversation scenarios)
CREATE TABLE IF NOT EXISTS conversation_scenarios (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Scenario Details
    topic TEXT NOT NULL,                    -- Topic (e.g., "Ordering Food")
    description TEXT,                       -- Scenario description
    interaction_type interaction_type_enum NOT NULL, -- Type (speech/chat)
    
    -- Meta Data for Scenario
    scenario_meta JSONB DEFAULT '{}',       -- Script, Objective (Structure: {objectives: [], script: [{role: "A", text: "..."}]})
    
    -- Difficulty Level
    difficulty_level INTEGER DEFAULT 1,     -- Difficulty level 1-5
    
    -- System fields
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
