package segment

import (
	"os"
	"fmt"
	"time"
	"hash/crc32"
	"path/filepath"

	"google.golang.org/protobuf/proto"
	"github.com/ccrabbai/logos/internal/util"
	"github.com/ccrabbai/logos/internal/core"
	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/recovery"

	api "github.com/ccrabbai/logos/api/logs/v1"
)

// Segment coordinates data persistence across unified store and index frames.
type Segment struct {
	Store      *core.Store
	Index      *core.Index
	BaseOffset uint64
	NextOffset uint64
	Cfg        config.Config
}

// NewSegment provisions a clean data coordinator instance linked to the filesystem.
func NewSegment(dir string, baseOffset uint64, c config.Config) (*Segment, error) {
	s := &Segment{
		BaseOffset: baseOffset,
		Cfg:        c,
	}

	storeFile, err := os.OpenFile(
		filepath.Join(dir, fmt.Sprintf("%d.log", baseOffset)),
		os.O_CREATE|os.O_RDWR, // os.O_APPEND
		0644,
	)
	if err != nil {
		return nil, err
	}
	if s.Store, err = core.NewStore(storeFile, c); err != nil {
		return nil, err
	}

	indexFile, err := os.OpenFile(
		filepath.Join(dir, fmt.Sprintf("%d.index", baseOffset)),
		os.O_CREATE|os.O_RDWR,
		0644,
	)
	if err != nil {
		return nil, err
	}
	if s.Index, err = core.NewIndex(indexFile, s.Cfg); err != nil {
		return nil, err
	}

	// 💡 CLEAN MODULARITY CALL: Outsource crash verification logic to our recovery sub-package
	s.NextOffset, err = recovery.AlignBoundaries2(s.Index, s.Store, s.BaseOffset)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// Append marshals a record and inserts it securely inside the storage engine layers.
func (s *Segment) Append(record *api.Record) (uint64, error) {
	cur := s.NextOffset

	// Assign values
	record.Offset = cur
	record.CreatedAt = time.Now().UnixMilli()
	record.Checksum = crc32.ChecksumIEEE(record.Value)
	if record.ProducerId == "" {
		record.ProducerId = util.DefaultProducerID
	}

	p, err := proto.Marshal(record)
	if err != nil {
		return 0, err
	}

	_, pos, err := s.Store.Append(p)
	if err != nil {
		return 0, err
	}

	if err = s.Index.Write(
		uint32(record.Offset-s.BaseOffset),
		pos,
	); err != nil {
		return 0, err
	}

	s.NextOffset++
	return cur, nil
}

// Read extracts structural Proto records matching an absolute sequence value.
func (s *Segment) Read(off uint64) (*api.Record, error) {
	// The requested global offset cannot belong to this segment.
	if off < s.BaseOffset {
		return nil, fmt.Errorf(
			"offset %d is before segment base offset %d",
			off,
			s.BaseOffset,
		)
	}
	_, pos, err := s.Index.Read(int64(off - s.BaseOffset))
	if err != nil {
		return nil, err
	}

	p, err := s.Store.Read(pos)
	if err != nil {
		return nil, err
	}

	record := &api.Record{}
	if err = proto.Unmarshal(p, record); err != nil {
		return nil, err
	}

	// Validate checksum.
	calculatedChecksum := crc32.ChecksumIEEE(record.Value)
	if calculatedChecksum != record.Checksum {
		return nil, fmt.Errorf(
			"checksum mismatch for offset %d: expected %d, got %d",
			record.Offset,
			record.Checksum,
			calculatedChecksum,
		)
	}

	return record, nil
}

// IsFull verifies if file sizes have broken configuration capacity criteria.
func (s *Segment) IsFull() bool {
	return s.Store.Size >= s.Cfg.Segment.MaxStoreBytes ||
		s.Index.Size >= s.Cfg.Segment.MaxIndexBytes
}
