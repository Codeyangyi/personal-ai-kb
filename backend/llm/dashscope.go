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

// DashScopeLLM 通义千问大语言模型客户端
type DashScopeLLM struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// DashScopeRequest 请求结构
type DashScopeRequest struct {
	Model      string              `json:"model"`
	Input      DashScopeInput      `json:"input"`
	Parameters DashScopeParameters `json:"parameters"`
}

// DashScopeInput 输入结构
type DashScopeInput struct {
	Messages []DashScopeMessage `json:"messages"`
}

// DashScopeMessage 消息结构
type DashScopeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DashScopeParameters 参数结构
type DashScopeParameters struct {
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

// DashScopeResponse 响应结构
type DashScopeResponse struct {
	Output DashScopeOutput `json:"output"`
	Usage  DashScopeUsage  `json:"usage"`
}

// DashScopeOutput 输出结构
// 注意：DashScope API 可能返回两种格式：
// 1. 包含 choices 数组的格式（某些模型）
// 2. 直接包含 text 字段的格式（text-generation API）
type DashScopeOutput struct {
	// 格式1：choices 数组
	Choices []DashScopeChoice `json:"choices,omitempty"`
	// 格式2：直接文本（text-generation API）
	Text         string `json:"text,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// DashScopeChoice 选择结构
type DashScopeChoice struct {
	Message DashScopeMessage `json:"message"`
}

// DashScopeUsage 使用量结构
type DashScopeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// NewDashScopeLLM 创建新的通义千问LLM客户端
func NewDashScopeLLM(apiKey, model string) (*DashScopeLLM, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("DASHSCOPE_API_KEY is required")
	}
	if model == "" {
		model = "qwen-turbo" // 默认模型
	}

	// DashScope API URL格式：https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation
	// 注意：模型名称在请求体中指定，不在URL中
	return &DashScopeLLM{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation",
		client: &http.Client{
			Timeout: 120 * time.Second, // 增加超时时间，因为LLM生成可能需要较长时间
		},
	}, nil
}

// Generate 生成回答
func (d *DashScopeLLM) Generate(ctx context.Context, prompt string) (string, error) {
	// 构建请求（使用DashScope API的正确格式）
	// DashScope API格式：https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation
	reqBody := map[string]interface{}{
		"model": d.model,
		"input": map[string]interface{}{
			"messages": []map[string]interface{}{
				{
					"role":    "user",
					"content": prompt,
				},
			},
		},
		"parameters": map[string]interface{}{
			"max_tokens":  2000,
			"temperature": 0.7,
			"top_p":       0.8,
		},
	}

	// 调试：记录请求信息（不记录完整prompt，可能很长）
	logger.Debug("[DashScope] 调用模型: %s, prompt长度: %d 字符\n", d.model, len(prompt))

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", d.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.apiKey))

	// 发送请求
	resp, err := d.client.Do(req)
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
			if code, ok := errorResp["code"].(string); ok {
				if message, ok := errorResp["message"].(string); ok {
					logger.Debug("[DashScope] API错误 [%s]: %s\n", code, message)
					return "", fmt.Errorf("DashScope API错误 [%s]: %s", code, message)
				}
			}
		}
		logger.Debug("[DashScope] HTTP错误 %d: %s\n", resp.StatusCode, string(body))
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var dashScopeResp DashScopeResponse
	if err := json.Unmarshal(body, &dashScopeResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(body))
	}

	// 支持两种响应格式：
	var answer string
	var finishReason string

	// 1. 如果 output 包含 text 字段（text-generation API 格式）
	if dashScopeResp.Output.Text != "" {
		answer = dashScopeResp.Output.Text
		finishReason = dashScopeResp.Output.FinishReason
	} else if len(dashScopeResp.Output.Choices) > 0 {
		// 2. 如果 output 包含 choices 数组（某些模型的格式）
		answer = dashScopeResp.Output.Choices[0].Message.Content
		finishReason = dashScopeResp.Output.FinishReason
	} else {
		// 如果两种格式都没有，返回错误
		return "", fmt.Errorf("no text or choices in response, body: %s", string(body))
	}

	// 调试：显示LLM响应的详细信息
	logger.Debug("[DashScope] 收到响应 - 答案长度: %d 字符, 完成原因: %s\n", len(answer), finishReason)
	if dashScopeResp.Usage.InputTokens > 0 || dashScopeResp.Usage.OutputTokens > 0 {
		totalTokens := dashScopeResp.Usage.InputTokens + dashScopeResp.Usage.OutputTokens
		logger.Debug("[DashScope] Token使用 - 输入: %d, 输出: %d, 总计: %d\n",
			dashScopeResp.Usage.InputTokens, dashScopeResp.Usage.OutputTokens, totalTokens)
	}

	// 调试：显示答案预览（前300字符）
	if len(answer) > 300 {
		logger.Debug("[DashScope] 答案预览: %s...\n", answer[:300])
	} else {
		logger.Debug("[DashScope] 完整答案: %s\n", answer)
	}

	return answer, nil
}
