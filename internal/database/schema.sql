CREATE TABLE pending (
 device_id TEXT PRIMARY KEY,
 attempts INTEGER NOT NULL CHECK(attempts >= 0),
 last_attempt_ns INTEGER NOT NULL,
 time_left_ns INTEGER NOT NULL CHECK(time_left_ns >= 0),
 grace_left_ns INTEGER NOT NULL CHECK(grace_left_ns >= 0)
);
