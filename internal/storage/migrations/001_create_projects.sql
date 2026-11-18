CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    root_path TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,

    CHECK(length(trim(name)) > 0),
    CHECK(length(trim(root_path)) > 0)
);