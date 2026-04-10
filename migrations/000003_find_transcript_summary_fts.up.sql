-- Combined FTS over transcript + summary for /find (replaces transcript-only index).
CREATE INDEX IF NOT EXISTS meetings_transcript_summary_fts_idx
    ON meetings USING GIN (
        to_tsvector('russian', coalesce(transcript, '') || ' ' || coalesce(summary, ''))
    );

DROP INDEX IF EXISTS meetings_transcript_fts_idx;
