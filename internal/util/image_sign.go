package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

func GenerateImageToken(path string, secret string, ttl int) string {
	expires := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
	signature := signImageURL(path, expires, secret)
	return strconv.FormatInt(expires, 10) + "." + signature
}

func signImageURL(path string, expires int64, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(path + ":" + strconv.FormatInt(expires, 10)))
	return hex.EncodeToString(h.Sum(nil))
}

func ValidateImageToken(path string, token string, secret string) bool {
	parts := splitToken(token)
	if len(parts) != 2 {
		return false
	}

	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}

	if time.Now().Unix() > expires {
		return false
	}

	expectedSig := signImageURL(path, expires, secret)
	return hmac.Equal([]byte(parts[1]), []byte(expectedSig))
}

func splitToken(token string) []string {
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			return []string{token[:i], token[i+1:]}
		}
	}
	return nil
}
