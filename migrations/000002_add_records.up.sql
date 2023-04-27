CREATE TABLE IF NOT EXISTS records (
    id TEXT PRIMARY KEY,
    feed_id TEXT,
    title TEXT,
    description TEXT,
    content TEXT,

    published_at DATETIME NOT NULL,
    link TEXT NOT NULL,

    UNIQUE(link, published_at),
    FOREIGN KEY (feed_id) REFERENCES feeds(feed_id)
);

