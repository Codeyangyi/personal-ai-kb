package llm

import (
	"context"
)

// LLM 大语言模型接口
type LLM interface {
	Generate(ctx context.Context, prompt string) (string, error)
}
