package middlewareproj

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	http.ResponseWriter
	gz         *gzip.Writer
	status     int
	wrote      bool
	clientGzip bool
}

func newGzipResponseWriter(w http.ResponseWriter, clientGzip bool) *gzipResponseWriter {
	return &gzipResponseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
		clientGzip:     clientGzip,
	}
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	g.status = code
}

func (g *gzipResponseWriter) Write(data []byte) (int, error) {
	if !g.wrote {
		g.wrote = true
		ct := g.Header().Get("Content-Type")
		shouldGzip := g.clientGzip &&
			(strings.Contains(ct, "application/json") || strings.Contains(ct, "text/html"))
		if shouldGzip {
			g.Header().Set("Content-Encoding", "gzip")
			g.gz = gzip.NewWriter(g.ResponseWriter)
		}
		g.ResponseWriter.WriteHeader(g.status)
	}
	if g.gz != nil {
		return g.gz.Write(data)
	}
	return g.ResponseWriter.Write(data)
}

func (g *gzipResponseWriter) close() {
	if g.gz != nil {
		g.gz.Close()
	}
	if !g.wrote {
		g.ResponseWriter.WriteHeader(g.status)
	}
}

func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Invalid gzip body", http.StatusBadRequest)
				return
			}
			defer gz.Close()
			r.Body = io.NopCloser(gz)
		}

		clientGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
		grw := newGzipResponseWriter(w, clientGzip)
		defer grw.close()
		next.ServeHTTP(grw, r)
	})
}
