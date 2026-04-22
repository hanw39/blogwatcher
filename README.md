<div align="center">

# BlogWatcher

**Never miss a post. Track any blog — RSS or not.**

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hanw39/blogwatcher?style=flat-square)](https://goreportcard.com/report/github.com/hanw39/blogwatcher)
[![GitHub release](https://img.shields.io/github/v/release/hanw39/blogwatcher?style=flat-square)](https://github.com/hanw39/blogwatcher/releases)

A Go CLI tool to track blog articles, detect new posts, and manage read/unread status.<br>
Supports RSS/Atom feeds and HTML scraping as fallback.

Forked from [Hyaxia/blogwatcher](https://github.com/Hyaxia/blogwatcher) — adds **category support** to organize your blogs.

</div>

---

## Quick Start

```bash
# Install
go install github.com/hanw39/blogwatcher/cmd/blogwatcher@latest

# Track a blog
blogwatcher add "Paul Graham" https://paulgraham.com/articles.html

# Scan for new articles
blogwatcher scan

# Read unread articles
blogwatcher articles
```

---

## Features

| | |
|---|---|
| 📡 **Dual Source Support** | Tries RSS feeds first, falls back to HTML scraping |
| 🔍 **Auto Feed Discovery** | Detects RSS/Atom URLs from blog homepages |
| 🗂️ **Category Support** | Organize blogs into categories, filter by category |
| ✅ **Read/Unread Tracking** | Keep track of what you've read |
| 🚫 **Duplicate Prevention** | Never tracks the same article twice |
| ⚡ **Concurrent Scanning** | Configurable parallel workers |

---

## Installation

```bash
# Install the CLI
go install github.com/hanw39/blogwatcher/cmd/blogwatcher@latest

# Or build locally
git clone https://github.com/hanw39/blogwatcher
cd blogwatcher
go build ./cmd/blogwatcher
```

Windows and Linux binaries are available on the [Releases](https://github.com/hanw39/blogwatcher/releases) page.

---

## Usage

### Adding Blogs

```bash
# Add a blog (auto-discovers RSS feed)
blogwatcher add "My Favorite Blog" https://example.com/blog

# Add with explicit feed URL
blogwatcher add "Tech Blog" https://techblog.com --feed-url https://techblog.com/rss.xml

# Add with HTML scraping selector (for blogs without feeds)
blogwatcher add "No-RSS Blog" https://norss.com --scrape-selector "article h2 a"

# Add and assign to a category (created automatically if it doesn't exist)
blogwatcher add "Tech Blog" https://techblog.com -c engineering
```

### Managing Blogs

```bash
# List all tracked blogs
blogwatcher blogs

# Filter blogs by category
blogwatcher blogs -c engineering

# Remove a blog (and all its articles)
blogwatcher remove "My Favorite Blog"

# Remove without confirmation
blogwatcher remove "My Favorite Blog" -y
```

### Editing Blogs

```bash
# Assign a blog to a category
blogwatcher edit "Tech Blog" -c engineering

# Remove a blog from its category
blogwatcher edit "Tech Blog" -c ""
```

### Managing Categories

```bash
# List all categories with blog counts
blogwatcher categories
```

```
Categories (3):

  changelog  3 blogs
  engineering  7 blogs
  research  3 blogs
```

### Scanning for New Articles

```bash
# Scan all blogs (8 concurrent workers by default)
blogwatcher scan

# Scan a specific blog
blogwatcher scan "Tech Blog"

# Custom workers
blogwatcher scan -w 4

# Silent mode (outputs "scan done" when complete — useful for cron)
blogwatcher scan -s
```

### Viewing Articles

```bash
# List unread articles
blogwatcher articles

# List all articles (including read)
blogwatcher articles -a

# Filter by blog
blogwatcher articles -b "Tech Blog"

# Filter by category
blogwatcher articles -c engineering

# Combine filters
blogwatcher articles -a -c engineering
```

### Managing Read Status

```bash
# Mark an article as read (use article ID shown in articles list)
blogwatcher read 42

# Mark as unread
blogwatcher unread 42

# Mark all unread as read
blogwatcher read-all

# Mark all unread from a specific blog as read
blogwatcher read-all -b "Tech Blog" -y
```

---

## How It Works

### Scanning Process

1. For each tracked blog, BlogWatcher attempts to parse its RSS/Atom feed
2. If no feed URL is configured, it tries to auto-discover one from the blog homepage
3. If RSS parsing fails and a `scrape_selector` is configured, it falls back to HTML scraping
4. New articles are saved to the database as unread
5. Already-tracked articles are skipped

### HTML Scraping

When RSS isn't available, provide a CSS selector that matches article links:

```bash
--scrape-selector "article h2 a"    # Links inside article h2 tags
--scrape-selector ".post-title a"   # Links with post-title class
--scrape-selector "#blog-posts a"   # Links inside blog-posts ID
```

---

## Database

SQLite database at `~/.blogwatcher/blogwatcher.db`:

| Table | Description |
|---|---|
| `categories` | Blog categories |
| `blogs` | Tracked blogs (name, URL, feed URL, scrape selector, category) |
| `articles` | Discovered articles (title, URL, dates, read status) |

---

## Development

**Requirements:** Go 1.24+

```bash
# Run tests
go test ./...

# Build
go build ./cmd/blogwatcher
```

### Publishing a Release

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

---

## License

MIT
