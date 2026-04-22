# Blog Categories Design

**Date:** 2026-04-22
**Issue:** https://github.com/hanw39/blogwatcher/issues/5

## Overview

Add category support to blogwatcher. Each blog belongs to at most one category. Categories are maintained in a dedicated `categories` table and are created automatically when first assigned to a blog. Empty categories are preserved.

---

## Data Model

### New table: `categories`

```sql
CREATE TABLE IF NOT EXISTS categories (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);
```

### Migration: `blogs` table

```sql
ALTER TABLE blogs ADD COLUMN category_id INTEGER REFERENCES categories(id);
```

Run at startup in `db.init()`. SQLite will error if the column already exists, so the migration must catch and ignore the "duplicate column name" error specifically.

### Go model changes

```go
// model.go

type Blog struct {
    ID             int64
    Name           string
    URL            string
    FeedURL        string
    ScrapeSelector string
    LastScanned    *time.Time
    CategoryID     *int64  // nil = uncategorized
}

type Category struct {
    ID        int64
    Name      string
    BlogCount int  // populated at query time, not stored
}
```

---

## CLI

### Modified commands

```
blogwatcher add <name> <url> [--category <cat>]
```
- If `--category` is provided and the category does not exist, it is created automatically.

```
blogwatcher blogs [--category <cat>]
```
- Without `--category`: lists all blogs (existing behavior).
- With `--category`: filters to blogs in that category.

```
blogwatcher articles [--all] [--blog <name>] [--category <cat>]
```
- `--category` filters articles to blogs belonging to that category.
- Can be combined with `--blog` and `--all`.

### New commands

```
blogwatcher edit <name> --category <cat>
```
- Updates the category of an existing blog.
- `--category ""` removes the blog from its current category (sets `category_id` to NULL).
- If the named category does not exist, it is created automatically.
- Scope: category only. Other fields (URL, feed-url, etc.) are out of scope for this feature.

```
blogwatcher categories
```
- Lists all categories with their blog count.
- Includes empty categories.

Example output:
```
Categories (3):

  tech        3 blogs
  design      1 blog
  personal    0 blogs
```

---

## Architecture

### `storage` (`database.go`)

- `init()`: create `categories` table; run `ALTER TABLE blogs ADD COLUMN category_id` migration.
- New: `GetOrCreateCategory(name string) (model.Category, error)`
- New: `ListCategories() ([]model.Category, error)` — includes blog count via LEFT JOIN
- New: `GetCategoryByName(name string) (*model.Category, error)`
- Modified: `AddBlog`, `UpdateBlog`, `scanBlog` — handle `category_id`
- Modified: `ListBlogs` — accept optional `categoryID *int64` filter

### `model` (`model.go`)

- Add `CategoryID *int64` to `Blog`
- Add `Category` struct

### `controller` (`controller.go`)

- `AddBlog`: accept `categoryName string`; if non-empty, call `GetOrCreateCategory` and set `CategoryID`
- New: `EditBlogCategory(db, blogName, categoryName string) (model.Blog, error)`
- New: `GetCategories(db) ([]model.Category, error)`
- `GetArticles`: accept `categoryName string`; if non-empty, resolve to `categoryID` and filter

### `cli` (`commands.go`, `root.go`)

- `newAddCommand`: add `--category` / `-c` flag
- New: `newEditCommand`: `edit <name> --category <cat>`
- New: `newCategoriesCommand`: `categories`
- `newBlogsCommand`: add `--category` / `-c` flag
- `newArticlesCommand`: add `--category` / `-c` flag

---

## Error Handling

| Scenario | Behavior |
|---|---|
| `edit` on non-existent blog | Return `BlogNotFoundError` |
| `blogs --category` with unknown category | Return empty list (not an error) |
| `articles --category` with unknown category | Return empty list (not an error) |
| `edit --category ""` when blog has no category | No-op, return success |

---

## Out of Scope

- Renaming categories
- `edit` modifying fields other than `category`
- Category descriptions or metadata
