package segment_test

import (
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	api "github.com/ccrabbai/logos/api/logs/v1"
	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/segment"
)

func newTestSegment(t *testing.T, base uint64) *segment.Segment {
	t.Helper()

	dir := t.TempDir()
	cfg := config.Config{}
	cfg.Segment.InitialBaseOffset = base
	cfg.Segment.MaxStoreBytes = 1024 * 1024
	cfg.Segment.MaxIndexBytes = 1024 * 1024

	s, err := segment.NewSegment(dir, base, cfg)
	if err != nil {
		t.Fatalf("NewSegment: %v", err)
	}

	t.Cleanup(func() {
		_ = s.Close()
	})

	return s
}

func TestSegmentAppendAssignsMetadata(t *testing.T) {
	s := newTestSegment(t, 10)

	record := &api.Record{
		Value: []byte("hello"),
	}

	off, err := s.Append(record)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	if off != 10 {
		t.Fatalf("offset = %d, want 10", off)
	}
	if record.Offset != 10 {
		t.Fatalf("record offset = %d, want 10", record.Offset)
	}
	if record.CreatedAt <= 0 {
		t.Fatalf("CreatedAt = %d, want positive timestamp", record.CreatedAt)
	}
	if record.Checksum != crc32.ChecksumIEEE(record.Value) {
		t.Fatalf("checksum = %d, want %d",
			record.Checksum, crc32.ChecksumIEEE(record.Value))
	}
	if record.ProducerId == "" {
		t.Fatal("default ProducerId was not assigned")
	}
	if s.NextOffset != 11 {
		t.Fatalf("NextOffset = %d, want 11", s.NextOffset)
	}
}

func TestSegmentAppendAndRead(t *testing.T) {
	s := newTestSegment(t, 20)

	inputs := []*api.Record{
		{Value: []byte("first"), ProducerId: "producer-a"},
		{Value: []byte("second"), ProducerId: "producer-b"},
		{Value: []byte{0, 1, 2, 255}, ProducerId: "producer-c"},
	}

	for i, input := range inputs {
		off, err := s.Append(input)
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}

		if off != 20+uint64(i) {
			t.Fatalf("offset = %d, want %d", off, 20+uint64(i))
		}
	}

	for i, input := range inputs {
		got, err := s.Read(20 + uint64(i))
		if err != nil {
			t.Fatalf("Read %d: %v", i, err)
		}

		if !bytes.Equal(got.Value, input.Value) {
			t.Errorf("record %d value = %v, want %v",
				i, got.Value, input.Value)
		}
		if got.Offset != input.Offset {
			t.Errorf("record %d offset = %d, want %d",
				i, got.Offset, input.Offset)
		}
		if got.ProducerId != input.ProducerId {
			t.Errorf("record %d producer = %q, want %q",
				i, got.ProducerId, input.ProducerId)
		}
		if got.Checksum != crc32.ChecksumIEEE(got.Value) {
			t.Errorf("record %d checksum mismatch", i)
		}
	}
}

func TestSegmentReadBeforeBaseOffset(t *testing.T) {
	s := newTestSegment(t, 10)

	_, err := s.Read(9)
	if err == nil {
		t.Fatal("Read below BaseOffset succeeded")
	}
}

func TestSegmentReadBeyondNextOffset(t *testing.T) {
	s := newTestSegment(t, 10)

	_, err := s.Append(&api.Record{Value: []byte("record")})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	_, err = s.Read(11)
	if err == nil {
		t.Fatal("Read beyond NextOffset succeeded")
	}
}

func TestSegmentIsFullByStoreCapacity(t *testing.T) {
	s := newTestSegment(t, 0)

	s.Cfg.Segment.MaxStoreBytes = 1

	// IsFull is based on the current Store size or Index size.
	if _, _, err := s.Store.Append([]byte("x")); err != nil {
		t.Fatalf("Store.Append: %v", err)
	}

	if !s.IsFull() {
		t.Fatalf(
			"segment should be full: Store.Size=%d, MaxStoreBytes=%d",
			s.Store.Size,
			s.Cfg.Segment.MaxStoreBytes,
		)
	}
}

