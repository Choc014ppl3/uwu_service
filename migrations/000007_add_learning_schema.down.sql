BEGIN;

DROP TABLE IF EXISTS user_retell_audio_logs;
DROP TABLE IF EXISTS user_retell_sessions;
DROP TABLE IF EXISTS retell_mission_points;
DROP TABLE IF EXISTS user_quiz_logs;
DROP TABLE IF EXISTS quiz_questions;
DROP TABLE IF EXISTS lessons;

DROP TYPE IF EXISTS retell_status_type;
DROP TYPE IF EXISTS quiz_question_type;

COMMIT;
