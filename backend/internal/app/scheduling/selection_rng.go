package scheduling

// SelectionRNG is used only to shuffle equal-priority, equal-load candidates.
type SelectionRNG struct{ state uint64 }

func NewSelectionRNG(seed uint64) SelectionRNG {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return SelectionRNG{state: seed}
}

func (r *SelectionRNG) NextUint64() uint64 {
	x := r.state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	return x * 2685821657736338717
}

func (r *SelectionRNG) NextFloat64() float64 {
	return float64(r.NextUint64()>>11) / (1 << 53)
}
