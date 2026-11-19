// Package crypto реализует гибридное шифрование полезной нагрузки метрик.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

const keySize = 32

// LoadPublicKey загружает открытый RSA-ключ в формате PEM (PKCS#1, PKIX или сертификат).
func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("decode public key PEM: no PEM block found")
	}

	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		key, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is %T, want RSA", parsed)
		}
		return key, nil
	}
	cert, certErr := x509.ParseCertificate(block.Bytes)
	if certErr == nil {
		key, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("certificate public key is %T, want RSA", cert.PublicKey)
		}
		return key, nil
	}
	return nil, fmt.Errorf("parse RSA public key: %w", err)
}

// LoadPrivateKey загружает закрытый RSA-ключ в формате PEM (PKCS#1 или PKCS#8).
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("decode private key PEM: no PEM block found")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is %T, want RSA", parsed)
	}
	return key, nil
}

// Encrypt шифрует данные произвольной длины с помощью AES-GCM, а свежий ключ
// AES-256 защищает алгоритмом RSA-OAEP-SHA256. Результат содержит длину
// зашифрованного RSA-ключа (4 байта, big-endian), RSA-шифротекст, nonce GCM
// и аутентифицированный шифротекст.
func Encrypt(publicKey *rsa.PublicKey, data []byte) ([]byte, error) {
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate data encryption key: %w", err)
	}
	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, key, nil)
	if err != nil {
		return nil, fmt.Errorf("encrypt data encryption key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate AES-GCM nonce: %w", err)
	}

	result := make([]byte, 4, 4+len(encryptedKey)+len(nonce)+len(data)+aead.Overhead())
	binary.BigEndian.PutUint32(result[:4], uint32(len(encryptedKey)))
	result = append(result, encryptedKey...)
	result = append(result, nonce...)
	result = aead.Seal(result, nonce, data, nil)
	return result, nil
}

// Decrypt выполняет обратное преобразование Encrypt и отклоняет повреждённые
// или не прошедшие аутентификацию данные.
func Decrypt(privateKey *rsa.PrivateKey, data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, errors.New("encrypted payload is too short")
	}
	keyLength := uint64(binary.BigEndian.Uint32(data[:4]))
	if keyLength == 0 || keyLength > uint64(len(data)-4) {
		return nil, errors.New("invalid encrypted key length")
	}
	keyEnd := 4 + int(keyLength)
	key, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, data[4:keyEnd], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt data encryption key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM: %w", err)
	}
	if len(data)-keyEnd < aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("encrypted payload is too short")
	}
	nonceEnd := keyEnd + aead.NonceSize()
	plaintext, err := aead.Open(nil, data[keyEnd:nonceEnd], data[nonceEnd:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}
	return plaintext, nil
}
