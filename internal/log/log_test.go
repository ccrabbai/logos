package log

import (
	"context"
	"log/slog"
	"os"
	"testing"

	// The stretchr/testify toolkit eliminates boilerplate 'if err != nil'
	// error handling blocks, keeping our test files scannable.
	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/observability"
	"github.com/ccrabbai/logos/internal/util"
	"github.com/stretchr/testify/require"

	api "github.com/ccrabbai/logos/api/logs/v1"
)

func init() {
	// Reconfigure our system logger to output strict JSON formatting straight to the terminal
	// Set the level to LevelDebug to make sure our 'Record committed successfully' traces appear
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))
}


// TestLog orchestrates the complete end-to-end integration testing of our log coordinator.
func TestLog(t *testing.T) {
	ctx := context.Background()
	
	// Step 1: Setup a completely isolated workspace folder on your hard drive.
	dir, err := os.MkdirTemp("", "log_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(dir) // Cleans up and deletes all generated test logs at the end.

	// Step 2: Inject an artificially small store size limit to trigger rollovers quickly.
	var cfg config.Config
	cfg.Segment.MaxStoreBytes = 16 // 32 bytes ensures writing 3 records causes a split.
	
	// Step 3: Launch the master parent coordinator.
	l, err := NewLog(dir, cfg, &observability.Metrics{})
	require.NoError(t, err)

	// Step 4: Validate the Lifecycle Loop (Happy Path Appends and Reads)
	record := &api.Record{Value: []byte("hello world")}

	// Append three entries. This will push total bytes past our 32-byte ceiling,
	// forcing the log engine to execute an internal segment rollover in the background.
	for i := 0; i < 3; i++ {
		off, err := l.Append(ctx, record)
		require.NoError(t, err)
		
		// Assert that global offsets increment perfectly from 0, 1, 2.
		require.Equal(t, uint64(i), off)
	}

	// Step 5: Validate the O(log N) Binary Search Read Router.
	// Query record 2. The binary search must look at segment base boundaries,
	// pick the correct file chunk, and extract the uncorrupted data payload.
	readRecord, err := l.Read(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, record.Value, readRecord.Value)

	// Step 6: Validate the Automatic Segment Rollover Trigger.
	// Because our MaxStoreBytes was only 32 bytes, writing 3 records must have forced
	// our segment slice length to grow from 1 to at least 2 distinct segment objects.
	require.True(t, len(l.segments) > 1, "Log failed to trigger an automatic segment rollover file split")

	// Step 7: Validate the Crash Recovery State Rebuilding.
	// Close down the parent log coordinator to release all active file handles and memory maps.
	require.Equal(t, uint64(3), l.activeSegment.NextOffset)
	err = l.Close()
	require.NoError(t, err)

	// Fire up a brand-new Log coordinator pointing to the exact same directory files.
	// This simulates a server booting back up after a sudden restart or crash.
	l, err = NewLog(dir, cfg, &observability.Metrics{})
	require.NoError(t, err)

	// The recovered log must scan the directory, look at the historical files using nearestMultiple,
	// and pick up its nextOffset exactly where it left off (Offset 3).
	require.Equal(t, uint64(3), l.activeSegment.NextOffset)

	// Confirm we can still read historical data perfectly from the recovered state.
	readRecord, err = l.Read(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, record.Value, readRecord.Value)

	// Step 8: Validate the Log Compaction Engine (Truncate)
	// Instruct the engine to delete any ancient history up to and including Offset 1.
	err = l.Truncate(1)
	require.NoError(t, err)
	require.Equal(t, len(l.segments), 1)

	// Verification A: Attempting to read an offset that was just purged (Offset 0)
	// must throw an error, proving the physical file data was erased cleanly.
	_, err = l.Read(ctx, 1)
	require.Error(t, err)

	// Verification B: The active, relevant data (Offset 2) must still remain fully intact
	// and readable, proving compaction did not destroy current runtime logs.
	readRecord, err = l.Read(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, record.Value, readRecord.Value)

	// Read non-existent offset
	readRecord, err = l.Read(ctx, 2121)
	require.Nil(t, readRecord)
	apiErr := err.(util.ErrOffsetOutOfRange)
	require.Equal(t, uint64(1), apiErr.Offset)
}
