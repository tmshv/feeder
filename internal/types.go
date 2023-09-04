package internal

import "time"

type Feed struct {
	ID        string    `json:"id" db:"id"`
	Slug      string    `json:"slug" db:"slug"`
	Url       string    `json:"url" db:"url"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
	RefreshMs int64     `json:"refreshMs" db:"refresh_ms"`
}

type Record struct {
	ID          string    `json:"id" db:"id"`
	FeedID      string    `json:"feed_id" db:"feed_id"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description" db:"description"`
	Content     string    `json:"content" db:"content"`
	PublishedAt time.Time `json:"published_at" db:"published_at"`
	Link        string    `json:"link" db:"link"`
}

type Page struct {
	Url       string    `json:"url" db:"url"`
	Html      string    `json:"html" db:"html"`
	Content   string    `json:"content" db:"content"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

