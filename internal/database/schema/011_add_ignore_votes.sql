-- Active community votes to place a normal user on the bot ignore list.
CREATE TABLE IF NOT EXISTS ignore_votes (
    target_nick TEXT NOT NULL,
    voter_key TEXT NOT NULL,
    voted_at INTEGER NOT NULL,
    PRIMARY KEY (target_nick, voter_key)
);

CREATE INDEX IF NOT EXISTS idx_ignore_votes_voted_at
    ON ignore_votes(voted_at);
