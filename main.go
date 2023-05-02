package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gilliek/go-opml/opml"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/cixtor/readability"
	"github.com/gosimple/slug"
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

type Page struct {
	Url       string    `json:"url" db:"url"`
	Html      string    `json:"html" db:"html"`
	Content   string    `json:"content" db:"content"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
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
	err = m.Up()
	if err != nil {
		if err == migrate.ErrNoChange {
			log.Print("Nothing to migrate")
			return nil
		}
		return err
	}

	log.Print("Successfully migrated to the latest version")
	return nil
}

func DropUtmMarkers(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr // return original URL in case of error
	}

	queryParams := u.Query()
	for key := range queryParams {
		if strings.HasPrefix(key, "utm_") {
			delete(queryParams, key)
		}
	}

	u.RawQuery = queryParams.Encode()

	return u.String()
}

func fetchFeedRecords(feed *Feed) ([]Record, error) {
	parser := gofeed.NewParser()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	f, err := parser.ParseURLWithContext(feed.Url, ctx)
	if err != nil {
		return nil, err
	}

	log.Printf("Fetch %s", feed.Url)

	result := make([]Record, 0)
	for _, item := range f.Items {
		var rec Record
		rec.ID = uuid.NewString()
		rec.FeedID = feed.ID
		rec.Title = item.Title
		rec.Description = item.Description
		rec.Content = item.Content
		rec.PublishedAt = *item.PublishedParsed
		rec.Link = DropUtmMarkers(item.Link)

		result = append(result, rec)
	}
	return result, nil
}

func runFeed(db *sql.DB, feed Feed, news chan string) {
	log.Printf("Run feed %s (%s)", feed.Slug, feed.Slug)
	for {
		records, err := fetchFeedRecords(&feed)
		if err != nil {
			log.Printf("Failed for fetch feed %s", feed.Url)
		}

		count := 0
		for _, rec := range records {
			added, err := addRecord(db, rec)
			if err != nil {
				log.Printf("Failed add record %s", rec.Link)
				continue
			}
			if added > 0 {
				count += 1
				news <- rec.Link
			}
		}

		log.Printf("Found %d new records (%d total) in feed %s. Falling to sleep %d ms", count, len(records), feed.Slug, feed.RefreshMs)
		time.Sleep(time.Duration(feed.RefreshMs) * time.Millisecond)
	}
}

func handleRecords(db *sql.DB, news chan string) error {
	log.Println("Wait for news to readability")

	for {
		if url, ok := <-news; ok {
            err := handlePage(db, url)
			if err != nil {
				log.Printf("Failed to get content of %s", url)
				continue
			}
            log.Printf("Added content of %s", url)
		} else {
			log.Println("Chan closed")
			return nil
		}
	}
}

func handlePage(db *sql.DB, url string) error {
	r := readability.New()
	res, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to get content of %s", url)
		return err
	}
	if res.StatusCode != 200 {
		log.Printf("Got not OK for %s", url)
		return err
	}

	bodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Failed to read body bytes of %s: %v", url, err)
		return err
	}

	bodyBuf := bytes.NewBuffer(bodyBytes)
	a, err := r.Parse(bodyBuf, url)
	if err != nil {
		log.Printf("Failed to readability parse %s: %v", url, err)
		return err
	}

	htmlStr := string(bodyBytes)
	if err != nil {
		log.Printf("%v", err)
		return err
	}

	err = addPage(db, url, htmlStr, a.Content)
	if err != nil {
		log.Printf("Failed to add Page %s", url)
		return err
	}

	return nil
}

func handleOldRecords(db *sql.DB, news chan string) error {
	urls, err := findRecordsWithNoPage(db)
	if err != nil {
		log.Printf("Failed to find urls: %v", err)
		return err
	}

	for _, url := range urls {
        news <- url
	}
    return nil
}

func addFeed(db *sql.DB, slug string, url string) error { stmt, err := db.Prepare("INSERT INTO feeds(id, slug, url, created_at, updated_at) VALUES(?, ?, ?, ?, ?)")
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

func addPage(db *sql.DB, url string, html string, content string) error {
	stmt, err := db.Prepare("INSERT INTO pages(url, created_at, html, content) VALUES(?, ?, ?, ?)")
	if err != nil {
		return err
	}

	now := time.Now()
	_, err = stmt.Exec(url, now, html, content)
	if err != nil {
		return err
	}
	return nil
}

func findFeedByUrl(db *sql.DB, feedUrl string) (Feed, error) {
	var feed Feed
	row := db.QueryRow("SELECT id, slug, url, created_at, updated_at, refresh_ms FROM feeds WHERE url = ? LIMIT 1;", feedUrl)
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

func getFeeds(db *sql.DB) ([]Feed, error) {
	result := make([]Feed, 0)

	rows, err := db.Query("SELECT id, slug, url, created_at, updated_at, refresh_ms FROM feeds;")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		feed := Feed{}
		err := rows.Scan(
			&feed.ID,
			&feed.Slug,
			&feed.Url,
			&feed.CreatedAt,
			&feed.UpdatedAt,
			&feed.RefreshMs,
		)
		if err != nil {
			log.Println("Failed to get row")
			continue
		}

		// TODO do someting with it
		feed.RefreshMs += int64(rand.Intn(10000))

		result = append(result, feed)
	}

	return result, nil
}

func addRecord(db *sql.DB, item Record) (int64, error) {
	stmt, err := db.Prepare("INSERT OR IGNORE INTO records(id, feed_id, title, description, content, published_at, link) VALUES(?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}

	res, err := stmt.Exec(item.ID, item.FeedID, item.Title, item.Description, item.Content, item.PublishedAt, item.Link)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func findRecordsWithNoPage(db *sql.DB) ([]string, error) {
	result := make([]string, 0)
	rows, err := db.Query(`
        SELECT records.link
        FROM records
        LEFT JOIN pages ON records.link = pages.url
        WHERE pages.url IS NULL;
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var url string
		err := rows.Scan(&url)
		if err != nil {
            log.Printf("Failed to get row: %v", err)
			continue
		}
		result = append(result, url)
	}

	return result, nil
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

