package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
)

const (
	passwordHashIterations = 100_000
	passwordSaltSize       = 16
)

var decoyPasswordHash = sync.OnceValue(func() string {
	salt := make([]byte, passwordSaltSize)
	hash := pbkdf2SHA256([]byte("decoy"), salt, passwordHashIterations, sha256.Size)

	return fmt.Sprintf(
		"pbkdf2-sha256$%d$%s$%s",
		passwordHashIterations,
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(hash),
	)
})

func hashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := pbkdf2SHA256([]byte(password), salt, passwordHashIterations, sha256.Size)

	return fmt.Sprintf(
		"pbkdf2-sha256$%d$%s$%s",
		passwordHashIterations,
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(hash),
	), nil
}

func checkPasswordHash(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}

	var iterations int

	if _, err := fmt.Sscanf(parts[1], "%d", &iterations); err != nil || iterations <= 0 {
		return false
	}

	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}

	actualHash := pbkdf2SHA256([]byte(password), salt, iterations, len(expectedHash))

	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hashLen := sha256.Size
	blockCount := (keyLen + hashLen - 1) / hashLen
	result := make([]byte, 0, blockCount*hashLen)

	for block := 1; block <= blockCount; block++ {
		u := pbkdf2Block(password, salt, iterations, block)
		result = append(result, u...)
	}

	return result[:keyLen]
}

func pbkdf2Block(password, salt []byte, iterations, blockNum int) []byte {
	mac := hmac.New(sha256.New, password)
	_, _ = mac.Write(salt)
	_, _ = mac.Write([]byte{
		byte(blockNum >> 24), //nolint:mnd // Standard byte shift for PBKDF2
		byte(blockNum >> 16), //nolint:mnd // Standard byte shift for PBKDF2
		byte(blockNum >> 8),  //nolint:mnd // Standard byte shift for PBKDF2
		byte(blockNum),
	})
	u := mac.Sum(nil)

	out := make([]byte, len(u))
	copy(out, u)

	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, password)
		_, _ = mac.Write(u)
		u = mac.Sum(nil)

		for j := range out {
			out[j] ^= u[j]
		}
	}

	return out
}
