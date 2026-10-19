CREATE TABLE history (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 at_ns INTEGER NOT NULL,
 kind TEXT NOT NULL,
 device_id TEXT NOT NULL,
 details TEXT NOT NULL CHECK(json_valid(details))
);
