# CLAUDE.md - GTD Tool Development Guidelines

## 1. Project Overview

職場のオフライン環境やプロキシ制限下でも軽快に動作する、個人用の GTD（Getting Things Done）タスク・情報管理ツール。

- **シングルバイナリ配布**: Go の `embed` で静的ファイル・テンプレートを同梱し、単一の実行ファイル（＋実行時に生成される `data/`）で完結させる。
- **外部依存ゼロ**: CDN 不使用。フロントエンドのビルドステップ（npm / Webpack / Vite 等）は一切設けない。
- **軽量・高速な UI**: `htmx` と同梱 CSS による 1 画面完結型（3 ペイン構成）。ページリロードなしで操作できること。
- **localhost 前提**: 既定で `127.0.0.1:8080` にバインドする個人利用ツール。ログイン認証は実装しない方針（経緯は `PLAN.md`）。

関連ドキュメント:

- `DESIGN.md` — 設計思想、DB スキーマ、画面仕様（UI 変更・スキーマ変更の前に読むこと）
- `PLAN.md` — 機能計画と実装状況のチェックリスト、設計判断メモ
- `README.md` — ユーザー向けの機能説明

## 2. Commands

```sh
go run .                 # 開発サーバー起動（PORT / HOST 環境変数で変更可）
go test ./...            # 全テスト
go build -o gtd .        # ビルド（CGO_ENABLED=0 でも通ること）
./gtd backup             # data/ を無停止で ./backup/*.zip にスナップショット
go run ./cmd/release v1.0.0 [-os windows] [-arch amd64]   # クロスビルドして dist/ に zip 出力
go run ./cmd/genfavicon  # static/ のファビコン類を再生成
```

## 3. Tech Stack

- **Language**: Go（`go.mod` 参照）。標準 `net/http` の拡張ルーティング（`GET /path/{id}` 等）を使う。ルーターは `main.go` の `setupRouter`。
- **Database**: SQLite（`modernc.org/sqlite`、Pure Go）
- **Templating**: 標準 `html/template`
- **Static Assets**: 標準 `embed`（`main.go` で `static/*` と `templates/*` を埋め込み）
- **Frontend**:
  - `static/htmx.min.js`（ローカル同梱）
  - `static/style.css` — Material Design 3 準拠の自前 CSS（トークンは `:root` に定義）
  - `static/app.js` — Vanilla JS（Ctrl+V 画像貼り付け、ショートカット等の最小限の補助）

## 4. Hard Constraints（厳守）

1. **CGO を使わない**: クロスコンパイルと職場への持ち込みを容易にするため、`mattn/go-sqlite3` ではなく `modernc.org/sqlite` を使う。
2. **外部リソースを読み込まない**: テンプレートに `https://unpkg.com/...` 等の CDN リンクや外部フォントを含めない。必要なものは `static/` に同梱する。
3. **フロントエンドを複雑化しない**:
   - 画像編集・キャンバス描画は実装しない（OS のスクショツールで加工 → `Ctrl+V` で貼る運用）。
   - CSV パーサーやグラフライブラリ（Chart.js 等）は導入しない。
   - リッチテキストエディタは導入しない（プレーンテキスト / 簡易表示のみ）。
   - npm パッケージやビルド工程を追加しない。
4. **UI に絵文字を使わない**: アイコンはインライン SVG 等を使う。見た目は Material Design 3 に合わせる。

## 5. Directory Structure

```
.
├── main.go                  # エントリーポイント、embed 定義、ルーター、backup サブコマンド
├── cmd/
│   ├── release/             # クロスビルド＆zip パッケージング
│   └── genfavicon/          # ファビコン生成
├── internal/
│   ├── db/
│   │   ├── db.go            # SQLite 接続、マイグレーション
│   │   └── queries.go       # CRUD、週次レビュー用クエリ
│   ├── model/models.go      # Project, Item, Context, Attachment 等の構造体
│   ├── smartadd/parser.go   # Quick Capture の Smart Add 記法パーサー（~日付 など）
│   ├── backup/              # オンラインバックアップ（`gtd backup`）
│   └── handler/
│       ├── handler.go       # Handler 構造体、共通処理
│       ├── views.go         # 画面・htmx パーシャル描画
│       ├── items.go         # Item 操作 API（CRUD、ステータス変更）
│       ├── projects.go      # Project 操作 API
│       ├── review.go        # 週次レビュー（3 ステップウィザード）
│       ├── attachments.go   # 添付（URL、ローカルパス起動、スクショ保存）
│       ├── launchers.go     # アプリケーションランチャー
│       └── search.go        # オムニサーチ
├── static/                  # htmx.min.js, style.css, app.js, favicon 類
├── templates/
│   ├── layout.html          # 3 ペイン全体レイアウト
│   └── partials/            # htmx で差し替える部分テンプレート
├── data/                    # 実行時に自動生成（gitignore）: gtd.db, images/
└── backup/                  # `gtd backup` の出力先（gitignore）
```

## 6. Coding Guidelines

### htmx Partial Rendering

- ハンドラは `HX-Request` ヘッダを見るか、専用エンドポイント（`/views/...`, `/partials/...`）を用意し、必要な HTML フラグメントだけを返す。
- 状態変更（POST/PUT/PATCH/DELETE）後は `HX-Trigger` レスポンスヘッダで関連ペイン（Inbox バッジ、中央リスト等）を更新させる。
- 新しい部分テンプレートは `templates/partials/` に置く。

### Static Asset Cache Busting

- `static/style.css` または `static/app.js` を変更したら、`templates/layout.html` の `?v=N` を両方インクリメントする（ブラウザキャッシュ対策）。

### Local OS Integration

- ローカルのファイル／フォルダ（`C:\...`, `\\server\...`）を開く処理は `os/exec` で行う（Windows: `cmd /c start "" "<path>"` または `explorer "<path>"`）。エンドポイントは `POST /attachments/{id}/open`。
- パスはシェル文字列に連結せず、引数として渡すこと。

### Tests

- DB クエリは `internal/db/*_test.go`、ハンドラは `internal/handler/handler_test.go` にテストがある。機能追加・修正時は対応するテストも追加・更新し、`go test ./...` が通ることを確認する。

## 7. Implementation Status

初期計画の Step 1〜4（Quick Capture・一覧・ステータス変更 → プロジェクト／コンテキスト・詳細ペイン自動保存 → 添付 → 週次レビュー）は実装済み。以降の追加機能（Smart Add の開始日、Scheduled ビュー、オムニサーチ、ランチャー、時計、バックアップ）の状況と今後の計画は `PLAN.md` を参照・更新する。
