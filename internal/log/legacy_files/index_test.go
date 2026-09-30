package logv

import (
	"io"
	"os"
	"testing"

	// The stretchr/testify toolkit removes ugly 'if err != nil'
	// boilerplate error tracking blocks, keeping code readable.
	"github.com/stretchr/testify/require"

	"github.com/ccrabbai/logos/internal/config"
)

// TestIndexOperation orchestrates the lifecycle validation of our memory-mapped index structure.
func TestIndexOperation(t *testing.T) {
	
	// Phase 1: Environment Provisioning (Isolation)
	// Create an isolated, blank temporal file handle on the host disk to run operations safely.
	file, err := os.CreateTemp("", "index_test_file")
	require.NoError(t, err)
	defer os.Remove(file.Name()) // Clean up and wipe the file path once our test engine completes.
	defer file.Close()           // Ensure any lingering file resources are returned to the OS.

	// Phase 2: Configuration Constraints Setup
	// Instruct the memory map module to claim a maximum pre-allocation footprint size of 1024 bytes.
	cfg := config.Config{}
	cfg.Segment.MaxIndexBytes = 1024

	// Phase 3: Component Lifecycle Initialization
	// Stretch the file descriptor to 1024 bytes and map it directly into the process memory layout.
	idx, err := newIndex(file, cfg)
	require.NoError(t, err)
	
	// Confirm that our custom interface abstraction mirrors the exact target filename path on disk.
	require.Equal(t, file.Name(), idx.Name())

	// Phase 4: Mock Data Generation
	// Establish structural records tracking logical offsets mapped to physical data positions.
	type entry struct {
		Off uint32 // 4-Byte Logical Sequence Array ID Counter.
		Pos uint64 // 8-Byte Physical Starting Byte Address inside store.log.
	}

	entries := []entry{
		{Off: 0, Pos: 0},  // Slot 0: Maps first record starting at the very beginning (byte 0).
		{Off: 1, Pos: 35}, // Slot 1: Maps second record packed tightly after frame 1 ends (byte 35).
		{Off: 2, Pos: 64}, // Slot 2: Maps third record packed tightly after frame 2 ends (byte 64).
	}

	// Phase 5: Sequential Append & Boundary Validation Execution
	for i, entry := range entries {
		// Serialize and stamp the 12-byte structural record into the active memory-mapped slice.
		err := idx.Write(entry.Off, entry.Pos)
		require.NoError(t, err)

		// SYSTEM CRITICAL CHECK: Confirm the internal data tracking ledger progresses linearly 
		// by exactly 12 bytes on each loop (Iteration 1 = 12B, Iteration 2 = 24B, Iteration 3 = 36B).
		// This proves there is no byte drifting or fragmentation corruption.
		require.Equal(t, entWidth*uint64(i+1), idx.size)

		// Immediately read back the record we just stored to confirm it matches perfectly.
		_, pos, err := idx.Read(int64(i))
		require.NoError(t, err)
		require.Equal(t, entry.Pos, pos)
	}

	// Phase 6: O(1) Random Access Verification
	// Target the 3rd record slot directly out of sequence, bypass reading slots 0 and 1.
	out, pos, err := idx.Read(2)
	require.NoError(t, err)
	// Assert that the pointer math located and returned the exact byte values for entry index 2.
	require.Equal(t, entries[2].Off, out)
	require.Equal(t, entries[2].Pos, pos)

	// Phase 7: Edge Case A Check (Cluster Optimization Hook)
	// Passing a slot value of -1 must instruct the index map to decode the absolute latest entry.
	out, pos, err = idx.Read(-1)
	require.NoError(t, err)
	require.Equal(t, entries[2].Off, out) // Assert it targets the last written offset.
	require.Equal(t, entries[2].Pos, pos) // Assert it targets the last written physical location.

	// Phase 8: Edge Case B Check (Safety Guard Railing)
	// Querying a slot index that goes past the written data zone must throw a predictable io.EOF
	// instead of crashing the system with an uncatchable runtime segmentation memory fault.
	_, _, err = idx.Read(100) 
	require.Equal(t, io.EOF, err)

	// Phase 9: Operating System Sync & File Allocation Shrinkage Check
	fileName := idx.Name() // Preserve the disk file target name string before tearing it down.

	// Close down the index engine wrapper. This must flush RAM buffers, tear down the 
	// virtual memory section map, and truncate the 1024-byte file down to exactly 36 bytes.
	err = idx.Close()
	require.NoError(t, err)

	// Inquire with the OS kernel to get the final metadata state properties of the file on disk.
	fi, err := os.Stat(fileName)
	require.NoError(t, err)

	// CRITICAL ALLOCATION CHECK: Confirm the physical file on disk shrank from its 1024-byte 
	// running capacity down to exactly 36 bytes (3 records * 12 bytes = 36 bytes).
	// This proves that your Windows-safe truncation fix strips out empty zero-padding cleanly.
	require.Equal(t, int64(len(entries)*int(entWidth)), fi.Size())

	// Phase 10: Server Crash Recovery State Rebuilding Validation
	// Simulate a node booting back up after a sudden restart or crash.
	f, _ := os.OpenFile(file.Name(), os.O_RDWR, 0600)
	
	// Create a new index manager wrapping the newly re-opened historical file.
	idx, err = newIndex(f, cfg)
	require.NoError(t, err)

	// Attempt to query the latest record from the recovered instance.
	// If 'CreateNewIndex' properly discovered the historical data size boundaries,
	// it should locate and return the 3rd index frame perfectly.
	off, pos, err := idx.Read(-1)
	require.NoError(t, err)
	require.Equal(t, uint32(2), off)
	require.Equal(t, entries[2].Pos, pos)
}
