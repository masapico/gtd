package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Target struct {
	OS   string
	Arch string
}

var allTargets = []Target{
	{OS: "darwin", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "windows", Arch: "amd64"},
	{OS: "windows", Arch: "arm64"},
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
}

type ReleaseResult struct {
	Target     Target
	ZipName    string
	ZipPath    string
	ZipSize    int64
	BinarySize int64
	SHA256     string
	Duration   time.Duration
}

func printUsage() {
	fmt.Println("GTD Release Packaging Tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  release -version <version> [options]")
	fmt.Println("  go run ./cmd/release -version <version> [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -version string   Release version (e.g. v1.0.0, v.1.0.0) [required]")
	fmt.Println("  -os string        Target OS filter (darwin, windows, linux)")
	fmt.Println("  -arch string      Target architecture filter (amd64, arm64)")
	fmt.Println("  -out string       Output directory (default \"dist\")")
	fmt.Println("  -help             Show this help message")
}

func main() {
	var (
		versionFlag string
		targetOS    string
		targetArch  string
		outDir      string
		showHelp    bool
	)

	flag.StringVar(&versionFlag, "version", "", "Release version tag (e.g. v1.0.0)")
	flag.StringVar(&targetOS, "os", "", "Filter target OS (darwin, windows, linux)")
	flag.StringVar(&targetArch, "arch", "", "Filter target architecture (amd64, arm64)")
	flag.StringVar(&outDir, "out", "dist", "Output directory for zip archives")
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showHelp, "h", false, "Show help")

	flag.Parse()

	if showHelp {
		printUsage()
		return
	}

	// 引数からのフォールバック (例: release v1.0.0)
	version := strings.TrimSpace(versionFlag)
	if version == "" && flag.NArg() > 0 {
		version = strings.TrimSpace(flag.Arg(0))
	}

	if version == "" {
		fmt.Fprintf(os.Stderr, "❌ Error: -version is required (e.g. -version v1.0.0)\n\n")
		printUsage()
		os.Exit(1)
	}

	// フィルタリング対象の選定
	var targets []Target
	for _, t := range allTargets {
		if targetOS != "" && t.OS != targetOS {
			continue
		}
		if targetArch != "" && t.Arch != targetArch {
			continue
		}
		targets = append(targets, t)
	}

	if len(targets) == 0 {
		fmt.Fprintf(os.Stderr, "❌ No matching target platforms for os=%q, arch=%q\n", targetOS, targetArch)
		os.Exit(1)
	}

	// 出力ディレクトリ作成
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to create output directory %s: %v\n", outDir, err)
		os.Exit(1)
	}

	// 作業用一時ディレクトリ
	tempWorkDir, err := os.MkdirTemp("", "gtd-release-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to create temporary work directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempWorkDir)

	fmt.Println("🚀 Starting GTD Release Build")
	fmt.Printf("   Version:    %s\n", version)
	fmt.Printf("   Output Dir: %s\n", outDir)
	fmt.Printf("   Targets:    %d platform(s)\n\n", len(targets))

	startTime := time.Now()
	var results []ReleaseResult

	for idx, t := range targets {
		tStartTime := time.Now()
		platformName := fmt.Sprintf("%s/%s", t.OS, t.Arch)
		fmt.Printf("[%d/%d] 🔨 Building for %s ... ", idx+1, len(targets), platformName)

		binName := "gtd"
		if t.OS == "windows" {
			binName = "gtd.exe"
		}

		buildDir := filepath.Join(tempWorkDir, fmt.Sprintf("%s-%s", t.OS, t.Arch))
		if err := os.MkdirAll(buildDir, 0755); err != nil {
			fmt.Printf("FAILED\n❌ Mkdir failed: %v\n", err)
			os.Exit(1)
		}

		binPath := filepath.Join(buildDir, binName)

		// CGO_ENABLED=0 GOOS=... GOARCH=... go build ...
		cmd := exec.Command("go", "build",
			"-trimpath",
			"-ldflags", fmt.Sprintf("-s -w -X main.Version=%s", version),
			"-o", binPath,
			".",
		)
		cmd.Env = append(os.Environ(),
			"CGO_ENABLED=0",
			"GOOS="+t.OS,
			"GOARCH="+t.Arch,
		)

		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("FAILED\n❌ Build failed for %s:\n%s\n", platformName, string(output))
			os.Exit(1)
		}

		binFi, err := os.Stat(binPath)
		if err != nil {
			fmt.Printf("FAILED\n❌ Stat binary failed: %v\n", err)
			os.Exit(1)
		}

		// ZIPアーカイブ作成
		zipBaseName := fmt.Sprintf("gtd_%s_%s_%s.zip", version, t.OS, t.Arch)
		zipPath := filepath.Join(outDir, zipBaseName)

		if err := createZip(zipPath, binPath, binName, t.OS); err != nil {
			fmt.Printf("FAILED\n❌ Create zip failed: %v\n", err)
			os.Exit(1)
		}

		zipFi, err := os.Stat(zipPath)
		if err != nil {
			fmt.Printf("FAILED\n❌ Stat zip failed: %v\n", err)
			os.Exit(1)
		}

		// SHA256ハッシュ計算
		hash, err := calculateSHA256(zipPath)
		if err != nil {
			fmt.Printf("FAILED\n❌ Calculate SHA256 failed: %v\n", err)
			os.Exit(1)
		}

		dur := time.Since(tStartTime)
		fmt.Printf("DONE (%v)\n", dur.Round(time.Millisecond))
		fmt.Printf("       Archive: %s (%.2f MB)\n", zipBaseName, float64(zipFi.Size())/(1024*1024))

		results = append(results, ReleaseResult{
			Target:     t,
			ZipName:    zipBaseName,
			ZipPath:    zipPath,
			ZipSize:    zipFi.Size(),
			BinarySize: binFi.Size(),
			SHA256:     hash,
			Duration:   dur,
		})
	}

	// checksums.txt 生成
	checksumPath := filepath.Join(outDir, "checksums.txt")
	if err := writeChecksums(checksumPath, results); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ Failed to write checksums.txt: %v\n", err)
	} else {
		fmt.Printf("\n📄 Generated %s\n", checksumPath)
	}

	totalDuration := time.Since(startTime)
	fmt.Printf("\n✨ Successfully generated %d release package(s) in %v!\n\n", len(results), totalDuration.Round(time.Millisecond))
	fmt.Println("Artifacts:")
	for _, res := range results {
		fmt.Printf("  • %-35s %6.2f MB  (SHA256: %s...)\n", res.ZipName, float64(res.ZipSize)/(1024*1024), res.SHA256[:12])
	}
	fmt.Println()
}

