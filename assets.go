package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
	type stream struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	type probeResult struct {
		Streams []stream `json:"streams"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffprobe failed: %w (stderr: %s)", err, stderr.String())
	}

	var result probeResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return "", fmt.Errorf("decode ffprobe output: %w", err)
	}

	if len(result.Streams) == 0 {
		return "", fmt.Errorf("no streams found")
	}

	w := result.Streams[0].Width
	h := result.Streams[0].Height

	ratio := float64(w) / float64(h)

	switch {
	case almostEqual(ratio, 16.0/9.0):
		return "16:9", nil
	case almostEqual(ratio, 9.0/16.0):
		return "9:16", nil
	default:
		return "other", nil
	}
}

func almostEqual(a, b float64) bool {
	const epsilon = 0.05
	return math.Abs(a-b) < epsilon
}

func processVideoForFastStart(filePath string) (string, error) {
	nFilePath := filePath + ".processing"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	com := exec.CommandContext(ctx,
		"ffmpeg",
		"-i", filePath,
		"-c", "copy",
		"-movflags", "faststart",
		"-f", "mp4", nFilePath)

	var stderr bytes.Buffer
	com.Stderr = &stderr

	if err := com.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w (stderr: %s)", err, stderr.String())
	}

	return nFilePath, nil
}
