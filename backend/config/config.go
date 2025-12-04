package config

import (
	"fmt"
	"os"
)

// Config 系统配置
type Config struct {
	// LLM配置（支持Ollama、通义千问和Kimi2）
	LLMProvider     string // "ollama"、"dashscope" 或 "kimi"
	OllamaBaseURL   string
	OllamaModel     string
	DashScopeAPIKey string // 通义千问API Key
	DashScopeModel  string // 通义千问模型名称
	MoonshotAPIKey  string // Kimi2 (Moonshot AI) API Key
	MoonshotModel   string // Kimi2 模型名称

	// Qdrant配置
	QdrantURL      string
	QdrantAPIKey   string
	CollectionName string

	// 嵌入模型配置
	EmbeddingProvider  string // "ollama" 或 "siliconflow"
	EmbeddingModelName string
	EmbeddingModelURL  string
	SiliconFlowAPIKey  string // 硅基流动API Key

	// 文本切分配置
	ChunkSize    int
	ChunkOverlap int

	// 服务器配置
	ServerMode string // 默认运行模式: "server", "query", "load", "load-dir"
	ServerPort string // 服务器端口

	// MySQL 配置（用于意见反馈等业务数据存储）
	MySQLDSN string // 例如: user:password@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=true&loc=Local
}

// LoadConfig 从环境变量加载配置
func LoadConfig() *Config {
	return &Config{
		// LLM配置（默认使用通义千问）
		LLMProvider:   getEnv("LLM_PROVIDER", "dashscope"), // 默认使用通义千问，可选: "ollama", "dashscope", "kimi"
		OllamaBaseURL: getEnv("OLLAMA_BASE_URL", "http://localhost:11434"),
		OllamaModel:   getEnv("OLLAMA_MODEL", "qwen2.5:1.5b"),
		//DashScopeAPIKey: getEnv("DASHSCOPE_API_KEY", "sk-cde2d6e9f3a84e17a8e58ef474b8ce7c"),                // 通义千问API Key
		DashScopeAPIKey: getEnv("DASHSCOPE_API_KEY", "sk-737971a5532646ea932b4be190fa1325"),                // 通义千问API Key
		DashScopeModel:  getEnv("DASHSCOPE_MODEL", "qwen-turbo"),                                           // 默认使用qwen-turbo
		MoonshotAPIKey:  getEnv("MOONSHOT_API_KEY", "sk-xvtLcD5Gvzq8vxCOeEo8pEqMeqss8T8jIBx0Xdr8BcgX6aog"), // Kimi2 (Moonshot AI) API Key
		MoonshotModel:   getEnv("MOONSHOT_MODEL", "moonshot-v1-8k"),                                        // 默认使用moonshot-v1-8k

		QdrantURL:      getEnv("QDRANT_URL", "http://localhost:6333"),
		QdrantAPIKey:   getEnv("QDRANT_API_KEY", ""),
		CollectionName: getEnv("QDRANT_COLLECTION", "personal_kb"),

		// 嵌入模型配置
		// 支持 provider: "ollama" 或 "siliconflow"
		// 使用硅基流动时，设置 EMBEDDING_PROVIDER=siliconflow 和 SILICONFLOW_API_KEY
		// 注意：硅基流动的模型名称格式可能不同，请访问 https://siliconflow.cn/ 查看可用模型
		// 常见模型名称：BAAI/bge-large-zh-v1.5, BAAI/bge-base-zh-v1.5, BAAI/bge-small-zh-v1.5 等
		// 注意：bge-large-zh-v1.5 的向量维度是 1024，bge-small-zh-v1.5 是 512
		EmbeddingProvider:  getEnv("EMBEDDING_PROVIDER", "siliconflow"),         // 默认使用硅基流动
		EmbeddingModelName: getEnv("EMBEDDING_MODEL", "BAAI/bge-large-zh-v1.5"), // 默认使用BAAI/bge-large-zh-v1.5（带前缀）
		EmbeddingModelURL:  getEnv("EMBEDDING_MODEL_URL", ""),
		SiliconFlowAPIKey:  getEnv("SILICONFLOW_API_KEY", "sk-nbgejyepvdcheitaxawefhnyorxzkyphxwzndxfamgfkhwdb"),

		// 注意：BAAI/bge-large-zh-v1.5 有512 tokens的限制，建议使用较小的chunk-size
		ChunkSize:    500, // 默认500字符，适合BAAI/bge-large-zh-v1.5的token限制
		ChunkOverlap: 100, // 默认100字符重叠

		// 服务器配置（默认启动服务器模式）
		ServerMode: getEnv("SERVER_MODE", "server"), // 默认模式: server（启动API服务器）
		ServerPort: getEnv("SERVER_PORT", "8005"),   // 默认端口: 8005

		// MySQL 配置（可选，如果不配置则不启用数据库相关功能）
		MySQLDSN: getEnv("MYSQL_DSN", "root:123456@tcp(127.0.0.1:3306)/ai_kb?charset=utf8mb4"),
		//MySQLDSN: getEnv("MYSQL_DSN", "personal-ai-kb:6mcETznRjwdmK7XN@tcp(127.0.0.1:3306)/ai_kb?charset=utf8mb4"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证LLM配置
	if c.LLMProvider == "ollama" {
		if c.OllamaBaseURL == "" {
			return fmt.Errorf("使用Ollama时需要设置 OLLAMA_BASE_URL 环境变量")
		}
		if c.OllamaModel == "" {
			return fmt.Errorf("使用Ollama时需要设置 OLLAMA_MODEL 环境变量")
		}
	} else if c.LLMProvider == "dashscope" {
		if c.DashScopeAPIKey == "" {
			return fmt.Errorf("使用通义千问时需要设置 DASHSCOPE_API_KEY 环境变量")
		}
	} else if c.LLMProvider == "kimi" {
		if c.MoonshotAPIKey == "" {
			return fmt.Errorf("使用Kimi2时需要设置 MOONSHOT_API_KEY 环境变量")
		}
	} else {
		return fmt.Errorf("不支持的LLM Provider: %s，支持的值: ollama, dashscope, kimi", c.LLMProvider)
	}

	if c.QdrantURL == "" {
		return fmt.Errorf("QDRANT_URL is required")
	}
	if c.CollectionName == "" {
		return fmt.Errorf("QDRANT_COLLECTION is required")
	}
	// 如果使用硅基流动，需要API Key
	if c.EmbeddingProvider == "siliconflow" && c.SiliconFlowAPIKey == "" {
		return fmt.Errorf("使用硅基流动时需要设置 SILICONFLOW_API_KEY 环境变量")
	}
	return nil
}
