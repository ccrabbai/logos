package logv

import(
	"testing"
	"os"

	// The stretchr/testify toolkit eliminates boilerplate 'if err != nil' 
	// error handling blocks, keeping our test files scannable.
	"github.com/stretchr/testify/require"
)

var (
	// The canonical payload message frame we will use throughout our test executions.
	write = []byte("Hello World")
	
	// width calculates the exact, absolute footprint size of a single frame entry on disk.
	// It adds our 8-byte uint64 length header width to the raw message character count.
	width = lenWidth + uint64(len(write))
)

// TestStoreAppendRead orchestrates the lifecycle validation of our storage engine.
func TestStoreAppendRead(t *testing.T) {
	// 1. Provision a isolated temporary file path on our host operating system disk.
	// f, err := ioutil.TempFile("", "store_append_read_test*")
	f, err := os.CreateTemp("", "store_append_read_test*")
	require.NoError(t, err)
	defer os.Remove(f.Name()) // Ensure the file is deleted when the test completes.

	// 2. Initialize a fresh store instance.
	s, err := newStore(f)
	require.NoError(t, err)

	// 3. Run localized function checks against our running engine instance.
	testAppend(t, s)
	testRead(t, s)
	testReadAt(t, s)

	// 4. PERSISTENCE/RECOVERY TEST: Close out the active store object, then create 
	// a completely new store instance targeting the exact same physical file data.
	s, err = newStore(f)
	require.NoError(t, err)
	
	// If our 'newStore' logic properly reads historical disk headers, it should
	// find our previously appended records perfectly intact.
	testRead(t, s)
}

// testAppend verifies that sequential entries pack tightly against one another on disk.
func testAppend(t *testing.T, s *store) {
	t.Helper() // Identifies this function to Go as a test setup helper.
	
	// Append the test record 3 separate times.
	for i := uint64(1); i < 4; i++ {
		n, pos, err := s.Append(write)
		require.NoError(t, err)
		
		// CRITICAL MATH ASSERTION: 'n' is bytes written, 'pos' is the record's start offset.
		// Together, (n + pos) must exactly match (width * current_iteration).
		// This proves that records stack continuously without data-corrupting gaps.
		require.Equal(t, width*i, n+pos)
	}
}

// testRead validates that high-level random access reads correctly parse length prefixes.
func testRead(t *testing.T, s *store) {
	t.Helper()
	var pos uint64
	
	// Sequentially step through the file jumping forward exactly 'width' bytes each iteration.
	for i := uint64(1); i < 4; i++ {
		content, err := s.Read(pos)
		require.NoError(t, err)
		
		// Verify that the store engine extracted the raw payload string safely.
		require.Equal(t, content, write)
		
		// Advance our lookup pointer exactly past the data frame boundaries.
		pos += width
	}
}

// testReadAt simulates how external modules bypass record boundaries to read raw bytes.
func testReadAt(t *testing.T, s *store) {
	t.Helper()
	
	// Track iteration checks alongside our low-level int64 file position offsets.
	for i, off := uint64(1), int64(0); i < 4; i++ {
		
		// STEP 1: Allocate a blank 8-byte slice to manually intercept the length prefix frame.
		b := make([]byte, lenWidth)
		n, err := s.ReadAt(b, off)
		require.NoError(t, err)
		require.Equal(t, lenWidth, n) // Ensure exactly 8 bytes were fetched.
		
		off += int64(n) // Advance our manual cursor past the header bytes.

		// STEP 2: Decode the 8 bytes back into a numeric size variable.
		size := enc.Uint64(b)
		
		// STEP 3: Provision a second slice matching that exact payload allocation footprint.
		b = make([]byte, size)
		n, err = s.ReadAt(b, off)
		require.NoError(t, err)
		
		// Assert that the raw bytes pulled directly from the index match our written payload.
		require.Equal(t, write, b)
		require.Equal(t, int(size), n)
		
		off += int64(n) // Move past the record data to position the next iteration's header.
	}
}

// TestStoreClose verifies our buffering optimization works without abandoning data in RAM.
func TestStoreClose(t *testing.T) {
	f, err := os.CreateTemp("", "store_close_test")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	s, err := newStore(f)
	require.NoError(t, err)

	// Append data. This writes to our bufio.Writer RAM cache, not the hard drive.
	_, _, err = s.Append(write)
	require.NoError(t, err)

	// Check the physical file state on the disk right now.
	f, beforeSize, err := openFile(f.Name())
	require.NoError(t, err)

	// Invoke Close(). This triggers the mandatory data cache flush to the physical file system.
	err = s.Close()
	require.NoError(t, err)

	// Check the physical file state on disk again after closing.
	_, afterSize, err := openFile(f.Name())
	require.NoError(t, err)

	// CRITICAL ASSERTION: The file size on disk MUST be larger after Close() runs.
	// This proves data was successfully flushed out of RAM cache safely down to disk.
	require.True(t, afterSize > beforeSize)
}

// openFile is an internal TEST tool/helper utility used to safely peek at active OS handles.
func openFile(name string) (file *os.File, size int64, err error) {
	f, err := os.OpenFile(
		name,
		os.O_RDWR|os.O_CREATE|os.O_APPEND,
		0644,
	)
	if err != nil {
		return nil, 0, err
	}
	
	fi, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	
	return f, fi.Size(), nil
}
