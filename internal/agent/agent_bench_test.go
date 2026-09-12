package agent

import (
	"net/http"
	"testing"
)

func BenchmarkCollectorUpdateMetrics(b *testing.B) {
	c := NewCollector(100, &http.Client{}, "http://localhost:8080")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.UpdateMetrics()
	}
}

func BenchmarkCollectorGetAllMetrics(b *testing.B) {
	c := NewCollector(100, &http.Client{}, "http://localhost:8080")
	c.UpdateMetrics()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.GetAllMetrics()
	}
}

func BenchmarkCollectorGetGauges(b *testing.B) {
	c := NewCollector(100, &http.Client{}, "http://localhost:8080")
	c.UpdateMetrics()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.GetGauges()
	}
}

func BenchmarkCollectorGetCounters(b *testing.B) {
	c := NewCollector(100, &http.Client{}, "http://localhost:8080")
	c.UpdateMetrics()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.GetCounters()
	}
}

func BenchmarkCompress(b *testing.B) {
	// Realistic payload: batch of 29 metrics as JSON (~1.5 KB)
	data := []byte(`[{"id":"Alloc","type":"gauge","value":1234567},{"id":"BuckHashSys","type":"gauge","value":7654},{"id":"Frees","type":"gauge","value":9999},{"id":"GCCPUFraction","type":"gauge","value":0.001},{"id":"GCSys","type":"gauge","value":4096},{"id":"HeapAlloc","type":"gauge","value":1234567},{"id":"HeapIdle","type":"gauge","value":8388608},{"id":"HeapInuse","type":"gauge","value":2097152},{"id":"HeapObjects","type":"gauge","value":1500},{"id":"HeapReleased","type":"gauge","value":4194304},{"id":"HeapSys","type":"gauge","value":10485760},{"id":"LastGC","type":"gauge","value":1234567890},{"id":"Lookups","type":"gauge","value":0},{"id":"MCacheInuse","type":"gauge","value":9600},{"id":"MCacheSys","type":"gauge","value":15600},{"id":"MSpanInuse","type":"gauge","value":57344},{"id":"MSpanSys","type":"gauge","value":65536},{"id":"Mallocs","type":"gauge","value":11499},{"id":"NextGC","type":"gauge","value":2097152},{"id":"NumForcedGC","type":"gauge","value":0},{"id":"NumGC","type":"gauge","value":3},{"id":"OtherSys","type":"gauge","value":1048576},{"id":"PauseTotalNs","type":"gauge","value":123456},{"id":"StackInuse","type":"gauge","value":425984},{"id":"StackSys","type":"gauge","value":425984},{"id":"Sys","type":"gauge","value":12582912},{"id":"TotalAlloc","type":"gauge","value":2345678},{"id":"RandomValue","type":"gauge","value":0.7342},{"id":"PollCount","type":"counter","delta":42}]`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Compress(data)
	}
}

func BenchmarkCompressSingle(b *testing.B) {
	data := []byte(`{"id":"Alloc","type":"gauge","value":1234567}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Compress(data)
	}
}

func BenchmarkComputeHMAC(b *testing.B) {
	data := []byte(`{"id":"Alloc","type":"gauge","value":1234567}`)
	key := "benchmark-secret-key"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ComputeHMAC(data, key)
	}
}
