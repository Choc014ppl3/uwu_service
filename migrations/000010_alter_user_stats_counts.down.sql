BEGIN;

-- Revert columns back to INTEGER. This may lose fraction data (e.g. 0.5 -> 0)
ALTER TABLE user_stats 
    ALTER COLUMN listen_count TYPE INTEGER,
    ALTER COLUMN speak_count TYPE INTEGER,
    ALTER COLUMN read_count TYPE INTEGER,
    ALTER COLUMN write_count TYPE INTEGER;

COMMIT;
