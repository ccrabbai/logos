package logv

import (
	// "encoding/binary"
	"io"
	"os"

	"github.com/tysonmote/gommap"
	"github.com/ccrabbai/logos/internal/config"
)

var (
	// enc specifies our standard byte translation strategy (Big-Endian).
	// This ensures that our index files read identically on any hardware architecture.
	//enc = binary.BigEndian //Already declared in same package (store.go)

	// offWidth represents the size of our logical record sequence ID -- See DD-003.
	// A 32-bit unsigned integer (uint32) requires exactly 4 bytes.
	offWidth uint32 = 4

	// posWidth represents the size of our physical store log address pointer -- See DD-003.
	// A 64-bit unsigned integer (uint64) requires exactly 8 bytes.
	posWidth uint64 = 8

	// entWidth represents the absolute size of a single complete index entry slot.
	// Combining the 4-byte offset and 8-byte position yields exactly 12 bytes.
	entWidth = uint64(offWidth) + posWidth
)

// index maps our logical offsets to physical positions using memory-mapped files.
type index struct {
	file *os.File      // Underlying operating system file descriptor handle.
	mmap gommap.MMap   // Memory-mapped byte slice linked directly to the physical storage drive.
	size uint64        // Memory tracker counting only the bytes that contain REAL data.
}

// newIndex provisions a high-speed memory-mapped entry scanner over a raw file descriptor.
func newIndex(f *os.File, c config.Config) (*index, error) {
	idx := &index{
		file: f,
	}

	// Step 1: Check the file size BEFORE altering it to handle crash recovery states.
	// If the server restarted, this file might already have historical data entries.
	fi, err := os.Stat(f.Name())
	if err != nil {
		return nil, err
	}
	idx.size = uint64(fi.Size())

	// Step 2: Enforce Pre-allocation. We must stretch the physical file size to its 
	// maximum capacity configuration upfront because memory maps cannot grow dynamically.
	if err = os.Truncate(f.Name(), int64(c.Segment.MaxIndexBytes)); err != nil {
		return nil, err
	}

	// Step 3: Trigger the OS Memory Map system call.
	// PROT_READ|PROT_WRITE gives our code permissions to read and write directly to this region.
	// MAP_SHARED means modifications to the mapping are shared with the underlying file mapping.
	if idx.mmap, err = gommap.Map(
		idx.file.Fd(),
		gommap.PROT_READ|gommap.PROT_WRITE,
		gommap.MAP_SHARED,
	); err != nil {
		return nil, err
	}

	return idx, nil
}

// Close gracefully strips out pre-allocated zero-padding and preserves physical resources.
func (i *index) Close() error {
	// Step 1: Sync memory map changes cleanly down to the physical silicon storage.
	if err := i.mmap.Sync(gommap.MS_SYNC); err != nil {
		return err
	}

	// Step 2: Force the physical operating system file descriptor cache to commit.
	if err := i.file.Sync(); err != nil {
		return err
	}

	// 3. 🛡️ WINDOWS FIX: Explicitly unmap the memory block first!
	// This tears down the user-mapped section and releases the Windows file system lock.
	if err := i.mmap.UnsafeUnmap(); err != nil {
		return err
	}

	// Step 4: Strip away the unwritten, empty pre-allocated padding blocks.
	// We shrink the file size down to match exactly our real data tracker boundary ('i.size').
	if err := i.file.Truncate(int64(i.size)); err != nil {
		return err
	}

	// Step 4: Disconnect and release the file descriptor back to the operating system kernel.
	return i.file.Close()
}

// Read retrieves a specific index frame using a relative entry slot index pointer.
func (i *index) Read(in int64) (out uint32, pos uint64, err error) {
	// Guard Clause: If size is zero, there are no entries available to parse.
	if i.size == 0 {
		return 0, 0, io.EOF
	}

	// Pattern Translation: If the requested slot is -1, fetch the absolute latest entry in the log.
	if in == -1 {
		out = uint32((i.size / entWidth) - 1)
	} else {
		out = uint32(in) // Otherwise, extract the specific relative slot requested.
	}

	// Mathematical Lookup: Multiply the slot index by 12 to find the absolute starting byte address.
	pos = uint64(out) * entWidth
 
	// Boundary Verification: Ensure we aren't reading past our valid data zone into empty zero padding.
	if i.size < pos+entWidth {
		return 0, 0, io.EOF
	}

	// Slice Window Extraction:
	// Line A: Isolate bytes [pos : pos+4] to decode the logical record sequence ID.
	out = enc.Uint32(i.mmap[pos : pos+uint64(offWidth)])
	
	// Line B: Isolate bytes [pos+4 : pos+12] to decode the physical starting address within store.log.
	pos = enc.Uint64(i.mmap[pos+uint64(offWidth) : pos+entWidth])

	return out, pos, nil
}

// Write serializes and packs a logical offset and physical position tightly into the memory map.
func (i *index) Write(off uint32, pos uint64) error {
	// Capacity Verification: Check if adding another 12-byte slot breaches our max file boundaries. i.mmap has been stretched already
	if i.size+entWidth > uint64(len(i.mmap)) {
		return io.EOF
	}

	// Slice Window Packing:
	// Line A: Serialize the 4-byte offset integer into the memory map space starting at 'i.size'.
	enc.PutUint32(i.mmap[i.size : i.size+uint64(offWidth)], off)
	
	// Line B: Serialize the 8-byte position address directly behind the 4 bytes we just stamped.
	enc.PutUint64(i.mmap[i.size+uint64(offWidth) : i.size+entWidth], pos)

	// Increment our data accounting boundary tracker forward by exactly 12 bytes.
	i.size += uint64(entWidth)
	
	return nil
}

// Name returns the absolute path name of the index file sitting on our storage drive.
func (i *index) Name() string {
	return i.file.Name()
}
