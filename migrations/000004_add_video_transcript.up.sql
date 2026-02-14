-- 000004_add_video_transcript.up.sql
-- Add transcript and language detection fields to videos table

ALTER TABLE videos
  ADD COLUMN transcript JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN detected_language VARCHAR(10) NOT NULL DEFAULT '',
  ADD COLUMN processing_status VARCHAR(50) NOT NULL DEFAULT 'pending';
