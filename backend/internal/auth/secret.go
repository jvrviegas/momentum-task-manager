package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// tokenPrefix marks Momentum tokens so secret scanners can recognize them.
const tokenPrefix = "mtk_"

// newSecret returns 32 random bytes as URL-safe text, after prefix, and the
// hash to store in its place.
func newSecret(prefix string) (string, []byte) {
	data := make([]byte, 32)
	_, _ = rand.Read(data)
	secret := prefix + base64.RawURLEncoding.EncodeToString(data)
	return secret, hashSecret(secret)
}

// hashSecret hashes a high-entropy secret for storage and lookup. A plain
// SHA-256 is enough: the secrets are random, so there is nothing to guess.
func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// userCodeAlphabet has no vowels, so codes do not spell words, and no
// letters that look alike (RFC 8628, section 6.1).
const userCodeAlphabet = "BCDFGHJKLMNPQRSTVWXZ"

// newUserCode returns eight letters as XXXX-XXXX.
func newUserCode() string {
	data := make([]byte, 8)
	_, _ = rand.Read(data)
	code := make([]byte, 0, 9)
	for index, b := range data {
		if index == 4 {
			code = append(code, '-')
		}
		// 256 is not a multiple of 20; the bias is under 1% per letter,
		// which leaves well over 30 bits of entropy.
		code = append(code, userCodeAlphabet[int(b)%len(userCodeAlphabet)])
	}
	return string(code)
}

// normalizeUserCode accepts a code in any case, with or without the dash and
// spaces, and returns it as newUserCode writes it, or "" if it cannot be one.
func normalizeUserCode(text string) string {
	var letters []byte
	for _, r := range strings.ToUpper(text) {
		switch {
		case r == '-' || r == ' ':
		case strings.ContainsRune(userCodeAlphabet, r):
			letters = append(letters, byte(r))
		default:
			return ""
		}
	}
	if len(letters) != 8 {
		return ""
	}
	return string(letters[:4]) + "-" + string(letters[4:])
}
