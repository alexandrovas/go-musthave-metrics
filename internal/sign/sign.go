// Package sign реализует подпись передаваемых данных по алгоритму SHA256:
// hash(value, key) = SHA256(value || key), в шестнадцатеричном представлении.
package sign

import (
	"crypto/sha256"
	"encoding/hex"
)

// Compute возвращает шестнадцатеричный SHA256-хеш от data с учётом key.
func Compute(data []byte, key string) string {
	h := sha256.New()
	h.Write(data)
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))
}
