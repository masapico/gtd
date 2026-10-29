package backup

import (
	"archive/zip"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBackupRun(t *testing.T) {
	tempDir := t.TempDir()

	dbDir := filepath.Join(tempDir, "data")
	imagesDir := filepath.Join(dbDir, "images")
	backupDir := filepath.Join(tempDir, "backup")

	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		t.Fatalf("failed to create images dir: %v", err)
	}

	// 1. SQLite DB初期化 (WALモード)
	dbPath := filepath.Join(dbDir, "gtd.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		t.Fatalf("failed to set WAL mode: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE items (id INTEGER PRIMARY KEY, title TEXT);
		INSERT INTO items (title) VALUES ('Task 1'), ('Task 2');
	`); err != nil {
		t.Fatalf("failed to populate test db: %v", err)
	}
	db.Close()

	// 2. テスト用画像ファイルの作成
	img1Path := filepath.Join(imagesDir, "shot1.png")
	img1Content := []byte("fake png image 1 content")
	if err := os.WriteFile(img1Path, img1Content, 0644); err != nil {
		t.Fatalf("failed to write img1: %v", err)
	}

	img2Path := filepath.Join(imagesDir, "shot2.jpg")
	img2Content := []byte("fake jpg image 2 content")
	if err := os.WriteFile(img2Path, img2Content, 0644); err != nil {
		t.Fatalf("failed to write img2: %v", err)
	}

	// 3. バックアップ実行
	res, err := Run(dbPath, imagesDir, backupDir)
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	if res.ZipPath == "" {
		t.Fatal("expected non-empty ZipPath")
	}
	if res.FileSize <= 0 {
		t.Fatalf("expected FileSize > 0, got %d", res.FileSize)
	}
	if res.ImageCount != 2 {
		t.Fatalf("expected ImageCount == 2, got %d", res.ImageCount)
	}

	// 4. ZIPアーカイブの内容確認
	r, err := zip.OpenReader(res.ZipPath)
	if err != nil {
		t.Fatalf("failed to open generated zip file: %v", err)
	}
	defer r.Close()

	foundEntries := make(map[string]bool)
	var extractedDBBytes []byte
	var extractedImg1Bytes []byte

	for _, f := range r.File {
		foundEntries[f.Name] = true

		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open zip entry %q: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed to read zip entry %q: %v", f.Name, err)
		}

		if f.Name == "gtd.db" {
			extractedDBBytes = data
		} else if f.Name == "images/shot1.png" {
			extractedImg1Bytes = data
		}
	}

	// 必須ファイルのエントリ確認
	if !foundEntries["gtd.db"] {
		t.Errorf("zip missing entry 'gtd.db'")
	}
	if !foundEntries["images/shot1.png"] {
		t.Errorf("zip missing entry 'images/shot1.png'")
	}
	if !foundEntries["images/shot2.jpg"] {
		t.Errorf("zip missing entry 'images/shot2.jpg'")
	}

	// 画像内容の一致確認
	if string(extractedImg1Bytes) != string(img1Content) {
		t.Errorf("extracted image content mismatch: got %q, want %q", string(extractedImg1Bytes), string(img1Content))
	}

	// 5. 展開したDBの整合性確認
	restoredDBPath := filepath.Join(tempDir, "restored.db")
	if err := os.WriteFile(restoredDBPath, extractedDBBytes, 0644); err != nil {
		t.Fatalf("failed to write restored db: %v", err)
	}

	rdb, err := sql.Open("sqlite", restoredDBPath)
	if err != nil {
		t.Fatalf("failed to open restored db: %v", err)
	}
	defer rdb.Close()

	var count int
	if err := rdb.QueryRow("SELECT count(*) FROM items").Scan(&count); err != nil {
		t.Fatalf("failed to query restored db: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 items in restored db, got %d", count)
	}
}

func TestConcurrentBackupRun(t *testing.T) {
	tempDir := t.TempDir()

	dbDir := filepath.Join(tempDir, "data")
	imagesDir := filepath.Join(dbDir, "images")
	backupDir := filepath.Join(tempDir, "backup")

	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		t.Fatalf("failed to create images dir: %v", err)
	}

	dbPath := filepath.Join(dbDir, "gtd.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;"); err != nil {
		t.Fatalf("failed to set WAL mode: %v", err)
	}

	if _, err := db.Exec("CREATE TABLE logs (id INTEGER PRIMARY KEY, msg TEXT);"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	stopWrites := make(chan struct{})
	writeDone := make(chan struct{})

	// バックグラウンドで書き込みを継続実行
	go func() {
		defer close(writeDone)
		i := 0
		for {
			select {
			case <-stopWrites:
				return
			default:
				i++
				_, _ = db.Exec("INSERT INTO logs (msg) VALUES (?)", i)
			}
		}
	}()

	// 書き込みが始まっている状態でバックアップ実行
	res, err := Run(dbPath, imagesDir, backupDir)
	if err != nil {
		close(stopWrites)
		<-writeDone
		t.Fatalf("concurrent Run() failed: %v", err)
	}

	close(stopWrites)
	<-writeDone

	if res.ZipPath == "" || res.FileSize <= 0 {
		t.Fatalf("invalid backup result: %+v", res)
	}
}

