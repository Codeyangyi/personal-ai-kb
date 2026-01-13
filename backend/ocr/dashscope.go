package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Codeyangyi/personal-ai-kb/logger"
)

// DashScopeOCR 通义千问OCR实现（纯Go实现）
type DashScopeOCR struct {
	apiKey string
	client *http.Client
}

// NewDashScopeOCR 创建通义千问OCR实例
func NewDashScopeOCR(apiKey string) *DashScopeOCR {
	return &DashScopeOCR{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 5 * time.Minute, // OCR可能需要较长时间
		},
	}
}

// ExtractTextFromPDF 从PDF文件中提取文本（OCR）
// 纯Go实现：将PDF页面转换为图片，然后使用DashScope API进行OCR
func (d *DashScopeOCR) ExtractTextFromPDF(ctx context.Context, pdfPath string) (string, error) {
	// 提取PDF中的图片，然后对每张图片进行OCR
	return d.extractTextFromPDFViaImages(ctx, pdfPath)
}

// extractTextFromPDFViaImages 通过提取图片的方式处理PDF
func (d *DashScopeOCR) extractTextFromPDFViaImages(ctx context.Context, pdfPath string) (string, error) {
	// 使用纯Go PDF库提取图片
	imagePaths, err := ExtractImagesFromPDF(pdfPath)
	if err != nil {
		return "", fmt.Errorf("提取PDF图片失败: %w\n提示：纯Go PDF库可能无法直接渲染页面为图片，建议使用支持页面渲染的库", err)
	}
	defer CleanupTempImages(imagePaths)

	if len(imagePaths) == 0 {
		return "", fmt.Errorf("PDF中未找到图片")
	}

	// 对每张图片进行OCR识别
	var allText strings.Builder
	for i, imagePath := range imagePaths {
		logger.Info("正在对PDF第%d/%d页进行OCR识别...", i+1, len(imagePaths))
		
		text, err := d.recognizeImage(ctx, imagePath)
		if err != nil {
			logger.Warn("第%d页OCR识别失败: %v", i+1, err)
			continue
		}

		if text != "" {
			allText.WriteString(text)
			allText.WriteString("\n\n") // 页面之间添加分隔
		}
	}

	result := strings.TrimSpace(allText.String())
	if result == "" {
		return "", fmt.Errorf("OCR识别未提取到任何文本")
	}

	logger.Info("OCR识别完成，共提取%d页，文本长度: %d字符", len(imagePaths), len(result))
	return result, nil
}

// sendOCRRequest 发送OCR请求
func (d *DashScopeOCR) sendOCRRequest(ctx context.Context, requestBody map[string]interface{}) (string, error) {
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.apiKey))

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OCR API返回错误: %d, %s", resp.StatusCode, string(body))
	}

	// 解析响应 - content 可能是字符串或数组
	var response struct {
		Output struct {
			Choices []struct {
				Message struct {
					Content interface{} `json:"content"` // 可能是字符串或数组
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if response.Code != "" && response.Code != "Success" {
		return "", fmt.Errorf("OCR API错误: %s - %s", response.Code, response.Message)
	}

	if len(response.Output.Choices) > 0 {
		content := response.Output.Choices[0].Message.Content
		
		// 处理 content 可能是字符串或数组的情况
		var text string
		switch v := content.(type) {
		case string:
			text = v
		case []interface{}:
			// content 是数组，提取所有文本内容
			var parts []string
			for _, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if textVal, ok := itemMap["text"].(string); ok {
						parts = append(parts, textVal)
					}
				} else if strVal, ok := item.(string); ok {
					parts = append(parts, strVal)
				}
			}
			text = strings.Join(parts, "\n")
		default:
			// 尝试转换为字符串
			text = fmt.Sprintf("%v", content)
		}
		
		if text == "" {
			return "", fmt.Errorf("OCR响应中文本内容为空")
		}
		
		return text, nil
	}

	return "", fmt.Errorf("OCR响应中未找到文本内容")
}

// recognizeImage 识别单张图片
func (d *DashScopeOCR) recognizeImage(ctx context.Context, imagePath string) (string, error) {
	// 读取图片文件
	imageData, err := ReadImageFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}

	// 将图片编码为base64
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	// 构建请求
	requestBody := map[string]interface{}{
		"model": "qwen-vl-max",
		"input": map[string]interface{}{
			"messages": []map[string]interface{}{
				{
					"role": "user",
					"content": []map[string]interface{}{
						{
							"type": "image",
							"image": fmt.Sprintf("data:image/png;base64,%s", imageBase64),
						},
						{
							"type": "text",
							"text": "请识别这张图片中的所有文字，保持原有的格式和布局。",
						},
					},
				},
			},
		},
		"parameters": map[string]interface{}{
			"max_tokens": 4096,
		},
	}

	return d.sendOCRRequest(ctx, requestBody)
}
