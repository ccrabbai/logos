package log_test

import (
	"context"
	"fmt"
	"testing"

	api "github.com/ccrabbai/logos/api/logs/v1"
	"github.com/ccrabbai/logos/internal/config"
	logstore "github.com/ccrabbai/logos/internal/log"
	"github.com/ccrabbai/logos/internal/observability"
)

func newTestLog(t *testing.T, maxStore, maxIndex uint64) *logstore.Log {
	t.Helper()

	dir := t.TempDir()

	cfg := config.Config{}
	cfg.Segment.InitialBaseOffset = 0
	cfg.Segment.MaxStoreBytes = maxStore
	cfg.Segment.MaxIndexBytes = maxIndex

	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	l, err := logstore.NewLog(dir, cfg, metrics)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}

	t.Cleanup(func() {
		_ = l.Close()
	})

	return l
}

func TestLogAppendAndRead(t *testing.T) {
	l := newTestLog(t, 1024*1024, 1024*1024)
	ctx := context.Background()

	input := &api.Record{Value: []byte("hello")}

	off, err := l.Append(ctx, input)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if off != 0 {
		t.Fatalf("offset = %d, want 0", off)
	}

	got, err := l.Read(ctx, off)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got.Value) != "hello" {
		t.Fatalf("value = %q, want hello", got.Value)
	}
}

func TestLogOffsetsAndMultipleRecords(t *testing.T) {
	l := newTestLog(t, 1024*1024, 1024*1024)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		off, err := l.Append(ctx, &api.Record{
			Value: []byte(fmt.Sprintf("record-%d", i)),
		})
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		if off != uint64(i) {
			t.Fatalf("offset = %d, want %d", off, i)
		}
	}

	low, err := l.LowestOffset()
	if err != nil {
		t.Fatalf("LowestOffset: %v", err)
	}
	high, err := l.HighestOffset()
	if err != nil {
		t.Fatalf("HighestOffset: %v", err)
	}

	if low != 0 || high != 4 {
		t.Fatalf("offset range = [%d,%d], want [0,4]", low, high)
	}

	for i := 0; i < 5; i++ {
		got, err := l.Read(ctx, uint64(i))
		if err != nil {
			t.Fatalf("Read %d: %v", i, err)
		}
		want := fmt.Sprintf("record-%d", i)
		if string(got.Value) != want {
			t.Fatalf("record %d = %q, want %q", i, got.Value, want)
		}
	}
}

func TestLogReadOutOfRange(t *testing.T) {
	l := newTestLog(t, 1024*1024, 1024*1024)

	_, err := l.Read(context.Background(), 0)
	if err == nil {
		t.Fatal("Read from empty log succeeded")
	}

	if _, err := l.Append(context.Background(),
		&api.Record{Value: []byte("one")}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	_, err = l.Read(context.Background(), 1)
	if err == nil {
		t.Fatal("Read beyond highest offset succeeded")
	}
}

func TestLogRollover(t *testing.T) {
	l := newTestLog(t, 1, 1024*1024)
	ctx := context.Background()

	const count = 3

	for i := 0; i < count; i++ {
		off, err := l.Append(ctx, &api.Record{
			Value: []byte(fmt.Sprintf("record-%d", i)),
		})
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		if off != uint64(i) {
			t.Fatalf("offset = %d, want %d", off, i)
		}
	}

	for i := 0; i < count; i++ {
		got, err := l.Read(ctx, uint64(i))
		if err != nil {
			t.Fatalf("Read %d after rollover: %v", i, err)
		}
		if string(got.Value) != fmt.Sprintf("record-%d", i) {
			t.Fatalf("unexpected value at offset %d: %q", i, got.Value)
		}
	}
}

func TestLogPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{}
	cfg.Segment.MaxStoreBytes = 1024 * 1024
	cfg.Segment.MaxIndexBytes = 1024 * 1024

	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	l, err := logstore.NewLog(dir, cfg, metrics)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := l.Append(ctx, &api.Record{
			Value: []byte(fmt.Sprintf("record-%d", i)),
		}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := logstore.NewLog(dir, cfg, metrics)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	high, err := reopened.HighestOffset()
	if err != nil {
		t.Fatalf("HighestOffset: %v", err)
	}
	if high != 2 {
		t.Fatalf("highest offset = %d, want 2", high)
	}

	for i := 0; i < 3; i++ {
		got, err := reopened.Read(ctx, uint64(i))
		if err != nil {
			t.Fatalf("Read %d: %v", i, err)
		}
		if string(got.Value) != fmt.Sprintf("record-%d", i) {
			t.Fatalf("unexpected value at %d: %q", i, got.Value)
		}
	}
}

func TestLogTruncateRemovesCompleteSegments(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{}
	cfg.Segment.MaxStoreBytes = 1
	cfg.Segment.MaxIndexBytes = 1024 * 1024

	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	l, err := logstore.NewLog(dir, cfg, metrics)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	defer l.Close()

	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if _, err := l.Append(ctx, &api.Record{
			Value: []byte(fmt.Sprintf("record-%d", i)),
		}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	if err := l.Truncate(1); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	low, err := l.LowestOffset()
	if err != nil {
		t.Fatalf("LowestOffset: %v", err)
	}
	if low < 2 {
		t.Fatalf("lowest offset after truncation = %d, want >= 2", low)
	}

	// This is segment-granular truncation. The test intentionally does
	// not require deletion of a partial segment.
}