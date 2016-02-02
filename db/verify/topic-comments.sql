-- Verify votebot:topic-comments on pg

BEGIN;

SELECT topic_comment FROM topics WHERE FALSE ;

ROLLBACK;
