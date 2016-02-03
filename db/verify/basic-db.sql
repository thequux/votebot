-- Verify votebot:basic-db on sqlite

BEGIN;

SELECT team_id, team_name, team_authtoken FROM teams WHERE FALSE ;
SELECT team_id, topic_channel, topic_name, topic_open FROM topics WHERE FALSE ;
SELECT vote_id, team_id, topic_id, user_id, vote_value, vote_comment FROM votes WHERE FALSE ;

ROLLBACK;
