package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

func VerifyAPIKey(key, hash string) bool {
	return HashAPIKey(key) == hash
}
