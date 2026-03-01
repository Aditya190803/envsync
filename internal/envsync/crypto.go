package envsync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	"golang.org/x/crypto/argon2"
)

func deriveKey(phrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(phrase), salt, 1, 64*1024, 4, 32)
}

func keyCheck(key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte("envsync-key-check"))
	return h.Sum(nil)
}

func encrypt(key []byte, plaintext string) ([]byte, []byte, string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	h := sha256.Sum256([]byte(plaintext))
	return ct, nonce, hex.EncodeToString(h[:]), nil
}

func decrypt(key []byte, v SecretVersion) (string, error) {
	nonce, err := base64.StdEncoding.DecodeString(v.NonceB64)
	if err != nil {
		return "", err
	}
	ct, err := base64.StdEncoding.DecodeString(v.CipherB64)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generatePhrase(words int) (string, error) {
	parts := make([]string, words)
	for i := 0; i < words; i++ {
		idx, err := randomWordIndex(len(bip39WordList))
		if err != nil {
			return "", err
		}
		parts[i] = bip39WordList[idx]
	}
	return strings.Join(parts, " "), nil
}

func randomWordIndex(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("invalid word list")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
