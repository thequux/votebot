-- Revert votebot:basic-db from sqlite

BEGIN;

DROP TABLE votes;
DROP TABLE topics;
DROP TABLE topics;
DROP TABLE teams;

COMMIT;
