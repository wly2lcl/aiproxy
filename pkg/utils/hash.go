package utils

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

func VerifyAPIKey(key, hash string) bool {
	keyHash := HashAPIKey(key)
	return subtle.ConstantTimeCompare([]byte(keyHash), []byte(hash)) == 1
}

func GenerateAccountID(providerID, apiKeyHash string) string {
	hash := sha256.Sum256([]byte(providerID + ":" + apiKeyHash))
	return hex.EncodeToString(hash[:])
}
