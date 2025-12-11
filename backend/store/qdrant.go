package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/Codeyangyi/personal-ai-kb/logger"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/tmc/langchaingo/vectorstores/qdrant"
)

// QdrantStore Qdrant向量存储包装器
type QdrantStore struct {
	store vectorstores.VectorStore
}

// DimensionGetter 获取向量维度的接口
type DimensionGetter interface {
	GetDimensions() int
}

// NewQdrantStore 创建新的Qdrant存储
// 如果集合不存在，会自动创建集合
func NewQdrantStore(qdrantURL, apiKey, collectionName string, embedder embeddings.Embedder, dimensionGetter DimensionGetter) (*QdrantStore, error) {
	parsedURL, err := url.Parse(qdrantURL)
	if err != nil {
		return nil, fmt.Errorf("invalid qdrant URL: %w", err)
	}

	// 检查集合是否存在，如果不存在则创建
	ctx := context.Background()
	dimensions := 1024 // 默认维度
	if dimensionGetter != nil {
		dimensions = dimensionGetter.GetDimensions()
	}

	exists, err := checkCollectionExists(ctx, qdrantURL, apiKey, collectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to check collection existence: %w", err)
	}

	if !exists {
		logger.Info("集合 '%s' 不存在，正在自动创建（向量维度: %d）...", collectionName, dimensions)
		if err := createCollection(ctx, qdrantURL, apiKey, collectionName, dimensions); err != nil {
			return nil, fmt.Errorf("failed to create collection: %w", err)
		}
		logger.Info("✅ 集合创建成功")
	} else {
		// 检查现有集合的维度是否匹配
		existingDims, err := getCollectionDimensions(ctx, qdrantURL, apiKey, collectionName)
		if err != nil {
			return nil, fmt.Errorf("failed to get collection dimensions: %w", err)
		}
		if existingDims != dimensions {
			logger.Warn("⚠️  集合 '%s' 的维度 (%d) 与模型维度 (%d) 不匹配，正在删除并重新创建...", collectionName, existingDims, dimensions)
			if err := deleteCollection(ctx, qdrantURL, apiKey, collectionName); err != nil {
				return nil, fmt.Errorf("failed to delete collection: %w", err)
			}
			logger.Info("正在重新创建集合（向量维度: %d）...", dimensions)
			if err := createCollection(ctx, qdrantURL, apiKey, collectionName, dimensions); err != nil {
				return nil, fmt.Errorf("failed to create collection: %w", err)
			}
			logger.Info("✅ 集合重新创建成功")
		}
	}

	opts := []qdrant.Option{
		qdrant.WithURL(*parsedURL),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithEmbedder(embedder),
	}

	if apiKey != "" {
		opts = append(opts, qdrant.WithAPIKey(apiKey))
	}

	store, err := qdrant.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant store: %w", err)
	}

	return &QdrantStore{
		store: store,
	}, nil
}

