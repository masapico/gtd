# CLAUDE.md \- GTD Tool Development Guidelines

## 1\. Project Overview

職場のオフライン環境やプロキシ制限下でも軽快に動作する、個人用のGTD（Getting Things Done）タスク・情報管理ツール。

- **シングルバイナリ 배포**: Goの `embed` を活用し、単一の実行ファイル（＋ローカルDB・画像保存フォルダ）で完結させる。  
- **外部依存ゼロ**: CDN不使用。フロントエンドのビルドステップ（npm / Webpack / Vite等）も一切設けない。  
- **軽量・高速なUI**: `htmx` と軽量CSS（同梱）を用いた1画面完結型（3ペイン構成）で、ページリロードのない快適な操作感を提供する。

## 2\. Tech Stack & Constraints

- **Language**: Go (1.22+ 推奨。標準の `net/http` の強化されたルーティング `GET /path/{id}` 等を使用)  
- **Database**: SQLite (`modernc.org/sqlite` を使用し、CGO\_ENABLED=0 のPure Goでビルド可能にする)  
- **Templating**: Go標準 `html/template`  
- **Static Assets**: Go標準 `embed` (`//go:embed static/* templates/*`)  
- **Frontend**:  
  - `htmx.min.js` (ローカルの `static/` に配置して配信)  
  - 軽量クラスレス/マイクロCSS (`Pico.css` または `Chota` 等を `static/` に配置)  
  - Vanilla JS (クリップボードからの画像ペースト処理など、最小限の補助スクリプトのみ)

### 厳守すべき制約（Anti-Patterns / Out of Scope）

1. **CGOを使わない**: 職場環境への持ち込みやクロスコンパイルを容易にするため、`mattn/go-sqlite3` ではなく `modernc.org/sqlite` を使う。  
2. **CDNリンクを貼らない**: HTMLテンプレート内に `https://unpkg.com/...` などの外部リソースを含めない。  
3. **フロントエンドの複雑化を避ける**:  
   - ツール内での画像編集・キャンバス描画機能は実装しない（OS標準のスクショツールで加工してから `Ctrl + V` で貼る運用とする）。  
   - CSVパーサーやグラフライブラリ（Chart.js等）は導入しない。  
   - リッチテキストエディタは導入せず、プレーンテキスト（またはシンプルな表示）にとどめる。

## 3\. Recommended Directory Structure

.

├── main.go                 \# エントリーポイント、サーバー起動、embed定義

├── go.mod

├── go.sum

├── internal/

│   ├── db/

│   │   ├── db.go           \# SQLite接続、マイグレーション（テーブル初期化）

│   │   └── queries.go      \# CRUDおよび週次レビュー用クエリ

│   ├── model/

│   │   └── models.go       \# Project, Item, Context, Attachment 構造体

│   └── handler/

│       ├── views.go        \# 画面・htmxパーシャル描画ハンドラ

│       ├── items.go        \# Item操作API (CRUD, ステータス変更)

│       ├── projects.go     \# Project操作API

│       ├── review.go       \# 週次レビュー専用ウィザードハンドラ

│       └── attachments.go  \# ファイルパス起動(exec)、スクショ画像保存

├── static/

│   ├── htmx.min.js

│   ├── style.css

│   └── app.js              \# Ctrl+V スクショ貼り付け用の最小限のJS

├── templates/

│   ├── layout.html         \# 3ペイン全体レイアウト

│   ├── partials/           \# htmxで差し替える部分テンプレート群

│   │   ├── item\_list.html

│   │   ├── item\_detail.html

│   │   └── review\_step.html

└── data/                   \# 実行時に自動生成（バイナリと同階層）

    ├── gtd.db              \# SQLiteデータベースファイル

    └── images/             \# Ctrl+Vで保存されたスクリーンショット実体

## 4\. Coding Guidelines

- **htmx Partial Rendering**:  
  - ハンドラは `HX-Request` ヘッダの有無を確認するか、専用エンドポイント（`/views/...`, `/partials/...`）を用意し、必要なHTMLフラグメントのみを返すこと。  
  - 状態変更（POST/PUT/PATCH/DELETE）後は、`HX-Trigger` レスポンスヘッダを活用して関連ペイン（未処理バッジの数字や中央リストなど）を自動更新させる設計を推奨する。  
- **Local OS Integration**:  
  - ローカルフォルダやファイルパス（`C:\...` や `\\server\...`）を開く機能は、localhost環境で動作する前提のもと、Goの `os/exec`（Windowsの場合は `cmd /c start "" "<path>"` または `explorer "<path>"`）で安全に呼び出すエンドポイント `POST /attachments/{id}/open` として実装する。  
- **Step-by-Step Implementation**:  
  1. **Step 1**: DB初期化と `items` のQuick Capture（Inbox投入）・一覧・ステータス変更。  
  2. **Step 2**: `projects` と `contexts` の紐づけ、右ペイン（詳細インスペクタ）の自動保存実装。  
  3. **Step 3**: `attachments`（URL、ローカルパス起動、`Ctrl+V` スクショ保存）の実装。  
  4. **Step 4**: 週次レビュー（3ステップウィザード）の実装。