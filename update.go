package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	owner = "sevenam"
	repo  = "gitraffe"
)

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

func checkUpdate() error {
	fmt.Println("Checking for updates...")

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}

	currentTag := "v" + version
	if release.TagName == currentTag {
		fmt.Println("You are already on the latest version:", currentTag)
		return nil
	}

	fmt.Printf("New version available: %s (current: %s)\n", release.TagName, currentTag)
	fmt.Println("Downloading...")

	if err := downloadAndUpdate(release); err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}

	fmt.Printf("Successfully updated to %s\n", release.TagName)
	return nil
}

func fetchLatestRelease() (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// fetchLatestVersion checks for the latest version without downloading
// Returns the tag name (e.g., "v0.2.0") or empty string on error
func fetchLatestVersion() string {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ""
	}

	return release.TagName
}

func downloadAndUpdate(release *Release) error {
	assetName := getBinaryName()
	var downloadURL string

	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.DownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Download to temporary file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download binary: HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "gitraffe-update-*")
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}

	// Get the path to the current executable
	exePath, err := os.Executable()
	if err != nil {
		os.Remove(tmpFile.Name())
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	// Make the temp file executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}

	// Replace the old executable with the new one
	if runtime.GOOS == "windows" {
		// On Windows, spawn a helper process to do the replacement after we exit
		// This is necessary because the running executable is locked
		return replaceOnWindows(exePath, tmpFile.Name())
	} else {
		// On Unix-like systems, we can use atomic rename
		if err := os.Rename(tmpFile.Name(), exePath); err != nil {
			os.Remove(tmpFile.Name())
			return fmt.Errorf("failed to install new executable: %w", err)
		}
	}

	return nil
}

func getBinaryName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	return fmt.Sprintf("gitraffe-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

// replaceOnWindows spawns a helper process to replace the executable
// since the running binary is locked and can't be renamed
func replaceOnWindows(exePath, newBinaryPath string) error {
	// Create a batch script that will:
	// 1. Wait a moment for the main process to exit
	// 2. Move the new binary to the target location
	// 3. Clean up

	tmpDir := filepath.Dir(newBinaryPath)
	scriptPath := filepath.Join(tmpDir, "gitraffe-update.bat")

	// Create a batch script
	script := fmt.Sprintf(`@echo off
timeout /t 1 /nobreak > nul
move /Y "%s" "%s"
del "%s" 2>nul
exit /b 0
`, newBinaryPath, exePath, scriptPath)

	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		os.Remove(newBinaryPath)
		return fmt.Errorf("failed to create update script: %w", err)
	}

	// Spawn the batch script in the background
	cmd := exec.Command("cmd", "/C", "start", "/B", scriptPath)
	if err := cmd.Start(); err != nil {
		os.Remove(scriptPath)
		os.Remove(newBinaryPath)
		return fmt.Errorf("failed to start update process: %w", err)
	}

	// Exit after spawning the updater
	fmt.Println("Update will be applied in the background. Exiting...")
	os.Exit(0)

	return nil // Never reached, but keeps compiler happy
}
