package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	// On Windows, we need to remove the old file first
	if runtime.GOOS == "windows" {
		backupPath := exePath + ".bak"
		if err := os.Rename(exePath, backupPath); err != nil {
			os.Remove(tmpFile.Name())
			return fmt.Errorf("failed to backup old executable: %w", err)
		}
		if err := os.Rename(tmpFile.Name(), exePath); err != nil {
			// Try to restore from backup
			os.Rename(backupPath, exePath)
			os.Remove(tmpFile.Name())
			return fmt.Errorf("failed to install new executable: %w", err)
		}
		os.Remove(backupPath)
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
