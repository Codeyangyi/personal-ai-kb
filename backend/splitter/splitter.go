package splitter

import (
	"strings"
	"unicode/utf8"

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

	// 在分割后清理每个文档片段的编码，确保没有乱码
	for i := range allSplits {
		allSplits[i].PageContent = cleanTextEncoding(allSplits[i].PageContent)
	}

	return allSplits, nil
}

// cleanTextEncoding 清理和修复文本编码，确保是有效的UTF-8
// 移除无效的UTF-8字符、控制字符和乱码字符，替换为空格或删除
func cleanTextEncoding(text string) string {
	if text == "" {
		return text
	}

	var result strings.Builder
	result.Grow(len(text)) // 预分配容量以提高性能

	// 逐字符处理，确保所有字符都是有效的UTF-8
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		
		// 处理无效的UTF-8字符
		if r == utf8.RuneError && size == 1 {
			// 遇到无效的UTF-8字符，跳过
			text = text[size:]
			continue
		}

		// 过滤掉Unicode替换字符（U+FFFD，通常显示为）
		if r == '\uFFFD' {
			text = text[size:]
			continue
		}

		// 过滤掉控制字符（除了换行符、制表符等常见空白字符）
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			// 用空格替换控制字符
			result.WriteRune(' ')
			text = text[size:]
			continue
		}

		// 过滤掉某些特殊Unicode字符范围（可能产生乱码的字符）
		// 这些范围包括：私有使用区、代理对区域等
		if (r >= 0xE000 && r <= 0xF8FF) || // 私有使用区
			(r >= 0xF0000 && r <= 0xFFFFD) || // 补充私有使用区-A
			(r >= 0x100000 && r <= 0x10FFFD) { // 补充私有使用区-B
			text = text[size:]
			continue
		}

		// 保留有效的字符
		result.WriteRune(r)
		text = text[size:]
	}

	text = result.String()

	// 清理连续的乱码字符模式
	// 移除连续的无效字符序列
	text = strings.ReplaceAll(text, "\uFFFD", " ")
	
	// 清理多余的空白字符
	text = strings.ReplaceAll(text, "  ", " ")
	text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	text = strings.TrimSpace(text)

	return text
}
