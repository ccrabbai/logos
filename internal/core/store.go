package core

import (
	"bufio"
	"fmt"
	"os"
	"sync"

	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/util"
)

// Store represents our low-level append-only data persistence engine wrapper.
type Store struct {
	File *os.File      // Exposed file handle so recovery loops can run Stat checks.
	Mu   sync.Mutex    // Protects file references from concurrent read/write data races.
	Buf  *bufio.Writer // In-memory cache layer to batch filesystem write system calls.
	Size uint64        // Publicly readable tracker counting valid physical bytes.
}

// NewStore initializes a brand new storage instance wrapping an active filesystem object.
func NewStore(f *os.File, c config.Config) (*Store, error) {
	fi, err := os.Stat(f.Name())
	if err != nil {
		return nil, err
	}

	var writer *bufio.Writer
	bufferBytes := c.Store.BufferBytes

	if bufferBytes == 0 {
		writer = bufio.NewWriter(f)
	} else {
		maxInt := uint64(^uint(0) >> 1)
		if bufferBytes > maxInt {
			return nil, fmt.Errorf(
				"store buffer size %d exceeds maximum supported size",
				bufferBytes,
			)
		}

		writer = bufio.NewWriterSize(f, int(bufferBytes))
	}

	return &Store{
		File: f,
		Size: uint64(fi.Size()),
		Buf:  writer,
	}, nil
}

// Append inserts raw record payloads into our storage layer using Length-Prefixed Framing.
func (s *Store) Append(record []byte) (n uint64, pos uint64, err error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	pos = s.Size

	// Serialize and merge the 8-byte uint64 record length and the record into a frame.
	frame := make([]byte, util.LenWidth+len(record))
	util.Enc.PutUint64(frame[:util.LenWidth], uint64(len(record)))
	copy(frame[util.LenWidth:], record)

	// w = (record + 8 metadata header bytes)
	w, err := s.Buf.Write(frame)
	if err != nil {
		return 0, 0, err
	}

	s.Size += uint64(w)

	return uint64(w), pos, nil
}

// Read retrieves a specific record payload from an absolute disk address point.
func (s *Store) Read(pos uint64) ([]byte, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if s.Buf != nil {
		if err := s.Buf.Flush(); err != nil {
			return nil, err
		}
	}

	size := make([]byte, util.LenWidth)
	if _, err := s.File.ReadAt(size, int64(pos)); err != nil {
		return nil, err
	}

	b := make([]byte, util.Enc.Uint64(size))
	if _, err := s.File.ReadAt(b, int64(pos+util.LenWidth)); err != nil {
		return nil, err
	}

	return b, nil
}

// ReadAt extracts raw bytes directly from the physical disk layer for io.ReaderAt interfaces.
func (s *Store) ReadAt(p []byte, off int64) (int, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if s.Buf != nil {
		if err := s.Buf.Flush(); err != nil {
			return 0, err
		}
	}

	return s.File.ReadAt(p, off)
}

// Close gracefully flushes data and shuts down underlying file descriptors.
func (s *Store) Close() error {
	if err := s.Flush(true); err != nil {
		return err
	}
	return s.File.Close()
}

// Flush empties buffers to disk and conditionally de-allocates RAM allocations.
func (s *Store) Flush(setBufferToNil bool) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if s.Buf != nil {
		if err := s.Buf.Flush(); err != nil {
			return err
		}
		if setBufferToNil {
			s.Buf = nil
		}
	}
	return nil
}
