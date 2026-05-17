# 短时博客（Ephemeral Blogs）设计

**日期：** 2026-05-17

## 概述

为被追踪的博客支持两种已读语义：

- **普通博客（默认）：** 文章标记为已读后永久保持已读。（当前行为）
- **短时博客（如 GitHub Trending）：** 文章标记为已读后，到下一个本地自然日就重新回到「未读」。后续扫描到同一个 URL 在新的一天里被当作新文章对待，即便数据库里那一行没变。

按 blog 级别开关。已有博客不受影响。

---

## 数据模型

### 迁移

```sql
ALTER TABLE blogs    ADD COLUMN is_ephemeral BOOLEAN DEFAULT 0;
ALTER TABLE articles ADD COLUMN read_at      TIMESTAMP;

UPDATE articles
   SET read_at = COALESCE(discovered_date, CURRENT_TIMESTAMP)
 WHERE is_read = 1 AND read_at IS NULL;
```

在 `db.init()` 启动时执行。`ALTER` 语句要捕获并忽略「duplicate column name」错误（沿用现有 `category_id` 迁移的模式）。

旧的 `is_read` 列**保留不动**：新代码既不读也不写。它只作为回滚保险存在。

### 模型变更（`internal/model/model.go`）

```go
type Blog struct {
    // 现有字段...
    IsEphemeral bool
}

type Article struct {
    // 现有字段，但：
    // - 移除 IsRead bool
    // + 新增 ReadAt *time.Time
    ReadAt *time.Time
}
```

`IsRead` 不再是存储字段。需要 bool 的调用方根据 `ReadAt` 加所属博客的 `IsEphemeral` 动态计算（见下面「有效已读状态」）。

---

## 有效已读状态

一个小辅助函数，是**唯一**做日期计算的地方：

```go
// 在 model 包（或一个小的辅助包）中
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

SQL 端（`ListArticles` 使用）：

```sql
JOIN blogs b ON a.blog_id = b.id
WHERE (
    a.read_at IS NULL
 OR (b.is_ephemeral = 1
     AND date(a.read_at, 'localtime') < date('now', 'localtime'))
)
```

这是定义「今天」的唯一位置。未来要调整重置语义只动这里。

---

## 扫描器行为

当前行为（`internal/scanner/scanner.go`）：扫描到的 URL 如果已经在 `articles` 里就跳过。

新行为：

```
对扫描到的每篇文章：
  if URL 不在库中：
    INSERT 新行（discovered_date = now）
  else if blog.IsEphemeral：
    if 现有 discovered_date < 今天本地零点：
      UPDATE articles SET discovered_date = now WHERE id = ?
    else：
      跳过（今天已经抬过 —— 避免写放大）
  else：
    跳过（保持现状）
```

效果：一篇短时文章无论你扫多少次，**每个本地自然日最多被「抬」一次**。每 5 分钟跑一次 `scan` 是安全的。

### Storage 层 API 变化

- `GetExistingArticleURLs(urls []string) (map[string]struct{}, error)` → 返回更多信息，让扫描器无需二次查询就能判断 `discovered_date < today`。改签名返回 `map[string]ExistingArticle`：
  ```go
  type ExistingArticle struct {
      ID             int64
      DiscoveredDate *time.Time
  }
  ```
- 新增 `TouchArticlesBulk(ids []int64, ts time.Time) error` —— 一个事务里批量 `UPDATE articles SET discovered_date = ? WHERE id IN (...)`。

### ScanResult

`scanner.ScanResult` 加一个字段：

```go
type ScanResult struct {
    // 现有...
    Refreshed int  // 本轮被「抬」过 discovered_date 的文章数
}
```

普通博客的 CLI 输出不变。短时博客在 `N > 0` 时追加 ` | Refreshed: N`：

```
  github-trending
    Source: HTML | Found: 25 | New: 3 | Refreshed: 5
```

---

## CLI

### `add`

加 `--ephemeral` flag（默认 false）：

```
blogwatcher add github-trending https://github.com/trending --ephemeral
```

### `edit`

加 `--ephemeral` / `--no-ephemeral`，允许在已有博客上切换：

```
blogwatcher edit github-trending --ephemeral
blogwatcher edit github-trending --no-ephemeral
```

用 cobra 标准做法：声明 Bool flag，配合 `cmd.Flags().Changed("ephemeral")` 区分「用户没设」和「用户显式设为 false」。`edit` 在其他字段上已经这么做了。

### `blogs`

短时博客的名字后面追加 `[daily]`：

```
Tracked blogs (3):

  github-trending  [daily]
    URL: https://github.com/trending
    Category: opensource

  some-tech-blog
    ...
