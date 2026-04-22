package controller

import (
	"fmt"

	"github.com/hanw39/blogwatcher/internal/model"
	"github.com/hanw39/blogwatcher/internal/storage"
)

type BlogNotFoundError struct {
	Name string
}

func (e BlogNotFoundError) Error() string {
	return fmt.Sprintf("Blog '%s' not found", e.Name)
}

type BlogAlreadyExistsError struct {
	Field string
	Value string
}

func (e BlogAlreadyExistsError) Error() string {
	return fmt.Sprintf("Blog with %s '%s' already exists", e.Field, e.Value)
}

type ArticleNotFoundError struct {
	ID int64
}

func (e ArticleNotFoundError) Error() string {
	return fmt.Sprintf("Article %d not found", e.ID)
}
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

func RemoveBlog(db *storage.Database, name string) error {
	blog, err := db.GetBlogByName(name)
	if err != nil {
		return err
	}
	if blog == nil {
		return BlogNotFoundError{Name: name}
	}
	_, err = db.RemoveBlog(blog.ID)
	return err
}

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
			// Unknown category — return empty result, not an error
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

func MarkArticleRead(db *storage.Database, articleID int64) (model.Article, error) {
	article, err := db.GetArticle(articleID)
	if err != nil {
		return model.Article{}, err
	}
	if article == nil {
		return model.Article{}, ArticleNotFoundError{ID: articleID}
	}
	if !article.IsRead {
		_, err = db.MarkArticleRead(articleID)
		if err != nil {
			return model.Article{}, err
		}
	}
	return *article, nil
}

func MarkAllArticlesRead(db *storage.Database, blogName string) ([]model.Article, error) {
	var blogID *int64
	if blogName != "" {
		blog, err := db.GetBlogByName(blogName)
		if err != nil {
			return nil, err
		}
		if blog == nil {
			return nil, BlogNotFoundError{Name: blogName}
		}
		blogID = &blog.ID
	}

	articles, err := db.ListArticles(true, blogID, nil)
	if err != nil {
		return nil, err
	}

	for i := range articles {
		_, err := db.MarkArticleRead(articles[i].ID)
		if err != nil {
			return nil, err
		}
		articles[i].IsRead = true
	}

	return articles, nil
}

func MarkArticleUnread(db *storage.Database, articleID int64) (model.Article, error) {
	article, err := db.GetArticle(articleID)
	if err != nil {
		return model.Article{}, err
	}
	if article == nil {
		return model.Article{}, ArticleNotFoundError{ID: articleID}
	}
	if article.IsRead {
		_, err = db.MarkArticleUnread(articleID)
		if err != nil {
			return model.Article{}, err
		}
	}
	return *article, nil
}

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
