package core

import (
	"io"
	"os"

	"github.com/tysonmote/gommap"
	"github.com/ccrabbai/logos/internal/util"
	"github.com/ccrabbai/logos/internal/config"
)

// Index maps positions across memory-mapped grids.
type Index struct {
	File *os.File      // Operating system file descriptor handle.
	Mmap gommap.MMap   // Memory-mapped byte slice linked to virtual memory.
	Size uint64        // Memory tracker counting bytes that contain valid data.
}

// NewIndex provisions a memory-mapped entry scanner over a raw file descriptor.
func NewIndex(f *os.File, c config.Config) (*Index, error) {
	idx := &Index{File: f}

	fi, err := os.Stat(f.Name())
	if err != nil {
		return nil, err
	}
	initialFileSize := uint64(fi.Size())

	if err = os.Truncate(f.Name(), int64(c.Segment.MaxIndexBytes)); err != nil {
		return nil, err
	}

	if idx.Mmap, err = gommap.Map(
		idx.File.Fd(),
		gommap.PROT_READ|gommap.PROT_WRITE,
		gommap.MAP_SHARED,
	); err != nil {
		return nil, err
	}

	// 💡 Your Zero-Padding Scanner extracted inside core primitive setup!
	if initialFileSize == c.Segment.MaxIndexBytes {
		var realSize uint64 = 0
		for realSize+util.EntWidth <= uint64(len(idx.Mmap)) {
			posBytes := idx.Mmap[realSize+uint64(util.OffWidth) : realSize+util.EntWidth]
			if realSize > 0 && util.Enc.Uint64(posBytes) == 0 {
				break
			}
			
			if realSize == 0 {
				offBytes := idx.Mmap[realSize : realSize+uint64(util.OffWidth)]
				if util.Enc.Uint32(offBytes) == 0 && util.Enc.Uint64(posBytes) == 0 {
					if util.Enc.Uint64(idx.Mmap[realSize+util.EntWidth+uint64(util.OffWidth):realSize+util.EntWidth+util.EntWidth]) == 0 {
						realSize = 0
						break
					}
				}
			}
			realSize += util.EntWidth
		}
		idx.Size = realSize
	} else {
		idx.Size = initialFileSize
	}

	return idx, nil
}

func (i *Index) Close() error {
	if err := i.Mmap.Sync(gommap.MS_SYNC); err != nil {
		return err
	}
	if err := i.File.Sync(); err != nil {
		return err
	}
	if err := i.Mmap.UnsafeUnmap(); err != nil {
		return err
	}
	if err := i.File.Truncate(int64(i.Size)); err != nil {
		return err
	}
	return i.File.Close()
}

func (i *Index) Read(in int64) (out uint32, pos uint64, err error) {
	if i.Size == 0 {
		return 0, 0, io.EOF
	}

	if in == -1 {
		out = uint32((i.Size / util.EntWidth) - 1)
	} else {
		out = uint32(in)
	}

	pos = uint64(out) * util.EntWidth
	if i.Size < pos+util.EntWidth {
		return 0, 0, io.EOF
	}

	out = util.Enc.Uint32(i.Mmap[pos : pos+uint64(util.OffWidth)])
	pos = util.Enc.Uint64(i.Mmap[pos+uint64(util.OffWidth) : pos+util.EntWidth])
	return out, pos, nil
}

func (i *Index) Write(off uint32, pos uint64) error {
	if i.Size+util.EntWidth > uint64(len(i.Mmap)) {
		return io.EOF
	}

	util.Enc.PutUint32(i.Mmap[i.Size : i.Size+uint64(util.OffWidth)], off)
	util.Enc.PutUint64(i.Mmap[i.Size+uint64(util.OffWidth) : i.Size+util.EntWidth], pos)
	i.Size += uint64(util.EntWidth)
	return nil
}

func (i *Index) Truncate(targetSize uint64) error {
	i.Size = targetSize
	return nil
}

func (i *Index) Flush() error {
	if err := i.Mmap.Sync(gommap.MS_SYNC); err != nil {
		return err
	}
	return i.File.Sync()
}

func (i *Index) Name() string {
	return i.File.Name()
}
