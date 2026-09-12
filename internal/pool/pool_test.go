package pool_test

import (
	"testing"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/pool"
)

type item struct {
	Value int
	Tags  []string
}

func (i *item) Reset() {
	i.Value = 0
	i.Tags = i.Tags[:0]
}

func TestGetReturnsNonNil(t *testing.T) {
	p := pool.New(func() *item { return &item{} })
	got := p.Get()
	if got == nil {
		t.Fatal("Get returned nil")
	}
}

func TestPutResetsState(t *testing.T) {
	p := pool.New(func() *item { return &item{} })

	v := p.Get()
	v.Value = 42
	v.Tags = append(v.Tags, "x", "y")
	p.Put(v)

	got := p.Get()
	if got.Value != 0 {
		t.Errorf("Value after Put: got %d, want 0", got.Value)
	}
	if len(got.Tags) != 0 {
		t.Errorf("Tags after Put: got %v, want empty", got.Tags)
	}
}

func TestGetPutRoundtrip(t *testing.T) {
	p := pool.New(func() *item { return &item{Tags: make([]string, 0, 4)} })

	v := p.Get()
	v.Value = 7
	p.Put(v)

	v2 := p.Get()
	if v2.Value != 0 {
		t.Errorf("expected reset value, got %d", v2.Value)
	}
}
