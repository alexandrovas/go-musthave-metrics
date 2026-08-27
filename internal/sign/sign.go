// Package sign реализует подпись передаваемых данных по алгоритму SHA256:
// hash(value, key) = SHA256(value || key), в base64-представлении.
package sign

import (
	"crypto/sha256"
	"encoding/base64"
)

// Compute возвращает base64-кодированный SHA256-хеш от data с учётом key.
func Compute(data []byte, key string) string {
	h := sha256.New()
	h.Write(data)
	h.Write([]byte(key))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
