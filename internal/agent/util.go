package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

var retryDelays = []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}

// retryOnConnErr повторяет fn до 3 раз с интервалами 1s/3s/5s при сетевых ошибках соединения.
// Отменяется при завершении ctx.
func retryOnConnErr(ctx context.Context, fn func() error) error {
	err := fn()
	for _, delay := range retryDelays {
		if err == nil || !isRetriableNetError(err) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		err = fn()
	}
	return err
}

func isRetriableNetError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

var (
	gzipWriterPool = sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(io.Discard)
		},
	}
	compressBufPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
)

// Compress gzip-compresses data and returns the compressed bytes.
// gzip.Writer and bytes.Buffer are reused via sync.Pool to minimise allocations.
func Compress(data []byte) []byte {
	buf := compressBufPool.Get().(*bytes.Buffer)
	buf.Reset()

	gz := gzipWriterPool.Get().(*gzip.Writer)
	gz.Reset(buf)
	gz.Write(data)
	gz.Close()
	gzipWriterPool.Put(gz)

	// Copy result before returning buf to the pool.
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	compressBufPool.Put(buf)
	return result
}

// ComputeHMAC returns the hex-encoded HMAC-SHA256 of message signed with key.
func ComputeHMAC(message []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}
