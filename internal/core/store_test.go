package core_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"

	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/core"
	"github.com/ccrabbai/logos/internal/util"
)

func newTestStore(t *testing.T, buffer uint64) (*core.Store, string) {
	t.Helper()

	cfg := config.Config{}
	cfg.Store.BufferBytes = buffer

	dir := t.TempDir()
	path := dir + "/store.log"

	f, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_RDWR,
		0600,
	)
	if err != nil {
		t.Fatalf("open store file: %v", err)
	}

	store, err := core.NewStore(f, cfg)
	if err != nil {
		f.Close()
		t.Fatalf("create store: %v", err)
	}

	t.Cleanup(func() {
		_ = store.Close()
	})

	return store, path
}


var errWriteFailed = errors.New("write failed")

type failWriter struct{}

func (failWriter) Write(p []byte) (int, error) {
	return 0, errWriteFailed
}

func TestStoreAppendWriteFailure(t *testing.T) {
	s := &core.Store{
		Buf:  bufio.NewWriterSize(failWriter{}, 16),
		Size: 0,
	}

	// 8-byte length header + 32-byte record = 40 bytes,
	// which is larger than the 16-byte buffer and forces
	// bufio.Writer to call the underlying writer.
	record := make([]byte, 32)

	n, pos, err := s.Append(record)

	if !errors.Is(err, errWriteFailed) {
		t.Fatalf("expected %v, got %v", errWriteFailed, err)
	}

	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}

	if pos != 0 {
		t.Errorf("expected pos=0, got %d", pos)
	}

	if s.Size != 0 {
		t.Errorf("expected Size=0, got %d", s.Size)
	}
}

func TestStoreAppendAndRead(t *testing.T) {
	store, _ := newTestStore(t,0)

	payload := []byte("hello logos")

	n, pos, err := store.Append(payload)
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	wantSize := uint64(util.LenWidth + len(payload))

	if n != wantSize {
		t.Fatalf("written bytes = %d, want %d", n, wantSize)
	}

	if pos != 0 {
		t.Fatalf("first position = %d, want 0", pos)
	}

	got, err := store.Read(pos)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("read payload = %q, want %q", got, payload)
	}

	if store.Size != wantSize {
		t.Fatalf("store size = %d, want %d", store.Size, wantSize)
	}
}

func TestStoreAppendReturnsSequentialPositions(t *testing.T) {
	store, _ := newTestStore(t,0)

	payloads := [][]byte{
		[]byte("first"),
		[]byte("second"),
		[]byte("third"),
	}

	var positions []uint64

	for _, payload := range payloads {
		_, pos, err := store.Append(payload)
		if err != nil {
			t.Fatalf("append: %v", err)
		}

		positions = append(positions, pos)
	}

	want := []uint64{
		0,
		uint64(util.LenWidth + len(payloads[0])),
		uint64(
			2*util.LenWidth +
				len(payloads[0]) +
				len(payloads[1]),
		),
	}

	for i := range want {
		if positions[i] != want[i] {
			t.Fatalf(
				"position[%d] = %d, want %d",
				i,
				positions[i],
				want[i],
			)
		}
	}
}

func TestStoreReadMultipleRecords(t *testing.T) {
	store, _ := newTestStore(t,0)

	payloads := [][]byte{
		[]byte("one"),
		[]byte("two"),
		[]byte("three"),
		[]byte("four"),
	}

	positions := make([]uint64, 0, len(payloads))

	for _, payload := range payloads {
		_, pos, err := store.Append(payload)
		if err != nil {
			t.Fatalf("append: %v", err)
		}

		positions = append(positions, pos)
	}

	for i, pos := range positions {
		got, err := store.Read(pos)
		if err != nil {
			t.Fatalf("read record %d: %v", i, err)
		}

		if !bytes.Equal(got, payloads[i]) {
			t.Fatalf(
				"record %d = %q, want %q",
				i,
				got,
				payloads[i],
			)
		}
	}
}

func TestStoreHandlesEmptyAndBinaryPayloads(t *testing.T) {
	store, _ := newTestStore(t,0)

	payloads := [][]byte{
		{},
		{0x00, 0x01, 0x02, 0xff, 0xfe},
	}

	for _, payload := range payloads {
		_, pos, err := store.Append(payload)
		if err != nil {
			t.Fatalf("append: %v", err)
		}

		got, err := store.Read(pos)
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		if !bytes.Equal(got, payload) {
			t.Fatalf("payload = %v, want %v", got, payload)
		}
	}
}

