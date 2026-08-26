DROP TABLE programs;

CREATE TABLE slots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    media_item_id INTEGER REFERENCES media_items(id) ON DELETE CASCADE,
    gap_duration_sec REAL,
    gap_label TEXT NOT NULL DEFAULT '',
    recurring INTEGER NOT NULL DEFAULT 1,
    day_of_week INTEGER,
    position INTEGER,
    start_time TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_slots_channel_id ON slots(channel_id);
CREATE INDEX idx_slots_channel_recurring_day ON slots(channel_id, day_of_week) WHERE recurring = 1;