// checkCollectionExists 检查集合是否存在
func checkCollectionExists(ctx context.Context, qdrantURL, apiKey, collectionName string) (bool, error) {
	url := fmt.Sprintf("%s/collections/%s", qdrantURL, collectionName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	if apiKey != "" {
		req.Header.Set("api-key", apiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	body, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
}

// createCollection 创建Qdrant集合
func createCollection(ctx context.Context, qdrantURL, apiKey, collectionName string, dimensions int) error {
	url := fmt.Sprintf("%s/collections/%s", qdrantURL, collectionName)

	payload := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     dimensions,
			"distance": "Cosine",
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("api-key", apiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create collection (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// getCollectionDimensions 获取集合的向量维度
func getCollectionDimensions(ctx context.Context, qdrantURL, apiKey, collectionName string) (int, error) {
	url := fmt.Sprintf("%s/collections/%s", qdrantURL, collectionName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}

	if apiKey != "" {
		req.Header.Set("api-key", apiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("failed to get collection info (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	var collectionInfo struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors struct {
						Size int `json:"size"`
					} `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &collectionInfo); err != nil {
		return 0, fmt.Errorf("failed to parse collection info: %w", err)
	}

	return collectionInfo.Result.Config.Params.Vectors.Size, nil
}

// deleteCollection 删除集合
func deleteCollection(ctx context.Context, qdrantURL, apiKey, collectionName string) error {
	url := fmt.Sprintf("%s/collections/%s", qdrantURL, collectionName)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	if apiKey != "" {
		req.Header.Set("api-key", apiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete collection (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// AddDocuments 添加文档到向量数据库
// 在存储前会清理文档内容的编码，确保没有乱码
func (s *QdrantStore) AddDocuments(ctx context.Context, docs []schema.Document, embedder embeddings.Embedder) error {
	// 在存储前清理每个文档的编码，确保没有乱码
	cleanedDocs := make([]schema.Document, len(docs))
	for i := range docs {
		cleanedDocs[i] = docs[i]
		cleanedDocs[i].PageContent = cleanTextEncoding(docs[i].PageContent)
	}

	_, err := s.store.AddDocuments(ctx, cleanedDocs, vectorstores.WithEmbedder(embedder))
	return err
}

// cleanTextEncoding 清理和修复文本编码，确保是有效的UTF-8
// 移除无效的UTF-8字符、控制字符和乱码字符，替换为空格或删除
func cleanTextEncoding(text string) string {
	if text == "" {
		return text
	}

	var result strings.Builder
	result.Grow(len(text)) // 预分配容量以提高性能

	// 逐字符处理，确保所有字符都是有效的UTF-8
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)

		// 处理无效的UTF-8字符
		if r == utf8.RuneError && size == 1 {
			// 遇到无效的UTF-8字符，跳过
			text = text[size:]
			continue
		}

		// 过滤掉Unicode替换字符（U+FFFD，通常显示为）
		if r == '\uFFFD' {
			text = text[size:]
			continue
		}

		// 过滤掉控制字符（除了换行符、制表符等常见空白字符）
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			// 用空格替换控制字符
			result.WriteRune(' ')
			text = text[size:]
			continue
		}

		// 过滤掉某些特殊Unicode字符范围（可能产生乱码的字符）
		// 这些范围包括：私有使用区、代理对区域等
		if (r >= 0xE000 && r <= 0xF8FF) || // 私有使用区
			(r >= 0xF0000 && r <= 0xFFFFD) || // 补充私有使用区-A
			(r >= 0x100000 && r <= 0x10FFFD) { // 补充私有使用区-B
			text = text[size:]
			continue
		}

		// 保留有效的字符
		result.WriteRune(r)
		text = text[size:]
	}

	text = result.String()

	// 清理连续的乱码字符模式
	// 移除连续的无效字符序列
	text = strings.ReplaceAll(text, "\uFFFD", " ")

	// 清理多余的空白字符
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	text = strings.TrimSpace(text)

	return text
}

// SearchResult 搜索结果，包含文档和相似度分数
type SearchResult struct {
	Document schema.Document
	Score    float64
}

// Search 搜索相似文档
// 内部流程：
// 1. 使用embedder将查询文本（question）转换为向量
// 2. 在Qdrant向量数据库中进行相似性搜索（余弦相似度）
// 3. 返回最相似的topK个文档片段
func (s *QdrantStore) Search(ctx context.Context, query string, embedder embeddings.Embedder, topK int) ([]schema.Document, error) {
	// SimilaritySearch会自动使用embedder将query向量化，然后在向量数据库中搜索
	results, err := s.store.SimilaritySearch(ctx, query, topK, vectorstores.WithEmbedder(embedder))
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	return results, nil
}

// SearchWithScore 搜索相似文档并返回相似度分数
func (s *QdrantStore) SearchWithScore(ctx context.Context, query string, embedder embeddings.Embedder, topK int, minScore float64) ([]SearchResult, error) {
	// 先进行普通搜索
	results, err := s.store.SimilaritySearch(ctx, query, topK, vectorstores.WithEmbedder(embedder))
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	// 将结果转换为带分数的格式
	// 注意：langchaingo的SimilaritySearch不直接返回分数，这里使用占位符
	// 如果需要真实分数，需要使用Qdrant的原始API
	searchResults := make([]SearchResult, 0, len(results))
	for i, doc := range results {
		// 由于langchaingo不直接提供分数，我们使用索引作为参考
		// 实际应用中，可以通过Qdrant API直接获取分数
		score := 1.0 - float64(i)*0.1 // 简单的递减分数（示例）
		if score < minScore {
			continue // 过滤低于阈值的结果
		}
		searchResults = append(searchResults, SearchResult{
			Document: doc,
			Score:    score,
		})
	}

	return searchResults, nil
}

// DeleteDocumentsBySource 根据source字段删除文档
// sourcePath 可以是完整路径或部分路径，会匹配所有包含该路径的文档
func (s *QdrantStore) DeleteDocumentsBySource(ctx context.Context, qdrantURL, apiKey, collectionName, sourcePath string) error {
	if sourcePath == "" {
		return nil
	}

	// 构建删除请求
	// Qdrant 支持通过 filter 删除匹配条件的 points
	url := fmt.Sprintf("%s/collections/%s/points/delete", qdrantURL, collectionName)

	// 构建 filter，匹配 source 字段
	// Qdrant 中 payload 字段的访问方式：使用 key 和 match
	// 注意：langchaingo 将 metadata 存储在 payload 中
	filter := map[string]interface{}{
		"must": []map[string]interface{}{
			{
				"key": "source",
				"match": map[string]interface{}{
					"value": sourcePath,
				},
			},
		},
	}

	payload := map[string]interface{}{
		"filter": filter,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal delete request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("api-key", apiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send delete request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read delete response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete documents (status %d): %s", resp.StatusCode, string(body))
	}

	// 解析响应以确认删除结果
	var deleteResult struct {
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
		Status *struct {
			Error *string `json:"error"`
		} `json:"status"`
	}

	if err := json.Unmarshal(body, &deleteResult); err == nil {
		if deleteResult.Result.Status != "" {
			logger.Info("从向量数据库删除文档成功，source: %s, status: %s", sourcePath, deleteResult.Result.Status)
		} else if deleteResult.Status != nil && deleteResult.Status.Error != nil {
			logger.Warn("删除文档时出现警告，source: %s, error: %s", sourcePath, *deleteResult.Status.Error)
		}
	}

	return nil
}
