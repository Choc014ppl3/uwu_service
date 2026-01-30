-- Enable UUID extension (if not exists)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create ENUM types for Data Consistency
CREATE TYPE lang_code_enum AS ENUM (
    'en-US',  -- English (US)
    'zh-CN'   -- Chinese (Simplified)
);
CREATE TYPE item_type_enum AS ENUM ('word', 'character', 'phrase', 'sentence');
CREATE TYPE interaction_type_enum AS ENUM ('speech', 'chat');

-- Table learning_items (for vocabulary/grammar)
CREATE TABLE IF NOT EXISTS learning_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- 1. Content (Target Language) - ยังคงเป็น Text เพราะคือภาษาเป้าหมาย
    content TEXT NOT NULL,                      -- e.g., "Water"
    lang_code lang_code_enum NOT NULL,          -- e.g., "en" (Target Language)

    -- 2. Meanings (Native Language Support) - ปรับเป็น JSONB
    -- Structure: { "th": "น้ำ", "cn": "水", "jp": "水" }
    meanings JSONB DEFAULT '{}'::jsonb NOT NULL, 

    -- 3. Readings (Target Language Standard)
    -- Structure: { "ipa": "/ˈwɔːtər/", "standard": "wǒ ter" }
    reading JSONB DEFAULT '{}'::jsonb,
    
    -- 4. Classification
    type item_type_enum NOT NULL,
    tags TEXT[],        -- e.g., ["food", "drink", "common"]
    
    -- 5. Rich Data
    -- Media: { "image": "...", "audio": "..." } (ยังคงเป็น Universal มักใช้ร่วมกัน)
    media JSONB DEFAULT '{}'::jsonb, 
    
    -- Metadata: Hard Facts (โครงสร้างภาษาที่ไม่เปลี่ยนตามคนเรียน)
    -- { "pos": "noun", "plural": "waters" }
    metadata JSONB DEFAULT '{}'::jsonb,

    -- System
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Table conversation_scenarios (for conversation scenarios)
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
