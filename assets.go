package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	filename := make([]byte, 32)
	rand.Read(filename)

	return base64.RawURLEncoding.EncodeToString(filename) + mediaTypeToExt(mediaType)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func (cfg apiConfig) getVideoURL(assetPath string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, assetPath)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

func getVideoAspectRatio(filePath string) (string, error) {
	type VideoMetaData struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	com := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var stdout, stderr bytes.Buffer
	com.Stdout = &stdout
	com.Stderr = &stderr
	err := com.Run()
	if err != nil {
		return "", fmt.Errorf("ffprobe failed: %w, stderr: %s", err, stderr.String())
	}
	var meta VideoMetaData
	err = json.Unmarshal(stdout.Bytes(), &meta)
	if err != nil {
		return "", err
	}

	ratio := meta.Width / meta.Height
	if ratio == 2 {
		return "16:9", nil
	} else if ratio == 1 {
		return "9:16", nil
	}
	return "other", nil
}
