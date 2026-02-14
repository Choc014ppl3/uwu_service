ALTER TABLE videos
  DROP COLUMN IF EXISTS raw_response,
  DROP COLUMN IF EXISTS updated_at;
