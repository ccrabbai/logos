package util

import (
	"encoding/binary"
)

var (
	// Enc specifies our standard byte translation strategy (Big-Endian).
	Enc = binary.BigEndian

	// OffWidth represents the size of our logical record sequence ID (uint32 = 4 bytes).
	OffWidth uint32 = 4

	// PosWidth represents the size of our physical store log address pointer (uint64 = 8 bytes).
	PosWidth uint64 = 8

	// EntWidth represents the absolute size of a single complete index entry slot (12 bytes).
	EntWidth = uint64(OffWidth) + PosWidth
)

const (
	// LenWidth represents the size of our record length prefix header (uint64 = 8 bytes).
	LenWidth = 8

	// Default producer id
	DefaultProducerID = "unknown"
)