```

### `articles`

flag 不变。但显示层挑 `[read]` 还是 `[new]` 的代码已经知道所属博客（通过 `blogNames`），把 `controller.GetArticles` 改成同时返回 `blogIsEphemeral map[int64]bool`，`printArticle` 用 `ArticleIsRead(article, blogIsEphemeral[article.BlogID], time.Now())` 判定。

### `read` / `unread`

- `read <id>` 设置 `read_at = CURRENT_TIMESTAMP`。
- `unread <id>` 设置 `read_at = NULL`。
- 「已经是已读 / 未读」的提示语用 `ArticleIsRead` 结合博客的 `is_ephemeral` 判定，所以「昨天已读」的短时文章会正确显示为「当前未读」并被刷上新的 `read_at`。

### `read-all`

把当前未读的全部标为已读。底层查询已经把「昨天读过的短时文章」算成未读，所以 `read-all` 今天会自然地把它们重新标读。

---

## Storage / Controller API 变更

移除 `IsRead` 波及多层，逐一列清楚：

### Storage（`internal/storage/database.go`）

- `MarkArticleRead(id)`：`UPDATE articles SET read_at = CURRENT_TIMESTAMP WHERE id = ?`
- `MarkArticleUnread(id)`：`UPDATE articles SET read_at = NULL WHERE id = ?`
- `scanArticle` / `scanBlog`：把 `read_at` 扫进 `*time.Time`、`is_ephemeral` 扫进 `bool`。旧的 `is_read` 列不再被 SELECT。
- `ListArticles` 的 SQL 切换到上面「有效已读状态」里的 join + OR 写法。
- `GetExistingArticleURLs` 返回值更丰富（见「扫描器行为」）。
- 新增 `TouchArticlesBulk(ids []int64, ts time.Time) error`。
- `AddBlog` / `UpdateBlog`：INSERT/UPDATE 的列表里加上 `is_ephemeral`。

### Controller（`internal/controller/controller.go`）

`MarkArticleRead`、`MarkArticleUnread`、`MarkAllArticlesRead` 当前都基于 `article.IsRead` 分支。改造后：

```go
func MarkArticleRead(db, articleID) (model.Article, bool /*wasAlreadyRead*/, error) {
    article := db.GetArticle(articleID)
    blog := db.GetBlog(article.BlogID)
    already := model.ArticleIsRead(*article, blog.IsEphemeral, time.Now())
    if !already {
        db.MarkArticleRead(articleID)
        now := time.Now()
        article.ReadAt = &now
    }
    return *article, already, nil
}
```

`MarkArticleUnread` 对称处理。`MarkAllArticlesRead` 仍按当前逻辑循环调用 `db.MarkArticleRead(id)` —— 不需要逐行查 blog，因为 `ListArticles(unreadOnly=true, ...)` 返回的就是当前未读集合。

`GetArticles` 返回值变为 `(articles, blogNames, blogIsEphemeral, error)`。`blogIsEphemeral map[int64]bool` 从同一次 `ListBlogs` 调用里顺手填好。

`AddBlog` 和 `EditBlog*` 的签名加一个 `isEphemeral` 参数（详见下面「开放问题」）。

### CLI（`internal/cli/commands.go`）

- `printArticle` 多收一个 `blogIsEphemeral bool` 参数，用 `ArticleIsRead` 判定显示。
- `read` / `unread` 改用新的 `(article, wasAlreadyRead, err)` 返回。

---

## 不在本次范围内的事

- **per-blog 扫描频率 / `min_scan_interval`。** 调度是调用方的职责（cron、任务计划程序、systemd timer）。如果用户想让 GitHub Trending 每 6 小时扫一次、普通博客每 5 分钟扫一次，他们写两条 cron 即可。本项目不需要知道挂钟节奏。
- **非「按日」的重置周期（小时、周、月）。** 布尔 `is_ephemeral` 只能表达「按日」。如果未来真有需要，演进路径很清晰：加一列 `ephemeral_period TEXT`，把 `is_ephemeral = 1` 的行回填为 `'daily'`，在 SQL / 辅助函数里 branch。现在不做（YAGNI）。
- **`scan --category` 过滤。** 本项目里分类是按内容主题划分的，不是按更新节奏，所以按分类过滤跟「短时源扫得少一点」这个用例对不齐。等真有需要再加。

---

## 开放问题

`controller.AddBlog` 现在已经有 6 个位置参数（db, name, url, feedURL, scrapeSelector, categoryName）。再加一个 `isEphemeral bool` 就是 7 个。值得顺手重构成 `BlogInput` 结构体吗？

- 重构本身不大，但会牵动所有 `AddBlog` 的调用方：CLI、`import` 命令、若干测试
- 不重构的代价是函数签名继续变长，调用处布尔/字符串混在一起容易出错
- **默认方案：本次只加位置参数**，保持本 spec 范围聚焦。重构如果以后觉得碍眼了，单独开一个清理 PR 做

---

## 测试

- `storage` 测试：迁移正确回填 `read_at`；`TouchArticlesBulk` 只更新指定的行；`ListArticles` 正确把「昨天已读的短时文章」当成未读返回。
- `scanner` 测试：短时博客遇到已存在 URL 时，跨过本地零点会抬一次 `discovered_date`，同一天内的后续扫描不写；普通博客仍然跳过重复。
- `model` 测试：`ArticleIsRead` 覆盖（read_at 为 nil）、（普通博客 + 任意 read_at）、（短时博客 + 当天 read_at）、（短时博客 + 昨天 read_at）、（短时博客 + read_at 恰好等于今天本地零点）。
- CLI 冒烟：`add --ephemeral`、`edit --ephemeral` / `--no-ephemeral`、`blogs` 显示 `[daily]`、短时博客在 `scan` 输出里出现 `Refreshed: N`。
