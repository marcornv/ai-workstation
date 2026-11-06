package main

// images.go: image inputs.
//
// A dataset column can hold images. Each cell is either:
//   - a path relative to the datasets folder, e.g. "images/receipts/001.jpg", or
//   - an http(s) URL.
//
// The experiment lists those columns in dataset.image_columns. When a job runs,
// the images of its row are read (or downloaded) and attached to the message
// sent to the model, in the format each provider expects (see providers.go).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Image struct {
	MediaType string // e.g. "image/png"
	Data      []byte
}

const maxImageBytes = 20 << 20 // 20 MB, the most the providers accept

// Rough token cost of one image, used only for the estimate before a run.
// (Real counts depend on size and provider; the results show the real value.)
const approxImageTokens = 1000

var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// looksLikeImage is used to guess which dataset columns hold images.
func looksLikeImage(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	// Ignore query strings like "photo.jpg?size=large".
	path, _, _ := strings.Cut(value, "?")
	_, ok := imageTypes[filepath.Ext(path)]
	return ok
}

func isURL(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
}

// imageRefs returns the image references of one dataset row.
func imageRefs(row map[string]string, columns []string) []string {
	var refs []string
	for _, col := range columns {
		if ref := strings.TrimSpace(row[col]); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

// safeDataPath turns a path relative to the datasets folder into a real path,
// refusing anything that would escape the folder ("../", absolute paths).
func safeDataPath(cfg Config, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf(tr("caminho fora da pasta de dados: %q", "path outside the data folder: %q"), rel)
	}
	return filepath.Join(cfg.Paths.Datasets, clean), nil
}

// checkImageRef verifies, without reading it, that a local image exists.
// URLs are only checked when the run actually downloads them.
func checkImageRef(cfg Config, ref string) error {
	if isURL(ref) {
		return nil
	}
	path, err := safeDataPath(cfg, ref)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf(tr("imagem não encontrada: %s (o caminho é relativo à pasta %s/)", "image not found: %s (the path is relative to the %s/ folder)"), ref, cfg.Paths.Datasets)
	}
	return nil
}

// loadImage reads a local image or downloads it from a URL.
func loadImage(ctx context.Context, cfg Config, ref string) (Image, error) {
	var data []byte
	if isURL(ref) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
		if err != nil {
			return Image{}, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return Image{}, fmt.Errorf(tr("não foi possível baixar a imagem %s: %w", "could not download image %s: %w"), ref, friendlyNetworkError(err))
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return Image{}, fmt.Errorf(tr("não foi possível baixar a imagem %s (HTTP %d)", "could not download image %s (HTTP %d)"), ref, resp.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
		if err != nil {
			return Image{}, err
		}
	} else {
		path, err := safeDataPath(cfg, ref)
		if err != nil {
			return Image{}, err
		}
		data, err = os.ReadFile(path)
		if err != nil {
			return Image{}, fmt.Errorf(tr("imagem não encontrada: %s", "image not found: %s"), ref)
		}
	}
	if len(data) > maxImageBytes {
		return Image{}, fmt.Errorf(tr("imagem maior que 20 MB: %s", "image larger than 20 MB: %s"), ref)
	}
	mediaType, err := imageMediaType(data)
	if err != nil {
		return Image{}, fmt.Errorf("%s: %w", ref, err)
	}
	return Image{MediaType: mediaType, Data: data}, nil
}

// imageMediaType looks at the file's first bytes (not its name) to know its type.
func imageMediaType(data []byte) (string, error) {
	mediaType := http.DetectContentType(data)
	switch mediaType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return mediaType, nil
	}
	return "", errors.New(tr("formato de imagem não suportado (use PNG, JPEG, GIF ou WEBP)", "unsupported image format (use PNG, JPEG, GIF or WEBP)"))
}
