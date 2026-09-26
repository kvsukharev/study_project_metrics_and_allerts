package crypto_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/crypto"
)

func generateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	priv := generateKey(t)
	plaintext := []byte(`[{"id":"cpu","type":"gauge","value":0.42}]`)

	ciphertext, err := crypto.Encrypt(&priv.PublicKey, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext must differ from plaintext")
	}

	got, err := crypto.Decrypt(priv, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("roundtrip mismatch:\n got  %q\n want %q", got, plaintext)
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	priv := generateKey(t)
	plaintext := []byte("same message")

	c1, err := crypto.Encrypt(&priv.PublicKey, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := crypto.Encrypt(&priv.PublicKey, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	// Random nonce + random AES key → ciphertexts must differ.
	if bytes.Equal(c1, c2) {
		t.Error("two encryptions of the same plaintext must produce different ciphertexts")
	}
}

func TestDecryptTruncatedData(t *testing.T) {
	priv := generateKey(t)
	_, err := crypto.Decrypt(priv, []byte{0x00}) // too short
	if err == nil {
		t.Error("expected error for truncated data")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	priv1 := generateKey(t)
	priv2 := generateKey(t)

	ciphertext, err := crypto.Encrypt(&priv1.PublicKey, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = crypto.Decrypt(priv2, ciphertext)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}
