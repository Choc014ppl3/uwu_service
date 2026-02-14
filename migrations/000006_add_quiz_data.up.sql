ALTER TABLE videos 
ADD COLUMN quiz_data JSONB DEFAULT NULL,
ADD COLUMN quiz_generated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL;

COMMENT ON COLUMN videos.quiz_data IS 'Stores generated quiz and retell check in JSON format';
