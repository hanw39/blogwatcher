package controller

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hanw39/blogwatcher/internal/model"
	"github.com/hanw39/blogwatcher/internal/storage"
)

func TestAddBlogAndRemoveBlog(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := AddBlog(db, "Test", "https://example.com", "", "", "", false)
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}

	if _, err := AddBlog(db, "Test", "https://other.com", "", "", "", false); err == nil {
		t.Fatalf("expected duplicate name error")
	}

	if _, err := AddBlog(db, "Other", "https://example.com", "", "", "", false); err == nil {
		t.Fatalf("expected duplicate url error")
	}

	if err := RemoveBlog(db, blog.Name); err != nil {
		t.Fatalf("remove blog: %v", err)
	}
}

func TestAddBlogWithCategory(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := AddBlog(db, "TechBlog", "https://tech.example.com", "", "", "tech", false)
	if err != nil {
		t.Fatalf("add blog with category: %v", err)
	}
	if blog.CategoryID == nil {
		t.Fatalf("expected CategoryID to be set")
	}

	// Same category again — must reuse existing, not error
	blog2, err := AddBlog(db, "TechBlog2", "https://tech2.example.com", "", "", "tech", false)
	if err != nil {
		t.Fatalf("add second blog with same category: %v", err)
	}
	if blog2.CategoryID == nil || *blog2.CategoryID != *blog.CategoryID {
		t.Fatalf("expected same category ID for both blogs")
	}
}

func TestArticleReadUnread(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := AddBlog(db, "Test", "https://example.com", "", "", "", false)
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	article, err := db.AddArticle(model.Article{BlogID: blog.ID, Title: "Title", URL: "https://example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	// First mark-read: was not read before.
	_, wasAlreadyRead, err := MarkArticleRead(db, article.ID)
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if wasAlreadyRead {
		t.Fatalf("expected wasAlreadyRead=false on first mark")
	}

	// Second mark-read: already read.
	_, wasAlreadyRead, err = MarkArticleRead(db, article.ID)
	if err != nil {
		t.Fatalf("mark read 2: %v", err)
	}
	if !wasAlreadyRead {
		t.Fatalf("expected wasAlreadyRead=true on second mark")
	}

	// Mark unread.
	_, wasAlreadyUnread, err := MarkArticleUnread(db, article.ID)
	if err != nil {
		t.Fatalf("mark unread: %v", err)
	}
	if wasAlreadyUnread {
		t.Fatalf("expected wasAlreadyUnread=false after read")
	}
}

func TestGetArticlesFilters(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := AddBlog(db, "Test", "https://example.com", "", "", "", false)
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	_, err = db.AddArticle(model.Article{BlogID: blog.ID, Title: "Title", URL: "https://example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	articles, blogNames, _, err := GetArticles(db, false, "", "")
	if err != nil {
		t.Fatalf("get articles: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected article")
	}
	if blogNames[blog.ID] != blog.Name {
		t.Fatalf("expected blog name")
	}

	if _, _, _, err := GetArticles(db, false, "Missing", ""); err == nil {
		t.Fatalf("expected blog not found error")
	}
}

func TestEditBlogCategory(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := AddBlog(db, "Test", "https://example.com", "", "", "", false)
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

	_, err = AddBlog(db, "TechBlog", "https://tech.example.com", "", "", "tech", false)
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

	techBlog, err := AddBlog(db, "TechBlog", "https://tech.example.com", "", "", "tech", false)
	if err != nil {
		t.Fatalf("add tech blog: %v", err)
	}
	otherBlog, err := AddBlog(db, "Other", "https://other.example.com", "", "", "", false)
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

	articles, _, _, err := GetArticles(db, false, "", "tech")
	if err != nil {
		t.Fatalf("get articles by category: %v", err)
	}
	if len(articles) != 1 || articles[0].Title != "Tech Post" {
		t.Fatalf("expected 1 tech article, got %d", len(articles))
	}

	// Unknown category → empty list, no error
	articles, _, _, err = GetArticles(db, false, "", "unknown")
	if err != nil {
		t.Fatalf("unexpected error for unknown category: %v", err)
	}
	if len(articles) != 0 {
		t.Fatalf("expected empty list for unknown category")
	}
}

func TestMarkArticleReadOnEphemeralBlogAfterDayResets(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	blog, err := db.AddBlog(model.Blog{Name: "Eph", URL: "https://eph.example.com", IsEphemeral: true})
	if err != nil {
		t.Fatalf("add blog: %v", err)
	}
	article, err := db.AddArticle(model.Article{BlogID: blog.ID, Title: "T", URL: "https://eph.example.com/1"})
	if err != nil {
		t.Fatalf("add article: %v", err)
	}

	// Backdate read_at to yesterday using raw SQL via Exec helper.
	yesterday := time.Now().Add(-26 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE articles SET read_at = ?, is_read = 1 WHERE id = ?`, yesterday, article.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	updated, wasAlreadyRead, err := MarkArticleRead(db, article.ID)
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
	db := openTestDB(t)
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

	_, blogNames, blogEph, err := GetArticles(db, true, "", "")
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

func openTestDB(t *testing.T) *storage.Database {
	t.Helper()
	path := filepath.Join(t.TempDir(), "blogwatcher.db")
	db, err := storage.OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	return db
}
