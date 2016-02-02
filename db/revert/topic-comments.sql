-- Revert votebot:topic-comments from pg

BEGIN;

ALTER TABLE topics DROP COLUMN topic_comment;

COMMIT;
