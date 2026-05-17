---
name: blogwatcher
description: Monitor blogs and RSS/Atom feeds for updates using the blogwatcher CLI.
---

# blogwatcher

Track blog and RSS/Atom feed updates with the `blogwatcher` CLI.

Install

- Go (global):
  `go install github.com/hanw39/blogwatcher/cmd/blogwatcher@latest`
- Go (China mainland), CMD:
  `set GOPROXY=https://goproxy.cn,direct && set GOSUMDB=sum.golang.google.cn && go install github.com/hanw39/blogwatcher/cmd/blogwatcher@latest`
- Go (China mainland), PowerShell:
  `$env:GOPROXY="https://goproxy.cn,direct"; $env:GOSUMDB="sum.golang.google.cn"; go install github.com/hanw39/blogwatcher/cmd/blogwatcher@latest`
- Homebrew (Linux/macOS): `brew install hanw39/tap/blogwatcher`

Quick start

- `blogwatcher --help`

Common commands

- Add a blog: `blogwatcher add "My Blog" https://example.com`
- Add with explicit feed: `blogwatcher add "My Blog" https://example.com --feed-url https://example.com/rss.xml`
- Add with category: `blogwatcher add "My Blog" https://example.com -c engineering`
- Add ephemeral (daily-reset) blog: `blogwatcher add "GitHub Trending" https://github.com/trending --ephemeral --scrape-selector "h3 a"`
- List blogs: `blogwatcher blogs`
- List blogs by category: `blogwatcher blogs -c engineering`
- Edit a blog: `blogwatcher edit "My Blog" -c research`
- Mark a blog as ephemeral: `blogwatcher edit "GitHub Trending" --ephemeral`
- Unmark ephemeral: `blogwatcher edit "GitHub Trending" --ephemeral=false`
- List categories: `blogwatcher categories`
- Scan for updates: `blogwatcher scan`
- Scan silently (for cron): `blogwatcher scan -s`
- Scan with custom workers: `blogwatcher scan -w 4`
- List unread articles: `blogwatcher articles`
- List all articles: `blogwatcher articles -a`
- Filter articles by category: `blogwatcher articles -c engineering`
- Mark an article read: `blogwatcher read 1`
- Mark an article unread: `blogwatcher unread 1`
- Mark all unread as read: `blogwatcher read-all`
- Mark all from a blog as read: `blogwatcher read-all -b "Tech Blog" -y`
- Import from OPML: `blogwatcher import subscriptions.opml`
- Remove a blog: `blogwatcher remove "My Blog"`

Example output

```
$ blogwatcher blogs
Tracked blogs (3):

  xkcd [engineering]
    URL: https://xkcd.com

  GitHub Trending [engineering] [daily]
    URL: https://github.com/trending

  Paul Graham [uncategorized]
    URL: https://paulgraham.com/articles.html
```

```
$ blogwatcher scan
Scanning 3 blog(s)...

  xkcd
    Source: RSS | Found: 4 | New: 4

  GitHub Trending [daily]
    Source: Scrape | Found: 25 | New: 2 | Refreshed: 23

  Paul Graham
    Source: Scrape | Found: 3 | New: 1

Found 3 new article(s) total!
```

```
$ blogwatcher categories
Categories (2):

  engineering  1 blog
  research  1 blog
```

Database

- Default path: `~/.blogwatcher/blogwatcher.db`
- Override via `BLOGWATCHER_DB`: `BLOGWATCHER_DB=/tmp/isolated.db blogwatcher scan`

Notes

- Ephemeral blogs are for daily-rotating sources (e.g. GitHub Trending). Read articles return to "unread" on the next local day, and the scanner refreshes their `discovered_date` so they appear at the top.
- Use `blogwatcher <command> --help` to discover flags and options.
- HTML scraping fallback is available via `--scrape-selector` for blogs without RSS feeds.
- OPML import supports OPML 1.0/2.0 from Feedly, Inoreader, and other feed readers. Duplicate blogs are reported but not re-added.
