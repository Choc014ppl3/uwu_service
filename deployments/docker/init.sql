-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Create ENUMs (แก้ Syntax Comma แล้ว)
DO $$ BEGIN
    CREATE TYPE lang_code_enum AS ENUM ('en-US', 'zh-CN');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE item_type_enum AS ENUM ('word', 'character', 'phrase', 'sentence');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE interaction_type_enum AS ENUM ('speech', 'chat');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

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
    is_active BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Table conversation_scenarios (for conversation scenarios)
CREATE TABLE IF NOT EXISTS conversation_scenarios (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- --- INPUTS (ใช้สำหรับส่งเข้า Prompt) ---
    topic TEXT NOT NULL,                    -- หัวข้อ (Prompt: Topic)
    description TEXT,                       -- รายละเอียดบริบท (Prompt: Description)
    interaction_type interaction_type_enum NOT NULL, -- ประเภท (Prompt: Scenario Type)
    
    target_lang lang_code_enum NOT NULL DEFAULT 'en-US', -- ภาษาที่เรียน (Prompt: Target Language)
    
    estimated_turns VARCHAR(50),            -- ความยาวบทสนทนา เก็บเป็น String เพื่อรองรับ Range ได้ (เช่น "10" หรือ "8-12")
    difficulty_level INTEGER DEFAULT 1,     -- ระดับความยาก (1-5)
    
    -- --- OUTPUTS (เก็บผลลัพธ์จาก AI) ---
    -- เก็บ JSON Response ทั้งก้อนที่ AI ส่งกลับมา
    -- Structure Speech: { "type": "speech", "image_prompt": "...", "script": [...] }
    -- Structure Chat:   { "type": "chat", "image_prompt": "...", "objectives": {...} }
    metadata JSONB DEFAULT '{}'::jsonb,
    
    -- --- SYSTEM ---
    is_active BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);