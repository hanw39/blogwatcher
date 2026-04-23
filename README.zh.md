<div align="center">

# BlogWatcher

**再也不错过任何一篇文章。追踪任意博客 — 无论有没有 RSS。**

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hanw39/blogwatcher?style=flat-square)](https://goreportcard.com/report/github.com/hanw39/blogwatcher)
[![GitHub release](https://img.shields.io/github/v/release/hanw39/blogwatcher?style=flat-square)](https://github.com/hanw39/blogwatcher/releases)

Go 语言编写的 CLI 工具，用于追踪博客文章、发现新内容并管理已读/未读状态。<br>
支持 RSS/Atom 订阅源，无订阅源时自动降级为 HTML 抓取。

Fork 自 [Hyaxia/blogwatcher](https://github.com/Hyaxia/blogwatcher)。

[English](README.md) | 中文

</div>

---

## 快速开始

```bash
# 安装
go install github.com/hanw39/blogwatcher/cmd/blogwatcher@latest

# 添加一个博客
blogwatcher add "Paul Graham" https://paulgraham.com/articles.html

# 扫描新文章
blogwatcher scan

# 查看未读文章
blogwatcher articles
```

---

## 新增功能

> 在 [Hyaxia/blogwatcher](https://github.com/Hyaxia/blogwatcher) 基础上新增

**OPML 导入** — 从 Feedly、Inoreader 等阅读器的导出文件批量导入订阅：

```bash
blogwatcher import subscriptions.opml
```

**RSS 订阅源发现优化** — 通过 `Content-Type` 响应头和 `rel="self"` 链接更精准地识别订阅源，修复了部分博客（如 TechCrunch 标签页）无法自动发现的问题。

**分类管理** — 将博客归入命名分组，按分类过滤文章：

```bash
blogwatcher add "技术博客" https://example.com -c engineering
blogwatcher edit "技术博客" -c research
blogwatcher blogs -c engineering
blogwatcher articles -c engineering
blogwatcher categories
```

---

## 功能特性

| | |
|---|---|
| 📡 **双源支持** | 优先使用 RSS，无订阅源时自动降级为 HTML 抓取 |
| 🔍 **自动发现订阅源** | 从博客主页自动检测 RSS/Atom 地址 |
| 📥 **OPML 导入** 🆕 | 从 Feedly / Inoreader 导出文件批量导入订阅 |
| 🔗 **订阅源识别优化** 🆕 | 通过 Content-Type 和 `rel="self"` 更精准地识别订阅源 |
| 🗂️ **分类管理** | 将博客归入命名分组，按分类过滤文章 |
| ✅ **已读/未读追踪** | 记录哪些文章已经读过 |
| 🚫 **去重** | 同一篇文章永远不会重复收录 |
| ⚡ **并发扫描** | 可配置并发数，批量扫描更快 |

---

## 安装

```bash
# 通过 go install 安装
go install github.com/hanw39/blogwatcher/cmd/blogwatcher@latest

# 或本地构建
git clone https://github.com/hanw39/blogwatcher
cd blogwatcher
go build ./cmd/blogwatcher
```

Windows 和 Linux 二进制文件可在 [Releases](https://github.com/hanw39/blogwatcher/releases) 页面下载。

---

## 使用方法

### 从 OPML 导入

```bash
# 从阅读器导出文件批量导入（Feedly、Inoreader 等）
blogwatcher import subscriptions.opml
```

支持 OPML 1.0/2.0 及嵌套分类。重复的博客会在导入报告中标注，不会重复添加。

### 添加博客

```bash
# 添加博客（自动发现 RSS 订阅源）
blogwatcher add "我的博客" https://example.com/blog

# 指定 RSS 地址
blogwatcher add "技术博客" https://techblog.com --feed-url https://techblog.com/rss.xml

# 指定 HTML 抓取选择器（适用于没有 RSS 的博客）
blogwatcher add "无RSS博客" https://norss.com --scrape-selector "article h2 a"

# 添加时指定分类（分类不存在会自动创建）
blogwatcher add "技术博客" https://techblog.com -c engineering
```

### 管理博客

```bash
# 列出所有已追踪的博客
blogwatcher blogs

# 按分类过滤
blogwatcher blogs -c engineering

# 删除博客（同时删除其所有文章）
blogwatcher remove "我的博客"

# 跳过确认直接删除
blogwatcher remove "我的博客" -y
```

### 编辑博客

```bash
# 为博客指定分类
blogwatcher edit "技术博客" -c engineering

# 移除博客的分类
blogwatcher edit "技术博客" -c ""
```

### 管理分类

```bash
# 列出所有分类及其博客数量
blogwatcher categories
```

```
Categories (3):

  changelog  3 blogs
  engineering  7 blogs
  research  3 blogs
```

### 扫描新文章

```bash
# 扫描所有博客（默认 8 个并发）
blogwatcher scan

# 扫描指定博客
blogwatcher scan "技术博客"

# 自定义并发数
blogwatcher scan -w 4

# 静默模式（完成后只输出 "scan done"，适合配合系统定时任务使用）
blogwatcher scan -s
```

### 查看文章

```bash
# 列出未读文章
blogwatcher articles

# 列出所有文章（含已读）
blogwatcher articles -a

# 按博客过滤
blogwatcher articles -b "技术博客"

# 按分类过滤
blogwatcher articles -c engineering

# 组合过滤
blogwatcher articles -a -c engineering
```

### 管理已读状态

```bash
# 标记文章为已读（文章 ID 见 articles 列表）
blogwatcher read 42

# 标记为未读
blogwatcher unread 42

# 全部标记为已读
blogwatcher read-all

# 将指定博客的文章全部标记为已读
blogwatcher read-all -b "技术博客" -y
```

---

## 工作原理

### 扫描流程

1. 对每个已追踪的博客，优先尝试解析 RSS/Atom 订阅源
2. 若未配置订阅源地址，自动从博客主页发现
3. RSS 解析失败且配置了 `scrape_selector` 时，降级为 HTML 抓取
4. 新文章以未读状态存入数据库
5. 已存在的文章自动跳过

### HTML 抓取

无 RSS 时，提供匹配文章链接的 CSS 选择器：

```bash
--scrape-selector "article h2 a"    # article 标签内 h2 里的链接
--scrape-selector ".post-title a"   # post-title class 下的链接
--scrape-selector "#blog-posts a"   # blog-posts ID 下的链接
```

---

## 数据库

数据存储在 `~/.blogwatcher/blogwatcher.db`（SQLite）：

| 表 | 说明 |
|---|---|
| `categories` | 博客分类 |
| `blogs` | 已追踪的博客（名称、URL、订阅源、选择器、分类） |
| `articles` | 已发现的文章（标题、URL、日期、已读状态） |

---

## 开发

**环境要求：** Go 1.24+

```bash
# 运行测试
go test ./...

# 构建
go build ./cmd/blogwatcher
```

---

## License

MIT
