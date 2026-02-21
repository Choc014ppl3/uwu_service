-- 000003_create_videos_table.up.sql
-- Videos table for Wavvy short video uploads

CREATE TABLE videos (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    video_url TEXT NOT NULL DEFAULT '',
    status VARCHAR(50) NOT NULL DEFAULT 'processing',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_videos_user_id ON videos(user_id);
CREATE INDEX idx_videos_status ON videos(status);
