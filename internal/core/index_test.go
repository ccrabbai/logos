package core_test

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/core"
	"github.com/ccrabbai/logos/internal/util"
)

type Config struct {
	Segment struct {
		maxBytes uint64
		MaxIndexBytes uint64
		InitialBaseOffset uint64
	}
}

func newTestIndex(t *testing.T, maxBytes uint64) (*core.Index, string) {
	t.Helper()

	cfg := config.Config{}
	cfg.Segment.MaxIndexBytes = maxBytes

	dir := t.TempDir()
	path := dir + "/index.index"

	f, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_RDWR,
		0600,
	)
	if err != nil {
		t.Fatalf("open index file: %v", err)
	}

	index, err := core.NewIndex(f, cfg)
	if err != nil {
		f.Close()
		t.Fatalf("create index: %v", err)
	}

	t.Cleanup(func() {
		_ = index.Close()
	})

	return index, path
}

func TestIndexWriteAndRead(t *testing.T) {
	index, _ := newTestIndex(t, 1024)

	if err := index.Write(0, 0); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := index.Write(1, 17); err != nil {
		t.Fatalf("write: %v", err)
	}

	off, pos, err := index.Read(0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if off != 0 || pos != 0 {
		t.Fatalf("entry = (%d, %d), want (0, 0)", off, pos)
	}

	off, pos, err = index.Read(1)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if off != 1 || pos != 17 {
		t.Fatalf("entry = (%d, %d), want (1, 17)", off, pos)
	}
}

func TestIndexReadLastEntry(t *testing.T) {
	index, _ := newTestIndex(t, 1024)

	entries := []struct {
		off uint32
		pos uint64
	}{
		{0, 10},
		{1, 25},
		{2, 40},
	}

	for _, entry := range entries {
		if err := index.Write(entry.off, entry.pos); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	off, pos, err := index.Read(-1)
	if err != nil {
		t.Fatalf("read last: %v", err)
	}

	if off != 2 || pos != 40 {
		t.Fatalf(
			"last entry = (%d, %d), want (2, 40)",
			off,
			pos,
		)
	}
}

func TestIndexEmptyReadReturnsEOF(t *testing.T) {
	index, _ := newTestIndex(t, 1024)

	_, _, err := index.Read(0)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("read empty index error = %v, want EOF", err)
	}

	_, _, err = index.Read(-1)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("read last empty index error = %v, want EOF", err)
	}
}

func TestIndexOutOfRangeReturnsEOF(t *testing.T) {
	index, _ := newTestIndex(t, 1024)

	if err := index.Write(0, 100); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := index.Read(1)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("out-of-range error = %v, want EOF", err)
	}
}

func TestIndexPersistsAcrossReopen(t *testing.T) {
	cfg := config.Config{}
	cfg.Segment.MaxIndexBytes = 1024

	index, path := newTestIndex(t, 1024)

	entries := []struct {
		off uint32
		pos uint64
	}{
		{0, 100},
		{1, 200},
		{2, 300},
	}

	for _, entry := range entries {
		if err := index.Write(entry.off, entry.pos); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	if err := index.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	reopened, err := core.NewIndex(f, cfg)
	if err != nil {
		f.Close()
		t.Fatalf("recreate index: %v", err)
	}

	t.Cleanup(func() {
		_ = reopened.Close()
	})

	for i, want := range entries {
		off, pos, err := reopened.Read(int64(i))
		if err != nil {
			t.Fatalf("read entry %d: %v", i, err)
		}

		if off != want.off || pos != want.pos {
			t.Fatalf(
				"entry %d = (%d, %d), want (%d, %d)",
				i,
				off,
				pos,
				want.off,
				want.pos,
			)
		}
	}
}

func TestIndexTruncateRemovesLogicalEntries(t *testing.T) {
	index, _ := newTestIndex(t, 1024)

	for i := uint32(0); i < 4; i++ {
		if err := index.Write(i, uint64(i*100)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	wantSize := uint64(2 * util.EntWidth)

	if err := index.Truncate(wantSize); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	if index.Size != wantSize {
		t.Fatalf("size = %d, want %d", index.Size, wantSize)
	}

	off, pos, err := index.Read(1)
	if err != nil {
		t.Fatalf("read retained entry: %v", err)
	}

	if off != 1 || pos != 100 {
		t.Fatalf(
			"retained entry = (%d, %d), want (1, 100)",
			off,
			pos,
		)
	}

	_, _, err = index.Read(2)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("read truncated entry error = %v, want EOF", err)
	}
}

func TestIndexCapacity(t *testing.T) {
	maxBytes := uint64(2 * util.EntWidth)

	index, _ := newTestIndex(t, maxBytes)

	if err := index.Write(0, 100); err != nil {
		t.Fatalf("first write: %v", err)
	}

	if err := index.Write(1, 200); err != nil {
		t.Fatalf("second write: %v", err)
	}

	if index.Size != maxBytes {
		t.Fatalf("size = %d, want %d", index.Size, maxBytes)
	}

	if err := index.Write(2, 300); err == nil {
		t.Fatal("write beyond index capacity succeeded")
	}
}