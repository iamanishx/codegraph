package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

func contentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}
