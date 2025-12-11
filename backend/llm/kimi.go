package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Codeyangyi/personal-ai-kb/logger"
)

// KimiLLM Kimi2大语言模型客户端（Moonshot AI）
type KimiLLM struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// KimiRequest 请求结构（兼容OpenAI格式）
type KimiRequest struct {
	Model       string        `json:"model"`
	Messages    []KimiMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
}

// KimiMessage 消息结构
type KimiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// KimiResponse 响应结构（兼容OpenAI格式）
type KimiResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []KimiChoice `json:"choices"`
	Usage   KimiUsage    `json:"usage"`
}

// KimiChoice 选择结构
type KimiChoice struct {
	Index        int         `json:"index"`
	Message      KimiMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// KimiUsage 使用量结构
type KimiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// NewKimiLLM 创建新的Kimi2 LLM客户端
func NewKimiLLM(apiKey, model string) (*KimiLLM, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("MOONSHOT_API_KEY is required")
	}
	if model == "" {
		model = "moonshot-v1-8k" // 默认模型
	}

	// Moonshot AI API URL
	// 注意：Moonshot AI 使用类似 OpenAI 的 API 格式
	return &KimiLLM{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.moonshot.cn/v1/chat/completions",
		client: &http.Client{
			Timeout: 120 * time.Second, // 增加超时时间，因为LLM生成可能需要较长时间
		},
	}, nil
}

// Generate 生成回答
func (k *KimiLLM) Generate(ctx context.Context, prompt string) (string, error) {
	// 构建请求（使用OpenAI兼容格式）
	reqBody := KimiRequest{
		Model: k.model,
		Messages: []KimiMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.7,
		MaxTokens:   2000,
		TopP:        0.8,
	}

	// 调试：记录请求信息（不记录完整prompt，可能很长）
	logger.Debug("[Kimi2] 调用模型: %s, prompt长度: %d 字符\n", k.model, len(prompt))

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", k.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", k.apiKey))

	// 发送请求
	resp, err := k.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		// 尝试解析错误响应
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if error, ok := errorResp["error"].(map[string]interface{}); ok {
				if message, ok := error["message"].(string); ok {
					logger.Debug("[Kimi2] API错误: %s\n", message)
					return "", fmt.Errorf("Kimi2 API错误: %s", message)
				}
			}
		}
		logger.Debug("[Kimi2] HTTP错误 %d: %s\n", resp.StatusCode, string(body))
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var kimiResp KimiResponse
	if err := json.Unmarshal(body, &kimiResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(body))
	}

	// 检查是否有错误
	if len(kimiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response, body: %s", string(body))
	}

	answer := kimiResp.Choices[0].Message.Content
	finishReason := kimiResp.Choices[0].FinishReason

	// 调试：显示LLM响应的详细信息
	logger.Debug("[Kimi2] 收到响应 - 答案长度: %d 字符, 完成原因: %s\n", len(answer), finishReason)
	if kimiResp.Usage.TotalTokens > 0 {
		logger.Debug("[Kimi2] Token使用 - 输入: %d, 输出: %d, 总计: %d\n",
			kimiResp.Usage.PromptTokens, kimiResp.Usage.CompletionTokens, kimiResp.Usage.TotalTokens)
	}

	// 调试：显示答案预览（前300字符）
	if len(answer) > 300 {
		logger.Debug("[Kimi2] 答案预览: %s...\n", answer[:300])
	} else {
		logger.Debug("[Kimi2] 完整答案: %s\n", answer)
	}

	return answer, nil
}
