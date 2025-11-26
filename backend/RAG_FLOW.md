# RAG（检索增强生成）工作流程说明

## 系统工作流程

本系统完全按照标准的RAG流程实现，具体步骤如下：

### 步骤1: 将问题转换为向量

**位置**: `rag/rag.go` → `Query()` → `store.Search()`

**实现**:
- 用户提出问题（文本）
- 调用 `r.store.Search(ctx, question, r.embedder.GetEmbedder(), r.topK)`
- `Search` 方法内部使用 `SimilaritySearch`，会自动调用 `embedder` 将问题文本转换为向量
- 使用的嵌入模型：**BAAI/bge-large-zh-v1.5**（通过硅基流动API）
- 向量维度：**1024维**

**代码位置**:
```go
// store/qdrant.go
func (s *QdrantStore) Search(ctx context.Context, query string, embedder embeddings.Embedder, topK int) ([]schema.Document, error) {
    // SimilaritySearch会自动使用embedder将query向量化，然后在向量数据库中搜索
    results, err := s.store.SimilaritySearch(ctx, query, topK, vectorstores.WithEmbedder(embedder))
    ...
}
```

### 步骤2: 在向量数据库中进行相似性搜索

**位置**: `store/qdrant.go` → `Search()` → `SimilaritySearch()`

**实现**:
- 使用转换后的查询向量在 **Qdrant** 向量数据库中进行相似性搜索
- 搜索算法：**余弦相似度（Cosine Similarity）**
- 返回最相关的 `topK` 个文本块（默认3个）
- 这些文本块是从之前上传的文档中切分出来的

**代码位置**:
```go
// rag/rag.go
results, err := r.store.Search(ctx, question, r.embedder.GetEmbedder(), r.topK)
// 返回最相关的topK个文档片段
```

### 步骤3: 构建增强提示

**位置**: `rag/rag.go` → `buildPrompt()`

**实现**:
- 将"原始问题" + "检索到的上下文"组合成一个增强的提示
- 提示格式：
  ```
  你是一个专业的AI助手。请基于以下上下文信息回答问题。
  
  上下文信息：
  [文档片段 1]
  ...检索到的文本内容...
  
  [文档片段 2]
  ...检索到的文本内容...
  
  问题: [用户的原始问题]
  
  请基于上述上下文信息回答问题：
  ```
- 每个文档片段最多800字符，避免提示词过长

**代码位置**:
```go
// rag/rag.go
prompt := r.buildPrompt(question, results)
```

### 步骤4: 发送给LLM生成答案

**位置**: `rag/rag.go` → `Query()` → `llm.Generate()`

**实现**:
- 将增强提示发送给本地运行的 **Ollama** LLM
- 使用配置的模型（默认：qwen2.5:1.5b）
- LLM基于上下文信息生成精准、基于知识库的答案
- 生成参数：
  - MaxTokens: 10000
  - Temperature: 0.5（降低随机性，提高准确性）
  - TopP: 0.8

**代码位置**:
```go
// rag/rag.go
answer, err := r.llm.Generate(llmCtx, prompt)
```

## 完整流程图

```
用户提问
    ↓
[步骤1] 问题向量化
    ↓ (使用 BAAI/bge-large-zh-v1.5)
查询向量 (1024维)
    ↓
[步骤2] 向量数据库相似性搜索
    ↓ (在 Qdrant 中搜索)
检索到最相关的文本块 (topK个)
    ↓
[步骤3] 构建增强提示
    ↓ (原始问题 + 检索到的上下文)
增强提示 (Prompt)
    ↓
[步骤4] 发送给 LLM
    ↓ (通过 Ollama)
生成精准、基于知识库的答案
    ↓
返回给用户
```

## 技术细节

### 嵌入模型
- **模型**: BAAI/bge-large-zh-v1.5
- **提供商**: 硅基流动API
- **向量维度**: 1024
- **特点**: 专门优化中文语义理解

### 向量数据库
- **数据库**: Qdrant
- **相似度算法**: 余弦相似度（Cosine Similarity）
- **搜索方式**: 向量相似性搜索

### 大语言模型
- **框架**: Ollama
- **默认模型**: qwen2.5:1.5b
- **特点**: 本地运行，保护隐私

## 优势

1. **准确性**: 基于知识库内容回答，不编造信息
2. **相关性**: 通过向量相似度找到最相关的上下文
3. **可追溯**: 可以查看答案来源的文档片段
4. **隐私**: 完全本地运行，数据不出本地

## 性能优化

- 向量检索耗时：通常 < 100ms
- LLM生成耗时：取决于模型大小和问题复杂度
- 总耗时：通常 < 5秒（取决于LLM模型）

