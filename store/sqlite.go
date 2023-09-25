package store

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tmshv/feeder/internal"
)

type SqliteStore struct {
	logger *log.Logger
	db     *sql.DB
}

func (s *SqliteStore) Close() error {
    return s.db.Close()
}

func (s *SqliteStore) setup(migrationsPath string) error {
	driver, err := sqlite3.WithInstance(s.db, &sqlite3.Config{
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
			s.logger.Print("Nothing to migrate")
			return nil
		}
		return err
	}

	s.logger.Print("Successfully migrated to the latest version")
	return nil
}

func (s *SqliteStore) AddFeed(slug string, url string) error {
	stmt, err := s.db.Prepare(`
        INSERT INTO
        feeds(id, slug, url, created_at, updated_at)
        VALUES
        (?, ?, ?, ?, ?)
    `)
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

func (s *SqliteStore) AddPage(url string, html string, content string) error {
	stmt, err := s.db.Prepare(`
        INSERT INTO
        pages(url, created_at, html, content)
        VALUES
        (?, ?, ?, ?)
    `)
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

func (s *SqliteStore) UpdatePageContent(page *internal.Page, content string) error {
	stmt, err := s.db.Prepare(`
        UPDATE pages
        SET content = ?
        WHERE url = ? AND created_at = ?
    `)
	if err != nil {
		return err
	}
	res, err := stmt.Exec(content, page.Url, page.CreatedAt)
	if err != nil {
		return err
	}

	x, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if x == 0 {
		log.Printf("Content of Page %s is not updated", page.Url)
	}

	return nil
}

func (s *SqliteStore) GetFeedBySlug(slug string) (internal.Feed, error) {
	var feed internal.Feed
	row := s.db.QueryRow(`
        SELECT id, slug, url, created_at, updated_at, refresh_ms
        FROM feeds
        WHERE slug = ?
        LIMIT 1
        ;
    `, slug)
	err := row.Scan(
		&feed.ID,
		&feed.Slug,
		&feed.Url,
		&feed.CreatedAt,
		&feed.UpdatedAt,
		&feed.RefreshMs,
	)
	if err != nil {
		return internal.Feed{}, err
	}
	return feed, nil
}

func (s *SqliteStore) FindFeedByUrl(feedUrl string) (internal.Feed, error) {
	var feed internal.Feed
	row := s.db.QueryRow(`
        SELECT id, slug, url, created_at, updated_at, refresh_ms
        FROM feeds
        WHERE url = ?
        LIMIT 1
        ;
    `, feedUrl)
	err := row.Scan(
		&feed.ID,
		&feed.Slug,
		&feed.Url,
		&feed.CreatedAt,
		&feed.UpdatedAt,
		&feed.RefreshMs,
	)
	if err != nil {
		return internal.Feed{}, err
	}
	return feed, nil
}

func (s *SqliteStore) GetFeeds() ([]internal.Feed, error) {
	result := make([]internal.Feed, 0)

	rows, err := s.db.Query(`
        SELECT
        id, slug, url, created_at, updated_at, refresh_ms
        FROM feeds
        ;
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		feed := internal.Feed{}
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

func (s *SqliteStore) AddRecord(item internal.Record) (int64, error) {
	stmt, err := s.db.Prepare(`
        INSERT OR IGNORE INTO
        records(id, feed_id, title, description, content, published_at, link)
        VALUES
        (?, ?, ?, ?, ?, ?, ?)
    `)
	if err != nil {
		return 0, err
	}

	res, err := stmt.Exec(item.ID, item.FeedID, item.Title, item.Description, item.Content, item.PublishedAt, item.Link)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SqliteStore) FindRecordsWithNoPage() ([]string, error) {
	result := make([]string, 0)
	rows, err := s.db.Query(`
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

func (s *SqliteStore) GetAllPages() ([]internal.Page, error) {
	result := make([]internal.Page, 0)
	rows, err := s.db.Query(`
        SELECT
            url,
            html,
            content,
            created_at
        FROM pages
        ;
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var page internal.Page
		err := rows.Scan(
			&page.Url,
			&page.Html,
			&page.Content,
			&page.CreatedAt,
		)
		if err != nil {
			log.Printf("Failed to get row: %v", err)
			continue
		}
		result = append(result, page)
	}

	return result, nil
}

func (s *SqliteStore) GetFeedRecords(feedId string, mdContent bool) ([]internal.Record, error) {
	log.Print("[WARN] mdContent is not implemented")
	result := make([]internal.Record, 0)
	rows, err := s.db.Query(`
        SELECT
            r.id,
            r.title,
            r.description,
            p.content,
            r.published_at,
            r.link
        FROM records r
        JOIN pages p
        ON p.url = r.link
        WHERE r.feed_id = ?
        ;
    `, feedId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var rec internal.Record
		err := rows.Scan(
			&rec.ID,
			&rec.Title,
			&rec.Description,
			&rec.Content,
			&rec.PublishedAt,
			&rec.Link,
		)
		if err != nil {
			log.Printf("Failed to get row: %v", err)
			continue
		}
		result = append(result, rec)
	}

	return result, nil
}

func NewSqliteStore(dbpath string, logger *log.Logger) (*SqliteStore, error) {
	// Connect to the SQLite database.
	db, err := sql.Open("sqlite3", dbpath)
	if err != nil {
		return nil, err
	}

	store := SqliteStore{
		db: db,
        logger: logger,
	}

	err = store.setup("migrations")
	if err != nil {
		log.Fatal(err)
	}

	return &store, nil
}
