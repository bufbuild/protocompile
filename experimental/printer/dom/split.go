package dom

// TODO: docs
type SplitKind int8

const (
	SplitKindUnknown = iota
	// SplitKindSoft represents a soft split, which means that when the block containing the
	// chunk is evaluated, this chunk may be split to a hard split.
	//
	// If the chunk remains a soft split, spaceWhenUnsplit will add a space after the chunk if
	// true and will add nothing if false. spaceWhenUnsplit is ignored for all other split kinds.
	SplitKindSoft
	// splitKindHard represents a hard split, which means the chunk must be followed by a newline.
	SplitKindHard
	// splitKindDouble represents a double hard split, which means the chunk must be followed by
	// two newlines.
	SplitKindDouble
	// splitKindNever represents a chunk that must never be split. This is treated similar to
	// a soft split, in that it will respect spaceWhenUnsplit.
	SplitKindNever
)
