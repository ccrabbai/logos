package logv

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ccrabbai/logos/internal/config"
	api "github.com/ccrabbai/logos/api/logs/v1"

	// The stretchr/testify toolkit eliminates boilerplate 'if err != nil' 
	// error handling blocks, keeping our test files scannable.
	"github.com/stretchr/testify/require"
)

// TestSegment validates coordinate mapping, capacity triggers, and crash recovery.
func TestSegment(t *testing.T) {
	//------------------------------ Sub-section 1: Environment Setup and Initialization
	// Step 1: Provision an isolated, clean workspace directory on your drive.
	// dir, err := os.MkdirTemp("", "segment_test_*")
	dir, err := ioutil.TempDir("", "segment-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir) // Automatically wipes files when the test completes.

	// Step 2: Formulate artificial sizing boundaries to trigger limits quickly.
	var cfg config.Config
	cfg.Segment.MaxStoreBytes = 1024 // Room for our serialized data records.
	cfg.Segment.MaxIndexBytes = 36   // Fits exactly 3 index entries (3 * 12 bytes = 36).

	// Step 3: Instantiate a fresh segment anchored at a non-zero base offset (16).
	// This proves that relative subtraction translations work under real conditions.
	baseOffset := uint64(16)
	s, err := newSegment(dir, baseOffset, cfg)
	require.NoError(t, err)

	// Verify that a brand new segment sets its initial nextOffset to match the baseOffset.
	require.Equal(t, baseOffset, s.nextOffset)
	require.False(t, s.IsFull()) // Ensure capacity metrics start empty.

	//------------------------------ Sub-section 2: Happy Path Execution (Writes and Reads)
	// Step 4: Write and verify consecutive log entries.
	want := &api.Record{Value: []byte("hello world")}

	// We append exactly 3 items to completely saturate our configured 36-byte index map limit.
	for i := range uint64(3) {
		// Append returns the assigned global sequence ID.
		off, err := s.Append(want)
		require.NoError(t, err)
		
		// Assert that global offsets increment linearly: 16, 17, 18.
		require.Equal(t, baseOffset+i, off)

		// Immediately read the data back out to verify zero memory/disk drift.
		got, err := s.Read(off)
		require.NoError(t, err)
		require.Equal(t, want.Value, got.Value)
	}

	//------------------------------ Sub-section 3: Boundary Collision Check (IsFull)
	// Step 5: Assert that our capacity engine successfully flags the boundary breach.
	// Since 3 entries use 36 bytes, IsFull() must instantly return true.
	require.True(t, s.IsFull())

	// Step 6: Verify that further appends to a saturated segment throw a predictable error.
	_, err = s.Append(want)
	require.Equal(t, io.EOF, err)

	//------------------------------ Sub-section 4: Disaster Recovery State Rebuilding
	// Step 7: Gracefully dismantle active file descriptors to mimic a server shutdown.
	// This flushes RAM buffers and strips pre-allocated padding blocks.
	err = s.Close()
	require.NoError(t, err)

	// Step 8: CRASH RECOVERY SIMULATION
	// Launch a completely new segment manager referencing the historical disk directory.
	s, err = newSegment(dir, baseOffset, cfg)
	require.NoError(t, err)

	// Verification A: Ensure it scans the existing index file, determines that
	// offsets 16, 17, and 18 are already spoken for, and parks nextOffset on 19.
	require.Equal(t, uint64(19), s.nextOffset)
	
	// Verification B: Confirm that the recovered index file boundary is flagged as full
	// to prevent subsequent writes from corrupting historical logs.
	require.True(t, s.IsFull())

	err = s.Close()
	require.NoError(t, err)

	//------------------------------

	cfg.Segment.MaxStoreBytes = uint64(len(want.Value) * 3)
	cfg.Segment.MaxIndexBytes = 1024

	s, err = newSegment(dir, baseOffset, cfg)
	require.NoError(t, err)
	// maxed store
	require.True(t, s.IsFull())
	
	err = s.Remove()
	require.NoError(t, err)
	s, err = newSegment(dir, baseOffset, cfg)
	require.NoError(t, err)
	require.False(t, s.IsFull())
}