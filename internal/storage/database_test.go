package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanw39/blogwatcher/internal/model"
)

func TestDatabaseCreatesFileAndCRUD(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected db file to exist: %v", err)
	}

	blog, err := db.AddBlog(model.Blog{Name: "Test", URL: "https://example.com"})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	if blog.ID == 0 {
		t.Fatal("expected blog ID")
	}

	fetched, err := db.GetBlog(blog.ID)
	if err != nil {
		t.Fatalf("get blog: %v", err)
	}
	if fetched == nil || fetched.Name != "Test" {
		t.Fatalf("unexpected blog: %+v", fetched)
	}

	articles := []model.Article{
		{BlogID: blog.ID, Title: "One", URL: "https://example.com/1"},
		{BlogID: blog.ID, Title: "Two", URL: "https://example.com/2"},
	}
	count, err := db.AddArticlesBulk(articles)
	if err != nil {
		t.Fatalf("add articles bulk: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 articles, got %d", count)
	}

	list, err := db.ListArticles(false, nil)
	if err != nil {
		t.Fatalf("list articles: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(list))
	}

	ok, err := db.MarkArticleRead(list[0].ID)
	if err != nil || !ok {
		t.Fatalf("mark read: %v", err)
	}

	updated, err := db.GetArticle(list[0].ID)
	if err != nil {
		t.Fatalf("get article: %v", err)
	}
	if updated == nil || !updated.IsRead {
		t.Fatalf("expected article read: %+v", updated)
	}

	now := time.Now()
	if err := db.UpdateBlogLastScanned(blog.ID, now); err != nil {
		t.Fatalf("update last scanned: %v", err)
	}

	deleted, err := db.RemoveBlog(blog.ID)
	if err != nil {
		t.Fatalf("remove blog: %v", err)
	}
	if !deleted {
		t.Fatalf("expected blog removal")
	}
}

func TestGetExistingArticleURLs(t *testing.T) {
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

	_, err = db.AddArticle(model.Article{BlogID: blog.ID, Title: "One", URL: "https://example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	existing, err := db.GetExistingArticleURLs([]string{"https://example.com/1", "https://example.com/2"})
	if err != nil {
		t.Fatalf("get existing: %v", err)
	}
	if _, ok := existing["https://example.com/1"]; !ok {
		t.Fatalf("expected existing url")
	}
	if _, ok := existing["https://example.com/2"]; ok {
		t.Fatalf("did not expect url")
	}
}

func TestDatabaseForeignKeyEnforced(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if _, err := db.AddArticle(model.Article{BlogID: 9999, Title: "Orphan", URL: "https://example.com/orphan"}); err == nil {
		t.Fatalf("expected foreign key error for missing blog")
	}
}

func TestBlogOptionalFieldsRoundTrip(t *testing.T) {
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

	fetched, err := db.GetBlog(blog.ID)
	if err != nil {
		t.Fatalf("get blog: %v", err)
	}
	if fetched == nil {
		t.Fatalf("expected blog")
	}
	if fetched.FeedURL != "" || fetched.ScrapeSelector != "" {
		t.Fatalf("expected empty optional fields: %+v", fetched)
	}
}

func TestBlogTimeRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	now := time.Date(2025, 1, 2, 3, 4, 5, 6, time.UTC)
	blog, err := db.AddBlog(model.Blog{
		Name:        "Test",
		URL:         "https://example.com",
		LastScanned: &now,
	})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	fetched, err := db.GetBlog(blog.ID)
	if err != nil {
		t.Fatalf("get blog: %v", err)
	}
	if fetched == nil || fetched.LastScanned == nil {
		t.Fatalf("expected last scanned")
	}
	if !fetched.LastScanned.Equal(now) {
		t.Fatalf("expected last scanned %s, got %s", now.Format(time.RFC3339Nano), fetched.LastScanned.Format(time.RFC3339Nano))
	}
}

func TestArticleTimeRoundTripAndNilDiscoveredDate(t *testing.T) {
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

	published := time.Date(2024, 12, 31, 23, 59, 59, 123, time.UTC)
	article, err := db.AddArticle(model.Article{
		BlogID:        blog.ID,
		Title:         "Title",
		URL:           "https://example.com/1",
		PublishedDate: &published,
	})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	fetched, err := db.GetArticle(article.ID)
	if err != nil {
		t.Fatalf("get article: %v", err)
	}
	if fetched == nil || fetched.PublishedDate == nil {
		t.Fatalf("expected published date")
	}
	if !fetched.PublishedDate.Equal(published) {
		t.Fatalf("expected published date %s, got %s", published.Format(time.RFC3339Nano), fetched.PublishedDate.Format(time.RFC3339Nano))
	}
	if fetched.DiscoveredDate != nil {
		t.Fatalf("expected discovered date nil when not set")
	}
}

func TestListArticlesFiltersAndOrdering(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	blogA, err := db.AddBlog(model.Blog{Name: "A", URL: "https://a.example.com"})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	blogB, err := db.AddBlog(model.Blog{Name: "B", URL: "https://b.example.com"})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)

	first, err := db.AddArticle(model.Article{BlogID: blogA.ID, Title: "Old", URL: "https://a.example.com/old", DiscoveredDate: &t1})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}
	second, err := db.AddArticle(model.Article{BlogID: blogA.ID, Title: "New", URL: "https://a.example.com/new", DiscoveredDate: &t2})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}
	_, err = db.AddArticle(model.Article{BlogID: blogB.ID, Title: "Other", URL: "https://b.example.com/1", DiscoveredDate: &t2})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	if _, err := db.MarkArticleRead(first.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	all, err := db.ListArticles(false, nil)
	if err != nil {
		t.Fatalf("list articles: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 articles, got %d", len(all))
	}
	if all[0].ID != second.ID {
		t.Fatalf("expected newest article first")
	}

	unread, err := db.ListArticles(true, nil)
	if err != nil {
		t.Fatalf("list unread: %v", err)
	}
	if len(unread) != 2 {
		t.Fatalf("expected 2 unread articles, got %d", len(unread))
	}

	blogID := blogB.ID
	filtered, err := db.ListArticles(false, &blogID)
	if err != nil {
		t.Fatalf("list by blog: %v", err)
	}
	if len(filtered) != 1 || filtered[0].BlogID != blogB.ID {
		t.Fatalf("expected one article for blog B")
	}
}

