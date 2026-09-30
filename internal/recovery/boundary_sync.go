package recovery

import (
	"io"

	"github.com/ccrabbai/logos/internal/util"
	"github.com/ccrabbai/logos/internal/core"
)

// AlignBoundaries scans historical disk frames to calculate a synchronized state boundary.
// It mitigates asymmetric storage failures without executing historical file truncations.
func AlignBoundaries(idx *core.Index, store *core.Store, baseOffset uint64) (uint64, error) {
	for {
		off, pos, err := idx.Read(-1)
		if err != nil {
			if err == io.EOF {
				// If the index file is empty or all corrupted frames were successfully unrolled,
				// the valid active log sequence boundaries default back to 0.
				store.Size = 0
				return baseOffset, nil
			}
			return 0, err
		}

		storeInfo, err := store.File.Stat()
		if err != nil {
			return 0, err
		}
		actualStoreSize := uint64(storeInfo.Size())

		// GUARD 1: Mitigate Store Crashes (Phantom Index Entry Detection)
		if actualStoreSize < (pos + util.LenWidth) {
			newIndexSize := idx.Size - util.EntWidth
			if err := idx.Truncate(newIndexSize); err != nil {
				return 0, err
			}
			continue // Shift backward and re-evaluate previous frame slot
		}

		// Safely extract length prefix since file boundaries are verified
		sizeBuffer := make([]byte, util.LenWidth)
		if _, readErr := store.File.ReadAt(sizeBuffer, int64(pos)); readErr != nil {
			return 0, readErr
		}
		payloadLength := util.Enc.Uint64(sizeBuffer)
		logicalStoreSize := pos + util.LenWidth + payloadLength

		// GUARD 2: Validate Incomplete Frame Content
		if actualStoreSize < logicalStoreSize {
			newIndexSize := idx.Size - util.EntWidth
			if err := idx.Truncate(newIndexSize); err != nil {
				return 0, err
			}
			continue
		}

		// 💡 APPEND-ONLY GARBAGE TOLERANCE FIX:
		// We override the in-memory ledger size. If actualStoreSize > logicalStoreSize,
		// any orphaned unindexed blocks will be naturally overwritten on subsequent writes.
		store.Size = logicalStoreSize

		// Alignment successful! Return the next available sequential sequence slot
		return baseOffset + uint64(off) + 1, nil
	}
}


// AlignBoundaries scans historical disk frames to calculate a synchronized state boundary.
// It truncates corrupted index frames and truncates trailing unindexed garbage from the store.
func AlignBoundaries2(idx *core.Index, store *core.Store, baseOffset uint64) (uint64, error) {

	// CRITICAL: Protect the store structures from concurrent mutations during recovery
	store.Mu.Lock()
	defer store.Mu.Unlock()
	
	// Step 1: Drain and clear stale memory structures before restructuring disk assets.
	// Pass the embedded file descriptor (store.File) which satisfies io.Writer.
	if store.Buf != nil {
		store.Buf.Reset(store.File)
	}

	for {
		off, pos, err := idx.Read(-1)
		if err != nil {
			if err == io.EOF {
				// EDGE CASE: Index is empty or completely rolled back.
				// Wiping the store back to 0 is the only safe option.
				return handleTotalTruncation(idx, store, baseOffset)
			}
			return 0, err
		}

		// Explicitly invoke Stat on the underlying *os.File descriptor field
		storeInfo, err := store.File.Stat()
		if err != nil {
			return 0, err
		}
		actualStoreSize := uint64(storeInfo.Size())

		// GUARD 1: Mitigate Store Crashes (Phantom Index Entry Detection)
		if actualStoreSize < (pos + util.LenWidth) {
			if err := rollbackIndex(idx); err != nil {
				return 0, err
			}
			continue // Step backward and evaluate previous index frame
		}

		// Safely extract length prefix since file boundaries are verified
		sizeBuffer := make([]byte, util.LenWidth)
		if _, readErr := store.File.ReadAt(sizeBuffer, int64(pos)); readErr != nil {
			return 0, readErr
		}
		payloadLength := util.Enc.Uint64(sizeBuffer)
		logicalStoreSize := pos + util.LenWidth + payloadLength

		// GUARD 2: Validate Incomplete Frame Content (Torn Write Detection)
		if actualStoreSize < logicalStoreSize {
			if err := rollbackIndex(idx); err != nil {
				return 0, err
			}
			continue
		}

		// PRODUCTION RESOLUTION: The boundaries are valid. 
		// If trailing unindexed garbage exists, physically clean the log file.
		if actualStoreSize > logicalStoreSize {
			if err := store.File.Truncate(int64(logicalStoreSize)); err != nil {
				return 0, err
			}
		}

		// Re-align internal tracking state and OS write pointers
		store.Size = logicalStoreSize
		if _, err := store.File.Seek(int64(logicalStoreSize), io.SeekStart); err != nil {
			return 0, err
		}

		// Re-initialize the buffered writer to point back to the clean file state
		if store.Buf != nil {
			store.Buf.Reset(store.File)
		}

		// Alignment successful! Return the next sequence offset slot
		return baseOffset + uint64(off) + 1, nil
	}
}

// rollbackIndex removes the last entry from the index file
func rollbackIndex(idx *core.Index) error {
	newIndexSize := idx.Size - util.EntWidth
	return idx.Truncate(newIndexSize)
}

// handleTotalTruncation resets both the index and store back to byte 0
func handleTotalTruncation(idx *core.Index, store *core.Store, baseOffset uint64) (uint64, error) {
	if err := idx.Truncate(0); err != nil {
		return 0, err
	}
	if err := store.File.Truncate(0); err != nil {
		return 0, err
	}
	
	store.Size = 0
	if _, err := store.File.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	if store.Buf != nil {
		store.Buf.Reset(store.File)
	}

	return baseOffset, nil
}
