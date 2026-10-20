CREATE TABLE pending (
 device_id TEXT PRIMARY KEY,
 attempts INTEGER NOT NULL CHECK(attempts >= 0),
 last_attempt_ns INTEGER NOT NULL,
 time_left_ns INTEGER NOT NULL CHECK(time_left_ns >= 0)
);
CREATE TABLE history (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 at_ns INTEGER NOT NULL,
 kind TEXT NOT NULL,
 device_id TEXT NOT NULL,
 details TEXT NOT NULL CHECK(json_valid(details))
);
