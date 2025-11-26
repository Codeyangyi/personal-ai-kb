package splitter

import (
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/textsplitter"
)

// TextSplitter 文本切分器
type TextSplitter struct {
	chunkSize    int
	chunkOverlap int
}

// NewTextSplitter 创建新的文本切分器
func NewTextSplitter(chunkSize, chunkOverlap int) *TextSplitter {
	return &TextSplitter{
		chunkSize:    chunkSize,
		chunkOverlap: chunkOverlap,
	}
}

// SplitDocuments 切分文档
func (s *TextSplitter) SplitDocuments(docs []schema.Document) ([]schema.Document, error) {
	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(s.chunkSize),
		textsplitter.WithChunkOverlap(s.chunkOverlap),
	)

	// 使用textsplitter包的SplitDocuments函数
	allSplits, err := textsplitter.SplitDocuments(splitter, docs)
	if err != nil {
		return nil, err
	}

	return allSplits, nil
}
