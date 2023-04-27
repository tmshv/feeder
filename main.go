package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/cixtor/readability"
	"github.com/mmcdole/gofeed"
)

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

func setupDb(migrationsPath string, db *sql.DB) error {
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{
		MigrationsTable: "migrations",
	})
	if err != nil {
		return err
	}

	sourceUrl := fmt.Sprintf("file://%s", migrationsPath)
	m, err := migrate.NewWithDatabaseInstance(sourceUrl, "sqlite3", driver)
	if err != nil {
		return err
	}
	m.Up()
	return nil
}

func runFeed(db *sql.DB, feedr *Feed) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	fp := gofeed.NewParser()
	feed, _ := fp.ParseURLWithContext(feedr.Url, ctx)
	fmt.Println(feed.Title)
	fmt.Println(feed.Description)

	r := readability.New()

	for _, item := range feed.Items {
		var rec Record
		rec.ID = uuid.NewString()
		rec.FeedID = feedr.ID
		rec.Title = item.Title
		rec.Description = item.Description
		rec.Content = item.Content
		rec.PublishedAt = *item.PublishedParsed
		rec.Link = item.Link

        err := addRecord(db, rec)
        if err != nil {
            log.Fatalf("Failed add record %s", item.Link)
        }
		fmt.Printf("Got Record %v\n\n", rec)

		// fmt.Printf("Title: %s\n", item.Title)
		// fmt.Printf("Date: %s\n", item.PublishedParsed)
		// fmt.Printf("Link: %s\n", item.Link)

		res, err := http.Get(item.Link)
		if err != nil {
            log.Printf("Failed to get content of %s", item.Link)
            continue
		}
		if res.StatusCode != 200 {
            log.Printf("Got not OK for %s", item.Link)
            continue
		}

		a, err := r.Parse(res.Body, item.Link)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Parserd content has %d len\n\n", len(a.Content))
	}

	return nil
}

func addFeed(db *sql.DB, slug string, url string) error {
	stmt, err := db.Prepare("INSERT INTO feeds(id, slug, url, created_at, updated_at) VALUES(?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}

	id := uuid.NewString()
	now := time.Now()
	_, err = stmt.Exec(id, slug, url, now, now)
	if err != nil {
		return err
	}
	return nil
}

func findFeedByUrl(db *sql.DB, feedUrl string) (Feed, error) {
	var feed Feed
	row := db.QueryRow("SELECT id, slug, url, created_at, updated_at, refresh_ms feed FROM feeds WHERE url = ? LIMIT 1;", feedUrl)
	err := row.Scan(
		&feed.ID,
		&feed.Slug,
		&feed.Url,
		&feed.CreatedAt,
		&feed.UpdatedAt,
		&feed.RefreshMs,
	)
	if err != nil {
		return Feed{}, err
	}
	return feed, nil
}

func addRecord(db *sql.DB, item Record) error {
	stmt, err := db.Prepare("INSERT OR IGNORE INTO records(id, feed_id, title, description, content, published_at, link) VALUES(?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}

	_, err = stmt.Exec(item.ID, item.FeedID, item.Title, item.Description, item.Content, item.PublishedAt, item.Link)
	if err != nil {
		return err
	}
	return nil
}

func createJsonFeed() {
	// Select record
	// var selectedRecord Record
	// row := db.QueryRow("SELECT id, feed_id, title, description, content, published_at, link FROM records WHERE id = ?", "123")
	// err = row.Scan(&selectedRecord.ID, &selectedRecord.FeedID, &selectedRecord.Title, &selectedRecord.Description, &selectedRecord.Content, &selectedRecord.PublishedAt, &selectedRecord.Link)
	//
	//	if err != nil {
	//	    panic(err)
	//	}
	//
	// fmt.Println(selectedRecord)
}

func main() {
	// take rss
	// load items
	// load each items raw html
	// put to items table
	// put read mode content to table

	DATABASE_URI := "feed.db"

	// Connect to the SQLite database.
	db, err := sql.Open("sqlite3", DATABASE_URI)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = setupDb("migrations", db)
	if err != nil {
		log.Fatal(err)
	}

	// err = addFeed(db, "hacker-news", feedUrl)

	var feedUrl string
	err = db.QueryRow("SELECT url FROM feeds LIMIT 1;").Scan(&feedUrl)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(feedUrl)

	feed, err := findFeedByUrl(db, feedUrl)
	if err != nil {
		log.Fatal(err)
	}

    runFeed(db, &feed)
}

// func initPragmas(db *dbx.DB) error {
// 	// note: the busy_timeout pragma must be first because
// 	// the connection needs to be set to block on busy before WAL mode
// 	// is set in case it hasn't been already set by another connection
// 	_, err := db.NewQuery(`
// 		PRAGMA busy_timeout       = 10000;
// 		PRAGMA journal_mode       = WAL;
// 		PRAGMA journal_size_limit = 200000000;
// 		PRAGMA synchronous        = NORMAL;
// 		PRAGMA foreign_keys       = TRUE;
// 	`).Execute()

// 	return err
// }
