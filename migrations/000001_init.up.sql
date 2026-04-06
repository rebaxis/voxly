CREATE TABLE IF NOT EXISTS users (
    user_id    BIGINT      PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS meetings (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    BIGINT      NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    file_id    TEXT        NOT NULL,
    transcript TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for fast per-user listing sorted by date
CREATE INDEX IF NOT EXISTS meetings_user_created_idx
    ON meetings (user_id, created_at DESC);

-- GIN index for full-text search on the Russian-language transcript
CREATE INDEX IF NOT EXISTS meetings_transcript_fts_idx
    ON meetings USING GIN (to_tsvector('russian', transcript));
