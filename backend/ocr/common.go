package ocr

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// OCR OCR识别接口
type OCR interface {
	// ExtractTextFromPDF 从PDF文件中提取文本（OCR）
	ExtractTextFromPDF(ctx context.Context, pdfPath string) (string, error)
}

// OCRProcessor OCR处理器
type OCRProcessor struct {
	ocr OCR
}

// NewOCRProcessor 创建OCR处理器
func NewOCRProcessor(ocr OCR) *OCRProcessor {
	return &OCRProcessor{
		ocr: ocr,
	}
}

// ProcessPDF 处理PDF文件，提取文本
func (p *OCRProcessor) ProcessPDF(ctx context.Context, pdfPath string) (string, error) {
	if p.ocr == nil {
		return "", fmt.Errorf("OCR未配置，无法处理扫描件")
	}
	return p.ocr.ExtractTextFromPDF(ctx, pdfPath)
}

// CleanupTempImages 清理临时图片文件
func CleanupTempImages(imagePaths []string) {
	for _, path := range imagePaths {
		if err := os.Remove(path); err != nil {
			// 忽略错误
		}
	}
	// 尝试删除临时目录（如果为空）
	if len(imagePaths) > 0 {
		tempDir := filepath.Dir(imagePaths[0])
		if err := os.Remove(tempDir); err != nil {
			// 忽略错误
		}
	}
}

// ReadImageFile 读取图片文件内容
func ReadImageFile(imagePath string) ([]byte, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}