func TestBulkInsertDuplicateRollbackAndEmpty(t *testing.T) {
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

	if count, err := db.AddArticlesBulk(nil); err != nil || count != 0 {
		t.Fatalf("expected empty bulk insert to be no-op, got %d, %v", count, err)
	}

	_, err = db.AddArticle(model.Article{BlogID: blog.ID, Title: "Existing", URL: "https://example.com/existing"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	dupArticles := []model.Article{
		{BlogID: blog.ID, Title: "Dup", URL: "https://example.com/dup"},
		{BlogID: blog.ID, Title: "Dup2", URL: "https://example.com/dup"},
	}
	if _, err := db.AddArticlesBulk(dupArticles); err == nil {
		t.Fatalf("expected bulk insert to fail on duplicate url")
	}

	articles, err := db.ListArticles(false, nil)
	if err != nil {
		t.Fatalf("list articles: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected rollback on duplicate, got %d articles", len(articles))
	}
}

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

func TestLookupHelpers(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blogwatcher.db")
	db, err := OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if blog, err := db.GetBlogByName("missing"); err != nil || blog != nil {
		t.Fatalf("expected missing blog by name")
	}
	if blog, err := db.GetBlogByURL("https://missing.example.com"); err != nil || blog != nil {
		t.Fatalf("expected missing blog by url")
	}

	blog, err := db.AddBlog(model.Blog{Name: "Test", URL: "https://example.com"})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	article, err := db.AddArticle(model.Article{BlogID: blog.ID, Title: "Title", URL: "https://example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	if found, err := db.GetArticleByURL(article.URL); err != nil || found == nil {
		t.Fatalf("expected article by url")
	}
	if exists, err := db.ArticleExists(article.URL); err != nil || !exists {
		t.Fatalf("expected article to exist")
	}
	if exists, err := db.ArticleExists("https://example.com/missing"); err != nil || exists {
		t.Fatalf("expected missing article to not exist")
	}
}
