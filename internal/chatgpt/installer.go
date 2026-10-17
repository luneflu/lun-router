package chatgpt

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type InstallStatus struct {
	State    string `json:"state"` // "idle", "downloading", "extracting", "ready", "error"
	Progress int    `json:"progress"`
	Error    string `json:"error,omitempty"`
}

type Installer struct {
	dataDir string
	mu      sync.RWMutex
	status  InstallStatus
	cancel  context.CancelFunc
}

func NewInstaller(dataDir string) *Installer {
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		absDataDir = dataDir
	}
	return &Installer{
		dataDir: absDataDir,
		status: InstallStatus{
			State:    "idle",
			Progress: 0,
		},
	}
}

func (inst *Installer) GetStatus() InstallStatus {
	inst.mu.RLock()
	defer inst.mu.RUnlock()
	return inst.status
}

func (inst *Installer) setStatus(state string, progress int, errMsg string) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.status = InstallStatus{
		State:    state,
		Progress: progress,
		Error:    errMsg,
	}
}

// BrowserInstallDir returns the directory where the browser binary is installed.
func (inst *Installer) BrowserInstallDir() string {
	return filepath.Join(inst.dataDir, "browser")
}

// DetectedExecutable returns the path to the executable if present on disk.
func (inst *Installer) DetectedExecutable() string {
	dir := inst.BrowserInstallDir()
	switch runtime.GOOS {
	case "darwin":
		cands := []string{
			filepath.Join(dir, "Chromium.app/Contents/MacOS/Chromium"),
			filepath.Join(dir, "cloakbrowser-darwin-arm64/Chromium.app/Contents/MacOS/Chromium"),
			filepath.Join(dir, "cloakbrowser-darwin-x64/Chromium.app/Contents/MacOS/Chromium"),
			filepath.Join(dir, "chrome"),
			filepath.Join(dir, "chromium"),
		}
		for _, c := range cands {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				return c
			}
		}
	case "linux":
		cands := []string{
			filepath.Join(dir, "chrome"),
			filepath.Join(dir, "chromium"),
			filepath.Join(dir, "cloakbrowser"),
			filepath.Join(dir, "cloakbrowser-linux-x64/chrome"),
			filepath.Join(dir, "cloakbrowser-linux-arm64/chrome"),
		}
		for _, c := range cands {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				return c
			}
		}
	case "windows":
		cands := []string{
			filepath.Join(dir, "chrome.exe"),
			filepath.Join(dir, "chromium.exe"),
			filepath.Join(dir, "cloakbrowser.exe"),
		}
		for _, c := range cands {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				return c
			}
		}
	}

	// Check explicit environment variables
	if p := os.Getenv("CHATGPT_BROWSER_PATH"); p != "" {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	if p := os.Getenv("CLOAKBROWSER_PATH"); p != "" {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}

	return ""
}

// IsInstalled returns true if a usable browser executable is detected.
func (inst *Installer) IsInstalled() bool {
	return inst.DetectedExecutable() != ""
}

// DownloadURL returns the official release asset URL for the current OS/Arch from CloakHQ/cloakbrowser.
func DownloadURL() (string, bool, error) {
	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			return "https://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v145.0.7632.109.2/cloakbrowser-darwin-arm64.tar.gz", false, nil
		case "amd64":
			return "https://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v145.0.7632.109.2/cloakbrowser-darwin-x64.tar.gz", false, nil
		default:
			return "", false, fmt.Errorf("unsupported darwin arch: %s", runtime.GOARCH)
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "https://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v146.0.7680.177.5/cloakbrowser-linux-x64.tar.gz", false, nil
		case "arm64":
			return "https://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v146.0.7680.177.4/cloakbrowser-linux-arm64.tar.gz", false, nil
		default:
			return "", false, fmt.Errorf("unsupported linux arch: %s", runtime.GOARCH)
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "https://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v146.0.7680.177.5/cloakbrowser-windows-x64.zip", true, nil
		}
		return "", true, fmt.Errorf("unsupported windows arch: %s", runtime.GOARCH)
	default:
		return "", false, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// StartInstall begins downloading and extracting the browser in the background.
func (inst *Installer) StartInstall(ctx context.Context) error {
	inst.mu.Lock()
	if inst.status.State == "downloading" || inst.status.State == "extracting" {
		inst.mu.Unlock()
		return errors.New("installation already in progress")
	}
	installCtx, cancel := context.WithCancel(context.Background())
	inst.cancel = cancel
	inst.status = InstallStatus{
		State:    "downloading",
		Progress: 0,
	}
	inst.mu.Unlock()

	go func() {
		err := inst.runInstall(installCtx)
		if err != nil {
			inst.setStatus("error", 0, err.Error())
		} else {
			inst.setStatus("ready", 100, "")
		}
	}()

	return nil
}

func (inst *Installer) runInstall(ctx context.Context) error {
	url, isZip, err := DownloadURL()
	if err != nil {
		return err
	}

	installDir := inst.BrowserInstallDir()
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %s", resp.Status)
	}

	totalSize := resp.ContentLength
	tempArchive := filepath.Join(installDir, "download.tmp")
	_ = os.Remove(tempArchive)

	out, err := os.Create(tempArchive)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		out.Close()
		_ = os.Remove(tempArchive)
	}()

	buf := make([]byte, 64*1024)
	var downloaded int64
	lastUpdate := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("write archive: %w", wErr)
			}
			downloaded += int64(n)
			if totalSize > 0 && time.Since(lastUpdate) > 200*time.Millisecond {
				lastUpdate = time.Now()
				pct := int((downloaded * 90) / totalSize) // 0-90% for download
				inst.setStatus("downloading", pct, "")
			}
		}
		if rErr != nil {
			if errors.Is(rErr, io.EOF) {
				break
			}
			return fmt.Errorf("read download stream: %w", rErr)
		}
	}
	_ = out.Close()

	inst.setStatus("extracting", 92, "")

	if isZip {
		if err := extractZip(tempArchive, installDir); err != nil {
			return fmt.Errorf("extract zip: %w", err)
		}
	} else {
		if err := extractTarGz(tempArchive, installDir); err != nil {
			return fmt.Errorf("extract tar.gz: %w", err)
		}
	}

	inst.setStatus("ready", 100, "")
	return nil
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		cleanPath := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanPath, "..") {
			continue // prevent zip slip
		}
		target := filepath.Join(destDir, cleanPath)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		case tar.TypeSymlink:
			_ = os.Remove(target)
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		cleanPath := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanPath, "..") {
			continue
		}
		target := filepath.Join(destDir, cleanPath)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
