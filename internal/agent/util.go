package agent

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"time"
)

var retryDelays = []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}

// retryOnConnErr повторяет fn до 3 раз с интервалами 1s/3s/5s при сетевых ошибках соединения.
func retryOnConnErr(fn func() error) error {
	err := fn()
	for _, delay := range retryDelays {
		if err == nil || !isRetriableNetError(err) {
			break
		}
		time.Sleep(delay)
		err = fn()
	}
	return err
}

func isRetriableNetError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

func Compress(data []byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write(data)
	gz.Close()
	return buf.Bytes()
}

func ComputeHMAC(message []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}
