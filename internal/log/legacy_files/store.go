package logv

import (
	"io"
	"os"
	"sync"
	"bufio"
	"encoding/binary"
)

var (
	// enc specifies our standard byte translation strategy. Big-Endian 
	// writes the most significant byte first, guaranteeing that numbers
	// are saved identically across differing CPU hardware types.
	enc = binary.BigEndian
)

const (
	// lenWidth represents the size of our record length prefix header.
	// A 64-bit integer (uint64) requires exactly 8 bytes of storage space.
	lenWidth = 8
)

// store represents our low-level append-only data persistence engine wrapper.
type store struct {
	*os.File              // Embedded standard library file descriptor to inherit standard operations.
	mu       sync.Mutex   // Protects our file handles and tracking variables from concurrent read/write data races.
	buf      *bufio.Writer // In-memory cache layer to batch filesystem write system calls for extreme performance.
	size     uint64       // In-memory ledger tracking total file size to minimize heavy os.Stat filesystem calls.
}

// newStore initializes a brand new storage instance wrapping around an active filesystem object.
func newStore(f *os.File) (*store, error) {
	// Query the OS metadata file attributes to determine if this file contains historical logs.
	fi, err := os.Stat(f.Name())
	if err != nil {
		return nil, err
	}
	
	// If the file already has data due to a past execution run or crash recovery,
	// our size tracking pointer must pick up exactly where the file leaves off.
	size := uint64(fi.Size())
	
	return &store{
		File: f,
		size: size,
		buf:  bufio.NewWriter(f), // Directing the caching layer to stage writes targeting our file descriptor.
	}, nil
}

func NewStore(f *os.File) (*store, error) {
	return newStore(f)
}

// Append inserts raw record payloads into our storage layer using Length-Prefixed Framing.
// It returns total bytes consumed by the entry frame, and the absolute starting point position on disk.
func (s *store) Append(record []byte) (n uint64, pos uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The record starts exactly where the file's current size boundary sits.
	pos = s.size

	// Step 1: Serialize and merge the 8-byte uint64 record length and the record into a frame.
	frame := make([]byte, lenWidth+len(record))
	enc.PutUint64(frame[:lenWidth], uint64(len(record)))
	copy(frame[lenWidth:], record)

	// Step 2: Append the raw frame in our RAM staging buffer.
	// w now becomes total written bytes (record + 8 metadata header bytes)
	w, err := s.buf.Write(frame)
	if err != nil {
		return 0, 0, err
	}

	if w != len(frame) {
		return 0, 0, io.ErrShortWrite
	}
	
	// Advance the tracking ledger forward so subsequent appends stack directly behind this point.
	s.size += uint64(w)
	
	return uint64(w), pos, nil
}

// Read retrieves a specific record out of our storage layer from an absolute file index address point.
func (s *store) Read(pos uint64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Critical Guard: Empty the memory staging buffer to disk. If the record we want to read
	// is still sitting inside RAM, a direct filesystem read call would return empty data bytes.
	if err := s.buf.Flush(); err != nil {
		return nil, err
	}

	// Step 1: Allocate a temporary 8-byte pad to read the upcoming frame size descriptor.
	size := make([]byte, lenWidth)
	if _, err := s.File.ReadAt(size, int64(pos)); err != nil {
		return nil, err
	}

	// Step 2: Convert those 8 raw bytes back into a usable integer, and allocate a slice matching it.
	b := make([]byte, enc.Uint64(size))
	
	// Step 3: Execute a direct read, offsetting past the 8-byte length header to grab the raw data frame.
	if _, err := s.File.ReadAt(b, int64(pos+lenWidth)); err != nil {
		return nil, err
	}
	
	return b, nil
}

// A low-level, public-facing interface implementation.
// ReadAt satisfies Go's native io.ReaderAt interface. It blindly copies whatever bytes sit at that exact disk position
//  into the provided slice p.
func (s *store) ReadAt(p []byte, off int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Flush memory cache buffer to ensure we aren't reading stale data on disk.
	if err := s.buf.Flush(); err != nil {
		return 0, err
	}
	
	// Read directly from the physical disk file layer.
	return s.File.ReadAt(p, off)
}

// Close gracefully flushes trailing buffer frames and releases operating system resources safely.
func (s *store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Ensure no data remnants are abandoned inside RAM buffers before cutting ties.
	err := s.buf.Flush()
	if err != nil {
		return err
	}
	
	// Disconnect and release the underlying file descriptor back to the OS Kernel.
	return s.File.Close()
}
