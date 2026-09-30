package logv

import (
	"io"
	"os"
	"fmt"
	"sync"
	"slices"
	"strings"
	"strconv"
	"log/slog" 

	"github.com/ccrabbai/logos/internal/config"
	api "github.com/ccrabbai/logos/api/logs/v1"
)

// Log acts as the parent controller and coordinator for a collection of log segments.
// It exposes public APIs for safe concurrent multi-reader, single-writer storage access.
type Log struct {
	mu sync.RWMutex // Protects our segment slice tracking indices from data race corruptions.

	Dir    string // The physical operating system directory path containing our database files.
	Config config.Config // System-wide storage boundaries passed down to child files during initialization.

	segments      []*segment // Sorted slice containing our read-only historical and active segments.
	activeSegment *segment   // Direct shortcut pointer pointing to the current write-active segment.
}

// NewLog instantiates a parent log manager, bootstrapping its memory state 
// by discovering and sorting historical segment files existing on disk.
func NewLog(dir string, c config.Config) (*Log, error) {
	// Step 1: Inject sensible fallback defaults if boundaries are left unconfigured.
	if c.Segment.MaxStoreBytes == 0 {
		c.Segment.MaxStoreBytes = 1024
	}
	if c.Segment.MaxIndexBytes == 0 {
		c.Segment.MaxIndexBytes = 1024
	}

	l := &Log{
		Dir:    dir,
		Config: c,
	}

	// Step 2: Query the OS to read all file descriptors present inside the target directory.
	files, err := os.ReadDir(dir)
	if err != nil {
		slog.Error("Failed to read storage directory during bootstrap", "dir", dir, "error", err)
		return nil, err
	}

	var baseOffsets []uint64
	for _, file := range files {
		// Filter out sub-directories and target only index mapping files.
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".index") {
			continue
		}
		
		// Strip the ".index" text extension string from the filename to isolate the number block.
		baseStr := strings.TrimSuffix(file.Name(), ".index")
		
		// Parse the file string digits into a clean, operational 64-bit integer.
		baseOffset, err := strconv.ParseUint(baseStr, 10, 64)
		if err != nil {
			slog.Error("Corrupted index file name encountered", "filename", file.Name(), "error", err)
			return nil, err
		}

		// 🛡️ THE MISSING SYSTEM FIX: Force the base offset to align with our segment bounds!
		// For example, if an offline crash left an unaligned index, this rounds it back 
		// safely to the proper segment boundary (e.g., 2005 becomes 2000).
		// We use c.Segment.MaxStoreBytes (or a dedicated message count configuration if present) 
		// to enforce uniform file-splitting increments.
		// alignedOffset := nearestMultiple(baseOffset, c.Segment.MaxStoreBytes) //this introduced a bug
		
		// baseOffsets = append(baseOffsets, alignedOffset)
		baseOffsets = append(baseOffsets, baseOffset)
	}

	// Step 3: Sort our offsets chronologically ascending to guarantee order consistency.
	slices.Sort(baseOffsets)

	// Step 4: Rebuild the active structures into our segment manager tracking array.
	for _, baseOffset := range baseOffsets {
		if err = l.newSegment(baseOffset); err != nil {
			return nil, err
		}
	}

	// Step 5: Bootstrap Edge Case Guard. If the directory had no pre-existing files,
	// generate the absolute first write segment tracking block to start the server.
	if l.segments == nil {
		if err = l.newSegment(c.Segment.InitialBaseOffset); err != nil {
			return nil, err
		}
	}

	// 📈 VISIBILITY HOOK 1: System Boot Diagnostics
	slog.Info("Storage engine initialized successfully",
		"directory", dir,
		"segments_scanned", len(baseOffsets),
		"active_segment_base", l.activeSegment.baseOffset,
		"next_write_offset", l.activeSegment.nextOffset,
	)

	return l, nil
}

// newSegment is an internal helper that wraps our low-level factory call, automatically
// appending the newly generated chunk structure onto our centralized tracking slice array.
func (l *Log) newSegment(baseOffset uint64) error {
	s, err := newSegment(l.Dir, baseOffset, l.Config)
	if err != nil {
		return err
	}
	
	l.segments = append(l.segments, s)
	l.activeSegment = s
	return nil
}

// Append pushes a new record frame onto the log cluster. If the active segment 
// reaches its size ceiling, it triggers an automatic multi-file segment rollover.
func (l *Log) Append(record *api.Record) (uint64, error) {
	// Step 1: Secure an exclusive write lock. This blocks all other parallel 
	// readers and appenders from altering or corrupting our storage arrays.
	l.mu.Lock()
	defer l.mu.Unlock()

	// Step 2: Capacity check. Assess if our current write target has filled up.
	if l.activeSegment.IsFull() {
		// Calculate the exact global offset where the new file pair must anchor.
		nextOffset := l.activeSegment.nextOffset

		// 📈 VISIBILITY HOOK 2: Automated Segment Rollover Tracker
		slog.Warn("Active segment capacity breached, triggering multi-file rollover",
			"old_segment_base", l.activeSegment.baseOffset,
			"new_segment_base", nextOffset,
			"store_bytes_used", l.activeSegment.store.size, )
		
		// Spawn a fresh, blank segment pair on disk and append it to our tracking slice.
		if err := l.newSegment(nextOffset); err != nil {
			return 0, err
		}
	}

	// Step 3: Delegate the actual serialization and index stamping down to the active segment.
	off, err := l.activeSegment.Append(record)
	if err != nil {
		slog.Error("Failed to write record to active segment", "target_offset", l.activeSegment.nextOffset, "error", err)
		return 0, err
	}

	// 📈 VISIBILITY HOOK 3: Write Throughput Tracker
	slog.Debug("Record committed successfully", 
		"global_offset", off, 
		"bytes_written", l.activeSegment.store.size - off, // Rough calculation of payload footprint
	)

	return off, nil
}

