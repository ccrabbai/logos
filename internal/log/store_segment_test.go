package log

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/core"
	"github.com/ccrabbai/logos/internal/observability"
	"github.com/ccrabbai/logos/internal/segment"
	"github.com/ccrabbai/logos/internal/util"

	api "github.com/ccrabbai/logos/api/logs/v1"
)

func setupQAWorkspace(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "logv-qa-test-*")
	if err != nil {
		t.Fatalf("QA-SETUP-ERR: Failed to create temporary test workspace: %v", err)
	}
	return dir, func() {
		_ = os.RemoveAll(dir)
	}
}

func TestQA_Store_FramingEngine(t *testing.T) {
	dir, cleanup := setupQAWorkspace(t)
	defer cleanup()

	file, err := os.OpenFile(filepath.Join(dir, "0.log"), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("TC-ST-ERR: Failed to open raw file: %v", err)
	}

	store, err := core.NewStore(file)
	if err != nil {
		t.Fatalf("TC-ST-ERR: Failed to instantiate store module: %v", err)
	}

	payload := []byte("QA-TEST-PAYLOAD-STRING")
	bytesWritten, pos, err := store.Append(payload)
	if err != nil {
		t.Errorf("TC-ST-001 FAILED: Unexpected append error: %v", err)
	}

	expectedTotalBytes := uint64(len(payload) + util.LenWidth)
	if bytesWritten != expectedTotalBytes {
		t.Errorf("TC-ST-001 FAILED: Expected frame width %d, captured %d", expectedTotalBytes, bytesWritten)
	}

	readPayload, err := store.Read(pos)
	if err != nil {
		t.Fatalf("TC-ST-001 FAILED: Read operation error: %v", err)
	}

	if !bytes.Equal(payload, readPayload) {
		t.Errorf("TC-ST-001 FAILED: Payload mismatch. Sent %s, read %s", payload, readPayload)
	}
	_ = store.Close()
}

func TestQA_Segment_CrashRecovery(t *testing.T) {
	dir, cleanup := setupQAWorkspace(t)
	defer cleanup()

	c := config.Config{}
	c.Segment.MaxStoreBytes = 1024
	c.Segment.MaxIndexBytes = 1024 

	baseOffset := uint64(10)

	seg, err := segment.NewSegment(dir, baseOffset, c)
	if err != nil {
		t.Fatalf("TC-SG-SETUP-ERR: Initial segment generation failed: %v", err)
	}

	r1 := &api.Record{Value: []byte("Valid Record A")}
	r2 := &api.Record{Value: []byte("Valid Record B")}

	_, _ = seg.Append(r1)
	_, _ = seg.Append(r2)

	err = seg.Close()
	if err != nil {
		t.Fatalf("TC-SG-SETUP-ERR: Graceful close failed: %v", err)
	}

	expectedHealthyIndexSize := uint64(24) 
	expectedNextOffset := baseOffset + 2 

	indexPath := filepath.Join(dir, "10.index")
	indexFile, err := os.OpenFile(indexPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("TC-SG-FAULT-ERR: Failed to open index file for manipulation: %v", err)
	}

	fakeIndexEntry := make([]byte, 12)
	util.Enc.PutUint32(fakeIndexEntry[0:4], 2)       
	util.Enc.PutUint64(fakeIndexEntry[4:12], 9999)   
	
	_, _ = indexFile.Write(fakeIndexEntry)
	_ = indexFile.Close()

	_ = os.Truncate(indexPath, int64(c.Segment.MaxIndexBytes))

	recoveredSeg, err := segment.NewSegment(dir, baseOffset, c)
	if err != nil {
		t.Fatalf("TC-SG-001 CRITICAL RECOVERY FAILURE: Engine crashed during boot loop processing: %v", err)
	}
	defer recoveredSeg.Close()

	if recoveredSeg.Index.Size != expectedHealthyIndexSize {
		t.Errorf("TC-SG-001 FAILED: Recovery engine failed to slice away phantom index frame. Index size: %d, expected: %d", 
			recoveredSeg.Index.Size, expectedHealthyIndexSize)
	}

	if recoveredSeg.NextOffset != expectedNextOffset {
		t.Errorf("TC-SG-001 FAILED: Incorrect sequence resolution. Tracked offset: %d, expected: %d", 
			recoveredSeg.NextOffset, expectedNextOffset)
	}
}

func TestQA_Integration_Concurrency(t *testing.T) {
	dir, cleanup := setupQAWorkspace(t)
	defer cleanup()

	c := config.Config{}
	c.Segment.MaxStoreBytes = 512 
	c.Segment.MaxIndexBytes = 512

	logCoordinator, err := NewLog(dir, c, &observability.Metrics{})
	if err != nil {
		t.Fatalf("TC-INT-SETUP-ERR: Log instance instantiation failed: %v", err)
	}
	defer logCoordinator.Close()

	stopTickerChan := make(chan struct{})
	var tickerWg sync.WaitGroup
	tickerWg.Add(1)
	go func() {
		defer tickerWg.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = logCoordinator.Flush(false)
			case <-stopTickerChan:
				return
			}
		}
	}()

	var workerWg sync.WaitGroup
	workerCount := 5
	appendsPerWorker := 20

	for i := 0; i < workerCount; i++ {
		workerWg.Add(1)
		go func(workerID int) {
			defer workerWg.Done()
			for j := 0; j < appendsPerWorker; j++ {
				record := &api.Record{
					Value: []byte("CONCURRENT-STRESS-DATA-STREAM"),
				}
				_, err := logCoordinator.Append(context.Background(), record)
				if err != nil {
					t.Errorf("TC-INT-002 RACE ERROR: Append transaction rejected: %v", err)
				}
			}
		}(i)
	}

	workerWg.Wait()
	close(stopTickerChan)
	tickerWg.Wait()

	if len(logCoordinator.segments) < 2 {
		t.Errorf("TC-INT-001 FAILED: Multi-file automated rollover sequence did not execute.")
	}
}
