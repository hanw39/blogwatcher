# Blog Categories Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add category support so each blog can belong to one named category, with CLI commands to assign, filter, and list categories.

**Architecture:** Add a `categories` table with a FK from `blogs.category_id`. Categories are created on-demand when assigned. New `edit` and `categories` commands are added; `add`, `blogs`, and `articles` commands gain a `--category` flag.

**Tech Stack:** Go 1.24+, SQLite (modernc.org/sqlite), cobra

---

### Task 1: Add Category to model

**Files:**
- Modify: `internal/model/model.go`

- [ ] **Step 1: Add Category struct and CategoryID field**

Replace the contents of `internal/model/model.go` with:

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
}

type Article struct {
	ID             int64
	BlogID         int64
	Title          string
	URL            string
	PublishedDate  *time.Time
	DiscoveredDate *time.Time
	IsRead         bool
}

type Category struct {
	ID        int64
	Name      string
	BlogCount int
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: no errors (CategoryID is unused by storage yet, that's fine).

- [ ] **Step 3: Commit**

```bash
git add internal/model/model.go
git commit -m "feat: add Category model and CategoryID to Blog"
```

---

### Task 2: Storage — categories table, GetOrCreateCategory, GetCategoryByName

**Files:**
- Modify: `internal/storage/database.go`
- Modify: `internal/storage/database_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/storage/database_test.go`:

```go
func TestGetOrCreateCategory(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	cat, err := db.GetOrCreateCategory("tech")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	if cat.ID == 0 || cat.Name != "tech" {
		t.Fatalf("unexpected category: %+v", cat)
	}

	cat2, err := db.GetOrCreateCategory("tech")
	if err != nil {
		t.Fatalf("get existing category: %v", err)
	}
	if cat2.ID != cat.ID {
		t.Fatalf("expected same ID on second call, got %d vs %d", cat.ID, cat2.ID)
	}
}

func TestGetCategoryByName(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	missing, err := db.GetCategoryByName("nope")
	if err != nil || missing != nil {
		t.Fatalf("expected nil for missing category")
	}

	_, err = db.GetOrCreateCategory("tech")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	found, err := db.GetCategoryByName("tech")
	if err != nil || found == nil || found.Name != "tech" {
		t.Fatalf("expected to find tech category: %v %+v", err, found)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/storage/... -run "TestGetOrCreateCategory|TestGetCategoryByName" -v
```

Expected: FAIL — `db.GetOrCreateCategory` undefined.

- [ ] **Step 3: Add categories table to init() and implement GetOrCreateCategory + GetCategoryByName**

In `internal/storage/database.go`, update `init()` to create the categories table before blogs:

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
	_, err := db.conn.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: add category_id to existing databases
	_, err = db.conn.Exec(`ALTER TABLE blogs ADD COLUMN category_id INTEGER REFERENCES categories(id)`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	return nil
}
```

Then add these two methods anywhere after `Close()`:

```go
func (db *Database) GetOrCreateCategory(name string) (model.Category, error) {
	_, err := db.conn.Exec(`INSERT OR IGNORE INTO categories (name) VALUES (?)`, name)
	if err != nil {
		return model.Category{}, err
	}
	row := db.conn.QueryRow(`SELECT id, name FROM categories WHERE name = ?`, name)
	var cat model.Category
	if err := row.Scan(&cat.ID, &cat.Name); err != nil {
		return model.Category{}, err
	}
	return cat, nil
}

func (db *Database) GetCategoryByName(name string) (*model.Category, error) {
	row := db.conn.QueryRow(`SELECT id, name FROM categories WHERE name = ?`, name)
	var cat model.Category
	if err := row.Scan(&cat.ID, &cat.Name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &cat, nil
}
```

Also add `"strings"` to the import block in `database.go` if not already present.

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/storage/... -run "TestGetOrCreateCategory|TestGetCategoryByName" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/database.go internal/storage/database_test.go
git commit -m "feat: add categories table and GetOrCreateCategory/GetCategoryByName"
```

---

### Task 3: Storage — update blog CRUD for category_id + ListCategories + ListBlogs filter

**Files:**
- Modify: `internal/storage/database.go`
- Modify: `internal/storage/database_test.go`
- Modify: `internal/scanner/scanner.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/storage/database_test.go`:

```go
func TestBlogCategoryRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	cat, err := db.GetOrCreateCategory("tech")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	blog, err := db.AddBlog(model.Blog{Name: "Test", URL: "https://example.com", CategoryID: &cat.ID})
	if err != nil {
		t.Fatalf("add blog with category: %v", err)
	}
	if blog.CategoryID == nil || *blog.CategoryID != cat.ID {
		t.Fatalf("expected CategoryID %d, got %v", cat.ID, blog.CategoryID)
	}

	fetched, err := db.GetBlog(blog.ID)
	if err != nil || fetched == nil {
		t.Fatalf("get blog: %v", err)
	}
	if fetched.CategoryID == nil || *fetched.CategoryID != cat.ID {
		t.Fatalf("expected CategoryID after fetch, got %v", fetched.CategoryID)
	}

	// Blog without category
	plain, err := db.AddBlog(model.Blog{Name: "Plain", URL: "https://plain.example.com"})
	if err != nil {
		t.Fatalf("add plain blog: %v", err)
	}
	if plain.CategoryID != nil {
		t.Fatalf("expected nil CategoryID for uncategorized blog")
	}
}

func TestListBlogsWithCategoryFilter(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	tech, err := db.GetOrCreateCategory("tech")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	_, err = db.AddBlog(model.Blog{Name: "TechBlog", URL: "https://tech.example.com", CategoryID: &tech.ID})
	if err != nil {
		t.Fatalf("add tech blog: %v", err)
	}
	_, err = db.AddBlog(model.Blog{Name: "Other", URL: "https://other.example.com"})
	if err != nil {
		t.Fatalf("add other blog: %v", err)
	}

	all, err := db.ListBlogs(nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("expected 2 blogs, got %d: %v", len(all), err)
	}

	filtered, err := db.ListBlogs(&tech.ID)
	if err != nil || len(filtered) != 1 || filtered[0].Name != "TechBlog" {
		t.Fatalf("expected 1 tech blog, got %d: %v", len(filtered), err)
	}
}

func TestListCategories(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	tech, err := db.GetOrCreateCategory("tech")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	_, err = db.GetOrCreateCategory("design")
	if err != nil {
		t.Fatalf("create empty category: %v", err)
	}

	_, err = db.AddBlog(model.Blog{Name: "TechBlog", URL: "https://tech.example.com", CategoryID: &tech.ID})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	cats, err := db.ListCategories()
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}

	counts := make(map[string]int)
	for _, c := range cats {
		counts[c.Name] = c.BlogCount
	}
	if counts["tech"] != 1 {
		t.Fatalf("expected tech BlogCount=1, got %d", counts["tech"])
	}
	if counts["design"] != 0 {
		t.Fatalf("expected design BlogCount=0, got %d", counts["design"])
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/storage/... -run "TestBlogCategoryRoundTrip|TestListBlogsWithCategoryFilter|TestListCategories" -v
```

Expected: FAIL — compile errors (ListBlogs wrong signature, CategoryID not scanned, etc.).

- [ ] **Step 3: Add nullableInt64 helper to database.go**

Add this function at the bottom of `internal/storage/database.go`, alongside `nullIfEmpty`:

```go
func nullableInt64(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}
```

- [ ] **Step 4: Update scanBlog to scan category_id**

Replace the existing `scanBlog` function in `internal/storage/database.go`:

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
	)
	if err := scanner.Scan(&id, &name, &url, &feedURL, &scrapeSelector, &lastScanned, &categoryID); err != nil {
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

- [ ] **Step 5: Update all blog SELECT queries to include category_id**

In `internal/storage/database.go`, update every SQL query that reads from `blogs`. There are four places — replace each:

`GetBlog`:
```go
func (db *Database) GetBlog(id int64) (*model.Blog, error) {
	row := db.conn.QueryRow(`SELECT id, name, url, feed_url, scrape_selector, last_scanned, category_id FROM blogs WHERE id = ?`, id)
	return scanBlog(row)
}
```

`GetBlogByName`:
```go
func (db *Database) GetBlogByName(name string) (*model.Blog, error) {
	row := db.conn.QueryRow(`SELECT id, name, url, feed_url, scrape_selector, last_scanned, category_id FROM blogs WHERE name = ?`, name)
	return scanBlog(row)
}
```

`GetBlogByURL`:
```go
func (db *Database) GetBlogByURL(url string) (*model.Blog, error) {
	row := db.conn.QueryRow(`SELECT id, name, url, feed_url, scrape_selector, last_scanned, category_id FROM blogs WHERE url = ?`, url)
	return scanBlog(row)
}
```

- [ ] **Step 6: Update AddBlog and UpdateBlog to include category_id**

Replace `AddBlog` in `internal/storage/database.go`:

```go
func (db *Database) AddBlog(blog model.Blog) (model.Blog, error) {
	result, err := db.conn.Exec(
		`INSERT INTO blogs (name, url, feed_url, scrape_selector, last_scanned, category_id)
		VALUES (?, ?, ?, ?, ?, ?)`,
		blog.Name,
		blog.URL,
		nullIfEmpty(blog.FeedURL),
		nullIfEmpty(blog.ScrapeSelector),
		formatTimePtr(blog.LastScanned),
		nullableInt64(blog.CategoryID),
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

Replace `UpdateBlog` in `internal/storage/database.go`:

```go
func (db *Database) UpdateBlog(blog model.Blog) error {
	_, err := db.conn.Exec(
		`UPDATE blogs SET name = ?, url = ?, feed_url = ?, scrape_selector = ?, last_scanned = ?, category_id = ? WHERE id = ?`,
		blog.Name,
		blog.URL,
		nullIfEmpty(blog.FeedURL),
		nullIfEmpty(blog.ScrapeSelector),
		formatTimePtr(blog.LastScanned),
		nullableInt64(blog.CategoryID),
		blog.ID,
	)
	return err
}
```

- [ ] **Step 7: Update ListBlogs to accept optional category filter**

Replace `ListBlogs` in `internal/storage/database.go`:

```go
func (db *Database) ListBlogs(categoryID *int64) ([]model.Blog, error) {
	query := `SELECT id, name, url, feed_url, scrape_selector, last_scanned, category_id FROM blogs WHERE 1=1`
	var args []interface{}
	if categoryID != nil {
		query += " AND category_id = ?"
		args = append(args, *categoryID)
	}
	query += " ORDER BY name"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blogs []model.Blog
	for rows.Next() {
		blog, err := scanBlog(rows)
		if err != nil {
			return nil, err
		}
		if blog != nil {
			blogs = append(blogs, *blog)
		}
	}
	return blogs, rows.Err()
}
```

- [ ] **Step 8: Add ListCategories**

Add after `ListBlogs` in `internal/storage/database.go`:

```go
func (db *Database) ListCategories() ([]model.Category, error) {
	rows, err := db.conn.Query(`
		SELECT c.id, c.name, COUNT(b.id) AS blog_count
		FROM categories c
		LEFT JOIN blogs b ON b.category_id = c.id
		GROUP BY c.id, c.name
		ORDER BY c.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []model.Category
	for rows.Next() {
		var cat model.Category
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.BlogCount); err != nil {
			return nil, err
		}
		categories = append(categories, cat)
	}
	return categories, rows.Err()
}
```

- [ ] **Step 9: Fix ListBlogs call site in scanner.go**

In `internal/scanner/scanner.go` line 115, change:

```go
blogs, err := db.ListBlogs()
```

to:

```go
blogs, err := db.ListBlogs(nil)
```

- [ ] **Step 10: Run all storage and scanner tests**

```bash
go test ./internal/storage/... ./internal/scanner/... -v
```

Expected: all PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/storage/database.go internal/storage/database_test.go internal/scanner/scanner.go
git commit -m "feat: update blog CRUD for category_id, add ListCategories and ListBlogs filter"
```

---

### Task 4: Storage — ListArticles with category filter

**Files:**
- Modify: `internal/storage/database.go`
- Modify: `internal/storage/database_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/storage/database_test.go`:

```go
func TestListArticlesWithCategoryFilter(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	tech, err := db.GetOrCreateCategory("tech")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	techBlog, err := db.AddBlog(model.Blog{Name: "TechBlog", URL: "https://tech.example.com", CategoryID: &tech.ID})
	if err != nil {
		t.Fatalf("add tech blog: %v", err)
	}
	otherBlog, err := db.AddBlog(model.Blog{Name: "Other", URL: "https://other.example.com"})
	if err != nil {
		t.Fatalf("add other blog: %v", err)
	}

	_, err = db.AddArticle(model.Article{BlogID: techBlog.ID, Title: "Tech Post", URL: "https://tech.example.com/1"})
	if err != nil {
		t.Fatalf("add tech article: %v", err)
	}
	_, err = db.AddArticle(model.Article{BlogID: otherBlog.ID, Title: "Other Post", URL: "https://other.example.com/1"})
	if err != nil {
		t.Fatalf("add other article: %v", err)
	}

	all, err := db.ListArticles(false, nil, nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("expected 2 articles, got %d: %v", len(all), err)
	}

	filtered, err := db.ListArticles(false, nil, &tech.ID)
	if err != nil || len(filtered) != 1 || filtered[0].Title != "Tech Post" {
		t.Fatalf("expected 1 tech article, got %d: %v", len(filtered), err)
	}

	// Unknown category ID → empty list
	unknown := int64(9999)
	none, err := db.ListArticles(false, nil, &unknown)
	if err != nil || len(none) != 0 {
		t.Fatalf("expected 0 articles for unknown category, got %d: %v", len(none), err)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/storage/... -run TestListArticlesWithCategoryFilter -v
```

Expected: FAIL — `ListArticles` called with wrong number of args.

- [ ] **Step 3: Update ListArticles to accept categoryID**

Replace `ListArticles` in `internal/storage/database.go`:

```go
func (db *Database) ListArticles(unreadOnly bool, blogID *int64, categoryID *int64) ([]model.Article, error) {
	query := `SELECT a.id, a.blog_id, a.title, a.url, a.published_date, a.discovered_date, a.is_read FROM articles a`
	if categoryID != nil {
		query += ` JOIN blogs b ON a.blog_id = b.id`
	}
	query += ` WHERE 1=1`
	var args []interface{}
	if unreadOnly {
		query += " AND a.is_read = 0"
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

- [ ] **Step 4: Fix existing ListArticles call sites in database_test.go**

In `internal/storage/database_test.go`, find all calls to `db.ListArticles` that pass only 2 args and add `nil` as the third argument. There are several in `TestDatabaseCreatesFileAndCRUD`, `TestListArticlesFiltersAndOrdering`, and `TestBulkInsertDuplicateRollbackAndEmpty`.

Search for the pattern: `db.ListArticles(` and update each:
- `db.ListArticles(false, nil)` → `db.ListArticles(false, nil, nil)`
- `db.ListArticles(true, nil)` → `db.ListArticles(true, nil, nil)`
- `db.ListArticles(false, &blogID)` → `db.ListArticles(false, &blogID, nil)`

- [ ] **Step 5: Run all storage tests**

```bash
go test ./internal/storage/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/storage/database.go internal/storage/database_test.go
git commit -m "feat: add category filter to ListArticles"
```

---

### Task 5: Controller — update AddBlog + fix ListArticles/ListBlogs call sites

**Files:**
- Modify: `internal/controller/controller.go`
- Modify: `internal/controller/controller_test.go`

- [ ] **Step 1: Write failing test for AddBlog with category**

In `internal/controller/controller_test.go`, replace `TestAddBlogAndRemoveBlog` with:

```go
func TestAddBlogAndRemoveBlog(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := AddBlog(db, "Test", "https://example.com", "", "", "")
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	if _, err := AddBlog(db, "Test", "https://other.com", "", "", ""); err == nil {
		t.Fatalf("expected duplicate name error")
	}

	if _, err := AddBlog(db, "Other", "https://example.com", "", "", ""); err == nil {
		t.Fatalf("expected duplicate url error")
	}

	if err := RemoveBlog(db, blog.Name); err != nil {
		t.Fatalf("remove blog: %v", err)
	}
}

func TestAddBlogWithCategory(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := AddBlog(db, "TechBlog", "https://tech.example.com", "", "", "tech")
	if err != nil {
		t.Fatalf("add blog with category: %v", err)
	}
	if blog.CategoryID == nil {
		t.Fatalf("expected CategoryID to be set")
	}

	// Same category again — must reuse existing, not error
	blog2, err := AddBlog(db, "TechBlog2", "https://tech2.example.com", "", "", "tech")
	if err != nil {
		t.Fatalf("add second blog with same category: %v", err)
	}
	if blog2.CategoryID == nil || *blog2.CategoryID != *blog.CategoryID {
		t.Fatalf("expected same category ID for both blogs")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/controller/... -run "TestAddBlog" -v
```

Expected: FAIL — `AddBlog` called with wrong number of args.

- [ ] **Step 3: Update AddBlog in controller.go**

Replace `AddBlog` in `internal/controller/controller.go`:

```go
func AddBlog(db *storage.Database, name string, url string, feedURL string, scrapeSelector string, categoryName string) (model.Blog, error) {
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

- [ ] **Step 4: Fix ListBlogs and ListArticles call sites in controller.go**

In `internal/controller/controller.go`, `GetArticles` calls `db.ListBlogs()` and `db.ListArticles(...)`. Update them:

```go
// Change:
articles, err := db.ListArticles(!showAll, blogID)
// To:
articles, err := db.ListArticles(!showAll, blogID, nil)

// Change:
blogs, err := db.ListBlogs()
// To:
blogs, err := db.ListBlogs(nil)
```

Also in `MarkAllArticlesRead`, update:
```go
// Change:
articles, err := db.ListArticles(true, blogID)
// To:
articles, err := db.ListArticles(true, blogID, nil)
```

- [ ] **Step 5: Fix controller_test.go call sites**

In `internal/controller/controller_test.go`, update `TestGetArticlesFilters`:

```go
func TestGetArticlesFilters(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := AddBlog(db, "Test", "https://example.com", "", "", "")
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	_, err = db.AddArticle(model.Article{BlogID: blog.ID, Title: "Title", URL: "https://example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	articles, blogNames, err := GetArticles(db, false, "", "")
	if err != nil {
		t.Fatalf("get articles: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected article")
	}
	if blogNames[blog.ID] != blog.Name {
		t.Fatalf("expected blog name")
	}

	if _, _, err := GetArticles(db, false, "Missing", ""); err == nil {
		t.Fatalf("expected blog not found error")
	}
}
```

Also update `TestArticleReadUnread` — it calls `AddBlog` with 5 args:

```go
blog, err := AddBlog(db, "Test", "https://example.com", "", "", "")
```

- [ ] **Step 6: Run all controller tests**

```bash
go test ./internal/controller/... -v
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/controller/controller.go internal/controller/controller_test.go
git commit -m "feat: update AddBlog to accept categoryName, fix ListBlogs/ListArticles call sites"
```

---

### Task 6: Controller — EditBlogCategory + GetCategories + GetArticles with category

**Files:**
- Modify: `internal/controller/controller.go`
- Modify: `internal/controller/controller_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/controller/controller_test.go`:

```go
func TestEditBlogCategory(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := AddBlog(db, "Test", "https://example.com", "", "", "")
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	if blog.CategoryID != nil {
		t.Fatalf("expected no category initially")
	}

	// Assign category
	updated, err := EditBlogCategory(db, "Test", "tech")
	if err != nil {
		t.Fatalf("edit category: %v", err)
	}
	if updated.CategoryID == nil {
		t.Fatalf("expected CategoryID after edit")
	}

	// Remove category
	cleared, err := EditBlogCategory(db, "Test", "")
	if err != nil {
		t.Fatalf("clear category: %v", err)
	}
	if cleared.CategoryID != nil {
		t.Fatalf("expected CategoryID nil after clearing")
	}

	// Non-existent blog
	if _, err := EditBlogCategory(db, "Missing", "tech"); err == nil {
		t.Fatalf("expected BlogNotFoundError")
	}
}

func TestGetCategories(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	cats, err := GetCategories(db)
	if err != nil {
		t.Fatalf("get categories: %v", err)
	}
	if len(cats) != 0 {
		t.Fatalf("expected empty categories")
	}

	_, err = AddBlog(db, "TechBlog", "https://tech.example.com", "", "", "tech")
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	cats, err = GetCategories(db)
	if err != nil {
		t.Fatalf("get categories: %v", err)
	}
	if len(cats) != 1 || cats[0].Name != "tech" || cats[0].BlogCount != 1 {
		t.Fatalf("unexpected categories: %+v", cats)
	}
}

func TestGetArticlesWithCategory(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	techBlog, err := AddBlog(db, "TechBlog", "https://tech.example.com", "", "", "tech")
	if err != nil {
		t.Fatalf("add tech blog: %v", err)
	}
	otherBlog, err := AddBlog(db, "Other", "https://other.example.com", "", "", "")
	if err != nil {
		t.Fatalf("add other blog: %v", err)
	}
	_, err = db.AddArticle(model.Article{BlogID: techBlog.ID, Title: "Tech Post", URL: "https://tech.example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}
	_, err = db.AddArticle(model.Article{BlogID: otherBlog.ID, Title: "Other Post", URL: "https://other.example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	articles, _, err := GetArticles(db, false, "", "tech")
	if err != nil {
		t.Fatalf("get articles by category: %v", err)
	}
	if len(articles) != 1 || articles[0].Title != "Tech Post" {
		t.Fatalf("expected 1 tech article, got %d", len(articles))
	}

	// Unknown category → empty list, no error
	articles, _, err = GetArticles(db, false, "", "unknown")
	if err != nil {
		t.Fatalf("unexpected error for unknown category: %v", err)
	}
	if len(articles) != 0 {
		t.Fatalf("expected empty list for unknown category")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/controller/... -run "TestEditBlogCategory|TestGetCategories|TestGetArticlesWithCategory" -v
```

Expected: FAIL — functions undefined.

- [ ] **Step 3: Add EditBlogCategory, GetCategories, and update GetArticles**

Add to `internal/controller/controller.go`:

```go
func EditBlogCategory(db *storage.Database, blogName string, categoryName string) (model.Blog, error) {
	blog, err := db.GetBlogByName(blogName)
	if err != nil {
		return model.Blog{}, err
	}
	if blog == nil {
		return model.Blog{}, BlogNotFoundError{Name: blogName}
	}

	if categoryName == "" {
		blog.CategoryID = nil
	} else {
		cat, err := db.GetOrCreateCategory(categoryName)
		if err != nil {
			return model.Blog{}, err
		}
		blog.CategoryID = &cat.ID
	}

	if err := db.UpdateBlog(*blog); err != nil {
		return model.Blog{}, err
	}
	return *blog, nil
}

func GetCategories(db *storage.Database) ([]model.Category, error) {
	return db.ListCategories()
}
```

Replace `GetArticles` in `internal/controller/controller.go`:

```go
func GetArticles(db *storage.Database, showAll bool, blogName string, categoryName string) ([]model.Article, map[int64]string, error) {
	var blogID *int64
	if blogName != "" {
		blog, err := db.GetBlogByName(blogName)
		if err != nil {
			return nil, nil, err
		}
		if blog == nil {
			return nil, nil, BlogNotFoundError{Name: blogName}
		}
		blogID = &blog.ID
	}

	var categoryID *int64
	if categoryName != "" {
		cat, err := db.GetCategoryByName(categoryName)
		if err != nil {
			return nil, nil, err
		}
		if cat == nil {
			return []model.Article{}, map[int64]string{}, nil
		}
		categoryID = &cat.ID
	}

	articles, err := db.ListArticles(!showAll, blogID, categoryID)
	if err != nil {
		return nil, nil, err
	}
	blogs, err := db.ListBlogs(nil)
	if err != nil {
		return nil, nil, err
	}
	blogNames := make(map[int64]string)
	for _, blog := range blogs {
		blogNames[blog.ID] = blog.Name
	}

	return articles, blogNames, nil
}
```

- [ ] **Step 4: Run all controller tests**

```bash
go test ./internal/controller/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/controller.go internal/controller/controller_test.go
git commit -m "feat: add EditBlogCategory, GetCategories, update GetArticles with category filter"
```

---

### Task 7: CLI — fix call sites + add --category to add command

**Files:**
- Modify: `internal/cli/commands.go`

- [ ] **Step 1: Fix AddBlog call site in newAddCommand**

In `internal/cli/commands.go`, the `newAddCommand` function:

1. Add `var category string` alongside the existing flag variables.
2. Update the `controller.AddBlog` call to pass `category` as the last argument.
3. Register the new flag.

Replace the entire `newAddCommand` function:

```go
func newAddCommand() *cobra.Command {
	var feedURL string
	var scrapeSelector string
	var category string

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
			_, err = controller.AddBlog(db, name, url, feedURL, scrapeSelector, category)
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
	return cmd
}
```

- [ ] **Step 2: Fix GetArticles call sites**

In `internal/cli/commands.go`, there are two calls to `controller.GetArticles`:

In `newArticlesCommand`:
```go
// Change:
articles, blogNames, err := controller.GetArticles(db, showAll, blogName)
// To:
articles, blogNames, err := controller.GetArticles(db, showAll, blogName, "")
```

In `newReadAllCommand`:
```go
// Change:
articles, blogNames, err := controller.GetArticles(db, false, blogName)
// To:
articles, blogNames, err := controller.GetArticles(db, false, blogName, "")
```

- [ ] **Step 3: Verify everything compiles and existing tests pass**

```bash
go build ./...
go test ./...
```

Expected: all PASS, no compile errors.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/commands.go
git commit -m "feat: add --category flag to add command, fix GetArticles call sites"
```

---

### Task 8: CLI — new edit and categories commands

**Files:**
- Modify: `internal/cli/commands.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Add newEditCommand to commands.go**

Add this function to `internal/cli/commands.go`:

```go
func newEditCommand() *cobra.Command {
	var category string

	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a tracked blog.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !cmd.Flags().Changed("category") {
				return fmt.Errorf("specify at least one field to edit (e.g. --category)")
			}
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			_, err = controller.EditBlogCategory(db, name, category)
			if err != nil {
				printError(err)
				return markError(err)
			}
			color.New(color.FgGreen).Printf("Updated blog '%s'\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "Assign to category (empty string removes category)")
	return cmd
}
```

- [ ] **Step 2: Add newCategoriesCommand to commands.go**

Add this function to `internal/cli/commands.go`:

```go
func newCategoriesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "categories",
		Short: "List all categories.",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			categories, err := controller.GetCategories(db)
			if err != nil {
				return err
			}
			if len(categories) == 0 {
				fmt.Println("No categories yet.")
				return nil
			}
			color.New(color.FgCyan, color.Bold).Printf("Categories (%d):\n\n", len(categories))
			for _, cat := range categories {
				blogWord := "blogs"
				if cat.BlogCount == 1 {
					blogWord = "blog"
				}
				color.New(color.FgWhite, color.Bold).Printf("  %s", cat.Name)
				fmt.Printf("  %d %s\n", cat.BlogCount, blogWord)
			}
			return nil
		},
	}
	return cmd
}
```

- [ ] **Step 3: Register both commands in root.go**

In `internal/cli/root.go`, add both new commands inside `NewRootCommand`:

```go
rootCmd.AddCommand(newEditCommand())
rootCmd.AddCommand(newCategoriesCommand())
```

Place them after `newRemoveCommand()` and before `newBlogsCommand()` to follow natural CLI ordering.

- [ ] **Step 4: Build and smoke test**

```bash
go build -o blogwatcher ./cmd/blogwatcher
./blogwatcher edit --help
./blogwatcher categories --help
```

Expected: both show help text without errors.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/commands.go internal/cli/root.go
git commit -m "feat: add edit and categories commands"
```

---

### Task 9: CLI — add --category filter to blogs and articles commands

**Files:**
- Modify: `internal/cli/commands.go`

- [ ] **Step 1: Add --category to newBlogsCommand**

Replace `newBlogsCommand` in `internal/cli/commands.go`:

```go
func newBlogsCommand() *cobra.Command {
	var categoryName string

	cmd := &cobra.Command{
		Use:   "blogs",
		Short: "List all tracked blogs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()

			var categoryID *int64
			if categoryName != "" {
				cat, err := db.GetCategoryByName(categoryName)
				if err != nil {
					return err
				}
				if cat != nil {
					categoryID = &cat.ID
				} else {
					// unknown category → empty list
					fmt.Println("No blogs tracked yet. Use 'blogwatcher add' to add one.")
					return nil
				}
			}

			blogs, err := db.ListBlogs(categoryID)
			if err != nil {
				return err
			}
			if len(blogs) == 0 {
				fmt.Println("No blogs tracked yet. Use 'blogwatcher add' to add one.")
				return nil
			}
			color.New(color.FgCyan, color.Bold).Printf("Tracked blogs (%d):\n\n", len(blogs))
			for _, blog := range blogs {
				color.New(color.FgWhite, color.Bold).Printf("  %s\n", blog.Name)
				fmt.Printf("    URL: %s\n", blog.URL)
				if blog.FeedURL != "" {
					fmt.Printf("    Feed: %s\n", blog.FeedURL)
				}
				if blog.ScrapeSelector != "" {
					fmt.Printf("    Selector: %s\n", blog.ScrapeSelector)
				}
				if blog.LastScanned != nil {
					fmt.Printf("    Last scanned: %s\n", blog.LastScanned.Format("2006-01-02 15:04"))
				}
				fmt.Println()
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&categoryName, "category", "c", "", "Filter blogs by category")
	return cmd
}
```

- [ ] **Step 2: Add --category to newArticlesCommand**

In `newArticlesCommand`, add `var categoryName string` alongside `showAll` and `blogName`, then:

1. Update `GetArticles` call to pass `categoryName`:

```go
articles, blogNames, err := controller.GetArticles(db, showAll, blogName, categoryName)
```

2. Register the flag:

```go
cmd.Flags().StringVarP(&categoryName, "category", "c", "", "Filter articles by category")
```

- [ ] **Step 3: Build and run all tests**

```bash
go build ./...
go test ./...
```

Expected: all PASS, no compile errors.

- [ ] **Step 4: Manual smoke test**

```bash
./blogwatcher add myblog https://example.com --category tech
./blogwatcher categories
./blogwatcher blogs --category tech
./blogwatcher scan myblog
./blogwatcher articles --category tech
./blogwatcher edit myblog --category design
./blogwatcher categories
./blogwatcher edit myblog --category ""
```

Expected: commands run without errors, categories output shows correct blog counts.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/commands.go
git commit -m "feat: add --category filter to blogs and articles commands"
```
