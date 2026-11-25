package anchor

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

func checksumFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
