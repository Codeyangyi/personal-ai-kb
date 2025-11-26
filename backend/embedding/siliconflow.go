package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SiliconFlowEmbedder 硅基流动嵌入向量生成器
type SiliconFlowEmbedder struct {
	apiKey  string
	baseURL string
	model   string
}

// SiliconFlowEmbeddingRequest 硅基流动API请求格式
type SiliconFlowEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// SiliconFlowEmbeddingResponse 硅基流动API响应格式
type SiliconFlowEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// normalizeModelName 规范化模型名称
// 注意：硅基流动API可能支持带BAAI/前缀的格式，所以保留原始格式
func normalizeModelName(model string) string {
	// 保留原始模型名称，不自动去除前缀
	// 如果用户明确指定了BAAI/前缀，则保留；否则使用不带前缀的格式
	return model
}

// NewSiliconFlowEmbedder 创建硅基流动嵌入向量生成器
func NewSiliconFlowEmbedder(apiKey, model string) (*SiliconFlowEmbedder, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("硅基流动API Key不能为空")
	}
	if model == "" {
		model = "BAAI/bge-large-zh-v1.5" // 默认模型（带前缀）
	}

	// 保留模型名称的原始格式（可能带BAAI/前缀）
	model = normalizeModelName(model)

	return &SiliconFlowEmbedder{
		apiKey:  apiKey,
		baseURL: "https://api.siliconflow.cn/v1", // 硅基流动API地址
		model:   model,
	}, nil
}

// EmbedDocuments 批量向量化文档
func (s *SiliconFlowEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("文本列表不能为空")
	}

	// 构建请求
	reqBody := SiliconFlowEmbeddingRequest{
		Model: s.model,
		Input: texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	// 发送请求
	client := &http.Client{
		Timeout: 60 * time.Second, // 设置超时时间
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		// 检查是否是认证错误
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("API认证失败 (401): API Key 无效或已过期。\n请检查 SILICONFLOW_API_KEY 环境变量是否正确，或访问 https://siliconflow.cn/ 获取新的 API Key。")
		}

		// 检查是否是速率限制错误（429 Too Many Requests）
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("API速率限制: 请求过于频繁，已达到TPM（每分钟token数）限制。\n建议：1) 减少批次大小 2) 增加批次之间的延迟 3) 等待一段时间后重试")
		}

		// 尝试解析错误信息
		var errorResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}

		// 尝试解析错误响应
		if err := json.Unmarshal(body, &errorResp); err == nil {
			errorMsg := errorResp.Message
			if errorMsg == "" && errorResp.Error.Message != "" {
				errorMsg = errorResp.Error.Message
			}

			if errorMsg != "" {
				// 检查是否是速率限制错误
				if strings.Contains(errorMsg, "rate limiting") || 
				   strings.Contains(errorMsg, "rate limit") ||
				   strings.Contains(errorMsg, "TPM limit") ||
				   strings.Contains(errorMsg, "tokens per minute") {
					return nil, fmt.Errorf("API速率限制: %s (错误码: %d)\n建议：1) 减少批次大小 2) 增加批次之间的延迟 3) 等待一段时间后重试", errorMsg, errorResp.Code)
				}
				
				if errorResp.Code == 20012 || strings.Contains(errorMsg, "不存在") || strings.Contains(errorMsg, "does not exist") {
					return nil, fmt.Errorf("模型 '%s' 不存在。\n错误信息: %s (错误码: %d)\n\n请访问 https://siliconflow.cn/ 查看可用的embedding模型列表。", s.model, errorMsg, errorResp.Code)
				}
				return nil, fmt.Errorf("API请求失败: %s (错误码: %d)", errorMsg, errorResp.Code)
			}
		}

		// 如果无法解析，返回原始响应
		return nil, fmt.Errorf("API请求失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var embeddingResp SiliconFlowEmbeddingResponse
	if err := json.Unmarshal(body, &embeddingResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 提取向量
	if len(embeddingResp.Data) != len(texts) {
		return nil, fmt.Errorf("返回的向量数量 (%d) 与输入文本数量 (%d) 不匹配",
			len(embeddingResp.Data), len(texts))
	}

	// 按索引排序（确保顺序正确）
	vectors := make([][]float32, len(texts))
	for _, item := range embeddingResp.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("无效的向量索引: %d", item.Index)
		}
		vectors[item.Index] = item.Embedding
	}

	return vectors, nil
}

// EmbedQuery 向量化查询
func (s *SiliconFlowEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := s.EmbedDocuments(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("未返回向量")
	}
	return vectors[0], nil
}

// GetDimensions 获取向量维度
// 根据模型名称返回正确的维度
func (s *SiliconFlowEmbedder) GetDimensions() int {
	model := strings.ToLower(s.model)
	// 检查模型名称，返回对应的维度
	if strings.Contains(model, "large") {
		return 1024 // BAAI/bge-large-zh-v1.5 是 1024 维
	}
	// BAAI/bge-small-zh-v1.5 和 BAAI/bge-base-zh-v1.5 是 512 维
	return 512
}

// GetModelName 获取模型名称（用于调试和日志）
func (s *SiliconFlowEmbedder) GetModelName() string {
	return s.model
}
