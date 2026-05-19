package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const githubRepo = "yeniklas/pvcmount"

func runSelfUpdate(currentVersion string) error {
	if currentVersion == "dev" {
		return fmt.Errorf("cannot update a dev build — use 'go build' with ldflags instead")
	}

	fmt.Println("checking for updates...")
	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("fetch latest release: %w", err)
	}

	if release.TagName == currentVersion {
		fmt.Printf("already up to date (%s)\n", currentVersion)
		return nil
	}

	fmt.Printf("current: %s  →  latest: %s\n", currentVersion, release.TagName)

	assetName := fmt.Sprintf("pvcmount-%s-%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no asset %q found in release %s", assetName, release.TagName)
	}

	fmt.Printf("update to %s? [y/N] ", release.TagName)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
		fmt.Println("update cancelled")
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("eval symlinks: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(execPath), ".pvcmount-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	fmt.Printf("downloading %s...\n", assetName)
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", resp.Status)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("write download: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmp.Name(), 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmp.Name(), execPath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	fmt.Printf("updated to %s — restart pvcmount\n", release.TagName)
	return nil
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchLatestRelease() (*githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %s", resp.Status)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}
