// Package crypto implements hybrid RSA-OAEP + AES-256-GCM encryption used
// to protect agent→server metric payloads.
//
// Wire format produced by Encrypt:
//
//	[2 bytes big-endian: len(encryptedKey)] [encryptedKey] [12-byte GCM nonce] [GCM ciphertext+tag]
//
// The AES-256 key is generated fresh for every message and encrypted with the
// RSA public key (OAEP/SHA-256). The payload is then encrypted with AES-256-GCM.
// This bounds RSA input size to 32 bytes regardless of payload length.
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
	"io"
	"os"
)

// LoadPublicKey reads a PEM-encoded RSA public key from path.
func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypto: read public key %s: %w", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("crypto: no PEM block in %s", path)
	}
	switch block.Type {
	case "RSA PUBLIC KEY":
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("crypto: parse PKCS1 public key: %w", err)
		}
		return key, nil
	case "PUBLIC KEY":
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("crypto: parse PKIX public key: %w", err)
		}
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("crypto: key is not RSA")
		}
		return rsaPub, nil
	default:
		return nil, fmt.Errorf("crypto: unexpected PEM type %q", block.Type)
	}
}

// LoadPrivateKey reads a PEM-encoded RSA private key from path.
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypto: read private key %s: %w", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("crypto: no PEM block in %s", path)
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("crypto: parse PKCS1 private key: %w", err)
		}
		return key, nil
	case "PRIVATE KEY":
		priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("crypto: parse PKCS8 private key: %w", err)
		}
		rsaPriv, ok := priv.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("crypto: key is not RSA")
		}
		return rsaPriv, nil
	default:
		return nil, fmt.Errorf("crypto: unexpected PEM type %q", block.Type)
	}
}

// Encrypt encrypts plaintext with pub using hybrid RSA-OAEP + AES-256-GCM.
func Encrypt(pub *rsa.PublicKey, plaintext []byte) ([]byte, error) {
	// Generate a random 32-byte AES-256 key.
	aesKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, aesKey); err != nil {
		return nil, fmt.Errorf("crypto: generate AES key: %w", err)
	}

	// Encrypt the AES key with RSA-OAEP.
	encKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, aesKey, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: RSA encrypt: %w", err)
	}

	// Encrypt the plaintext with AES-256-GCM.
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: new AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Assemble wire format: [2-byte key len][encKey][nonce][ciphertext].
	out := make([]byte, 2+len(encKey)+len(nonce)+len(ciphertext))
	binary.BigEndian.PutUint16(out[:2], uint16(len(encKey)))
	copy(out[2:], encKey)
	copy(out[2+len(encKey):], nonce)
	copy(out[2+len(encKey)+len(nonce):], ciphertext)
	return out, nil
}

// Decrypt decrypts data produced by Encrypt using the RSA private key.
func Decrypt(priv *rsa.PrivateKey, data []byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, errors.New("crypto: ciphertext too short")
	}
	keyLen := int(binary.BigEndian.Uint16(data[:2]))
	if len(data) < 2+keyLen {
		return nil, errors.New("crypto: ciphertext truncated (key)")
	}

	encKey := data[2 : 2+keyLen]
	rest := data[2+keyLen:]

	// Decrypt the AES key.
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, encKey, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: RSA decrypt: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: new AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(rest) < nonceSize {
		return nil, errors.New("crypto: ciphertext truncated (nonce)")
	}
	nonce, ciphertext := rest[:nonceSize], rest[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: AES-GCM decrypt: %w", err)
	}
	return plaintext, nil
}
