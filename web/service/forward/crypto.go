// Package forward implements the gost relay-forwarding module of XPanel.
// It talks to gost agents running on relay nodes over WebSocket and collects
// traffic reports over HTTP, fully compatible with the flux-panel agent
// protocol (AES-GCM encrypted, secret = node token).
package forward

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// AESCrypto mirrors the agent-side crypto: key = SHA256(secret), AES-GCM with
// the nonce prepended to the ciphertext, base64 encoded on the wire.
type AESCrypto struct {
	key []byte
}

// NewAESCrypto derives the 32-byte key from the node secret.
func NewAESCrypto(secret string) (*AESCrypto, error) {
	if secret == "" {
		return nil, fmt.Errorf("secret is empty")
	}
	hash := sha256.Sum256([]byte(secret))
	return &AESCrypto{key: hash[:]}, nil
}

// Decrypt opens a base64(nonce+ciphertext) payload produced by the agent.
func (a *AESCrypto) Decrypt(data string) ([]byte, error) {
	if data == "" {
		return nil, fmt.Errorf("empty payload")
	}
	encrypted, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(encrypted) < nonceSize {
		return nil, fmt.Errorf("payload too short")
	}
	return gcm.Open(nil, encrypted[:nonceSize], encrypted[nonceSize:], nil)
}

// Encrypt seals a payload into the base64(nonce+ciphertext) wire format.
func (a *AESCrypto) Encrypt(data []byte) (string, error) {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, data, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}
