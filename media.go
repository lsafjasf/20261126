package smptett

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const maxMediaBytes = 64 << 20

type loadedMedia struct {
	image   image.Image
	format  string
	display string
	size    int
	warning string
}

func loadMedia(source, baseDir string) loadedMedia {
	if strings.TrimSpace(source) == "" {
		return loadedMedia{warning: "empty media source"}
	}
	data, display, err := readMediaSource(source, baseDir)
	if err != nil {
		return loadedMedia{display: redactSource(source), warning: err.Error()}
	}
	if len(data) > maxMediaBytes {
		return loadedMedia{display: display, warning: "media exceeds 64 MiB limit"}
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return loadedMedia{display: display, size: len(data), warning: fmt.Sprintf("unsupported or corrupt image: %v", err)}
	}
	return loadedMedia{image: img, format: format, display: display, size: len(data)}
}

func readMediaSource(source, baseDir string) ([]byte, string, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "data:") {
		data, mime, err := decodeDataURI(source)
		if err != nil {
			return nil, redactSource(source), err
		}
		return data, fmt.Sprintf("embedded:%s (%d bytes)", mime, len(data)), nil
	}

	u, err := url.Parse(source)
	if err != nil {
		return nil, redactSource(source), fmt.Errorf("invalid media URL: %w", err)
	}
	var path string
	switch u.Scheme {
	case "":
		path = u.Path
		if path == "" {
			path = source
		}
	case "file":
		if u.Host != "" && u.Host != "localhost" {
			return nil, redactSource(source), fmt.Errorf("unsupported file host %q", u.Host)
		}
		path = u.Path
	case "http", "https":
		return nil, redactSource(source), fmt.Errorf("network media is not fetched; use a local file or data URI")
	default:
		return nil, redactSource(source), fmt.Errorf("unsupported media scheme %q", u.Scheme)
	}
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, filepath.FromSlash(path))
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, path, fmt.Errorf("read media: %w", err)
	}
	if info.IsDir() {
		return nil, path, fmt.Errorf("media path is a directory")
	}
	if info.Size() > maxMediaBytes {
		return nil, path, fmt.Errorf("media exceeds 64 MiB limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, fmt.Errorf("read media: %w", err)
	}
	return data, path, nil
}

func decodeDataURI(source string) ([]byte, string, error) {
	comma := strings.IndexByte(source, ',')
	if comma < 0 {
		return nil, "", fmt.Errorf("malformed data URI")
	}
	meta := source[len("data:"):comma]
	payload := source[comma+1:]
	parts := strings.Split(meta, ";")
	mime := "application/octet-stream"
	if len(parts) > 0 && strings.Contains(parts[0], "/") {
		mime = parts[0]
	}
	for _, part := range parts[1:] {
		if strings.EqualFold(part, "base64") {
			payload = strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) {
					return -1
				}
				return r
			}, payload)
			data, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				data, err = base64.RawStdEncoding.DecodeString(payload)
			}
			if err != nil {
				return nil, mime, fmt.Errorf("decode base64 media: %w", err)
			}
			return data, mime, nil
		}
	}
	decoded, err := url.PathUnescape(payload)
	if err != nil {
		return nil, mime, fmt.Errorf("decode data URI payload: %w", err)
	}
	return []byte(decoded), mime, nil
}

func redactSource(source string) string {
	if strings.HasPrefix(source, "data:") {
		comma := strings.IndexByte(source, ',')
		mime := "unknown"
		if comma > 5 {
			meta := source[5:comma]
			if semi := strings.IndexByte(meta, ';'); semi >= 0 {
				meta = meta[:semi]
			}
			if meta != "" {
				mime = meta
			}
		}
		return fmt.Sprintf("embedded:%s (%d encoded bytes)", mime, len(source))
	}
	return source
}
