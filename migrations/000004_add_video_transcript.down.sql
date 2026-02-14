-- 000004_add_video_transcript.down.sql
ALTER TABLE videos
  DROP COLUMN IF EXISTS transcript,
  DROP COLUMN IF EXISTS detected_language,
  DROP COLUMN IF EXISTS processing_status;
