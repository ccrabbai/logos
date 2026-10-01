package recovery_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/core"
	"github.com/ccrabbai/logos/internal/recovery"
	"github.com/ccrabbai/logos/internal/util"
)

type recoveryFixture struct {
	store *core.Store
	index *core.Index
}

func newRecoveryFixture(t *testing.T) recoveryFixture {
	t.Helper()

	cfg := config.Config{}
	cfg.Segment.MaxStoreBytes = 1024 * 1024
	cfg.Segment.MaxIndexBytes = 1024
	cfg.Store.BufferBytes = 0

	dir := t.TempDir()

	storePath := dir + "/store.log"
	indexPath := dir + "/index.index"

	storeFile, err := os.OpenFile(
		storePath,
		os.O_CREATE|os.O_RDWR,
		0600,
	)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	store, err := core.NewStore(storeFile, cfg)
	if err != nil {
		storeFile.Close()
		t.Fatalf("create store: %v", err)
	}

	indexFile, err := os.OpenFile(
		indexPath,
		os.O_CREATE|os.O_RDWR,
		0600,
	)
	if err != nil {
		store.Close()
		t.Fatalf("open index: %v", err)
	}

	index, err := core.NewIndex(indexFile, cfg)
	if err != nil {
		indexFile.Close()
		store.Close()
		t.Fatalf("create index: %v", err)
	}

	t.Cleanup(func() {
		_ = index.Close()
		_ = store.Close()
	})

	return recoveryFixture{
		store: store,
		index: index,
	}
}

func appendIndexed(
	t *testing.T,
	f recoveryFixture,
	offset uint32,
	payload []byte,
) uint64 {
	t.Helper()

	_, pos, err := f.store.Append(payload)
	if err != nil {
		t.Fatalf("store append: %v", err)
	}

	if err := f.index.Write(offset, pos); err != nil {
		t.Fatalf("index write: %v", err)
	}

	return pos
}

func TestAlignBoundariesCleanState(t *testing.T) {
	f := newRecoveryFixture(t)

	appendIndexed(t, f, 0, []byte("first"))
	appendIndexed(t, f, 1, []byte("second"))

	if err := f.store.Flush(false); err != nil {
		t.Fatalf("flush store: %v", err)
	}

	nextOffset, err := recovery.AlignBoundaries2(
		f.index,
		f.store,
		100,
	)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	if nextOffset != 102 {
		t.Fatalf("next offset = %d, want 102", nextOffset)
	}

	expectedSize :=
		uint64(util.LenWidth+len("first")) +
			uint64(util.LenWidth+len("second"))

	if f.store.Size != expectedSize {
		t.Fatalf(
			"store size = %d, want %d",
			f.store.Size,
			expectedSize,
		)
	}
}

func TestAlignBoundariesTruncatesUnindexedStoreData(t *testing.T) {
	f := newRecoveryFixture(t)

	appendIndexed(t, f, 0, []byte("committed"))

	// This record is deliberately not indexed.
	_, _, err := f.store.Append([]byte("uncommitted"))
	if err != nil {
		t.Fatalf("append unindexed record: %v", err)
	}

	if err := f.store.Flush(false); err != nil {
		t.Fatalf("flush: %v", err)
	}

	committedSize :=
		uint64(util.LenWidth + len("committed"))

	nextOffset, err := recovery.AlignBoundaries2(
		f.index,
		f.store,
		0,
	)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	if nextOffset != 1 {
		t.Fatalf("next offset = %d, want 1", nextOffset)
	}

	if f.store.Size != committedSize {
		t.Fatalf(
			"store size = %d, want %d",
			f.store.Size,
			committedSize,
		)
	}

	info, err := f.store.File.Stat()
	if err != nil {
		t.Fatalf("stat store: %v", err)
	}

	if uint64(info.Size()) != committedSize {
		t.Fatalf(
			"physical store size = %d, want %d",
			info.Size(),
			committedSize,
		)
	}

	got, err := f.store.Read(0)
	if err != nil {
		t.Fatalf("read committed record: %v", err)
	}

	if !bytes.Equal(got, []byte("committed")) {
		t.Fatalf("payload = %q, want committed", got)
	}
}

func TestAlignBoundariesRollsBackIndexPointingBeyondStore(t *testing.T) {
	f := newRecoveryFixture(t)

	appendIndexed(t, f, 0, []byte("valid"))

	if err := f.store.Flush(false); err != nil {
		t.Fatalf("flush store: %v", err)
	}

	// Create an index entry whose position does not exist in Store.
	if err := f.index.Write(1, 9999); err != nil {
		t.Fatalf("write invalid index entry: %v", err)
	}

	if err := f.index.Flush(); err != nil {
		t.Fatalf("flush index: %v", err)
	}

	nextOffset, err := recovery.AlignBoundaries2(
		f.index,
		f.store,
		10,
	)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	if nextOffset != 11 {
		t.Fatalf("next offset = %d, want 11", nextOffset)
	}

	off, pos, err := f.index.Read(-1)
	if err != nil {
		t.Fatalf("read recovered last index entry: %v", err)
	}

	if off != 0 || pos != 0 {
		t.Fatalf(
			"last index entry = (%d, %d), want (0, 0)",
			off,
			pos,
		)
	}
}

