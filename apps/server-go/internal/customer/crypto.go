package customer

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
)

type MobileCodec struct {
	aead    cipher.AEAD
	hashKey []byte
}

func NewMobileCodec(encryptionKey, hashKey string) (*MobileCodec, error) {
	a, e := base64.StdEncoding.DecodeString(encryptionKey)
	if e != nil || len(a) != 32 {
		return nil, errors.New("CUSTOMER_MOBILE_ENCRYPTION_KEY must be base64 of 32 bytes")
	}
	h, e := base64.StdEncoding.DecodeString(hashKey)
	if e != nil || len(h) < 32 || hmac.Equal(a, h) {
		return nil, errors.New("CUSTOMER_MOBILE_HASH_KEY must be independent base64 of at least 32 bytes")
	}
	block, _ := aes.NewCipher(a)
	g, _ := cipher.NewGCM(block)
	return &MobileCodec{g, h}, nil
}
func (c *MobileCodec) Hash(v string) []byte {
	h := hmac.New(sha256.New, c.hashKey)
	h.Write([]byte(v))
	return h.Sum(nil)
}
func (c *MobileCodec) Encrypt(v string) ([]byte, error) {
	n := make([]byte, c.aead.NonceSize())
	if _, e := rand.Read(n); e != nil {
		return nil, e
	}
	return c.aead.Seal(n, n, []byte(v), []byte("fixpro.customer.mobile.v1")), nil
}
func (c *MobileCodec) Decrypt(v []byte) (string, error) {
	n := c.aead.NonceSize()
	if len(v) < n {
		return "", errors.New("invalid encrypted mobile")
	}
	b, e := c.aead.Open(nil, v[:n], v[n:], []byte("fixpro.customer.mobile.v1"))
	return string(b), e
}

var phonePattern = regexp.MustCompile(`^1\d{10}$`)

func NormalizePhone(v string) (string, error) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "+86")
	if !phonePattern.MatchString(v) {
		return "", errors.New("unsupported phone")
	}
	return "+86" + v, nil
}
func MaskPhone(v string) string {
	v = strings.TrimPrefix(v, "+86")
	if len(v) != 11 {
		return ""
	}
	return v[:3] + "****" + v[7:]
}
