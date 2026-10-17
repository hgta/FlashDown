package engine

// Block is the only unit that is persisted, scheduled and drawn.
//
// Resume is nothing more than `Range: bytes=(Start+Done)-(End-1)`, which is why
// there is no bitfield layer: three integers per block already describe exactly
// how much of the file is on disk (design.md D2).
type Block struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`  // exclusive
	Done  int64 `json:"done"` // bytes already written from Start

	// Jet is the connection currently writing this block, or -1 when idle. It is
	// runtime-only: claims do not survive a restart, because the jets that made
	// them do not either. Never persisted, which is why 0 must not be the zero
	// value - every constructor below sets -1 explicitly.
	Jet int `json:"-"`
}

// Len is the block's full size in bytes.
func (b Block) Len() int64 { return b.End - b.Start }

// Remaining is how many bytes are still missing.
func (b Block) Remaining() int64 { return b.Len() - b.Done }

// Complete reports whether the block's bytes are all on disk.
func (b Block) Complete() bool { return b.Done >= b.Len() }

// Offset is the absolute file offset where the next byte goes. This is the only
// number a resume needs.
func (b Block) Offset() int64 { return b.Start + b.Done }

// BlockSize implements design.md D3:
//
//	clamp(align64KB(total / (maxJets x 8)), 256KB, 8MB)
//
// Eight blocks per jet is what keeps the tail short: a slow connection simply
// takes fewer blocks, and no block has to be split mid-flight.
func BlockSize(total int64, maxJets int) int64 {
	const (
		align = 64 << 10
		floor = 256 << 10
		ceil  = 8 << 20
	)
	if maxJets < 1 {
		maxJets = 1
	}

	size := (total / int64(maxJets*8) / align) * align
	if size < floor {
		size = floor
	}
	if size > ceil {
		size = ceil
	}
	return size
}

// PlanBlocks cuts [0,total) into blocks. M1 only ever runs one jet, but the
// layout is planned for maxJets from the start so M2 does not have to re-plan
// (and therefore re-download) existing files.
func PlanBlocks(total int64, maxJets int) []Block {
	if total <= 0 {
		return nil
	}

	size := BlockSize(total, maxJets)
	if size >= total {
		return []Block{{Start: 0, End: total, Jet: -1}}
	}

	blocks := make([]Block, 0, total/size+1)
	for off := int64(0); off < total; off += size {
		end := off + size
		if end > total {
			end = total
		}
		blocks = append(blocks, Block{Start: off, End: end, Jet: -1})
	}
	return blocks
}

// SingleBlock covers the whole file in one block. Used when the server refuses
// Range requests, and when the user asks to start over.
func SingleBlock(total int64) []Block {
	if total <= 0 {
		return nil
	}
	return []Block{{Start: 0, End: total, Jet: -1}}
}

// ClearJets marks every block idle. Blocks restored from a journal carry no claim
// (the jets that made them are long gone), so this runs before every plan is used.
func ClearJets(blocks []Block) {
	for i := range blocks {
		blocks[i].Jet = -1
	}
}

// TotalDone sums what is already on disk.
func TotalDone(blocks []Block) int64 {
	var done int64
	for _, b := range blocks {
		if b.Complete() {
			done += b.Len()
			continue
		}
		done += b.Done
	}
	return done
}

// AllDone reports whether every byte has been written.
func AllDone(blocks []Block) bool {
	for _, b := range blocks {
		if !b.Complete() {
			return false
		}
	}
	return len(blocks) > 0
}

// NextIncomplete returns the index of the first block with bytes missing, or -1.
func NextIncomplete(blocks []Block) int {
	for i, b := range blocks {
		if !b.Complete() {
			return i
		}
	}
	return -1
}
