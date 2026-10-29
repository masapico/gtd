package backup

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// BackupResult はバックアップ実行結果の情報を保持します
type BackupResult struct {
	ZipPath    string
	FileSize   int64
	ImageCount int
	Duration   time.Duration
}

// Run は稼働中のSQLiteデータベースと画像ディレクトリを安全にバックアップし、
// ./backup フォルダに YYYYMMDDHHMMSS_gtd.zip としてアーカイブします
func Run(dbPath, imagesDir, backupDir string) (*BackupResult, error) {
	startTime := time.Now()

	// 1. バックアップ出力先ディレクトリの確認・作成
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory %q: %w", backupDir, err)
	}

	// 2. データベース存在確認
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database file does not exist at %q", dbPath)
	}

	// 3. タイムスタンプとファイル名決定 (YYYYMMDDHHMMSS_gtd.zip)
	timestamp := startTime.Format("20060102150405")
	zipFileName := fmt.Sprintf("%s_gtd.zip", timestamp)
	zipFilePath := filepath.Join(backupDir, zipFileName)

	// 4. 一時DBスナップショットファイルの準備
	tempDBFileName := fmt.Sprintf(".temp_snapshot_%s_%d.db", timestamp, time.Now().UnixNano()%1000000)
	tempDBPath := filepath.Join(backupDir, tempDBFileName)
	defer os.Remove(tempDBPath)

	// 5. SQLiteから無停止オンラインスナップショットを生成 (VACUUM INTO)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source database for backup: %w", err)
	}
	defer db.Close()

	// ロック競合時の待機タイムアウトを設定
	if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		return nil, fmt.Errorf("failed to set busy_timeout: %w", err)
	}

	// WALモード下で整合性のある単一DBファイルを生成
	if _, err := db.Exec("VACUUM INTO ?", tempDBPath); err != nil {
		return nil, fmt.Errorf("failed to execute VACUUM INTO: %w", err)
	}

	// 6. ZIPアーカイブの作成
	zipFile, err := os.Create(zipFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create zip file %q: %w", zipFilePath, err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 6-a. gtd.db をZIP内に追加
	if err := addFileToZip(zipWriter, tempDBPath, "gtd.db"); err != nil {
		return nil, fmt.Errorf("failed to add database to zip: %w", err)
	}

	// 6-b. images フォルダ配下の全画像ファイルをZIPに追加
	imageCount := 0
	if _, err := os.Stat(imagesDir); err == nil {
		err = filepath.WalkDir(imagesDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			// imagesDir からの相対パスを取得
			relPath, err := filepath.Rel(imagesDir, path)
			if err != nil {
				return err
			}

			// ZIP内ではスラッシュ区切りで images/... として格納
			zipEntryPath := filepath.ToSlash(filepath.Join("images", relPath))
			if err := addFileToZip(zipWriter, path, zipEntryPath); err != nil {
				return fmt.Errorf("failed to add image %q to zip: %w", path, err)
			}
			imageCount++
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to archive images: %w", err)
		}
	}

	// ZIPライターをクローズしてフラッシュ
	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize zip archive: %w", err)
	}

	// ZIPファイルのサイズを取得
	fi, err := os.Stat(zipFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect created zip file: %w", err)
	}

	return &BackupResult{
		ZipPath:    zipFilePath,
		FileSize:   fi.Size(),
		ImageCount: imageCount,
		Duration:   time.Since(startTime),
	}, nil
}

// addFileToZip はローカルファイルをZIPアーカイブ内の指定された名前で圧縮格納します
func addFileToZip(zipWriter *zip.Writer, srcPath, zipEntryName string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %q: %w", srcPath, err)
	}
	defer srcFile.Close()

	fi, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file %q: %w", srcPath, err)
	}

	header, err := zip.FileInfoHeader(fi)
	if err != nil {
		return fmt.Errorf("failed to create zip header for %q: %w", srcPath, err)
	}

	header.Name = zipEntryName
	header.Method = zip.Deflate // 圧縮を有効化

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create writer for zip entry %q: %w", zipEntryName, err)
	}

	if _, err := io.Copy(writer, srcFile); err != nil {
		return fmt.Errorf("failed to write content to zip entry %q: %w", zipEntryName, err)
	}

	return nil
}

