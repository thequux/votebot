-- Deploy votebot:basic-db to postgres

BEGIN;

CREATE TABLE teams (
  team_id TEXT PRIMARY KEY,
  team_name TEXT NOT NULL,
  team_authtoken TEXT NOT NULL
);

CREATE TABLE topics (
  team_id TEXT NOT NULL REFERENCES teams(team_id),
  topic_channel TEXT NOT NULL,
  topic_name TEXT NOT NULL,
  topic_open BOOLEAN NOT NULL DEFAULT TRUE,
  UNIQUE(team_id, topic_name, topic_channel)
);

CREATE TABLE users (
  team_id TEXT NOT NULL REFERENCES teams(team_id),
  user_id TEXT NOT NULL,
  user_name TEXT NOT NULL,
  user_is_bot BOOL,
  PRIMARY KEY (team_id, user_id));

CREATE TABLE votes (
  vote_id SERIAL PRIMARY KEY,
  team_id TEXT NOT NULL REFERENCES teams(team_id),
  topic_name TEXT NOT NULL,
  user_id TEXT NOT NULL,
  vote_value DECIMAL NOT NULL, -- Fixed-point decimal with 2 fractional places. (i.e., 100 == +1)
  vote_comment TEXT,
  FOREIGN KEY (team_id, user_id) REFERENCES users(team_id, user_id),
  UNIQUE (topic_name, team_id, user_id)
);

COMMIT;
