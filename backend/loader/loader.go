package loader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/nguyenthenguyen/docx"
	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/schema"
)

// DocumentLoader 文档加载器接口
type DocumentLoader interface {
	Load(path string) ([]schema.Document, error)
}

// FileLoader 文件加载器
type FileLoader struct{}

// NewFileLoader 创建新的文件加载器
func NewFileLoader() *FileLoader {
	return &FileLoader{}
}

// Load 根据文件类型加载文档
func (l *FileLoader) Load(path string) ([]schema.Document, error) {
	ext := strings.ToLower(filepath.Ext(path))

	var loader documentloaders.Loader
	var err error

	switch ext {
	case ".txt":
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()
		loader = documentloaders.NewText(file)

	case ".pdf":
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()

		// 验证PDF文件格式（检查文件头）
		header := make([]byte, 4)
		if _, err := file.ReadAt(header, 0); err != nil {
			return nil, fmt.Errorf("failed to read PDF file header: %w", err)
		}
		// PDF文件应该以 %PDF 开头
		if string(header) != "%PDF" {
			return nil, fmt.Errorf("invalid PDF file format: file does not start with %%PDF signature")
		}

		fileInfo, err := file.Stat()
		if err != nil {
			return nil, fmt.Errorf("failed to get file info: %w", err)
		}

		// 检查文件大小（避免处理空文件或异常大的文件）
		if fileInfo.Size() == 0 {
			return nil, fmt.Errorf("PDF file is empty")
		}
		if fileInfo.Size() > 100*1024*1024 { // 100MB
			return nil, fmt.Errorf("PDF file is too large (max 100MB)")
		}

		// 重新定位到文件开头
		if _, err := file.Seek(0, 0); err != nil {
			return nil, fmt.Errorf("failed to seek to file start: %w", err)
		}

		loader = documentloaders.NewPDF(file, fileInfo.Size())

	case ".docx":
		// 使用docx库加载Word文档
		doc, err := docx.ReadDocxFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read docx file: %w", err)
		}
		defer doc.Close()

		// 提取所有文本内容（使用纯文本提取，去除XML格式）
		editable := doc.Editable()
		text := cleanWordText(editable.GetContent())

		// 清理和修复文本编码
		text = cleanTextEncoding(text)

		// 创建文档对象
		documents := []schema.Document{
			{
				PageContent: text,
				Metadata: map[string]interface{}{
					"source":    path,
					"file_name": filepath.Base(path),
					"file_type": "docx",
				},
			},
		}

		return documents, nil

	case ".doc":
		// .doc格式（旧版Word）暂不支持，建议转换为.docx
		return nil, fmt.Errorf("旧版Word文档(.doc)暂不支持，请转换为.docx格式")

	case ".html", ".htm":
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()
		loader = documentloaders.NewHTML(file)

	default:
		// 尝试作为文本文件加载
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("unsupported file type: %s", ext)
		}
		defer file.Close()
		loader = documentloaders.NewText(file)
	}

	ctx := context.Background()
	docs, err := loader.Load(ctx)
	if err != nil {
		// 提供更详细的错误信息
		if ext == ".pdf" {
			// PDF特定的错误处理
			errMsg := err.Error()
			if strings.Contains(errMsg, "encrypted") || strings.Contains(errMsg, "password") {
				return nil, fmt.Errorf("PDF文件已加密或受密码保护，无法读取。请先移除密码保护后再上传: %w", err)
			}
			if strings.Contains(errMsg, "corrupt") || strings.Contains(errMsg, "invalid") {
				return nil, fmt.Errorf("PDF文件可能已损坏或格式不正确。请尝试用PDF阅读器打开并重新保存: %w", err)
			}
			if strings.Contains(errMsg, "EOF") || strings.Contains(errMsg, "unexpected") {
				return nil, fmt.Errorf("PDF文件解析失败，可能是扫描版PDF（图片格式）或格式不标准。请尝试使用OCR工具提取文本: %w", err)
			}
			return nil, fmt.Errorf("加载PDF文件失败: %w。可能的原因：1) PDF文件已加密 2) PDF文件损坏 3) 扫描版PDF（无文本层）4) 格式不标准", err)
		}
		return nil, fmt.Errorf("failed to load documents: %w", err)
	}

	// 检查PDF是否成功提取到内容
	if ext == ".pdf" && len(docs) == 0 {
		return nil, fmt.Errorf("PDF文件加载成功但未提取到任何文本内容。可能是扫描版PDF（纯图片），请使用OCR工具提取文本后再上传")
	}

	// 添加文件路径作为元数据，并清理文本编码
	for i := range docs {
		if docs[i].Metadata == nil {
			docs[i].Metadata = make(map[string]interface{})
		}
		docs[i].Metadata["source"] = path
		docs[i].Metadata["file_name"] = filepath.Base(path)
		
		// 清理和修复文本编码，确保是有效的UTF-8
		docs[i].PageContent = cleanTextEncoding(docs[i].PageContent)
	}

	return docs, nil
}

// cleanWordText 清理Word文档文本，去除XML标签和格式标记
func cleanWordText(text string) string {
	// 提取 <w:t> 标签内的文本内容
	re := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
	matches := re.FindAllStringSubmatch(text, -1)

	var result strings.Builder
	for _, match := range matches {
		if len(match) > 1 && match[1] != "" {
			result.WriteString(match[1])
			result.WriteString(" ")
		}
	}

	cleaned := result.String()

	// 清理多余的空白字符
	cleaned = strings.ReplaceAll(cleaned, "  ", " ")
	cleaned = strings.ReplaceAll(cleaned, "\n\n", "\n")
	cleaned = strings.TrimSpace(cleaned)

	// 清理和修复文本编码
	cleaned = cleanTextEncoding(cleaned)

	return cleaned
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

	// 清理连续的乱码字符模式（如连续的替换字符或控制字符）
	// 移除连续的无效字符序列
	text = strings.ReplaceAll(text, "\uFFFD", " ")
	
	// 清理多余的空白字符
	// 多个空格/制表符替换为单个空格
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	// 多个换行符替换为两个
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	text = strings.TrimSpace(text)

	return text
}

// LoadFromURL 从URL加载网页内容
func LoadFromURL(url string) ([]schema.Document, error) {
	// 下载网页内容
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch URL: status code %d", resp.StatusCode)
	}

	// 使用HTML loader加载
	loader := documentloaders.NewHTML(resp.Body)
	ctx := context.Background()
	docs, err := loader.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load from URL: %w", err)
	}

	// 添加URL作为元数据
	for i := range docs {
		if docs[i].Metadata == nil {
			docs[i].Metadata = make(map[string]interface{})
		}
		docs[i].Metadata["source"] = url
		docs[i].Metadata["source_type"] = "url"
	}

	return docs, nil
}

// LoadFromReader 从io.Reader加载内容
func LoadFromReader(reader io.Reader, fileType string) ([]schema.Document, error) {
	var loader documentloaders.Loader

	switch strings.ToLower(fileType) {
	case "txt", "text":
		loader = documentloaders.NewText(reader)
	case "html", "htm":
		loader = documentloaders.NewHTML(reader)
	default:
		loader = documentloaders.NewText(reader)
	}

	ctx := context.Background()
	docs, err := loader.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load from reader: %w", err)
	}

	return docs, nil
}