func TestAlignBoundariesRollsBackIncompleteFinalFrame(t *testing.T) {
	f := newRecoveryFixture(t)

	appendIndexed(t, f, 0, []byte("valid"))

	if err := f.store.Flush(false); err != nil {
		t.Fatalf("flush valid store record: %v", err)
	}

	 // Add an index entry pointing to the beginning of a truncated frame.
	invalidPos := f.store.Size

	frameHeader := make([]byte, util.LenWidth)
	util.Enc.PutUint64(frameHeader, 100)

	if _, err := f.store.File.WriteAt(
		frameHeader,
		int64(invalidPos),
	); err != nil {
		t.Fatalf("write incomplete frame: %v", err)
	}

	if err := f.index.Write(1, invalidPos); err != nil {
		t.Fatalf("write invalid index: %v", err)
	}

	if err := f.index.Flush(); err != nil {
		t.Fatalf("flush index: %v", err)
	}

	nextOffset, err := recovery.AlignBoundaries2(
		f.index,
		f.store,
		0,
	)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	if nextOffset != 1 {
		t.Fatalf("next offset = %d, want 1", nextOffset)
	}

	off, pos, err := f.index.Read(-1)
	if err != nil {
		t.Fatalf("read last index: %v", err)
	}

	if off != 0 || pos != 0 {
		t.Fatalf(
			"last index = (%d, %d), want (0, 0)",
			off,
			pos,
		)
	}
}

func TestAlignBoundariesRollsBackMultipleInvalidIndexEntries(t *testing.T) {
	f := newRecoveryFixture(t)

	appendIndexed(t, f, 0, []byte("valid"))

	if err := f.store.Flush(false); err != nil {
		t.Fatalf("flush store: %v", err)
	}

	if err := f.index.Write(1, 999); err != nil {
		t.Fatalf("write invalid entry 1: %v", err)
	}

	if err := f.index.Write(2, 1999); err != nil {
		t.Fatalf("write invalid entry 2: %v", err)
	}

	if err := f.index.Flush(); err != nil {
		t.Fatalf("flush index: %v", err)
	}

	nextOffset, err := recovery.AlignBoundaries2(
		f.index,
		f.store,
		0,
	)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	if nextOffset != 1 {
		t.Fatalf("next offset = %d, want 1", nextOffset)
	}

	off, pos, err := f.index.Read(-1)
	if err != nil {
		t.Fatalf("read last index: %v", err)
	}

	if off != 0 || pos != 0 {
		t.Fatalf(
			"last index = (%d, %d), want (0, 0)",
			off,
			pos,
		)
	}
}

func TestAlignBoundariesEmptyIndexTruncatesStore(t *testing.T) {
	f := newRecoveryFixture(t)

	if _, _, err := f.store.Append([]byte("orphaned")); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := f.store.Flush(false); err != nil {
		t.Fatalf("flush: %v", err)
	}

	nextOffset, err := recovery.AlignBoundaries2(
		f.index,
		f.store,
		50,
	)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	if nextOffset != 50 {
		t.Fatalf("next offset = %d, want 50", nextOffset)
	}

	if f.store.Size != 0 {
		t.Fatalf("store size = %d, want 0", f.store.Size)
	}

	if f.index.Size != 0 {
		t.Fatalf("index size = %d, want 0", f.index.Size)
	}

	info, err := f.store.File.Stat()
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if info.Size() != 0 {
		t.Fatalf(
			"physical store size = %d, want 0",
			info.Size(),
		)
	}
}

func TestAlignBoundariesRecoveryAllowsSubsequentAppend(t *testing.T) {
	f := newRecoveryFixture(t)

	appendIndexed(t, f, 0, []byte("first"))

	// Simulate an unindexed trailing write.
	if _, _, err := f.store.Append([]byte("orphan")); err != nil {
		t.Fatalf("append orphan: %v", err)
	}

	if err := f.store.Flush(false); err != nil {
		t.Fatalf("flush: %v", err)
	}

	nextOffset, err := recovery.AlignBoundaries2(
		f.index,
		f.store,
		0,
	)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	if nextOffset != 1 {
		t.Fatalf("next offset = %d, want 1", nextOffset)
	}

	// Recovery must have restored Store.Size to the committed boundary.
	_, pos, err := f.store.Append([]byte("second"))
	if err != nil {
		t.Fatalf("append after recovery: %v", err)
	}

	expectedPos := uint64(util.LenWidth + len("first"))

	if pos != expectedPos {
		t.Fatalf(
			"new record position = %d, want %d",
			pos,
			expectedPos,
		)
	}

	got, err := f.store.Read(pos)
	if err != nil {
		t.Fatalf("read appended record: %v", err)
	}

	if !bytes.Equal(got, []byte("second")) {
		t.Fatalf("payload = %q, want second", got)
	}
}