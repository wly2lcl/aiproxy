package utils

import (
	"strings"
	"testing"
)

func TestHashAPIKey_Consistent(t *testing.T) {
	key := "test-api-key-12345"
	hash1 := HashAPIKey(key)
	hash2 := HashAPIKey(key)
	if hash1 != hash2 {
		t.Errorf("HashAPIKey should return consistent results: %s != %s", hash1, hash2)
	}
}

func TestHashAPIKey_DifferentKeys(t *testing.T) {
	key1 := "test-api-key-1"
	key2 := "test-api-key-2"
	hash1 := HashAPIKey(key1)
	hash2 := HashAPIKey(key2)
	if hash1 == hash2 {
		t.Errorf("HashAPIKey should return different hashes for different keys")
	}
}

func TestVerifyAPIKey_Valid(t *testing.T) {
	key := "test-api-key-12345"
	hash := HashAPIKey(key)
	if !VerifyAPIKey(key, hash) {
		t.Errorf("VerifyAPIKey should return true for valid key-hash pair")
	}
}

func TestVerifyAPIKey_Invalid(t *testing.T) {
	key := "test-api-key-12345"
	hash := HashAPIKey(key)
	if VerifyAPIKey("wrong-key", hash) {
		t.Errorf("VerifyAPIKey should return false for invalid key")
	}
}

func TestGenerateUUID_Format(t *testing.T) {
	uuid := GenerateUUID()
	if len(uuid) != 32 {
		t.Errorf("GenerateUUID should return 32 character string, got %d", len(uuid))
	}
	for _, c := range uuid {
		if !strings.Contains("0123456789abcdef", string(c)) {
			t.Errorf("GenerateUUID should return hex string, got %s", uuid)
			break
		}
	}
}

func TestGenerateUUID_Uniqueness(t *testing.T) {
	uuids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		uuid := GenerateUUID()
		if uuids[uuid] {
			t.Errorf("GenerateUUID should return unique values")
			break
		}
		uuids[uuid] = true
	}
}

func TestGenerateRequestID_Format(t *testing.T) {
	reqID := GenerateRequestID()
	if !strings.HasPrefix(reqID, "req_") {
		t.Errorf("GenerateRequestID should start with 'req_', got %s", reqID)
	}
	if len(reqID) != 20 {
		t.Errorf("GenerateRequestID should return 20 character string (req_ + 16 hex), got %d", len(reqID))
	}
	hexPart := reqID[4:]
	for _, c := range hexPart {
		if !strings.Contains("0123456789abcdef", string(c)) {
			t.Errorf("GenerateRequestID hex part should be valid hex, got %s", hexPart)
			break
		}
	}
}

func TestGenerateRequestID_Prefix(t *testing.T) {
	reqID := GenerateRequestID()
	if reqID[:4] != "req_" {
		t.Errorf("GenerateRequestID should have 'req_' prefix, got %s", reqID[:4])
	}
}
