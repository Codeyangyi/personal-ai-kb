package embedding

import (
	"context"
	"fmt"
	"time"

	"github.com/Codeyangyi/personal-ai-kb/logger"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/ollama"
)

// Embedder 嵌入向量生成器接口
type EmbedderInterface interface {
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	GetDimensions() int
}

// Embedder 嵌入向量生成器（支持多种后端）
type Embedder struct {
	embedder EmbedderInterface
	provider string
}

// NewEmbedder 创建新的嵌入向量生成器
// provider: "ollama" 或 "siliconflow"
// baseURL: Ollama服务器地址（仅用于ollama provider）
// modelName: 模型名称
// apiKey: API密钥（仅用于siliconflow provider）
func NewEmbedder(provider, baseURL, modelName, apiKey string) (*Embedder, error) {
	// 如果没有指定provider，默认使用ollama
	if provider == "" {
		provider = "ollama"
	}

	switch provider {
	case "siliconflow":
		// 使用硅基流动
		if modelName == "" {
			modelName = "BAAI/bge-large-zh-v1.5" // 默认模型（带前缀）
		}
		embedder, err := NewSiliconFlowEmbedder(apiKey, modelName)
		if err != nil {
			return nil, fmt.Errorf("failed to create siliconflow embedder: %w", err)
		}
		return &Embedder{
			embedder: embedder,
			provider: "siliconflow",
		}, nil

	case "ollama":
		fallthrough
	default:
		// 使用Ollama
		llm, err := ollama.New(
			ollama.WithModel(modelName),
			ollama.WithServerURL(baseURL),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create ollama client: %w", err)
		}

		embedder, err := embeddings.NewEmbedder(llm)
		if err != nil {
			return nil, fmt.Errorf("failed to create embedder: %w", err)
		}

		return &Embedder{
			embedder: &OllamaEmbedderWrapper{embedder: embedder},
			provider: "ollama",
		}, nil
	}
}

// OllamaEmbedderWrapper Ollama嵌入器包装
type OllamaEmbedderWrapper struct {
	embedder embeddings.Embedder
}

func (o *OllamaEmbedderWrapper) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return o.embedder.EmbedDocuments(ctx, texts)
}

func (o *OllamaEmbedderWrapper) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return o.embedder.EmbedQuery(ctx, text)
}

func (o *OllamaEmbedderWrapper) GetDimensions() int {
	return 512 // bge-small-zh-v1.5 的维度是 512
}

// EmbedDocuments 将文档转换为向量
func (e *Embedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	// 添加详细日志以便诊断
	logger.Info("    [向量化 %s] 开始处理 %d 个文档...", e.provider, len(texts))
	startTime := time.Now()

	vectors, err := e.embedder.EmbedDocuments(ctx, texts)

	duration := time.Since(startTime)
	if err != nil {
		logger.Error(" ❌ 失败 (耗时: %v)", duration.Round(time.Millisecond))
	} else {
		logger.Info(" ✅ 完成 (耗时: %v, 速度: %.1f 文档/秒)",
			duration.Round(time.Millisecond),
			float64(len(texts))/duration.Seconds())
	}

	if err != nil {
		return nil, fmt.Errorf("failed to embed documents: %w", err)
	}
	return vectors, nil
}

// EmbedQuery 将查询转换为向量
func (e *Embedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return e.embedder.EmbedQuery(ctx, text)
}

// GetDimensions 获取向量维度
func (e *Embedder) GetDimensions() int {
	return e.embedder.GetDimensions()
}

// Embedder 属性访问（用于兼容旧代码）
func (e *Embedder) GetEmbedder() embeddings.Embedder {
	// 如果是Ollama，返回原始embedder
	if wrapper, ok := e.embedder.(*OllamaEmbedderWrapper); ok {
		return wrapper.embedder
	}
	// 其他provider需要适配器
	return &EmbedderAdapter{embedder: e}
}

// EmbedderAdapter 适配器，将我们的接口转换为langchaingo的接口
type EmbedderAdapter struct {
	embedder *Embedder
}

func (e *EmbedderAdapter) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embedder.EmbedDocuments(ctx, texts)
}

func (e *EmbedderAdapter) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return e.embedder.EmbedQuery(ctx, text)
}
