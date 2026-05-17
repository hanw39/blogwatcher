package model

import "time"

type Blog struct {
	ID             int64
	Name           string
	URL            string
	FeedURL        string
	ScrapeSelector string
	LastScanned    *time.Time
	CategoryID     *int64
	IsEphemeral    bool
}

type Article struct {
	ID             int64
	BlogID         int64
	Title          string
	URL            string
	PublishedDate  *time.Time
	DiscoveredDate *time.Time
	IsRead         bool // legacy — kept until Task 9 removes it
	ReadAt         *time.Time
}

type Category struct {
	ID        int64
	Name      string
	BlogCount int
}

// ArticleIsRead returns whether the article should be displayed as read,
// given the owning blog's ephemeral flag and the current time.
//
//   - Regular blog: read iff ReadAt is non-nil.
//   - Ephemeral blog: read iff ReadAt is non-nil AND falls on the same local day as now.
func ArticleIsRead(a Article, blogIsEphemeral bool, now time.Time) bool {
	if a.ReadAt == nil {
		return false
	}
	if !blogIsEphemeral {
		return true
	}
	return sameLocalDay(*a.ReadAt, now)
}

func sameLocalDay(a, b time.Time) bool {
	al, bl := a.Local(), b.Local()
	return al.Year() == bl.Year() && al.YearDay() == bl.YearDay()
}
