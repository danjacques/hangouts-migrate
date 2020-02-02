package util

import (
	"crypto/sha256"
	"encoding/hex"
)

func HashForKey(v string) string {
	hashBytes := sha256.Sum256([]byte(v))
	return hex.EncodeToString(hashBytes[:])
}
