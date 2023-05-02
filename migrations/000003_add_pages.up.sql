CREATE TABLE IF NOT EXISTS pages (
    created_at DATETIME NOT NULL,
    url TEXT NOT NULL,
    html TEXT NOT NULL,
    content TEXT,

    UNIQUE(url, created_at)
);

