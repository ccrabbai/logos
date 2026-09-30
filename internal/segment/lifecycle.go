package segment

import (
	"os"
)

// Close flushes tracking grids and gracefully shuts down raw file descriptors.
func (s *Segment) Close() error {
	if err := s.Index.Close(); err != nil {
		return err
	}
	return s.Store.Close()
}

// Remove cleans up file paths and physically deletes data layers from the host disk.
func (s *Segment) Remove() error {
	if err := s.Close(); err != nil {
		return err
	}
	if err := os.Remove(s.Store.File.Name()); err != nil {
		return err
	}
	return os.Remove(s.Index.File.Name())
}

// Freeze forces open cache buffers onto memory mapping storage grids before resetting buffers.
func (s *Segment) Flush(setBufferToNil bool) error {
	if err := s.Index.Flush(); err != nil {
		return err
	}
	return s.Store.Flush(setBufferToNil)
}
