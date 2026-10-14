CREATE TABLE IF NOT EXISTS snapshots (
    date       TEXT NOT NULL,  -- YYYY-MM-DD, Chile time
    goal_id    TEXT NOT NULL,
    goal_name  TEXT NOT NULL,
    nav        REAL NOT NULL,  -- CLP
    deposited  REAL,           -- net of withdrawals; NULL if Fintual omits it
    profit     REAL,           -- unrealized gain, as Fintual shows it
    raw_json   TEXT NOT NULL,  -- full Fintual response, to keep unmapped fields
    fetched_at TEXT NOT NULL,  -- RFC3339 UTC
    PRIMARY KEY (date, goal_id)
);

-- Fintual's daily balance series (the web chart), kept apart from snapshots.
CREATE TABLE IF NOT EXISTS balance_history (
    goal_id         TEXT NOT NULL,
    date            TEXT NOT NULL,  -- YYYY-MM-DD
    valuation       REAL NOT NULL,
    cost_basis      REAL NOT NULL,  -- cost of the shares currently held
    unrealized_gain REAL NOT NULL,
    realized_gain   REAL NOT NULL,
    withdrawn       REAL NOT NULL,
    raw_json        TEXT NOT NULL,
    fetched_at      TEXT NOT NULL,  -- RFC3339 UTC
    PRIMARY KEY (goal_id, date)
);
