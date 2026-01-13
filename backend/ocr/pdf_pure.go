package ocr

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"github.com/Codeyangyi/personal-ai-kb/logger"
	"github.com/unidoc/unipdf/v3/model"
	"github.com/unidoc/unipdf/v3/render"
)

// ExtractImagesFromPDF 从PDF中提取图片（纯Go实现）
// 使用 unipdf 纯Go库将PDF页面渲染为图片
func ExtractImagesFromPDF(pdfPath string) ([]string, error) {
	// 打开PDF文件
	reader, file, err := model.NewPdfReaderFromFile(pdfPath, nil)
	if err != nil {
		return nil, fmt.Errorf("无法打开PDF文件: %w", err)
	}
	defer file.Close()

	numPages, err := reader.GetNumPages()
	if err != nil {
		return nil, fmt.Errorf("无法获取PDF页数: %w", err)
	}

	if numPages == 0 {
		return nil, fmt.Errorf("PDF文件没有页面")
	}

	// 创建临时目录存储提取的图片
	tempDir := filepath.Join(filepath.Dir(pdfPath), "temp_images")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}

	var imagePaths []string

	// 创建渲染器
	device := render.NewImageDevice()

	// 提取每一页为图片
	for i := 0; i < numPages; i++ {
		pageNum := i + 1
		logger.Info("正在渲染PDF第%d/%d页为图片...", pageNum, numPages)

		// 获取页面
		page, err := reader.GetPage(pageNum)
		if err != nil {
			logger.Warn("获取PDF第%d页失败: %v", pageNum, err)
			continue
		}

		// 渲染页面为图片
		img, err := device.Render(page)
		if err != nil {
			logger.Warn("渲染PDF第%d页失败: %v", pageNum, err)
			continue
		}

		// 保存图片到临时文件
		imagePath := filepath.Join(tempDir, fmt.Sprintf("page_%d.png", pageNum))
		file, err := os.Create(imagePath)
		if err != nil {
			logger.Warn("创建图片文件失败: %v", err)
			continue
		}

		// 编码为PNG格式
		err = png.Encode(file, img)
		file.Close()
		if err != nil {
			logger.Warn("保存图片失败: %v", err)
			os.Remove(imagePath)
			continue
		}

		imagePaths = append(imagePaths, imagePath)
		logger.Info("✅ PDF第%d页已渲染为图片: %s", pageNum, imagePath)
	}

	if len(imagePaths) == 0 {
		return nil, fmt.Errorf("未能成功渲染任何PDF页面为图片")
	}

	logger.Info("PDF渲染完成，共生成%d张图片", len(imagePaths))
	return imagePaths, nil
}
