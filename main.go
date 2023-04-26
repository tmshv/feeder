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

func runFeed(feedUrl string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	fp := gofeed.NewParser()
	feed, _ := fp.ParseURLWithContext(feedUrl, ctx)
	fmt.Println(feed.Title)
	fmt.Println(feed.Description)

	r := readability.New()

	for _, item := range feed.Items {
		fmt.Printf("Title: %s\n", item.Title)
		fmt.Printf("Date: %s\n", item.PublishedParsed)
		fmt.Printf("Link: %s\n", item.Link)

		res, err := http.Get(item.Link)
		if err != nil {
			panic(err)
		}
		if res.StatusCode != 200 {
			panic("Not OK")
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

	runFeed(feedUrl)
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
