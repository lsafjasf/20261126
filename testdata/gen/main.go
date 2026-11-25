// Command gen produces deterministic test fixtures for edlrelink.
//
// It writes two media libraries (the old offline one and the current online
// one), the offline change list, and small stand-in media files whose real
// SHA-256 hashes are computed on the fly. The online library deliberately
// contains the traps the resolver must survive:
//
//   - two old assets deleted outright (content hash absent);
//   - a clip ID reused by brand-new content;
//   - the same file name pointing at different content;
//   - the same content duplicated under two clip IDs;
//   - a moved/renamed asset whose content hash is unchanged.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"edlrelink/internal/edl"
)

const root = "testdata"

type spec struct {
	id   string
	path string
	seed string // content token; same seed => same media hash
}

func mediaContent(seed string) []byte {
	return []byte("stand-in media payload for seed " + seed + "\n")
}

func hashOf(seed string) string {
	sum := sha256.Sum256(mediaContent(seed))
	return hex.EncodeToString(sum[:])
}

func asset(s spec) edl.Asset {
	content := mediaContent(s.seed)
	return edl.Asset{
		ClipID:    s.id,
		Path:      s.path,
		MediaHash: hashOf(s.seed),
		SizeBytes: int64(len(content)),
	}
}

func writeMedia(libRoot string, specs []spec) error {
	for _, s := range specs {
		full := filepath.Join(root, libRoot, filepath.FromSlash(s.path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		content := mediaContent(s.seed)
		if err := os.WriteFile(full, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
