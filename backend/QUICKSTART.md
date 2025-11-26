# 快速开始指南

本指南将帮助你快速搭建和使用个人AI知识库系统。

## 系统要求

- Go 1.22 或更高版本
- Docker（用于运行Qdrant）
- Ollama（用于运行大语言模型）
- 硅基流动API Key（用于嵌入向量模型，免费注册：https://siliconflow.cn/）

## 快速安装

### 1. 安装依赖

```bash
# 安装Go依赖
go mod download
```

### 2. 启动Qdrant向量数据库

```bash
# 使用Docker运行Qdrant
docker run -d -p 6333:6333 -p 6334:6334 --name qdrant qdrant/qdrant
```

### 3. 安装并启动Ollama

访问 [Ollama官网](https://ollama.com/) 下载并安装。

```bash
# 拉取一个大语言模型（用于生成回答）
ollama pull llama3
# 或使用中文模型
ollama pull qwen2.5
```

### 4. 配置环境变量

创建 `.env` 文件或直接导出环境变量：

```bash
# Ollama配置
export OLLAMA_BASE_URL="http://localhost:11434"
export OLLAMA_MODEL="llama3"  # 或 qwen2.5

# Qdrant配置
export QDRANT_URL="http://localhost:6333"
export QDRANT_COLLECTION="personal_kb"

# 嵌入模型配置（使用硅基流动的 BAAI/bge-small-zh-v1.5）
export EMBEDDING_PROVIDER="siliconflow"
export EMBEDDING_MODEL="BAAI/bge-small-zh-v1.5"
export SILICONFLOW_API_KEY="your_api_key_here"  # 从 https://siliconflow.cn/ 获取
```

## 使用示例

### 1. 加载文档到知识库

#### 加载单个文件

```bash
# 加载TXT文件
go run main.go -mode=load -file=./documents/example.txt

# 加载PDF文件
go run main.go -mode=load -file=./documents/manual.pdf

# 加载Word文档
go run main.go -mode=load -file=./documents/report.docx

# 加载HTML文件
go run main.go -mode=load -file=./documents/page.html
```

#### 从网页加载

```bash
go run main.go -mode=load -url=https://example.com/article
```

#### 批量加载目录

```bash
# 加载整个目录中的所有文档
go run main.go -mode=load-dir -file=./documents/

# 使用快速模式（更大的文本块，减少向量化次数）
go run main.go -mode=load-dir -file=./documents/ -fast

# 使用极速模式（超大文本块，最快速度）
go run main.go -mode=load-dir -file=./documents/ -ultra-fast
```

### 2. 查询知识库

#### 交互式查询

```bash
go run main.go -mode=query
```

然后输入问题，输入 `exit` 或 `quit` 退出。

#### 单次查询

```bash
go run main.go -mode=query -question="你的问题是什么？"
```

#### 自定义检索数量

```bash
# 检索前5个最相关的文档片段
go run main.go -mode=query -question="问题" -topk=5
```

## 系统工作流程

### RAG（检索增强生成）流程

1. **文档加载** (`loader/loader.go`)
   - 支持TXT、PDF、Word、HTML等格式
   - 使用 langchaingo 的 documentloaders

2. **文本切分** (`splitter/splitter.go`)
   - 使用 langchaingo 的 textsplitters
   - 将长文档切分成语义完整的文本块
   - 支持自定义块大小和重叠大小

3. **向量化** (`embedding/embedding.go`)
   - 使用 BAAI/bge-small-zh-v1.5 模型
   - 通过硅基流动API进行向量化
   - 支持批量处理，提高效率

4. **存储** (`store/qdrant.go`)
   - 将向量存储到Qdrant向量数据库
   - 自动创建集合（如果不存在）
   - 自动检查并匹配向量维度

5. **检索** (`rag/rag.go`)
   - 用户提问时，将问题向量化
   - 在Qdrant中进行相似性搜索
   - 返回最相关的文档片段

6. **生成** (`llm/ollama.go`)
   - 将检索到的上下文和问题组合
   - 发送给Ollama的LLM生成回答
   - 返回基于知识库的精准答案

## 技术架构

```
┌─────────────┐
│  文档加载器  │ → langchaingo documentloaders
│  (loader)   │   (TXT, PDF, Word, HTML)
└──────┬──────┘
       │
┌──────▼──────┐
│  文本切分器  │ → langchaingo textsplitters
│  (splitter) │   (RecursiveCharacter)
└──────┬──────┘
       │
┌──────▼──────┐
│  向量化模块  │ → BAAI/bge-small-zh-v1.5
│ (embedding) │   (硅基流动API)
└──────┬──────┘
       │
┌──────▼──────┐
│  Qdrant存储  │ → 向量数据库
│   (store)   │   (本地或云端)
└──────┬──────┘
       │
┌──────▼──────┐
│   RAG系统   │ → 检索 + 生成
│    (rag)    │
└──────┬──────┘
       │
┌──────▼──────┐
│  Ollama LLM │ → 生成回答
│    (llm)    │   (llama3, qwen2.5等)
└─────────────┘
```

## 命令行参数

| 参数 | 说明 | 默认值 | 示例 |
|------|------|--------|------|
| `-mode` | 运行模式 | `query` | `load`, `query`, `load-dir` |
| `-file` | 文件或目录路径 | - | `./doc.txt` |
| `-url` | 网页URL | - | `https://example.com` |
| `-question` | 查询问题 | - | `"什么是RAG？"` |
| `-topk` | 检索文档数量 | `3` | `5` |
| `-chunk-size` | 文本块大小 | `1000` | `1500` |
| `-overlap` | 文本块重叠 | `200` | `300` |
| `-fast` | 快速模式 | `false` | 使用3000字符块 |
| `-ultra-fast` | 极速模式 | `false` | 使用10000字符块 |

## 常见问题

### Q: 如何获取硅基流动API Key？

A: 访问 https://siliconflow.cn/ 注册账号，在控制台获取API Key。

### Q: 支持哪些文档格式？

A: 支持TXT、PDF、Word (.docx)、HTML等格式。旧版Word (.doc) 需要转换为 .docx。

### Q: 如何切换嵌入模型？

A: 修改环境变量 `EMBEDDING_MODEL`，例如：
```bash
export EMBEDDING_MODEL="BAAI/bge-large-zh-v1.5"  # 使用更大的模型
```

### Q: 如何切换大语言模型？

A: 修改环境变量 `OLLAMA_MODEL`，例如：
```bash
export OLLAMA_MODEL="qwen2.5"  # 使用中文模型
```

### Q: 向量化速度慢怎么办？

A: 
1. 使用 `-fast` 或 `-ultra-fast` 模式增加文本块大小
2. 确保使用硅基流动等API服务（比本地Ollama快很多）
3. 检查网络连接

### Q: Qdrant连接失败？

A: 
1. 检查Qdrant是否运行：`docker ps | grep qdrant`
2. 检查端口是否正确：`curl http://localhost:6333/health`
3. 检查环境变量 `QDRANT_URL`

## 下一步

- 查看 [README.md](./README.md) 了解详细功能
- 查看 [CHINESE_APIS.md](./CHINESE_APIS.md) 了解其他API服务配置
- 开始加载你的文档并构建个人知识库！

