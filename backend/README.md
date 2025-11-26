# 个人AI知识库 (Personal AI Knowledge Base)

基于Go语言和RAG（检索增强生成）技术构建的个人AI知识库系统。

## 功能特性

- 📄 **多格式文档支持**: 支持TXT、PDF、Word、HTML等格式的文档加载
- 🔍 **智能检索**: 基于向量相似度的语义检索
- 🤖 **AI问答**: 基于知识库的智能问答系统
- 🌐 **Web界面**: 提供现代化的Vue前端界面，支持文档上传和在线问答
- 🔐 **权限控制**: 支持管理员权限，只有授权用户可以上传文档，其他人只能搜索
- 🚀 **本地部署**: 完全本地化运行，保护数据隐私
- 🇨🇳 **国内API支持**: 支持硅基流动等国内服务，无需国外API
- ⚡ **快速向量化**: 使用BAAI/bge-large-zh-v1.5模型，1024维向量，高质量中文语义理解

## 技术栈

- **框架**: [langchaingo](https://github.com/tmc/langchaingo) - Go语言的LangChain实现
- **嵌入模型**: 
  - 默认: BAAI/bge-large-zh-v1.5 (通过硅基流动API，1024维向量)
  - 本地: 支持通过Ollama运行其他模型
- **大语言模型**: Ollama (支持多种开源模型，如qwen2.5、llama3等)
- **向量数据库**: Qdrant
- **前端框架**: Vue 3 + Vite

## 系统架构

```
┌─────────────┐
│  文档加载器  │ → TXT, PDF, Word, HTML
└──────┬──────┘
       │
┌──────▼──────┐
│  文本切分器  │ → 语义完整的文本块
└──────┬──────┘
       │
┌──────▼──────┐
│  向量化模块  │ → BAAI/bge-large-zh-v1.5 (1024维)
└──────┬──────┘
       │
┌──────▼──────┐
│  Qdrant存储  │ → 向量数据库
└──────┬──────┘
       │
┌──────▼──────┐
│   RAG系统   │ → 检索 + 生成
└──────┬──────┘
       │
┌──────▼──────┐
│  Ollama LLM │ → 生成回答
└─────────────┘
```

## 前置要求

### 1. 安装Ollama

访问 [Ollama官网](https://ollama.com/) 下载并安装。

### 2. 拉取模型

```bash
# 拉取大语言模型 (例如: llama3, qwen2.5等)
ollama pull llama3

# 注意：嵌入模型默认使用硅基流动的 BAAI/bge-small-zh-v1.5
# 如需使用本地嵌入模型，请设置 EMBEDDING_PROVIDER=ollama 并拉取相应模型
```

### 3. 安装并启动Qdrant

#### 使用Docker运行Qdrant:

```bash
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
```

#### 或使用本地安装:

访问 [Qdrant官网](https://qdrant.tech/) 获取安装说明。

### 4. 安装Go

确保已安装Go 1.22或更高版本。

## 安装

```bash
# 克隆项目
git clone <repository-url>
cd personal-ai-kb

# 安装依赖
go mod download
```

## 配置

系统通过环境变量进行配置，支持以下环境变量：

```bash
# Ollama配置
export OLLAMA_BASE_URL="http://localhost:11434"
export OLLAMA_MODEL="llama3"

# Qdrant配置
export QDRANT_URL="http://localhost:6333"
export QDRANT_API_KEY=""  # 可选，本地运行通常不需要
export QDRANT_COLLECTION="personal_kb"

# 嵌入模型配置（默认使用硅基流动的 BAAI/bge-large-zh-v1.5）
export EMBEDDING_PROVIDER="siliconflow"  # 默认值，或使用 ollama, zhipu, baidu 等
export EMBEDDING_MODEL="BAAI/bge-large-zh-v1.5"  # 默认值（1024维向量）
export SILICONFLOW_API_KEY="your_api_key"  # 硅基流动API Key（必需）

# 管理员Token（用于Web界面的文档上传权限）
export ADMIN_TOKEN="your_admin_token"  # 可选，默认使用admin123

# 其他国内API服务配置（可选）
# export EMBEDDING_PROVIDER="zhipu"  # 或 baidu, doubao, volcengine, deepseek
# export ZHIPU_API_KEY="your_api_key"  # 智谱AI
# export BAIDU_API_KEY="your_api_key"  # 百度文心
# export BAIDU_SECRET_KEY="your_secret_key"
```

## 国内API服务配置

本项目支持多种国内嵌入向量API服务，无需使用国外服务即可实现快速向量化。

### 支持的Provider

- **智谱AI** (`zhipu`): 速度快，质量高
- **百度文心** (`baidu`): 专业中文模型
- **豆包** (`doubao`): 字节跳动服务
- **火山引擎** (`volcengine`): 字节跳动云服务
- **硅基流动** (`siliconflow`): 支持多种开源模型
- **DeepSeek** (`deepseek`): 高性能嵌入模型
- **本地ONNX** (`local`): 完全离线（开发中）

### 快速开始

```bash
# 使用智谱AI
export EMBEDDING_PROVIDER=zhipu
export ZHIPU_API_KEY=your_api_key
go run main.go -mode=load -file=./document.docx

# 使用百度文心
export EMBEDDING_PROVIDER=baidu
export BAIDU_API_KEY=your_api_key
export BAIDU_SECRET_KEY=your_secret_key
go run main.go -mode=load -file=./document.docx
```

详细配置说明请参考 [CHINESE_APIS.md](./CHINESE_APIS.md)

## 使用方法

### 方式一：Web界面（推荐）

#### 1. 启动API服务器

```bash
# 设置管理员token（可选，默认使用admin123）
export ADMIN_TOKEN="your_admin_token"

# 启动服务器（默认端口8080）
go run main.go -mode=server -port=8080
```

#### 2. 构建并运行前端

```bash
cd frontend

# 安装依赖
npm install

# 开发模式运行（热重载）
npm run dev

# 或构建生产版本
npm run build
```

#### 3. 访问Web界面

- 开发模式：访问 `http://localhost:3000`（前端开发服务器）
- 生产模式：构建后，前端文件会自动由后端服务器提供，访问 `http://localhost:8080`

#### 4. 使用说明

- **管理员登录**：首次访问会提示输入管理员token，输入正确的token后可以上传文档
- **文档上传**：支持单个文件上传和批量上传，支持PDF、Word、TXT格式
- **知识库问答**：所有人（包括未登录用户）都可以使用搜索功能提问

### 方式二：命令行模式

#### 1. 加载文档到知识库

##### 加载单个文件:

```bash
go run main.go -mode=load -file=./documents/example.txt
```

##### 加载网页:

```bash
go run main.go -mode=load -url=https://example.com/article
```

##### 批量加载目录:

```bash
go run main.go -mode=load-dir -file=./documents/
```

#### 2. 查询知识库

##### 交互式查询:

```bash
go run main.go -mode=query
```

##### 单次查询:

```bash
go run main.go -mode=query -question="你的问题"
```

#### 3. 高级选项

```bash
# 自定义检索文档数量
go run main.go -mode=query -question="问题" -topk=5

# 自定义文本块大小
go run main.go -mode=load -file=doc.txt -chunk-size=1500 -overlap=300
```

## 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-mode` | 运行模式: `load`, `query`, `load-dir`, `server` | `query` |
| `-file` | 要加载的文件或目录路径 | - |
| `-url` | 要加载的网页URL | - |
| `-question` | 要查询的问题 | - |
| `-topk` | 检索返回的文档数量 | 3 |
| `-chunk-size` | 文本块大小 | 500（适配bge-large-zh-v1.5） |
| `-overlap` | 文本块重叠大小 | 100 |
| `-port` | API服务器端口（仅用于server模式） | 8080 |

## 工作流程

### RAG (检索增强生成) 流程

1. **文档加载**: 从各种来源加载文档
2. **文本切分**: 将长文档切分成语义完整的文本块
3. **向量化**: 使用嵌入模型将文本转换为向量
4. **存储**: 将向量存储到Qdrant数据库
5. **检索**: 用户提问时，将问题向量化并在数据库中搜索相似文档
6. **生成**: 将检索到的上下文和问题组合，发送给LLM生成回答

## 项目结构

```
personal-ai-kb/
├── main.go           # 主程序入口
├── config/           # 配置管理
│   └── config.go
├── loader/           # 文档加载器
│   └── loader.go
├── splitter/         # 文本切分器
│   └── splitter.go
├── embedding/        # 向量化模块
│   ├── embedding.go
│   ├── siliconflow.go  # 硅基流动API实现
│   └── local.go       # 本地嵌入模型实现
├── store/            # 向量存储
│   └── qdrant.go
├── llm/              # LLM调用
│   └── ollama.go
├── rag/              # RAG核心逻辑
│   └── rag.go
├── api/              # HTTP API服务器
│   └── server.go
├── frontend/         # Vue前端应用
│   ├── src/
│   │   ├── App.vue   # 主组件
│   │   └── main.js   # 入口文件
│   ├── index.html
│   ├── package.json
│   └── vite.config.js
└── README.md
```

## 示例

### 示例1: 加载PDF文档并查询

```bash
# 加载PDF
go run main.go -mode=load -file=./documents/manual.pdf

# 查询
go run main.go -mode=query -question="文档中提到了哪些主要功能？"
```

### 示例2: 批量加载并交互式查询

```bash
# 批量加载
go run main.go -mode=load-dir -file=./documents/

# 交互式查询
go run main.go -mode=query
# 然后输入问题，输入 exit 退出
```

## 注意事项

1. **模型选择**: 
   - 嵌入模型默认使用硅基流动的 BAAI/bge-large-zh-v1.5（1024维向量，需要API Key）
   - 大语言模型使用Ollama，确保已拉取所需模型（如qwen2.5:1.5b、llama3等）
2. **Qdrant运行**: 确保Qdrant服务正在运行
3. **API Key**: 使用硅基流动API服务时需要配置 `SILICONFLOW_API_KEY` 环境变量
4. **管理员Token**: Web界面默认使用 `admin123` 作为管理员token，生产环境建议设置 `ADMIN_TOKEN` 环境变量
5. **内存要求**: 大文档和大量文档可能需要较多内存
6. **首次运行**: 首次向量化可能需要一些时间
7. **前端构建**: 使用Web界面时，需要先构建前端（`cd frontend && npm install && npm run build`）

## 故障排除

### Qdrant连接失败

- 检查Qdrant是否正在运行: `curl http://localhost:6333/health`
- 检查环境变量 `QDRANT_URL` 是否正确

### Ollama连接失败

- 检查Ollama是否正在运行: `ollama list`
- 检查环境变量 `OLLAMA_BASE_URL` 是否正确

### 模型未找到

- 确保已拉取模型: `ollama list`
- 检查环境变量中的模型名称是否正确

## 许可证

MIT License

## 贡献

欢迎提交Issue和Pull Request！

