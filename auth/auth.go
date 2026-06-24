package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type Tokens struct {
	Short   string `json:"token_short"`
	Refresh string `json:"token_refresh"`
}

func HashPassword(password string) (string, error) {
	salt, err := NewToken(16)
	if err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), []byte(salt), 1, 64*1024, 4, 32)
	return fmt.Sprintf("%s$%s", salt, base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	salt, _ := base64.RawStdEncoding.DecodeString(parts[0])

	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(hash) == parts[1]
}

func VerifyHMAC(sig, token, data []byte) bool {
	mac := hmac.New(sha256.New, token)
	mac.Write(data)
	verify := base64.RawStdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal(sig, []byte(verify))
}

// func VerifySignature(req *http.Request, body []byte) bool {
// 	sig := req.Header.Get("x-rikami-signature")
// 	if sig == "" {
// 		return false
// 	}
//
//
// }

func NewTokens() (*Tokens, error) {
	tokens := make([]string, 2)

	for i := range tokens {
		var err error
		tokens[i], err = NewToken(32)
		if err != nil {
			return nil, err
		}
	}

	return &Tokens{Short: tokens[0], Refresh: tokens[1]}, nil
}

func NewToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b), nil
}
