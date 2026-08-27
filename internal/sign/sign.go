// Package sign реализует подпись передаваемых данных по алгоритму HMAC-SHA256:
// hash(data, key) = HMAC-SHA256(key, data), в base64-представлении.
package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// Compute возвращает base64-кодированный HMAC-SHA256 от data с учётом key.
func Compute(data []byte, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write(data)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
