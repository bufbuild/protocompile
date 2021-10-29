package ast

import (
	"fmt"
	"sort"
)

// FileInfo contains information about the contents of a source file, including
// details about comments and tokens. A lexer accumulates these details as it
// scans the file contents. This allows efficient representation of things like
// source positions.
type FileInfo struct {
	// The name of the source file.
	name string
	// The raw contents of the source file.
	data []byte
	// The offsets for each line in the file. The value is the zero-based byte
	// offset for a given line. The line is given by its index. So the value at
	// index 0 is the offset for the first line (which is always zero). The
	// value at index 1 is the offset at which the second line begins. Etc.
	lines []int
	// The info for every comment in the file. This is empty if the file has no
	// comments. The first entry corresponds to the first comment in the file,
	// and so on.
	comments []commentInfo
	// The info for every token in the file. The last item in the slice
	// corresponds to the EOF, so every file (even an empty one) has at least
	// one element. This includes all terminal symbols in the AST as well as
	// all comments. However, it excludes rune nodes (which can be more
	// compactly represented by an offset into data).
	tokens []tokenSpan
}

type commentInfo struct {
	// the index of the token, in the file's tokens slice, that represents this
	// comment
	index int
	// the index of the token to which this comment is attributed.
	attributedToIndex int
}

type tokenSpan struct {
	// the offset into the file of the first character of a token.
	offset int
	// the length of the token
	length int
}

// NewFileInfo creates a new instance for the given file.
func NewFileInfo(filename string, contents []byte) *FileInfo {
	return &FileInfo{
		name:  filename,
		data:  contents,
		lines: []int{0},
	}
}

func (f *FileInfo) Name() string {
	return f.name
}

// AddLine adds the offset representing the beginning of the "next" line in the file.
// The first line always starts at offset 0, the second line starts at offset-of-newline-char+1.
func (f *FileInfo) AddLine(offset int) {
	if offset < 0 {
		panic(fmt.Sprintf("invalid offset: %d must not be negative", offset))
	}
	if offset > len(f.data) {
		panic(fmt.Sprintf("invalid offset: %d is greater than file size %d", offset, len(f.data)))
	}

	if len(f.lines) > 0 {
		lastOffset := f.lines[len(f.lines)-1]
		if offset <= lastOffset {
			panic(fmt.Sprintf("invalid offset: %d is not greater than previously observed line offset %d", offset, lastOffset))
		}
	}

	f.lines = append(f.lines, offset)
}

// AddToken adds info about a token at the given location to this file. It
// returns a value that allows access to all of the token's details.
func (f *FileInfo) AddToken(offset, length int) Token {
	if offset < 0 {
		panic(fmt.Sprintf("invalid offset: %d must not be negative", offset))
	}
	if length < 0 {
		panic(fmt.Sprintf("invalid length: %d must not be negative", length))
	}
	if offset+length > len(f.data) {
		panic(fmt.Sprintf("invalid offset+length: %d is greater than file size %d", offset+length, len(f.data)))
	}

	tokenID := len(f.tokens)
	if len(f.tokens) > 0 {
		lastToken := f.tokens[tokenID-1]
		lastEnd := lastToken.offset + lastToken.length - 1
		if offset <= lastEnd {
			panic(fmt.Sprintf("invalid offset: %d is not greater than previously observed token end %d", offset, lastEnd))
		}
	}

	f.tokens = append(f.tokens, tokenSpan{offset: offset, length: length})
	return Token(tokenID)
}

// AddComment adds info about a comment to this file. Comments must first be
// added as tokens via f.AddToken(). The given comment argument is the TokenInfo
// from that step. The given attributedTo argument indicates another token in the
// file with which the comment is associated. If comment's offset is before that
// of attributedTo, then this is a leading comment. Otherwise, it is a trailing
// comment.
func (f *FileInfo) AddComment(comment, attributedTo Token) Comment {
	if len(f.comments) > 0 {
		lastComment := f.comments[len(f.comments)-1]
		if int(comment) <= lastComment.index {
			panic(fmt.Sprintf("invalid index: %d is not greater than previously observed comment index %d", comment, lastComment.index))
		}
		if int(attributedTo) < lastComment.attributedToIndex {
			panic(fmt.Sprintf("invalid attribution: %d is not greater than previously observed comment attribution index %d", attributedTo, lastComment.attributedToIndex))
		}
	}

	f.comments = append(f.comments, commentInfo{index: int(comment), attributedToIndex: int(attributedTo)})
	return Comment{
		fileInfo: f,
		index:    len(f.comments) - 1,
	}
}

func (f *FileInfo) NodeInfo(n Node) NodeInfo {
	return NodeInfo{fileInfo: f, startIndex: int(n.Start()), endIndex: int(n.End())}
}

func (f *FileInfo) TokenInfo(t Token) NodeInfo {
	return NodeInfo{fileInfo: f, startIndex: int(t), endIndex: int(t)}
}

func (f *FileInfo) isDummyFile() bool {
	return f.lines == nil
}

func (f *FileInfo) SourcePos(offset int) SourcePos {
	lineNumber := sort.Search(len(f.lines), func(n int) bool {
		return f.lines[n] > offset
	})

	// If it weren't for tabs, we could trivially compute the column
	// just based on offset and the starting offset of lineNumber :(
	// Wish this were more efficient... that would require also storing
	// computed line+column information, which would triple the size of
	// f's tokens slice...
	col := 0
	for i := f.lines[lineNumber-1]; i < offset; i++ {
		if f.data[i] == '\t' {
			nextTabStop := 8 - (col % 8)
			col += nextTabStop
		} else {
			col++
		}
	}

	return SourcePos{
		Filename: f.name,
		Offset:   offset,
		Line:     lineNumber,
		// Columns are 1-indexed in this AST
		Col: col + 1,
	}
}

