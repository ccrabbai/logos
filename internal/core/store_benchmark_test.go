package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccrabbai/logos/internal/config"
)

const (
	benchmarkRecordBytes = 1024
	benchmarkBufferBytes = 0
)

// BenchmarkStoreAppend measures Store.Append throughput for a fixed-size
// record workload. Use -benchmem to capture allocation metrics.
func BenchmarkStoreAppend(b *testing.B) {
	cfg := config.Config{}

	bufferSizes := []uint64{
		0,       // Existing bufio.Writer default
		4096,
		16384,
		65536,
		262144,
	}

	record := make([]byte, benchmarkRecordBytes)
	for i := range record {
		record[i] = byte(i % 251)
	}

	for _, bufferBytes := range bufferSizes {
		cfg.Store.BufferBytes = bufferBytes
		name := fmt.Sprintf("buffer_%d", bufferBytes)

		b.Run(name, func(b *testing.B) {
			dir := b.TempDir()
			path := filepath.Join(dir, "store.log")

			f, err := os.OpenFile(
				path,
				os.O_CREATE|os.O_RDWR|os.O_TRUNC,
				0600,
			)
			if err != nil {
				b.Fatal(err)
			}

			store, err := NewStore(f, cfg)
			if err != nil {
				_ = f.Close()
				b.Fatal(err)
			}

			b.SetBytes(int64(benchmarkRecordBytes))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if _, _, err := store.Append(record); err != nil {
					b.Fatal(err)
				}
			}

			b.StopTimer()

			// Flush buffered data so the run includes successful completion
			// of the buffered write path. This does not imply fsync durability.
			if err := store.Flush(false); err != nil {
				b.Fatal(err)
			}

			expectedBytes := uint64(b.N * (benchmarkRecordBytes + 8))
			if store.Size != expectedBytes {
				b.Fatalf(
					"unexpected Store size: got %d, want %d",
					store.Size,
					expectedBytes,
				)
			}

			if err := store.Close(); err != nil {
				b.Fatal(err)
			}

			info, err := os.Stat(path)
			if err != nil {
				b.Fatal(err)
			}
			if uint64(info.Size()) != expectedBytes {
				b.Fatalf(
					"unexpected file size: got %d, want %d",
					info.Size(),
					expectedBytes,
				)
			}
		})
	}
}