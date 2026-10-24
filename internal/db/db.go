package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// InitDB はデータベース接続を初期化し、テーブルスキーマと初期データを設定します
func InitDB(dbPath string) (*sql.DB, error) {
	// データ用ディレクトリの自動作成
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	imagesDir := filepath.Join(dir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create images directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// 外部キー制約の有効化とWALモードによる高速化・並行アクセス向上
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("failed to set pragma %q: %w", p, err)
		}
	}

	if err := migrateSchema(db); err != nil {
		return nil, fmt.Errorf("failed to migrate schema: %w", err)
	}

	if err := seedDefaultContexts(db); err != nil {
		return nil, fmt.Errorf("failed to seed contexts: %w", err)
	}

	return db, nil
}

func migrateSchema(db *sql.DB) error {
	schema := `
	-- 1. プロジェクト
	CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		description TEXT,
		status TEXT NOT NULL DEFAULT 'active', -- 'active', 'someday', 'done'
		last_reviewed_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- 2. アイテム（Inbox / タスク / メモ / 繰り返しタスク統合）
	CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER REFERENCES projects(id) ON DELETE SET NULL,
		title TEXT NOT NULL,
		body TEXT,
		status TEXT NOT NULL DEFAULT 'inbox', -- 'inbox', 'next', 'waiting', 'someday', 'reference', 'done'
		start_date TEXT,
		due_date TEXT,
		recurrence TEXT NOT NULL DEFAULT 'none', -- 'none', 'daily', 'weekdays', 'weekly', 'monthly', 'yearly'
		last_reviewed_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- 3. コンテキスト（場所・状況・相手）
	CREATE TABLE IF NOT EXISTS contexts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE
	);

	-- 4. アイテムとコンテキストの中間テーブル
	CREATE TABLE IF NOT EXISTS item_contexts (
		item_id INTEGER REFERENCES items(id) ON DELETE CASCADE,
		context_id INTEGER REFERENCES contexts(id) ON DELETE CASCADE,
		PRIMARY KEY (item_id, context_id)
	);

	-- 5. 添付・関連リンク
	CREATE TABLE IF NOT EXISTS attachments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		item_id INTEGER REFERENCES items(id) ON DELETE CASCADE,
		label TEXT,
		target TEXT NOT NULL,
		type TEXT NOT NULL, -- 'url', 'path', 'image'
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- 6. アプリケーションランチャー
	CREATE TABLE IF NOT EXISTS launchers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		target TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'app', -- 'app', 'folder', 'file', 'url'
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- インデックス
	CREATE INDEX IF NOT EXISTS idx_items_status ON items(status);
	CREATE INDEX IF NOT EXISTS idx_items_project ON items(project_id);
	CREATE INDEX IF NOT EXISTS idx_attachments_item ON attachments(item_id);
	CREATE INDEX IF NOT EXISTS idx_launchers_name ON launchers(name);
	`

	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// 既存DBへの start_date カラム追加マイグレーション
	if err := addColumnIfNotExists(db, "items", "start_date", "TEXT"); err != nil {
		return err
	}

	// 既存DBへの sort_order カラム追加マイグレーション
	if err := addColumnIfNotExists(db, "items", "sort_order", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// 既存DBへの projects.type カラム追加マイグレーション ('parallel', 'sequential')
	if err := addColumnIfNotExists(db, "projects", "type", "TEXT NOT NULL DEFAULT 'parallel'"); err != nil {
		return err
	}

	// 既存アイテムの初期 sort_order を作成順（id）で設定
	if _, err := db.Exec("UPDATE items SET sort_order = id WHERE sort_order = 0;"); err != nil {
		return err
	}

	// インデックス作成
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_items_start_date ON items(start_date);"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_items_project_order ON items(project_id, sort_order);"); err != nil {
		return err
	}

	// item_state 導出ビューの作成（起動毎に最新ロジックを反映）
	viewSQL := `
	DROP VIEW IF EXISTS item_state;
	CREATE VIEW item_state AS
	SELECT 
		i.id AS id,
		CASE WHEN p.type = 'sequential'
			  AND i.status IN ('next', 'waiting')
			  AND EXISTS (
				SELECT 1 FROM items j 
				WHERE j.project_id = i.project_id 
				  AND j.status IN ('next', 'waiting') 
				  AND (j.sort_order < i.sort_order OR (j.sort_order = i.sort_order AND j.id < i.id))
			  ) THEN 1 ELSE 0 END AS is_blocked,
		CASE WHEN i.status = 'next'
			  AND (i.start_date IS NULL OR i.start_date = '' OR i.start_date <= date('now', 'localtime'))
			  AND NOT (
				p.type = 'sequential'
				AND EXISTS (
					SELECT 1 FROM items j 
					WHERE j.project_id = i.project_id 
					  AND j.status IN ('next', 'waiting') 
					  AND (j.sort_order < i.sort_order OR (j.sort_order = i.sort_order AND j.id < i.id))
				)
			  ) THEN 1 ELSE 0 END AS is_available
	FROM items i
	LEFT JOIN projects p ON p.id = i.project_id;
	`
	if _, err := db.Exec(viewSQL); err != nil {
		return fmt.Errorf("failed to create item_state view: %w", err)
	}

	return nil
}

func addColumnIfNotExists(db *sql.DB, tableName, columnName, columnType string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	exists := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == columnName {
			exists = true
			break
		}
	}

	if !exists {
		query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnType)
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to add column %s to %s: %w", columnName, tableName, err)
		}
	}
	return nil
}

func seedDefaultContexts(db *sql.DB) error {
	defaults := []string{"#PC", "#会議", "#連絡", "#5分", "#外出"}
	for _, name := range defaults {
		_, err := db.Exec("INSERT OR IGNORE INTO contexts (name) VALUES (?)", name)
		if err != nil {
			return err
		}
	}
	return nil
}

