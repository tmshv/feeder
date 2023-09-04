package store

import (
	. "github.com/tmshv/feedflow/internal"
)

type Store interface {
	AddFeed(string, string) error
	AddPage(string, string, string) error
	UpdatePageContent(*Page, string) error
	GetFeedBySlug(string) (Feed, error)
	FindFeedByUrl(string) (Feed, error)
	GetFeeds() ([]Feed, error)
	AddRecord(Record) (int64, error)
	FindRecordsWithNoPage() ([]string, error)
	GetAllPages() ([]Page, error)
	GetFeedRecords(string, bool) ([]Record, error)
}
