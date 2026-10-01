package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccrabbai/logos/internal/config"
)

func TestStoreBufferConfigurationsPreserveRecords(t *testing.T) {
	bufferSizes := []uint64{
		0,
		4096,
		16384,
		65536,
		262144,
	}

	records := [][]byte{
		[]byte("logos-experiment-record-0001"),
		[]byte("logos-experiment-record-0002"),
		[]byte("logos-experiment-record-0003"),
		[]byte("logos-experiment-record-0004"),
		[]byte("logos-experiment-record-0005"),
	}

	for _, bufferBytes := range bufferSizes {
		t.Run(
			"buffer_"+formatBufferSize(bufferBytes),
			func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "store.log")

				f, err := os.OpenFile(
					path,
					os.O_CREATE|os.O_RDWR|os.O_TRUNC,
					0600,
				)
				if err != nil {
					t.Fatal(err)
				}

				cfg := config.Config{}
				cfg.Store.BufferBytes = bufferBytes

				store, err := NewStore(f, cfg)
				if err != nil {
					_ = f.Close()
					t.Fatal(err)
				}

				positions := make([]uint64, 0, len(records))

				for _, record := range records {
					_, pos, err := store.Append(record)
					if err != nil {
						_ = store.Close()
						t.Fatal(err)
					}

					positions = append(positions, pos)
				}

				for i, pos := range positions {
					got, err := store.Read(pos)
					if err != nil {
						_ = store.Close()
						t.Fatalf(
							"Read failed for record %d: %v",
							i,
							err,
						)
					}

					if !bytes.Equal(got, records[i]) {
						_ = store.Close()
						t.Fatalf(
							"record %d mismatch: got %q, want %q",
							i,
							got,
							records[i],
						)
					}
				}

				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
			},
		)
	}
}

func formatBufferSize(size uint64) string {
	return fmt.Sprintf("%d", size)
}