func TestStoreReadFlushesBufferedWrites(t *testing.T) {
	store, _ := newTestStore(t,0)

	payload := []byte("buffered write")

	_, pos, err := store.Append(payload)
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// Append writes through bufio.Writer. Read must still be able
	// to observe the record.
	got, err := store.Read(pos)
	if err != nil {
		t.Fatalf("read after append: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	store, path := newTestStore(t,0)

	cfg := config.Config{}
	cfg.Store.BufferBytes = 0

	payloads := [][]byte{
		[]byte("persistent-one"),
		[]byte("persistent-two"),
	}

	positions := make([]uint64, 0, len(payloads))

	for _, payload := range payloads {
		_, pos, err := store.Append(payload)
		if err != nil {
			t.Fatalf("append: %v", err)
		}

		positions = append(positions, pos)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("reopen file: %v", err)
	}

	reopened, err := core.NewStore(f, cfg)
	if err != nil {
		f.Close()
		t.Fatalf("reopen store: %v", err)
	}

	t.Cleanup(func() {
		_ = reopened.Close()
	})

	for i, pos := range positions {
		got, err := reopened.Read(pos)
		if err != nil {
			t.Fatalf("read record %d after reopen: %v", i, err)
		}

		if !bytes.Equal(got, payloads[i]) {
			t.Fatalf(
				"record %d = %q, want %q",
				i,
				got,
				payloads[i],
			)
		}
	}

	expectedSize := uint64(0)
	for _, payload := range payloads {
		expectedSize += uint64(util.LenWidth + len(payload))
	}

	if reopened.Size != expectedSize {
		t.Fatalf(
			"reopened size = %d, want %d",
			reopened.Size,
			expectedSize,
		)
	}
}

func TestStoreReadInvalidPosition(t *testing.T) {
	store, _ := newTestStore(t,0)

	_, _, err := store.Append([]byte("valid"))
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	_, err = store.Read(store.Size + 1)
	if err == nil {
		t.Fatal("read beyond store size succeeded")
	}
}

func TestStoreReadIncompleteFrame(t *testing.T) {
	store, _ := newTestStore(t,0)

	// Write an invalid/truncated frame directly to the file.
	// The length prefix claims a payload larger than what exists.
	frame := make([]byte, util.LenWidth)
	util.Enc.PutUint64(frame, 100)

	if _, err := store.File.WriteAt(frame, 0); err != nil {
		t.Fatalf("write incomplete frame: %v", err)
	}

	store.Size = uint64(len(frame))

	_, err := store.Read(0)
	if err == nil {
		t.Fatal("read of incomplete frame succeeded")
	}
}

func TestStoreConcurrentAppend(t *testing.T) {
	store, _ := newTestStore(t,0)

	const writers = 16
	const recordsPerWriter = 50

	type result struct {
		pos     uint64
		payload []byte
	}

	results := make(chan result, writers*recordsPerWriter)

	var wg sync.WaitGroup

	for writer := 0; writer < writers; writer++ {
		writer := writer

		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := 0; i < recordsPerWriter; i++ {
				payload := []byte(fmt.Sprintf(
					"writer-%d-record-%d",
					writer,
					i,
				))

				_, pos, err := store.Append(payload)
				if err != nil {
					t.Errorf("append: %v", err)
					return
				}

				results <- result{
					pos:     pos,
					payload: payload,
				}
			}
		}()
	}

	wg.Wait()
	close(results)

	var all []result
	for result := range results {
		all = append(all, result)
	}

	if len(all) != writers*recordsPerWriter {
		t.Fatalf(
			"results = %d, want %d",
			len(all),
			writers*recordsPerWriter,
		)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].pos < all[j].pos
	})

	var expectedPos uint64

	for i, result := range all {
		if result.pos != expectedPos {
			t.Fatalf(
				"record %d position = %d, want %d",
				i,
				result.pos,
				expectedPos,
			)
		}

		got, err := store.Read(result.pos)
		if err != nil {
			t.Fatalf("read record %d: %v", i, err)
		}

		if !bytes.Equal(got, result.payload) {
			t.Fatalf(
				"record %d payload = %v, want %v",
				i,
				got,
				result.payload,
			)
		}

		expectedPos += uint64(util.LenWidth + len(result.payload))
	}

	if store.Size != expectedPos {
		t.Fatalf(
			"store size = %d, want %d",
			store.Size,
			expectedPos,
		)
	}
}