// Token represents a single lexed token.
type Token int

func (t Token) asTerminalNode() terminalNode {
	return terminalNode(t)
}

// NodeInfo represents the details for a node in the source file's AST.
type NodeInfo struct {
	fileInfo             *FileInfo
	startIndex, endIndex int
}

func (n NodeInfo) Start() SourcePos {
	if n.fileInfo.isDummyFile() {
		return UnknownPos(n.fileInfo.name)
	}

	tok := n.fileInfo.tokens[n.startIndex]
	return n.fileInfo.SourcePos(tok.offset)
}

func (n NodeInfo) End() SourcePos {
	if n.fileInfo.isDummyFile() {
		return UnknownPos(n.fileInfo.name)
	}

	tok := n.fileInfo.tokens[n.endIndex]
	// find offset of last character in the span
	offset := tok.offset
	if tok.length > 0 {
		offset += tok.length - 1
	}
	pos := n.fileInfo.SourcePos(offset)
	if tok.length > 0 {
		// We return "open range", so end is the position *after* the
		// last character in the span. So we adjust
		pos.Col = pos.Col + 1
	}
	return pos
}

func (n NodeInfo) LeadingWhitespace() string {
	if n.fileInfo.isDummyFile() {
		return ""
	}

	tok := n.fileInfo.tokens[n.startIndex]
	var prevEnd int
	if n.startIndex > 0 {
		prevTok := n.fileInfo.tokens[n.startIndex-1]
		prevEnd = prevTok.offset + prevTok.length
	}
	return string(n.fileInfo.data[prevEnd:tok.offset])
}

func (n NodeInfo) LeadingComments() Comments {
	if n.fileInfo.isDummyFile() {
		return Comments{}
	}

	start := sort.Search(len(n.fileInfo.comments), func(i int) bool {
		return n.fileInfo.comments[i].attributedToIndex >= n.startIndex
	})

	if start == len(n.fileInfo.comments) || n.fileInfo.comments[start].attributedToIndex != n.startIndex {
		// no comments associated with this token
		return Comments{}
	}

	numComments := 0
	for i := start; i < len(n.fileInfo.comments); i++ {
		comment := n.fileInfo.comments[i]
		if comment.attributedToIndex == n.startIndex &&
			comment.index < n.startIndex {
			numComments++
		} else {
			break
		}
	}

	return Comments{
		fileInfo: n.fileInfo,
		first:    start,
		num:      numComments,
	}
}

func (n NodeInfo) TrailingComments() Comments {
	if n.fileInfo.isDummyFile() {
		return Comments{}
	}

	start := sort.Search(len(n.fileInfo.comments), func(i int) bool {
		comment := n.fileInfo.comments[i]
		return comment.attributedToIndex >= n.endIndex &&
			comment.index > n.endIndex
	})

	if start == len(n.fileInfo.comments) || n.fileInfo.comments[start].attributedToIndex != n.endIndex {
		// no comments associated with this token
		return Comments{}
	}

	numComments := 0
	for i := start; i < len(n.fileInfo.comments); i++ {
		comment := n.fileInfo.comments[i]
		if comment.attributedToIndex == n.endIndex {
			numComments++
		} else {
			break
		}
	}

	return Comments{
		fileInfo: n.fileInfo,
		first:    start,
		num:      numComments,
	}
}

func (n NodeInfo) RawText() string {
	startTok := n.fileInfo.tokens[n.startIndex]
	endTok := n.fileInfo.tokens[n.endIndex]
	return string(n.fileInfo.data[startTok.offset : endTok.offset+endTok.length])
}

// SourcePos identifies a location in a proto source file.
type SourcePos struct {
	Filename  string
	Line, Col int
	Offset    int
}

func (pos SourcePos) String() string {
	if pos.Line <= 0 || pos.Col <= 0 {
		return pos.Filename
	}
	return fmt.Sprintf("%s:%d:%d", pos.Filename, pos.Line, pos.Col)
}

// Comments represents a range of sequential comments in a source file
// (e.g. no interleaving tokens or AST nodes).
type Comments struct {
	fileInfo   *FileInfo
	first, num int
}

func (c Comments) Len() int {
	return c.num
}

func (c Comments) Index(i int) Comment {
	if i < 0 || i >= c.num {
		panic(fmt.Sprintf("index %d out of range (len = %d)", i, c.num))
	}
	return Comment{
		fileInfo: c.fileInfo,
		index:    c.first + i,
	}
}

// Comment represents a single comment in a source file. It indicates
// the position of the comment and its contents.
type Comment struct {
	fileInfo *FileInfo
	index    int
}

func (c Comment) Start() SourcePos {
	comment := c.fileInfo.comments[c.index]
	tok := c.fileInfo.tokens[comment.index]
	return c.fileInfo.SourcePos(tok.offset)
}

func (c Comment) End() SourcePos {
	comment := c.fileInfo.comments[c.index]
	tok := c.fileInfo.tokens[comment.index]
	return c.fileInfo.SourcePos(tok.offset + tok.length - 1)
}

func (c Comment) LeadingWhitespace() string {
	comment := c.fileInfo.comments[c.index]
	tok := c.fileInfo.tokens[comment.index]
	var prevEnd int
	if comment.index > 0 {
		prevTok := c.fileInfo.tokens[comment.index-1]
		prevEnd = prevTok.offset + prevTok.length
	}
	return string(c.fileInfo.data[prevEnd:tok.offset])
}

func (c Comment) RawText() string {
	comment := c.fileInfo.comments[c.index]
	tok := c.fileInfo.tokens[comment.index]
	return string(c.fileInfo.data[tok.offset : tok.offset+tok.length])
}
