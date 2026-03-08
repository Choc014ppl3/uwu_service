BEGIN;

-- Alter integer columns to NUMERIC(10,2) to allow 0.5 and 0.25 additions
ALTER TABLE user_stats 
    ALTER COLUMN listen_count TYPE NUMERIC(10,2),
    ALTER COLUMN speak_count TYPE NUMERIC(10,2),
    ALTER COLUMN read_count TYPE NUMERIC(10,2),
    ALTER COLUMN write_count TYPE NUMERIC(10,2);

COMMIT;
