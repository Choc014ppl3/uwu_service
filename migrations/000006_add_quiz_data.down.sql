ALTER TABLE videos 
DROP COLUMN IF EXISTS quiz_data,
DROP COLUMN IF EXISTS quiz_generated_at;
