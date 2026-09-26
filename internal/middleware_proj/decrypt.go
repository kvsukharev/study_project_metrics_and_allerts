package middlewareproj

import (
	"bytes"
	"compress/gzip"
	"crypto/rsa"
	"io"
	"log"
	"net/http"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/crypto"
)

// DecryptMiddleware returns a middleware that decrypts request bodies sent
// by the agent when Content-Encoding: encrypted is set.
// The body is decrypted with privKey (RSA-OAEP + AES-256-GCM), then the
// inner gzip layer is decompressed, and the result is re-set as the request body.
// If privKey is nil the middleware is a no-op pass-through.
func DecryptMiddleware(privKey *rsa.PrivateKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if privKey == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Encoding") != "encrypted" {
				next.ServeHTTP(w, r)
				return
			}

			ciphertext, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil {
				log.Printf("decrypt middleware: read body: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			compressed, err := crypto.Decrypt(privKey, ciphertext)
			if err != nil {
				log.Printf("decrypt middleware: decrypt: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			gr, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				log.Printf("decrypt middleware: gzip reader: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			defer gr.Close()

			plain, err := io.ReadAll(gr)
			if err != nil {
				log.Printf("decrypt middleware: gzip read: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(plain))
			r.ContentLength = int64(len(plain))
			r.Header.Del("Content-Encoding")
			next.ServeHTTP(w, r)
		})
	}
}
