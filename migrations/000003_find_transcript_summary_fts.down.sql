DROP INDEX IF EXISTS meetings_transcript_summary_fts_idx;

CREATE INDEX IF NOT EXISTS meetings_transcript_fts_idx
    ON meetings USING GIN (to_tsvector('russian', transcript));
