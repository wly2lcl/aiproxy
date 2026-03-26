package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func GenerateUUID() string {
	uuid := make([]byte, 16)
	_, _ = rand.Read(uuid)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return hex.EncodeToString(uuid)
}

func GenerateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("req_%s", hex.EncodeToString(b))
}
