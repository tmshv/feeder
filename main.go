package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gilliek/go-opml/opml"
	"github.com/gofiber/fiber/v2"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tmshv/feeder/internal"
	"github.com/tmshv/feeder/store"
	"github.com/tmshv/feeder/utils"

	"github.com/cixtor/readability"
	"github.com/gosimple/slug"
	"github.com/mmcdole/gofeed"
	jsonfeed "github.com/mmcdole/gofeed/json"

	md "github.com/JohannesKaufmann/html-to-markdown"
)

func fetchFeedRecords(feed *internal.Feed) ([]internal.Record, error) {
	parser := gofeed.NewParser()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	f, err := parser.ParseURLWithContext(feed.Url, ctx)
	if err != nil {
		return nil, err
	}

	log.Printf("Fetch %s", feed.Url)

	result := make([]internal.Record, 0)
	for _, item := range f.Items {
		var rec internal.Record
		rec.ID = uuid.NewString()
		rec.FeedID = feed.ID
		rec.Title = item.Title
		rec.Description = item.Description
		rec.Content = item.Content
		rec.PublishedAt = *item.PublishedParsed
		rec.Link = utils.DropUtmMarkers(item.Link)

		result = append(result, rec)
	}
	return result, nil
}

func runFeed(db store.Store, feed internal.Feed, news chan string) {
	log.Printf("Run feed %s (%s)", feed.Slug, feed.Slug)
	for {
		records, err := fetchFeedRecords(&feed)
		if err != nil {
			log.Printf("Failed for fetch feed %s", feed.Url)
		}

		count := 0
		for _, rec := range records {
			added, err := db.AddRecord(rec)
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

func handleRecords(db store.Store, news chan string) error {
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

func handlePage(db store.Store, url string) error {
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

	md, err := htmlToMd(a.Content)
	if err != nil {
		log.Printf("Cannot create markdown of %s, %v", url, err)
	}

	err = db.AddPage(url, htmlStr, md)
	if err != nil {
		log.Printf("Failed to add Page %s", url)
		return err
	}

	return nil
}

func handleOldRecords(db store.Store, news chan string) error {
	urls, err := db.FindRecordsWithNoPage()
	if err != nil {
		log.Printf("Failed to find urls: %v", err)
		return err
	}

	for _, url := range urls {
		news <- url
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

func importOpml(db store.Store, filePath string) error {
	doc, err := opml.NewOPMLFromFile(filePath)
	if err != nil {
		log.Fatal(err)
	}

	for _, o := range doc.Outlines() {
		for _, item := range o.Outlines {
			if item.XMLURL == "" {
				continue
			}

			feed, err := db.FindFeedByUrl(item.XMLURL)
			if err != nil {
				slug := slugify(item.Title)
				log.Printf("Add %s", item.XMLURL)
				err = db.AddFeed(slug, item.XMLURL)
				continue
			}

			log.Printf("Skip imporing %s", feed.Slug)
		}
	}

	return nil
}

func htmlToMd(html string) (string, error) {
	converter := md.NewConverter("", true, nil)
	return converter.ConvertString(html)
}

func allToMd(db store.Store) {
	r := readability.New()
	pages, err := db.GetAllPages()
	if err != nil {
		log.Printf("Failed to all to md: %v", err)
	}

	for _, page := range pages {
		html := strings.NewReader(page.Html)
		a, err := r.Parse(html, page.Url)
		if err != nil {
			log.Printf("Readability failed to parse %s: %v", page.Url, err)
			continue
		}

		md, err := htmlToMd(a.Content)
		if err != nil {
			log.Printf("Markdown failed to convert from html %s: %v", page.Url, err)
			continue
		}

		err = db.UpdatePageContent(&page, md)
		if err != nil {
			log.Printf("Failed to update content of page %s: %v", page.Url, err)
			continue
		}
	}
}

func serve(db store.Store) {
	app := fiber.New()

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	app.Get("/feed/:slug", func(c *fiber.Ctx) error {
		slug := c.Params("slug")
		feed, err := db.GetFeedBySlug(slug)
		if err != nil {
			return c.Status(404).JSON(&fiber.Map{
				"error": "Feed not found",
			})
		}

		records, err := db.GetFeedRecords(feed.ID, true)
		if err != nil {
			return c.Status(404).JSON(&fiber.Map{
				"error": "Records not found",
			})
		}

		items := make([]*jsonfeed.Item, 0)
		for _, rec := range records {
			item := jsonfeed.Item{
				ID:  rec.ID,
				URL: rec.Link,
				Title: rec.Title,
				// ContentHTML   string  `json:"content_html,omitempty"`   // content_html and content_text are each optional strings — but one or both must be present. This is the HTML or plain text of the item. Important: the only place HTML is allowed in this format is in content_html. A Twitter-like service might use content_text, while a blog might use content_html. Use whichever makes sense for your resource. (It doesn’t even have to be the same for each item in a feed.)
				ContentText: rec.Content,
				Summary:     rec.Description,
				// Image         string  `json:"image,omitempty"`          // image (optional, string) is the URL of the main image for the item. This image may also appear in the content_html
				// BannerImage   string  `json:"banner_image,omitempty"`   // banner_image (optional, string) is the URL of an image to use as a banner.
				DatePublished: rec.PublishedAt.String(),
				// DateModified  string  `json:"date_modified,omitempty"`  // date_modified (optional, string) specifies the modification date in RFC 3339 format.
				// Author        *Author `json:"author,omitempty"`         // author (optional, object) has the same structure as the top-level author. If not specified in an item, then the top-level author, if present, is the author of the item.

				Tags: []string{"good", "trash", "travel"},
				// Attachments *[]Attachments `json:"attachments,omitempty"` // attachments (optional, array) lists related resources. Podcasts, for instance, would include an attachment that’s an audio or video file. An individual item may have one or more attachments.
				// Authors  []*Author `json:"authors,omitempty"`
				// Language string    `json:"language,omitempty"`
			}
			items = append(items, &item)
		}

		url := fmt.Sprintf("http://127.0.0.1:3000/feed/%s", slug)
		f := jsonfeed.Feed{
			Version: "https://jsonfeed.org/version/1.1",
			Title: feed.Slug,
			// HomePageURL string  `json:"home_page_url,omitempty"` // home_page_url (optional but strongly recommended, string) is the URL of the resource that the feed describes. This resource should be an HTML page
			FeedURL: url,
			// Description string  `json:"description,omitempty"`   // description (optional, string)
			// UserComment string  `json:"user_comment,omitempty"`  // user_comment (optional, string) is a description of the purpose of the feed. This is for the use of people looking at the raw JSON, and should be ignored by feed readers.
			// NextURL     string  `json:"next_url,omitempty"`      // next_url (optional, string) is the URL of a feed that provides the next n items. This allows for pagination
			// Icon        string  `json:"icon,omitempty"`          // icon (optional, string) is the URL of an image for the feed suitable to be used in a timeline. It should be square and relatively large — such as 512 x 512
			// Favicon     string  `json:"favicon,omitempty"`       // favicon (optional, string) is the URL of an image for the feed suitable to be used in a source list. It should be square and relatively small, but not smaller than 64 x 64
			// Author      *Author `json:"author,omitempty"`        // author (optional, object) specifies the feed author. The author object has several members. These are all optional — but if you provide an author object, then at least one is required:
			// Expired     bool    `json:"expired,omitempty"`       // expired (optional, boolean) says whether or not the feed is finished — that is, whether or not it will ever update again.
			Items: items,
			// Authors  []*Author `json:"authors,omitempty"`
			// Language string    `json:"language,omitempty"`
		}
		return c.JSON(f)
	})

	log.Print("Listening :3000")
	err := app.Listen(":3000")
	log.Fatal(err)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

	DATABASE_URI := "feed.db"

    db, err := store.NewSqliteStore(DATABASE_URI, logger)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// err = importOpml(db, "20230426-reeder.opml")
	// err = addFeed(db, "hacker-news", feedUrl)

	go serve(db)

	feeds, err := db.GetFeeds()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Found %d feeds", len(feeds))

	// Create a channel to communicate with the goroutine.
	done := make(chan bool)
	news := make(chan string, 1000)

	for i := 0; i < 3; i++ {
		go handleRecords(db, news)
	}
	go handleOldRecords(db, news)
	// go allToMd(db)

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
