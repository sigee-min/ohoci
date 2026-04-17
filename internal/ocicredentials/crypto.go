package ocicredentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

func (s *Service) encrypt(plain string) (string, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return "v1:" + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *Service) decrypt(ciphertext string) (string, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, "v1:") {
		return "", fmt.Errorf("unsupported ciphertext format")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, "v1:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext payload is too short")
	}
	nonce := raw[:gcm.NonceSize()]
	payload := raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
