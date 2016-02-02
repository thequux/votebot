-- Deploy votebot:topic-comments to pg
-- requires: basic-db

BEGIN;

ALTER TABLE topics ADD COLUMN topic_comment TEXT;

COMMIT;
