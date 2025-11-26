package llm

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

// OllamaLLM Ollama大语言模型客户端
type OllamaLLM struct {
	llm llms.Model
}

// NewOllamaLLM 创建新的Ollama LLM客户端
func NewOllamaLLM(baseURL, modelName string) (*OllamaLLM, error) {
	llm, err := ollama.New(
		ollama.WithModel(modelName),
		ollama.WithServerURL(baseURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama client: %w", err)
	}

	return &OllamaLLM{
		llm: llm,
	}, nil
}

// Generate 生成回答
func (o *OllamaLLM) Generate(ctx context.Context, prompt string) (string, error) {
	// 优化生成参数：平衡响应速度和回答完整性
	completion, err := o.llm.Call(ctx, prompt,
		llms.WithMaxTokens(10000),                  // 增加最大生成token数（1500），获取更完整的回答
		llms.WithTemperature(0.5),                  // 降低温度，减少随机性，加快生成
		llms.WithTopP(0.8),                         // 降低TopP，加快生成
		llms.WithStopWords([]string{"问题:", "回答:"}), // 减少停止词，允许更完整的回答
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate completion: %w", err)
	}
	return completion, nil
}

// GenerateWithOptions 使用选项生成回答
func (o *OllamaLLM) GenerateWithOptions(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	completion, err := o.llm.Call(ctx, prompt, options...)
	if err != nil {
		return "", fmt.Errorf("failed to generate completion: %w", err)
	}
	return completion, nil
}