func slugify(value string) string {
	// text := slug.Make("Hellö Wörld хелло ворлд")
	// fmt.Println(text) // Will print: "hello-world-khello-vorld"
	//
	// someText := slug.Make("影師")
	// fmt.Println(someText) // Will print: "ying-shi"
	//
	// enText := slug.MakeLang("This & that", "en")
	// fmt.Println(enText) // Will print: "this-and-that"
	//
	// deText := slug.MakeLang("Diese & Dass", "de")
	// fmt.Println(deText) // Will print: "diese-und-dass"
	//
	// slug.Lowercase = false // Keep uppercase characters
	// deUppercaseText := slug.MakeLang("Diese & Dass", "de")
	// fmt.Println(deUppercaseText) // Will print: "Diese-und-Dass"
	//
	// slug.CustomSub = map[string]string{
	// 	"water": "sand",
	// }
	// textSub := slug.Make("water is hot")
	// fmt.Println(textSub) // Will print: "sand-is-hot"

	return slug.MakeLang(value, "en")
}

func importOpml(db *sql.DB, filePath string) error {
	doc, err := opml.NewOPMLFromFile(filePath)
	if err != nil {
		log.Fatal(err)
	}

	for _, o := range doc.Outlines() {
		for _, item := range o.Outlines {
			if item.XMLURL == "" {
				continue
			}

			feed, err := findFeedByUrl(db, item.XMLURL)
			if err != nil {
				slug := slugify(item.Title)
				log.Printf("Add %s", item.XMLURL)
				err = addFeed(db, slug, item.XMLURL)
				continue
			}

			log.Printf("Skip imporing %s", feed.Slug)
		}
	}

	return nil
}

func main() {
	rand.Seed(time.Now().UnixNano())

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

	// err = importOpml(db, "20230426-reeder.opml")
	// err = addFeed(db, "hacker-news", feedUrl)

	feeds, err := getFeeds(db)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Found %d feeds", len(feeds))

	// Create a channel to communicate with the goroutine.
	done := make(chan bool)
	news := make(chan string, 10000)

	for i := 1; i < 3; i++ {
		go handleRecords(db, news)
	}
	go handleOldRecords(db, news)

	for _, feed := range feeds {
		go runFeed(db, feed, news)
	}

	// Wait for an interrupt signal (SIGINT or SIGTERM).
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Close the channel to signal the goroutine to exit.
	fmt.Println("closing channel...")
	close(news)
	close(done)
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
