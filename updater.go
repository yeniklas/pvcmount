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

const releaseAPI = "https://api.github.com/repos/yeniklas/pvcmount/releases/latest"

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func runSelfUpdate(currentVersion string) error {
	if currentVersion == "dev" {
		fmt.Println("cannot self-update a dev build")
		return nil
	}

	fmt.Println("Checking for updates…")
	rel, err := fetchLatest()
	if err != nil {
		return fmt.Errorf("fetch release info: %w", err)
	}

	if rel.TagName == currentVersion {
		fmt.Printf("pvcmount is already up to date (%s)\n", currentVersion)
		return nil
	}

	assetName := fmt.Sprintf("pvcmount-%s-%s-%s", rel.TagName, runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		var names []string
		for _, a := range rel.Assets {
			names = append(names, a.Name)
		}
		return fmt.Errorf("no binary found for %s/%s in release %s\navailable: %s",
			runtime.GOOS, runtime.GOARCH, rel.TagName, strings.Join(names, ", "))
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}
	dir := filepath.Dir(exe)

	probe, err := os.CreateTemp(dir, ".pvcmount-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w\nre-run with appropriate permissions", dir, err)
	}
	probe.Close()
	os.Remove(probe.Name())

	fmt.Printf("Update pvcmount %s → %s? [y/N] ", currentVersion, rel.TagName)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		fmt.Println("Update cancelled.")
		return nil
	}

	fmt.Printf("Downloading %s…\n", assetName)
	tmp, err := os.CreateTemp(dir, ".pvcmount-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	resp, err := http.Get(downloadURL) //nolint:gosec
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: unexpected status %s", resp.Status)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("write binary: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpName, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpName, exe); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	fmt.Printf("Updated to %s.\n", rel.TagName)
	return nil
}

func fetchLatest() (*release, error) {
	req, err := http.NewRequest(http.MethodGet, releaseAPI, nil)
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
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}