// createZip creates a zip file containing the binary and README.md (if present)
func createZip(zipPath, binPath, binName, targetOS string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)
	defer zw.Close()

	// 1. バイナリ追加
	binFile, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer binFile.Close()

	binFi, err := binFile.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(binFi)
	if err != nil {
		return err
	}
	header.Name = binName
	header.Method = zip.Deflate

	// Unix系バイナリの実行権限を保持 (0755)
	if targetOS != "windows" {
		header.SetMode(0755)
	} else {
		header.SetMode(0644)
	}

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	if _, err := io.Copy(writer, binFile); err != nil {
		return err
	}

	// 2. README.md の同梱 (存在する場合)
	if readmeData, err := os.ReadFile("README.md"); err == nil {
		readmeHeader := &zip.FileHeader{
			Name:     "README.md",
			Method:   zip.Deflate,
			Modified: time.Now(),
		}
		readmeHeader.SetMode(0644)
		readmeWriter, err := zw.CreateHeader(readmeHeader)
		if err == nil {
			_, _ = readmeWriter.Write(readmeData)
		}
	}

	return nil
}

// calculateSHA256 returns the hex string of SHA256 checksum of a file
func calculateSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// writeChecksums writes SHA256 hash list to file
func writeChecksums(checksumPath string, results []ReleaseResult) error {
	var sb strings.Builder
	for _, res := range results {
		sb.WriteString(fmt.Sprintf("%s  %s\n", res.SHA256, res.ZipName))
	}
	return os.WriteFile(checksumPath, []byte(sb.String()), 0644)
}