// retrieves a structured record from historical disk storage by executing an 
// optimized O(log N) binary search across all registered log segment ranges.
func (l *Log) Read(off uint64) (*api.Record, error) {
	// Step 1: Secure a shared reader lock. This permits unlimited concurrent readers 
	// to stream data from disk while preventing appends or rollovers from interrupting.
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Step 2: Initialize Binary Search boundary pointers.
	low := 0
	high := len(l.segments) - 1
	var targetSegment *segment

	// Step 3: Run the O(log N) split comparison algorithm.
	for low <= high {
		mid := (low + high) / 2
		
		// If our target offset matches or exceeds the middle segment's base anchor,
		// this segment is our current best candidate.
		if l.segments[mid].baseOffset <= off {
			targetSegment = l.segments[mid]
			// Check if there is an even newer segment to the right that fits the query.
			low = mid + 1
		} else {
			// The target offset is smaller than the middle segment's base anchor,
			// meaning the correct file chunk must reside to the left.
			high = mid - 1
		}
	}

	// Step 4: Safety Guard check. If no segment matched, or the request exceeds 
	// what our active storage has written, deny the request cleanly.
	if targetSegment == nil || off < targetSegment.baseOffset || targetSegment.nextOffset <= off {
		// return nil, api.ErrOffsetOutOfRange{Offset: off}
		return nil, fmt.Errorf("offset %d out of range", off)
	}

	// Step 5: Delegate coordinate extraction and unmarshaling down to the chosen segment.
	return targetSegment.Read(off)
}

// Close gracefully iterates through all memory-mapped indices and append-only 
// store files, forcing disk synchronization before releasing their file descriptors.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Cascade closures down across every managed segment block.
	slog.Info("Segments to be closed", "Value", strconv.Itoa(len(l.segments)))
	for _, segment := range l.segments {
		if err := segment.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Remove completely dismantles all child segment handles and wipes the entire 
// physical storage directory database from your hard drive filesystem.
func (l *Log) Remove() error {
	if err := l.Close(); err != nil {
		return err
	}
	return os.RemoveAll(l.Dir)
}

// Reset clears the directory completely and builds a fresh, initial base log state.
func (l *Log) Reset() error {
	if err := l.Remove(); err != nil {
		return err
	}
	return os.MkdirAll(l.Dir, 0755)
}

// Truncate implements Log Compaction by purging any historical segments whose absolute 
// capacity window falls entirely below the provided 'lowest' offset boundary point.
func (l *Log) Truncate(lowest uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var survivingSegments []*segment
	for _, s := range l.segments {
		// If a segment's next upcoming write sequence is lower than or equal to our 
		// cutoff line, it contains only ancient history. It is completely safe to delete.
		if s.nextOffset <= lowest + 1{
			// 📈 VISIBILITY HOOK 4: Log Compaction Disk Deletion Tracker
			slog.Warn("Purging historical log segment during compaction",
				"segment_base", s.baseOffset,
				"max_offset_contained", s.nextOffset-1,
				"lowest_threshold", lowest,
			)

			if err := s.Remove(); err != nil {
				slog.Error("Failed to delete historical segment files", "base", s.baseOffset, "error", err)
				return err
			}
			continue // Skip adding this to our surviving list since it has been destroyed.
		}
		
		// Retain any segments that still hold relevant, current data records.
		survivingSegments = append(survivingSegments, s)
	}

	// Update our master slice tracker to reference only the un-purged segments.
	l.segments = survivingSegments
	return nil
}

// These methods tell us the offset range stored in the log.
// when we work on supporting a replicated, coordinated cluster, we’ll need this information to know what nodes have the oldest 
// and newest data and what nodes are falling behind and need to replicate.
func (l *Log) LowestOffset() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.segments[0].baseOffset, nil
}

func (l *Log) HighestOffset() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	// off := l.segments[len(l.segments)-1].nextOffset
	off := l.activeSegment.nextOffset
	if off == 0 {
		return 0, nil
	}
	return off - 1, nil
}

// Reader() returns an io.Reader to read the whole log. We’ll need this capability when we implement coordinate consensus 
// and need to support snapshots and restoring a log. Reader() uses an io.MultiReader() call to concatenate the segments’ stores. 
func (l *Log) Reader() io.Reader {
	l.mu.RLock()
	defer l.mu.RUnlock()
	readers := make([]io.Reader, len(l.segments))
	for i, segment := range l.segments {
		readers[i] = &originReader{segment.store, 0}
	}
	return io.MultiReader(readers...)
}
type originReader struct {
	*store
	off int64
}

func (o *originReader) Read(p []byte) (int, error) {
	n, err := o.ReadAt(p, o.off)
	o.off += int64(n)
	return n, err
}



