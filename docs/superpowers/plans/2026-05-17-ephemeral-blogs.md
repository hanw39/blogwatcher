# Ephemeral Blogs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Support per-blog "ephemeral" semantics where articles marked read return to unread on the next local day, while keeping existing blogs untouched.

**Architecture:** Add `blogs.is_ephemeral` (bool) and `articles.read_at` (timestamp). Effective read-status is computed dynamically from `read_at` + blog's `is_ephemeral` (never stored as a bool). Scanner "lifts" `discovered_date` for re-encountered ephemeral articles at most once per local day. The legacy `is_read` column is kept untouched as a rollback safety net.

**Tech Stack:** Go 1.24+, SQLite (modernc.org/sqlite), cobra

**Migration strategy to keep code compiling:** `Article.IsRead` stays until the cleanup task. Storage layer writes both `is_read` and `read_at` during the transition; callers gradually migrate to `ArticleIsRead(article, blogIsEphemeral, now)`. The final task removes `Article.IsRead` and the double-write.

---

### Task 1: Model — add IsEphemeral, ReadAt, and ArticleIsRead helper

**Files:**
- Modify: `internal/model/model.go`
- Create: `internal/model/model_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/model/model_test.go`:

```go
package model

import (
	"testing"
	"time"
)

func TestArticleIsRead(t *testing.T) {
	loc := time.Local
	today := time.Date(2026, 5, 17, 10, 0, 0, 0, loc)
	yesterday := time.Date(2026, 5, 16, 23, 30, 0, 0, loc)
	todayMidnight := time.Date(2026, 5, 17, 0, 0, 0, 0, loc)

	tests := []struct {
		name      string
		readAt    *time.Time
		ephemeral bool
		now       time.Time
		want      bool
	}{
		{"nil read_at regular", nil, false, today, false},
		{"nil read_at ephemeral", nil, true, today, false},
		{"set read_at regular", &yesterday, false, today, true},
		{"ephemeral read today", &today, true, today, true},
		{"ephemeral read yesterday", &yesterday, true, today, false},
		{"ephemeral read at today midnight", &todayMidnight, true, today, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ArticleIsRead(Article{ReadAt: tc.readAt}, tc.ephemeral, tc.now)
			if got != tc.want {
				t.Fatalf("ArticleIsRead(readAt=%v, ephemeral=%v, now=%v) = %v, want %v",
					tc.readAt, tc.ephemeral, tc.now, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/...`
Expected: FAIL — `undefined: ArticleIsRead`, `unknown field 'ReadAt' in struct literal`.

- [ ] **Step 3: Update model and add helper**

Replace `internal/model/model.go` with:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/model.go internal/model/model_test.go
git commit -m "feat(model): add IsEphemeral/ReadAt fields and ArticleIsRead helper"
```

---

### Task 2: Storage — schema migration

**Files:**
- Modify: `internal/storage/database.go` (the `init` method)
- Modify: `internal/storage/database_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/storage/database_test.go`:

```go
func TestMigrationBackfillsReadAt(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	blog, err := db.AddBlog(model.Blog{Name: "Test", URL: "https://example.com"})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	// Insert an article and mark it read using the LEGACY path, then verify migration backfills read_at.
	discoveredAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	article, err := db.AddArticle(model.Article{
		BlogID:         blog.ID,
		Title:          "Old",
		URL:            "https://example.com/old",
		DiscoveredDate: &discoveredAt,
	})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	// Simulate legacy state: is_read=1, read_at=NULL.
	if _, err := db.conn.Exec(`UPDATE articles SET is_read = 1, read_at = NULL WHERE id = ?`, article.ID); err != nil {
		t.Fatalf("seed legacy state: %v", err)
	}

	// Re-run init to exercise the migration step.
	if err := db.init(); err != nil {
		t.Fatalf("re-init: %v", err)
	}

	row := db.conn.QueryRow(`SELECT read_at FROM articles WHERE id = ?`, article.ID)
	var readAt sql.NullString
	if err := row.Scan(&readAt); err != nil {
		t.Fatalf("scan read_at: %v", err)
	}
	if !readAt.Valid {
		t.Fatalf("expected read_at to be backfilled, got NULL")
	}
}
```

Add `"database/sql"` to the test file's imports if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/ -run TestMigrationBackfillsReadAt -v`
Expected: FAIL — `no such column: read_at`.

- [ ] **Step 3: Add migration statements**

In `internal/storage/database.go`, find the `init()` method. After the existing `category_id` migration, add the new migrations. The full updated `init` becomes:

```go
func (db *Database) init() error {
	schema := `
		CREATE TABLE IF NOT EXISTS categories (
			id   INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE
		);
		CREATE TABLE IF NOT EXISTS blogs (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			feed_url TEXT,
			scrape_selector TEXT,
			last_scanned TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS articles (
			id INTEGER PRIMARY KEY,
			blog_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			published_date TIMESTAMP,
			discovered_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			is_read BOOLEAN DEFAULT FALSE,
			FOREIGN KEY (blog_id) REFERENCES blogs(id)
		);
	`
	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}

	// Idempotent column additions. SQLite errors with "duplicate column name" if already present.
	migrations := []string{
		`ALTER TABLE blogs    ADD COLUMN category_id   INTEGER REFERENCES categories(id)`,
		`ALTER TABLE blogs    ADD COLUMN is_ephemeral  BOOLEAN DEFAULT 0`,
		`ALTER TABLE articles ADD COLUMN read_at       TIMESTAMP`,
	}
	for _, stmt := range migrations {
		if _, err := db.conn.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}

	// Backfill read_at from legacy is_read once. Safe to re-run: only touches rows where read_at is still NULL.
	if _, err := db.conn.Exec(
		`UPDATE articles SET read_at = COALESCE(discovered_date, CURRENT_TIMESTAMP) WHERE is_read = 1 AND read_at IS NULL`,
	); err != nil {
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run all storage tests**

Run: `go test ./internal/storage/...`
Expected: PASS (new test plus all existing tests).

- [ ] **Step 5: Commit**

```bash
git add internal/storage/database.go internal/storage/database_test.go
git commit -m "feat(storage): add is_ephemeral and read_at columns with backfill migration"
```

---

### Task 3: Storage — read/write `is_ephemeral` and `read_at`

**Files:**
- Modify: `internal/storage/database.go`
- Modify: `internal/storage/database_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/storage/database_test.go`:

```go
func TestBlogIsEphemeralRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	blog, err := db.AddBlog(model.Blog{Name: "Trending", URL: "https://example.com/trending", IsEphemeral: true})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	fetched, err := db.GetBlog(blog.ID)
	if err != nil || fetched == nil {
		t.Fatalf("get blog: %v %v", fetched, err)
	}
	if !fetched.IsEphemeral {
		t.Fatalf("expected IsEphemeral true, got false")
	}

	fetched.IsEphemeral = false
	if err := db.UpdateBlog(*fetched); err != nil {
		t.Fatalf("update blog: %v", err)
	}
	again, err := db.GetBlog(blog.ID)
	if err != nil || again.IsEphemeral {
		t.Fatalf("expected IsEphemeral false after update, got %+v %v", again, err)
	}
}

func TestArticleReadAtRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	blog, err := db.AddBlog(model.Blog{Name: "Test", URL: "https://example.com"})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	article, err := db.AddArticle(model.Article{BlogID: blog.ID, Title: "T", URL: "https://example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	if _, err := db.MarkArticleRead(article.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	fetched, err := db.GetArticle(article.ID)
	if err != nil || fetched == nil {
		t.Fatalf("get article: %v %v", fetched, err)
	}
	if fetched.ReadAt == nil {
		t.Fatalf("expected ReadAt set after MarkArticleRead")
	}

	if _, err := db.MarkArticleUnread(article.ID); err != nil {
		t.Fatalf("mark unread: %v", err)
	}
	fetched, err = db.GetArticle(article.ID)
	if err != nil || fetched == nil {
		t.Fatalf("get article 2: %v %v", fetched, err)
	}
	if fetched.ReadAt != nil {
		t.Fatalf("expected ReadAt nil after MarkArticleUnread")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/storage/ -run "TestBlogIsEphemeralRoundTrip|TestArticleReadAtRoundTrip" -v`
Expected: FAIL — IsEphemeral always false, ReadAt always nil.

- [ ] **Step 3: Update storage CRUD to include is_ephemeral and read_at**

In `internal/storage/database.go`:

(a) Update `AddBlog`:

```go
func (db *Database) AddBlog(blog model.Blog) (model.Blog, error) {
	result, err := db.conn.Exec(
		`INSERT INTO blogs (name, url, feed_url, scrape_selector, last_scanned, category_id, is_ephemeral)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		blog.Name,
		blog.URL,
		nullIfEmpty(blog.FeedURL),
		nullIfEmpty(blog.ScrapeSelector),
		formatTimePtr(blog.LastScanned),
		nullableInt64(blog.CategoryID),
		blog.IsEphemeral,
	)
	if err != nil {
		return blog, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return blog, err
	}
	blog.ID = id
	return blog, nil
}
```

(b) Update `UpdateBlog`:

```go
func (db *Database) UpdateBlog(blog model.Blog) error {
	_, err := db.conn.Exec(
		`UPDATE blogs SET name = ?, url = ?, feed_url = ?, scrape_selector = ?, last_scanned = ?, category_id = ?, is_ephemeral = ? WHERE id = ?`,
		blog.Name,
		blog.URL,
		nullIfEmpty(blog.FeedURL),
		nullIfEmpty(blog.ScrapeSelector),
		formatTimePtr(blog.LastScanned),
		nullableInt64(blog.CategoryID),
		blog.IsEphemeral,
		blog.ID,
	)
	return err
}
```

(c) Update SELECT queries for blogs. There are four: `GetBlog`, `GetBlogByName`, `GetBlogByURL`, `ListBlogs`. Each one passes a `SELECT ... FROM blogs` to `scanBlog`. Add `is_ephemeral` at the end of every column list:

```go
// GetBlog
row := db.conn.QueryRow(`SELECT id, name, url, feed_url, scrape_selector, last_scanned, category_id, is_ephemeral FROM blogs WHERE id = ?`, id)

// GetBlogByName
row := db.conn.QueryRow(`SELECT id, name, url, feed_url, scrape_selector, last_scanned, category_id, is_ephemeral FROM blogs WHERE name = ?`, name)

// GetBlogByURL
row := db.conn.QueryRow(`SELECT id, name, url, feed_url, scrape_selector, last_scanned, category_id, is_ephemeral FROM blogs WHERE url = ?`, url)

// ListBlogs
query := `SELECT id, name, url, feed_url, scrape_selector, last_scanned, category_id, is_ephemeral FROM blogs WHERE 1=1`
```

(d) Update `scanBlog` to scan the new column:

```go
func scanBlog(scanner interface{ Scan(dest ...any) error }) (*model.Blog, error) {
	var (
		id             int64
		name           string
		url            string
		feedURL        sql.NullString
		scrapeSelector sql.NullString
		lastScanned    sql.NullString
		categoryID     sql.NullInt64
		isEphemeral    bool
	)
	if err := scanner.Scan(&id, &name, &url, &feedURL, &scrapeSelector, &lastScanned, &categoryID, &isEphemeral); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	blog := &model.Blog{
		ID:             id,
		Name:           name,
		URL:            url,
		FeedURL:        feedURL.String,
		ScrapeSelector: scrapeSelector.String,
		IsEphemeral:    isEphemeral,
	}
	if lastScanned.Valid {
		if parsed, err := parseTime(lastScanned.String); err == nil {
			blog.LastScanned = &parsed
		}
	}
	if categoryID.Valid {
		blog.CategoryID = &categoryID.Int64
	}
	return blog, nil
}
```

(e) Update `MarkArticleRead` and `MarkArticleUnread` to write both `is_read` and `read_at` (double-write to keep legacy column meaningful during transition):

```go
func (db *Database) MarkArticleRead(id int64) (bool, error) {
	result, err := db.conn.Exec(`UPDATE articles SET is_read = 1, read_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (db *Database) MarkArticleUnread(id int64) (bool, error) {
	result, err := db.conn.Exec(`UPDATE articles SET is_read = 0, read_at = NULL WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
```

(f) Update SELECT queries for articles. Three places: `GetArticle`, `GetArticleByURL`, `ListArticles`. Add `read_at` to each column list:

```go
// GetArticle
row := db.conn.QueryRow(`SELECT id, blog_id, title, url, published_date, discovered_date, is_read, read_at FROM articles WHERE id = ?`, id)

// GetArticleByURL
row := db.conn.QueryRow(`SELECT id, blog_id, title, url, published_date, discovered_date, is_read, read_at FROM articles WHERE url = ?`, url)

// ListArticles (only the SELECT clause changes here; the WHERE clause is rewritten in Task 4)
query := `SELECT a.id, a.blog_id, a.title, a.url, a.published_date, a.discovered_date, a.is_read, a.read_at FROM articles a`
```

(g) Update `scanArticle`:

```go
func scanArticle(scanner interface{ Scan(dest ...any) error }) (*model.Article, error) {
	var (
		id            int64
		blogID        int64
		title         string
		url           string
		publishedDate sql.NullString
		discovered    sql.NullString
		isRead        bool
		readAt        sql.NullString
	)
	if err := scanner.Scan(&id, &blogID, &title, &url, &publishedDate, &discovered, &isRead, &readAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	article := &model.Article{
		ID:     id,
		BlogID: blogID,
		Title:  title,
		URL:    url,
		IsRead: isRead,
	}
	if publishedDate.Valid {
		if parsed, err := parseTime(publishedDate.String); err == nil {
			article.PublishedDate = &parsed
		}
	}
	if discovered.Valid {
		if parsed, err := parseTime(discovered.String); err == nil {
			article.DiscoveredDate = &parsed
		}
	}
	if readAt.Valid {
		if parsed, err := parseTime(readAt.String); err == nil {
			article.ReadAt = &parsed
		}
	}

	return article, nil
}
```

- [ ] **Step 4: Run all storage tests**

Run: `go test ./internal/storage/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/database.go internal/storage/database_test.go
git commit -m "feat(storage): persist is_ephemeral and read_at; double-write is_read/read_at"
```

---

### Task 4: Storage — dynamic unread query

**Files:**
- Modify: `internal/storage/database.go` (`ListArticles`)
- Modify: `internal/storage/database_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/storage/database_test.go`:

```go
func TestListArticlesEphemeralResetsAcrossDays(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	regular, err := db.AddBlog(model.Blog{Name: "Reg", URL: "https://reg.example.com"})
	if err != nil {
		t.Fatalf("add regular blog: %v", err)
	}
	ephemeral, err := db.AddBlog(model.Blog{Name: "Eph", URL: "https://eph.example.com", IsEphemeral: true})
	if err != nil {
		t.Fatalf("add ephemeral blog: %v", err)
	}

	regArt, err := db.AddArticle(model.Article{BlogID: regular.ID, Title: "R", URL: "https://reg.example.com/1"})
	if err != nil {
		t.Fatalf("add regular article: %v", err)
	}
	ephArt, err := db.AddArticle(model.Article{BlogID: ephemeral.ID, Title: "E", URL: "https://eph.example.com/1"})
	if err != nil {
		t.Fatalf("add ephemeral article: %v", err)
	}

	// Backdate read_at to yesterday for both.
	yesterday := time.Now().Add(-26 * time.Hour).Format(sqliteTimeLayout)
	if _, err := db.conn.Exec(`UPDATE articles SET read_at = ?, is_read = 1 WHERE id IN (?, ?)`, yesterday, regArt.ID, ephArt.ID); err != nil {
		t.Fatalf("backdate read_at: %v", err)
	}

	unread, err := db.ListArticles(true, nil, nil)
	if err != nil {
		t.Fatalf("list unread: %v", err)
	}
	if len(unread) != 1 || unread[0].ID != ephArt.ID {
		t.Fatalf("expected only ephemeral article in unread list, got %d items: %+v", len(unread), unread)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/storage/ -run TestListArticlesEphemeralResetsAcrossDays -v`
Expected: FAIL — current query uses `is_read = 0`, so yesterday-read ephemeral stays "read."

- [ ] **Step 3: Update ListArticles SQL**

Replace the body of `ListArticles` in `internal/storage/database.go`:

```go
func (db *Database) ListArticles(unreadOnly bool, blogID *int64, categoryID *int64) ([]model.Article, error) {
	query := `SELECT a.id, a.blog_id, a.title, a.url, a.published_date, a.discovered_date, a.is_read, a.read_at FROM articles a JOIN blogs b ON a.blog_id = b.id WHERE 1=1`
	var args []interface{}
	if unreadOnly {
		query += ` AND (
			a.read_at IS NULL
			OR (b.is_ephemeral = 1 AND date(a.read_at, 'localtime') < date('now', 'localtime'))
		)`
	}
	if blogID != nil {
		query += " AND a.blog_id = ?"
		args = append(args, *blogID)
	}
	if categoryID != nil {
		query += " AND b.category_id = ?"
		args = append(args, *categoryID)
	}
	query += " ORDER BY a.discovered_date DESC"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []model.Article
	for rows.Next() {
		article, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		if article != nil {
			articles = append(articles, *article)
		}
	}
	return articles, rows.Err()
}
```

(The JOIN to `blogs b` is now unconditional — needed for the `is_ephemeral` check. The `category_id` filter naturally reuses it.)

- [ ] **Step 4: Run all storage tests**

Run: `go test ./internal/storage/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/database.go internal/storage/database_test.go
git commit -m "feat(storage): treat yesterday-read ephemeral articles as unread"
```

---

### Task 5: Storage — `GetExistingArticleURLs` returns discovered_date; add `TouchArticlesBulk`

**Files:**
- Modify: `internal/storage/database.go`
- Modify: `internal/storage/database_test.go`
- Modify: `internal/scanner/scanner.go` (only the caller update — to keep build green)

- [ ] **Step 1: Write the failing tests**

Append to `internal/storage/database_test.go`:

```go
func TestGetExistingArticleURLsReturnsDiscoveredDate(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	blog, err := db.AddBlog(model.Blog{Name: "Test", URL: "https://example.com"})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	discoveredAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	_, err = db.AddArticle(model.Article{BlogID: blog.ID, Title: "One", URL: "https://example.com/1", DiscoveredDate: &discoveredAt})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	existing, err := db.GetExistingArticleURLs([]string{"https://example.com/1", "https://example.com/2"})
	if err != nil {
		t.Fatalf("get existing: %v", err)
	}
	got, ok := existing["https://example.com/1"]
	if !ok {
		t.Fatalf("expected existing url to be returned")
	}
	if got.ID == 0 {
		t.Fatalf("expected ID populated")
	}
	if got.DiscoveredDate == nil || !got.DiscoveredDate.Equal(discoveredAt) {
		t.Fatalf("expected DiscoveredDate %v, got %v", discoveredAt, got.DiscoveredDate)
	}
	if _, ok := existing["https://example.com/2"]; ok {
		t.Fatalf("did not expect missing URL to appear")
	}
}

func TestTouchArticlesBulk(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	blog, err := db.AddBlog(model.Blog{Name: "Test", URL: "https://example.com"})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	a, err := db.AddArticle(model.Article{BlogID: blog.ID, Title: "A", URL: "https://example.com/a", DiscoveredDate: &old})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}
	b, err := db.AddArticle(model.Article{BlogID: blog.ID, Title: "B", URL: "https://example.com/b", DiscoveredDate: &old})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	newTime := time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC)
	if err := db.TouchArticlesBulk([]int64{a.ID}, newTime); err != nil {
		t.Fatalf("touch bulk: %v", err)
	}

	fetchedA, _ := db.GetArticle(a.ID)
	fetchedB, _ := db.GetArticle(b.ID)
	if fetchedA.DiscoveredDate == nil || !fetchedA.DiscoveredDate.Equal(newTime) {
		t.Fatalf("expected A DiscoveredDate updated, got %v", fetchedA.DiscoveredDate)
	}
	if fetchedB.DiscoveredDate == nil || !fetchedB.DiscoveredDate.Equal(old) {
		t.Fatalf("expected B DiscoveredDate unchanged, got %v", fetchedB.DiscoveredDate)
	}

	// Empty input is a no-op.
	if err := db.TouchArticlesBulk(nil, newTime); err != nil {
		t.Fatalf("touch bulk empty: %v", err)
	}
}
```

Update the existing `TestGetExistingArticleURLs` test (line ~88 in `database_test.go`) so the assertion uses the new map value type:

```go
	if got, ok := existing["https://example.com/1"]; !ok || got.ID == 0 {
		t.Fatalf("expected existing url with ID")
	}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/storage/ -run "TestGetExistingArticleURLsReturnsDiscoveredDate|TestTouchArticlesBulk|TestGetExistingArticleURLs" -v`
Expected: FAIL — type mismatch on map value; `TouchArticlesBulk` undefined.

- [ ] **Step 3: Change signature and add TouchArticlesBulk**

In `internal/storage/database.go`:

(a) Add the new type and replace `GetExistingArticleURLs`:

```go
type ExistingArticle struct {
	ID             int64
	DiscoveredDate *time.Time
}

func (db *Database) GetExistingArticleURLs(urls []string) (map[string]ExistingArticle, error) {
	result := make(map[string]ExistingArticle)
	if len(urls) == 0 {
		return result, nil
	}

	chunkSize := 900
	for start := 0; start < len(urls); start += chunkSize {
		end := start + chunkSize
		if end > len(urls) {
			end = len(urls)
		}
		chunk := urls[start:end]
		placeholders := strings.TrimRight(strings.Repeat("?,", len(chunk)), ",")
		query := fmt.Sprintf("SELECT id, url, discovered_date FROM articles WHERE url IN (%s)", placeholders)
		rows, err := db.conn.Query(query, interfaceSlice(chunk)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var (
				id         int64
				url        string
				discovered sql.NullString
			)
			if err := rows.Scan(&id, &url, &discovered); err != nil {
				rows.Close()
				return nil, err
			}
			entry := ExistingArticle{ID: id}
			if discovered.Valid {
				if parsed, err := parseTime(discovered.String); err == nil {
					entry.DiscoveredDate = &parsed
				}
			}
			result[url] = entry
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return result, nil
}
```

(b) Add `TouchArticlesBulk`:

```go
func (db *Database) TouchArticlesBulk(ids []int64, ts time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`UPDATE articles SET discovered_date = ? WHERE id = ?`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	formatted := ts.Format(sqliteTimeLayout)
	for _, id := range ids {
		if _, err := stmt.Exec(formatted, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
```

(c) Update the existing caller in `internal/scanner/scanner.go` (only the inner check changes; rest of the scanner is rewritten in Task 7):

Find the loop that reads `existing[article.URL]`:

```go
	for _, article := range uniqueArticles {
		if _, exists := existing[article.URL]; exists {
			continue
		}
```

This still compiles — `existing` is now `map[string]ExistingArticle`, but the membership check works the same. No code change needed here for this task. Verify with a build.

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/database.go internal/storage/database_test.go
git commit -m "feat(storage): GetExistingArticleURLs returns discovered_date; add TouchArticlesBulk"
```

---

### Task 6: Controller — new return signatures and `IsEphemeral` plumbing

**Files:**
- Modify: `internal/controller/controller.go`
- Modify: `internal/controller/controller_test.go`
- Modify: `internal/cli/commands.go` (callers — minimal updates to keep things compiling)

- [ ] **Step 1: Write the failing tests**

Read `internal/controller/controller_test.go` first to follow the existing pattern, then append:

```go
func TestMarkArticleReadOnEphemeralBlogAfterDayResets(t *testing.T) {
	db := openControllerTestDB(t)
	defer db.Close()

	blog, err := db.AddBlog(model.Blog{Name: "Eph", URL: "https://eph.example.com", IsEphemeral: true})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	article, err := db.AddArticle(model.Article{BlogID: blog.ID, Title: "T", URL: "https://eph.example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	// Backdate read_at to yesterday using raw SQL.
	yesterday := time.Now().Add(-26 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE articles SET read_at = ?, is_read = 1 WHERE id = ?`, yesterday, article.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	updated, wasAlreadyRead, err := controller.MarkArticleRead(db, article.ID)
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if wasAlreadyRead {
		t.Fatalf("expected wasAlreadyRead=false for yesterday-read ephemeral article")
	}
	if updated.ReadAt == nil {
		t.Fatalf("expected ReadAt set after mark read")
	}
}

func TestGetArticlesReturnsBlogIsEphemeralMap(t *testing.T) {
	db := openControllerTestDB(t)
	defer db.Close()

	_, err := db.AddBlog(model.Blog{Name: "Eph", URL: "https://eph.example.com", IsEphemeral: true})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	regular, err := db.AddBlog(model.Blog{Name: "Reg", URL: "https://reg.example.com"})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	_, err = db.AddArticle(model.Article{BlogID: regular.ID, Title: "R", URL: "https://reg.example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	_, blogNames, blogEph, err := controller.GetArticles(db, true, "", "")
	if err != nil {
		t.Fatalf("get articles: %v", err)
	}
	if blogNames[regular.ID] != "Reg" {
		t.Fatalf("expected blogNames populated")
	}
	if blogEph[regular.ID] != false {
		t.Fatalf("expected regular blog IsEphemeral=false")
	}
}
```

Look up the existing helper in `controller_test.go` (likely called `openControllerTestDB` or similar). If it doesn't exist, add one mirroring `openTestDB` from `scanner_test.go`.

You also need to expose `db.Exec` for the test. If not already exported on `*storage.Database`, add a method to storage:

```go
// In storage/database.go
func (db *Database) Exec(query string, args ...any) (sql.Result, error) {
	return db.conn.Exec(query, args...)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/controller/...`
Expected: FAIL — wrong number of return values on `MarkArticleRead` / `GetArticles`; `IsEphemeral` field unused error possible.

- [ ] **Step 3: Update controller**

Replace the relevant functions in `internal/controller/controller.go`:

```go
import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/hanw39/blogwatcher/internal/model"
	"github.com/hanw39/blogwatcher/internal/opml"
	"github.com/hanw39/blogwatcher/internal/storage"
)

// ...

func MarkArticleRead(db *storage.Database, articleID int64) (model.Article, bool, error) {
	article, err := db.GetArticle(articleID)
	if err != nil {
		return model.Article{}, false, err
	}
	if article == nil {
		return model.Article{}, false, ArticleNotFoundError{ID: articleID}
	}
	blog, err := db.GetBlog(article.BlogID)
	if err != nil {
		return model.Article{}, false, err
	}
	already := blog != nil && model.ArticleIsRead(*article, blog.IsEphemeral, time.Now())
	if !already {
		if _, err := db.MarkArticleRead(articleID); err != nil {
			return model.Article{}, false, err
		}
		now := time.Now()
		article.ReadAt = &now
		article.IsRead = true
	}
	return *article, already, nil
}

func MarkArticleUnread(db *storage.Database, articleID int64) (model.Article, bool, error) {
	article, err := db.GetArticle(articleID)
	if err != nil {
		return model.Article{}, false, err
	}
	if article == nil {
		return model.Article{}, false, ArticleNotFoundError{ID: articleID}
	}
	blog, err := db.GetBlog(article.BlogID)
	if err != nil {
		return model.Article{}, false, err
	}
	alreadyUnread := !(blog != nil && model.ArticleIsRead(*article, blog.IsEphemeral, time.Now()))
	if !alreadyUnread {
		if _, err := db.MarkArticleUnread(articleID); err != nil {
			return model.Article{}, false, err
		}
		article.ReadAt = nil
		article.IsRead = false
	}
	return *article, alreadyUnread, nil
}

func GetArticles(db *storage.Database, showAll bool, blogName string, categoryName string) ([]model.Article, map[int64]string, map[int64]bool, error) {
	var blogID *int64
	if blogName != "" {
		blog, err := db.GetBlogByName(blogName)
		if err != nil {
			return nil, nil, nil, err
		}
		if blog == nil {
			return nil, nil, nil, BlogNotFoundError{Name: blogName}
		}
		blogID = &blog.ID
	}

	var categoryID *int64
	if categoryName != "" {
		cat, err := db.GetCategoryByName(categoryName)
		if err != nil {
			return nil, nil, nil, err
		}
		if cat == nil {
			return []model.Article{}, map[int64]string{}, map[int64]bool{}, nil
		}
		categoryID = &cat.ID
	}

	articles, err := db.ListArticles(!showAll, blogID, categoryID)
	if err != nil {
		return nil, nil, nil, err
	}
	blogs, err := db.ListBlogs(nil)
	if err != nil {
		return nil, nil, nil, err
	}
	blogNames := make(map[int64]string)
	blogEphemeral := make(map[int64]bool)
	for _, blog := range blogs {
		blogNames[blog.ID] = blog.Name
		blogEphemeral[blog.ID] = blog.IsEphemeral
	}

	return articles, blogNames, blogEphemeral, nil
}
```

Update `AddBlog` to accept `isEphemeral`:

```go
func AddBlog(db *storage.Database, name string, url string, feedURL string, scrapeSelector string, categoryName string, isEphemeral bool) (model.Blog, error) {
	if existing, err := db.GetBlogByName(name); err != nil {
		return model.Blog{}, err
	} else if existing != nil {
		return model.Blog{}, BlogAlreadyExistsError{Field: "name", Value: name}
	}
	if existing, err := db.GetBlogByURL(url); err != nil {
		return model.Blog{}, err
	} else if existing != nil {
		return model.Blog{}, BlogAlreadyExistsError{Field: "URL", Value: url}
	}

	blog := model.Blog{
		Name:           name,
		URL:            url,
		FeedURL:        feedURL,
		ScrapeSelector: scrapeSelector,
		IsEphemeral:    isEphemeral,
	}

	if categoryName != "" {
		cat, err := db.GetOrCreateCategory(categoryName)
		if err != nil {
			return model.Blog{}, err
		}
		blog.CategoryID = &cat.ID
	}

	return db.AddBlog(blog)
}
```

Add a sibling `EditBlogEphemeral`:

```go
func EditBlogEphemeral(db *storage.Database, blogName string, isEphemeral bool) (model.Blog, error) {
	blog, err := db.GetBlogByName(blogName)
	if err != nil {
		return model.Blog{}, err
	}
	if blog == nil {
		return model.Blog{}, BlogNotFoundError{Name: blogName}
	}
	blog.IsEphemeral = isEphemeral
	if err := db.UpdateBlog(*blog); err != nil {
		return model.Blog{}, err
	}
	return *blog, nil
}
```

Update `MarkAllArticlesRead` — it can stay almost identical, but it also reads `article.IsRead = true` for the returned slice. Keep that line for now (we still have the field). The unread-list it iterates is already correct because `ListArticles(true, ...)` uses the new dynamic query.

Update the existing call site in `ImportOPML` to pass `false` for the new arg:

```go
		_, err := AddBlog(db, feed.Title, siteURL, feed.FeedURL, "", "", false)
```

- [ ] **Step 4: Update CLI callers (minimal)**

In `internal/cli/commands.go`, update the call sites to match new signatures. The full UX improvements happen in Task 8 — here we only do whatever is needed to keep things compiling.

(a) `newAddCommand`:

```go
_, err = controller.AddBlog(db, name, url, feedURL, scrapeSelector, category, false)
```

(b) `newReadCommand`:

```go
article, wasAlreadyRead, err := controller.MarkArticleRead(db, articleID)
if err != nil {
	printError(err)
	return markError(err)
}
if wasAlreadyRead {
	fmt.Printf("Article %d is already marked as read.\n", articleID)
} else {
	color.New(color.FgGreen).Printf("Marked article %d as read\n", articleID)
}
_ = article
```

(c) `newUnreadCommand`:

```go
article, wasAlreadyUnread, err := controller.MarkArticleUnread(db, articleID)
if err != nil {
	printError(err)
	return markError(err)
}
if wasAlreadyUnread {
	fmt.Printf("Article %d is already marked as unread.\n", articleID)
} else {
	color.New(color.FgGreen).Printf("Marked article %d as unread\n", articleID)
}
_ = article
```

(d) `newArticlesCommand` and `newReadAllCommand` both call `controller.GetArticles`. Update to the new 4-value return:

```go
articles, blogNames, _, err := controller.GetArticles(db, showAll, blogName, categoryName)
```

(The `blogIsEphemeral` map gets wired into `printArticle` in Task 8.)

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/controller/controller.go internal/controller/controller_test.go internal/cli/commands.go internal/storage/database.go
git commit -m "feat(controller): use ArticleIsRead helper; return blogIsEphemeral map; accept isEphemeral on AddBlog"
```

---

### Task 7: Scanner — refresh ephemeral articles at most once per local day

**Files:**
- Modify: `internal/scanner/scanner.go`
- Modify: `internal/scanner/scanner_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/scanner/scanner_test.go`:

```go
func TestScanBlogEphemeralRefreshOncePerDay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer server.Close()

	db := openTestDB(t)
	defer db.Close()

	blog, err := db.AddBlog(model.Blog{Name: "Eph", URL: "https://eph.example.com", FeedURL: server.URL, IsEphemeral: true})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	// Seed an existing article with a yesterday discovered_date — should be lifted.
	yesterday := time.Now().Add(-26 * time.Hour)
	_, err = db.AddArticle(model.Article{BlogID: blog.ID, Title: "First", URL: "https://example.com/1", DiscoveredDate: &yesterday})
	if err != nil {
		t.Fatalf("seed article: %v", err)
	}

	first := ScanBlog(db, blog)
	if first.Refreshed != 1 {
		t.Fatalf("expected Refreshed=1 on first scan, got %d", first.Refreshed)
	}
	if first.NewArticles != 1 {
		t.Fatalf("expected NewArticles=1 (the second feed item), got %d", first.NewArticles)
	}

	// Second scan same day: nothing to refresh, nothing new.
	second := ScanBlog(db, blog)
	if second.Refreshed != 0 {
		t.Fatalf("expected Refreshed=0 on second same-day scan, got %d", second.Refreshed)
	}
	if second.NewArticles != 0 {
		t.Fatalf("expected NewArticles=0 on second scan, got %d", second.NewArticles)
	}
}

func TestScanBlogNonEphemeralDoesNotRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer server.Close()

	db := openTestDB(t)
	defer db.Close()

	blog, err := db.AddBlog(model.Blog{Name: "Reg", URL: "https://reg.example.com", FeedURL: server.URL})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	yesterday := time.Now().Add(-26 * time.Hour)
	_, err = db.AddArticle(model.Article{BlogID: blog.ID, Title: "First", URL: "https://example.com/1", DiscoveredDate: &yesterday})
	if err != nil {
		t.Fatalf("seed article: %v", err)
	}

	result := ScanBlog(db, blog)
	if result.Refreshed != 0 {
		t.Fatalf("expected Refreshed=0 for non-ephemeral blog, got %d", result.Refreshed)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/scanner/...`
Expected: FAIL — `Refreshed` field undefined on `ScanResult`.

- [ ] **Step 3: Update scanner**

In `internal/scanner/scanner.go`, replace the relevant section. Full updated `ScanBlog`:

```go
type ScanResult struct {
	BlogName    string
	NewArticles int
	Refreshed   int
	TotalFound  int
	Source      string
	Error       string
}

func ScanBlog(db *storage.Database, blog model.Blog) ScanResult {
	var (
		articles []model.Article
		source   = "none"
		errText  string
	)

	feedURL := blog.FeedURL
	if feedURL == "" {
		if discovered, err := rss.DiscoverFeedURL(blog.URL, 30*time.Second); err == nil && discovered != "" {
			feedURL = discovered
			blog.FeedURL = discovered
			_ = db.UpdateBlog(blog)
		}
	}

	if feedURL != "" {
		feedArticles, err := rss.ParseFeed(feedURL, 30*time.Second)
		if err != nil {
			errText = err.Error()
		} else {
			articles = convertFeedArticles(blog.ID, feedArticles)
			source = "rss"
		}
	}

	if len(articles) == 0 && blog.ScrapeSelector != "" {
		scrapedArticles, err := scraper.ScrapeBlog(blog.URL, blog.ScrapeSelector, 30*time.Second)
		if err != nil {
			if errText != "" {
				errText = fmt.Sprintf("RSS: %s; Scraper: %s", errText, err.Error())
			} else {
				errText = err.Error()
			}
		} else {
			articles = convertScrapedArticles(blog.ID, scrapedArticles)
			source = "scraper"
			errText = ""
		}
	}

	seenURLs := make(map[string]struct{})
	uniqueArticles := make([]model.Article, 0, len(articles))
	for _, article := range articles {
		if _, exists := seenURLs[article.URL]; exists {
			continue
		}
		seenURLs[article.URL] = struct{}{}
		uniqueArticles = append(uniqueArticles, article)
	}

	urlList := make([]string, 0, len(seenURLs))
	for url := range seenURLs {
		urlList = append(urlList, url)
	}

	existing, err := db.GetExistingArticleURLs(urlList)
	if err != nil {
		errText = err.Error()
	}

	now := time.Now()
	todayMidnightLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	newArticles := make([]model.Article, 0, len(uniqueArticles))
	refreshIDs := make([]int64, 0)
	for _, article := range uniqueArticles {
		entry, exists := existing[article.URL]
		if !exists {
			article.DiscoveredDate = &now
			newArticles = append(newArticles, article)
			continue
		}
		if !blog.IsEphemeral {
			continue
		}
		// Ephemeral: lift discovered_date to now if it hasn't been touched today.
		if entry.DiscoveredDate == nil || entry.DiscoveredDate.Before(todayMidnightLocal) {
			refreshIDs = append(refreshIDs, entry.ID)
		}
	}

	newCount := 0
	if len(newArticles) > 0 {
		count, err := db.AddArticlesBulk(newArticles)
		if err != nil {
			errText = err.Error()
		} else {
			newCount = count
		}
	}

	refreshedCount := 0
	if len(refreshIDs) > 0 {
		if err := db.TouchArticlesBulk(refreshIDs, now); err != nil {
			errText = err.Error()
		} else {
			refreshedCount = len(refreshIDs)
		}
	}

	_ = db.UpdateBlogLastScanned(blog.ID, time.Now())

	return ScanResult{
		BlogName:    blog.Name,
		NewArticles: newCount,
		Refreshed:   refreshedCount,
		TotalFound:  len(seenURLs),
		Source:      source,
		Error:       errText,
	}
}
```

- [ ] **Step 4: Run scanner tests**

Run: `go test ./internal/scanner/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "feat(scanner): lift ephemeral articles to top once per local day"
```

---

### Task 8: CLI — `--ephemeral` flag, `[daily]` tag, refreshed count

**Files:**
- Modify: `internal/cli/commands.go`

- [ ] **Step 1: Add `--ephemeral` flag to `add`**

In `newAddCommand`, declare the flag and pass it through:

```go
func newAddCommand() *cobra.Command {
	var feedURL string
	var scrapeSelector string
	var category string
	var ephemeral bool

	cmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add a new blog to track.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			url := args[1]
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			_, err = controller.AddBlog(db, name, url, feedURL, scrapeSelector, category, ephemeral)
			if err != nil {
				printError(err)
				return markError(err)
			}
			color.New(color.FgGreen).Printf("Added blog '%s'\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&feedURL, "feed-url", "", "RSS/Atom feed URL (auto-discovered if not provided)")
	cmd.Flags().StringVar(&scrapeSelector, "scrape-selector", "", "CSS selector for HTML scraping fallback")
	cmd.Flags().StringVarP(&category, "category", "c", "", "Assign blog to a category")
	cmd.Flags().BoolVar(&ephemeral, "ephemeral", false, "Mark as a daily-reset source (e.g. GitHub Trending)")
	return cmd
}
```

- [ ] **Step 2: Add `--ephemeral` to `edit`**

Update `newEditCommand`:

```go
func newEditCommand() *cobra.Command {
	var category string
	var ephemeral bool

	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a tracked blog.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			categoryChanged := cmd.Flags().Changed("category")
			ephemeralChanged := cmd.Flags().Changed("ephemeral")
			if !categoryChanged && !ephemeralChanged {
				return fmt.Errorf("specify at least one field to edit (e.g. --category, --ephemeral)")
			}
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			if categoryChanged {
				if _, err := controller.EditBlogCategory(db, name, category); err != nil {
					printError(err)
					return markError(err)
				}
			}
			if ephemeralChanged {
				if _, err := controller.EditBlogEphemeral(db, name, ephemeral); err != nil {
					printError(err)
					return markError(err)
				}
			}
			color.New(color.FgGreen).Printf("Updated blog '%s'\n", name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&category, "category", "c", "", "Assign to category (empty string removes category)")
	cmd.Flags().BoolVar(&ephemeral, "ephemeral", false, "Mark/unmark as a daily-reset source (use --ephemeral=false to unset)")
	return cmd
}
```

Note: `cobra`'s `BoolVar` accepts `--ephemeral` (true), `--ephemeral=false`, and `--no-ephemeral` automatically when registered this way only if Cobra's NoOptDefVal is set. Cobra 1.x does NOT auto-generate `--no-ephemeral`. So users will pass `--ephemeral=true` or `--ephemeral=false`. Add a short note in the flag help to make that clear (the help string above already does).

- [ ] **Step 3: Add `[daily]` tag to `blogs` output**

In `newBlogsCommand`, where the loop prints each blog, change:

```go
color.New(color.FgWhite, color.Bold).Printf("  %s\n", blog.Name)
```

to:

```go
if blog.IsEphemeral {
	color.New(color.FgWhite, color.Bold).Printf("  %s", blog.Name)
	color.New(color.FgMagenta).Printf("  [daily]\n")
} else {
	color.New(color.FgWhite, color.Bold).Printf("  %s\n", blog.Name)
}
```

- [ ] **Step 4: Make `printArticle` aware of ephemeral state**

Replace `printArticle`:

```go
func printArticle(article model.Article, blogName string, blogIsEphemeral bool) {
	status := color.New(color.FgYellow).Sprint("[new]")
	if model.ArticleIsRead(article, blogIsEphemeral, time.Now()) {
		status = color.New(color.FgHiBlack).Sprint("[read]")
	}
	idStr := color.New(color.FgCyan).Sprintf("[%d]", article.ID)
	fmt.Printf("  %s %s %s\n", idStr, status, article.Title)
	fmt.Printf("       Blog: %s\n", blogName)
	fmt.Printf("       URL: %s\n", article.URL)
	if article.PublishedDate != nil {
		fmt.Printf("       Published: %s\n", article.PublishedDate.Format("2006-01-02"))
	}
	fmt.Println()
}
```

Update `newArticlesCommand` to thread the map through:

```go
articles, blogNames, blogEphemeral, err := controller.GetArticles(db, showAll, blogName, categoryName)
// ...
for _, article := range articles {
	printArticle(article, blogNames[article.BlogID], blogEphemeral[article.BlogID])
}
```

Update `newReadAllCommand` similarly where it calls `controller.GetArticles`:

```go
articles, blogNames, _, err := controller.GetArticles(db, false, blogName, "")
```

- [ ] **Step 5: Show `Refreshed:` in scan output**

Replace `printScanResult`:

```go
func printScanResult(result scanner.ScanResult) {
	statusColor := color.FgWhite
	if result.NewArticles > 0 {
		statusColor = color.FgGreen
	}
	color.New(color.FgWhite, color.Bold).Printf("  %s\n", result.BlogName)
	if result.Error != "" {
		color.New(color.FgRed).Printf("    Error: %s\n", result.Error)
		return
	}
	if result.Source == "none" {
		color.New(color.FgYellow).Println("    No feed or scraper configured")
		return
	}
	sourceLabel := "HTML"
	if result.Source == "rss" {
		sourceLabel = "RSS"
	}
	fmt.Printf("    Source: %s | Found: %d | ", sourceLabel, result.TotalFound)
	color.New(statusColor).Printf("New: %d", result.NewArticles)
	if result.Refreshed > 0 {
		fmt.Printf(" | ")
		color.New(color.FgMagenta).Printf("Refreshed: %d", result.Refreshed)
	}
	fmt.Println()
}
```

- [ ] **Step 6: Run all tests and build**

Run: `go test ./...`
Expected: PASS.

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 7: Manual smoke test**

```bash
go run ./cmd/blogwatcher add github-trending https://github.com/trending --ephemeral
go run ./cmd/blogwatcher blogs
# expect: github-trending  [daily]
go run ./cmd/blogwatcher scan github-trending
# expect: Source: ... | Found: N | New: M
go run ./cmd/blogwatcher scan github-trending
# expect: same as above, New: 0, no Refreshed line (already lifted today)
```

- [ ] **Step 8: Commit**

```bash
git add internal/cli/commands.go
git commit -m "feat(cli): --ephemeral flag, [daily] tag, Refreshed scan output"
```

---

### Task 9: Cleanup — remove `Article.IsRead` and the double-write

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/storage/database.go`
- Modify: `internal/controller/controller.go`
- Modify: tests in `internal/storage/database_test.go`, `internal/controller/controller_test.go` that reference `Article.IsRead`

- [ ] **Step 1: Identify references**

Run: `grep -rn "\.IsRead" internal/`
Expected list (review before deletion):
- `internal/model/model.go`: the field definition
- `internal/controller/controller.go`: a couple of `article.IsRead = true` / `= false` writes inside `MarkArticleRead`/`Unread`/`MarkAllArticlesRead`
- Tests asserting `updated.IsRead`

- [ ] **Step 2: Remove the field from the model**

In `internal/model/model.go`, delete the `IsRead bool` line from the `Article` struct. The struct becomes:

```go
type Article struct {
	ID             int64
	BlogID         int64
	Title          string
	URL            string
	PublishedDate  *time.Time
	DiscoveredDate *time.Time
	ReadAt         *time.Time
}
```

- [ ] **Step 3: Stop double-writing `is_read` in storage**

In `internal/storage/database.go`:

```go
func (db *Database) MarkArticleRead(id int64) (bool, error) {
	result, err := db.conn.Exec(`UPDATE articles SET read_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (db *Database) MarkArticleUnread(id int64) (bool, error) {
	result, err := db.conn.Exec(`UPDATE articles SET read_at = NULL WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
```

Also remove `is_read` from the SELECT lists and `scanArticle`. The updated `scanArticle`:

```go
func scanArticle(scanner interface{ Scan(dest ...any) error }) (*model.Article, error) {
	var (
		id            int64
		blogID        int64
		title         string
		url           string
		publishedDate sql.NullString
		discovered    sql.NullString
		readAt        sql.NullString
	)
	if err := scanner.Scan(&id, &blogID, &title, &url, &publishedDate, &discovered, &readAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	article := &model.Article{
		ID:     id,
		BlogID: blogID,
		Title:  title,
		URL:    url,
	}
	if publishedDate.Valid {
		if parsed, err := parseTime(publishedDate.String); err == nil {
			article.PublishedDate = &parsed
		}
	}
	if discovered.Valid {
		if parsed, err := parseTime(discovered.String); err == nil {
			article.DiscoveredDate = &parsed
		}
	}
	if readAt.Valid {
		if parsed, err := parseTime(readAt.String); err == nil {
			article.ReadAt = &parsed
		}
	}
	return article, nil
}
```

Drop `is_read` from the SELECT clauses in `GetArticle`, `GetArticleByURL`, and `ListArticles`. (Leave the column itself in the schema — that's the documented rollback safety net.)

- [ ] **Step 4: Remove residual `article.IsRead = true/false` assignments in controller**

In `internal/controller/controller.go`, delete `article.IsRead = true` / `article.IsRead = false` lines inside `MarkArticleRead`, `MarkArticleUnread`, and `MarkAllArticlesRead`.

- [ ] **Step 5: Fix any test assertions referencing `IsRead`**

In the existing `TestDatabaseCreatesFileAndCRUD` (storage test), the assertion `if updated == nil || !updated.IsRead` becomes:

```go
if updated == nil || updated.ReadAt == nil {
	t.Fatalf("expected article read: %+v", updated)
}
```

In `TestListArticlesFiltersAndOrdering`, the call `db.MarkArticleRead(first.ID)` still works; no field reference to change.

Scan other test files for any remaining `.IsRead` and remove them. Run the grep again to confirm none remain (except inside test bodies that have been updated).

- [ ] **Step 6: Run all tests and build**

Run: `go test ./...`
Expected: PASS.

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: remove Article.IsRead field and is_read double-write"
```

---

## Verification checklist

After Task 9:

- [ ] `go test ./...` passes
- [ ] `go build ./...` clean
- [ ] `grep -rn "\.IsRead" internal/` returns nothing (or only comments)
- [ ] Manual: add an ephemeral blog, mark one of its articles read, run `articles` → article shows `[read]`. Wait until next local day (or backdate `read_at` via `sqlite3` shell to yesterday), run `articles` again → article shows `[new]`.
- [ ] Manual: run `scan` twice in a row on an ephemeral blog; second run shows no `Refreshed:` line.
- [ ] Manual: `blogs` lists ephemeral ones with `[daily]` tag.