func TestSegmentIsFullByIndexCapacity(t *testing.T) {
	s := newTestSegment(t, 0)

	s.Cfg.Segment.MaxIndexBytes = 1

	if err := s.Index.Write(0, 0); err != nil {
		t.Fatalf("Index.Write: %v", err)
	}

	if !s.IsFull() {
		t.Fatalf(
			"segment should be full: Index.Size=%d, MaxIndexBytes=%d",
			s.Index.Size,
			s.Cfg.Segment.MaxIndexBytes,
		)
	}
}

func TestSegmentReopensAndRecoversRecords(t *testing.T) {
	dir := t.TempDir()

	cfg := config.Config{}
	cfg.Segment.MaxStoreBytes = 1024 * 1024
	cfg.Segment.MaxIndexBytes = 1024 * 1024

	s, err := segment.NewSegment(dir, 5, cfg)
	if err != nil {
		t.Fatalf("NewSegment: %v", err)
	}

	for _, value := range []string{"one", "two"} {
		if _, err := s.Append(&api.Record{Value: []byte(value)}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := segment.NewSegment(dir, 5, cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	if reopened.NextOffset != 7 {
		t.Fatalf("NextOffset = %d, want 7", reopened.NextOffset)
	}

	for i, value := range []string{"one", "two"} {
		got, err := reopened.Read(5 + uint64(i))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if string(got.Value) != value {
			t.Fatalf("value = %q, want %q", got.Value, value)
		}
	}
}

func TestSegmentRecoveryTruncatesUnindexedSuffix(t *testing.T) {
	dir := t.TempDir()

	cfg := config.Config{}
	cfg.Segment.MaxStoreBytes = 1024 * 1024
	cfg.Segment.MaxIndexBytes = 1024 * 1024

	s, err := segment.NewSegment(dir, 0, cfg)
	if err != nil {
		t.Fatalf("NewSegment: %v", err)
	}

	if _, err := s.Append(&api.Record{Value: []byte("committed")}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := s.Flush(false); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	committedSize := s.Store.Size

	// Simulate a Store append that did not get an index entry.
	if _, _, err := s.Store.Append([]byte("orphaned")); err != nil {
		t.Fatalf("append orphan: %v", err)
	}
	if err := s.Store.Flush(false); err != nil {
		t.Fatalf("flush orphan: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := segment.NewSegment(dir, 0, cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	if reopened.NextOffset != 1 {
		t.Fatalf("NextOffset = %d, want 1", reopened.NextOffset)
	}
	if reopened.Store.Size != committedSize {
		t.Fatalf("Store.Size = %d, want %d",
			reopened.Store.Size, committedSize)
	}

	got, err := reopened.Read(0)
	if err != nil {
		t.Fatalf("Read committed record: %v", err)
	}
	if string(got.Value) != "committed" {
		t.Fatalf("value = %q, want committed", got.Value)
	}

	// Confirm the expected segment files still exist.
	for _, name := range []string{"0.log", "0.index"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("stat %s: %v", name, err)
		}
	}
}

func TestSegmentCorruptPayloadFailsChecksum(t *testing.T) {
	s := newTestSegment(t, 0)

	_, err := s.Append(&api.Record{Value: []byte("original")})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	// The Store contains a length-prefixed protobuf record. Change the
	// encoded record bytes without updating the checksum in the protobuf.
	// Replacing the serialized bytes with invalid protobuf must fail either
	// decoding or checksum validation.
	_, pos, err := s.Index.Read(0)
	if err != nil {
		t.Fatalf("Index.Read: %v", err)
	}

	encoded, err := s.Store.Read(pos)
	if err != nil {
		t.Fatalf("Store.Read: %v", err)
	}

	if len(encoded) == 0 {
		t.Fatal("unexpected empty protobuf record")
	}

	encoded[len(encoded)-1] ^= 0xff

	// Overwrite only the encoded payload, preserving the frame header.
	if _, err := s.Store.File.WriteAt(encoded, int64(pos)+8); err != nil {
		t.Fatalf("corrupt stored record: %v", err)
	}

	_, err = s.Read(0)
	if err == nil {
		t.Fatal("Read corrupted record succeeded")
	}
}