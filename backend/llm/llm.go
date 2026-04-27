package llm

import (
	"context"
)

// LLM 大语言模型接口
type LLM interface {
	Generate(ctx context.Context, prompt string) (string, error)
	GenerateStream(ctx context.Context, prompt string, onChunk func(string) error) (string, error)
}
