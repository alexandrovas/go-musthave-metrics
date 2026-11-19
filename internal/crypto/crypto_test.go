package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptLargePayload(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	message := make([]byte, 128*1024)
	_, err = rand.Read(message)
	require.NoError(t, err)

	ciphertext, err := Encrypt(&privateKey.PublicKey, message)
	require.NoError(t, err)
	plaintext, err := Decrypt(privateKey, ciphertext)
	require.NoError(t, err)
	require.Equal(t, message, plaintext)
}

func TestDecryptRejectsInvalidAndTamperedData(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	_, err = Decrypt(privateKey, []byte("bad"))
	require.Error(t, err)

	ciphertext, err := Encrypt(&privateKey.PublicKey, []byte("secret"))
	require.NoError(t, err)
	ciphertext[len(ciphertext)-1] ^= 0xff
	_, err = Decrypt(privateKey, ciphertext)
	require.Error(t, err)
}

func TestLoadKeys(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privatePath := filepath.Join(t.TempDir(), "private.pem")
	publicPath := filepath.Join(t.TempDir(), "public.pem")

	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: func() []byte {
		der, marshalErr := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
		require.NoError(t, marshalErr)
		return der
	}()})
	require.NoError(t, os.WriteFile(privatePath, privatePEM, 0o600))
	require.NoError(t, os.WriteFile(publicPath, publicPEM, 0o600))

	loadedPrivate, err := LoadPrivateKey(privatePath)
	require.NoError(t, err)
	loadedPublic, err := LoadPublicKey(publicPath)
	require.NoError(t, err)
	require.Equal(t, privateKey.N, loadedPrivate.N)
	require.Equal(t, privateKey.N, loadedPublic.N)
}
