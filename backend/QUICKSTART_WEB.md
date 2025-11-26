# Web界面快速启动指南

## 系统要求

1. **Go 1.22+**: 后端运行环境
2. **Node.js 16+**: 前端构建环境
3. **Ollama**: 大语言模型服务
4. **Qdrant**: 向量数据库
5. **硅基流动API Key**: 用于嵌入向量生成

## 完整启动步骤

### 1. 启动Qdrant

```bash
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
```

### 2. 启动Ollama

确保Ollama服务正在运行：

```bash
ollama serve
```

在另一个终端拉取模型：

```bash
ollama pull qwen2.5:1.5b
```

### 3. 配置环境变量

```bash
# 必需：硅基流动API Key
export SILICONFLOW_API_KEY="your_siliconflow_api_key"

# 可选：管理员Token（默认使用admin123）
export ADMIN_TOKEN="your_admin_token"

# 可选：Ollama配置（默认值已足够）
export OLLAMA_BASE_URL="http://localhost:11434"
export OLLAMA_MODEL="qwen2.5:1.5b"

# 可选：Qdrant配置（默认值已足够）
export QDRANT_URL="http://localhost:6333"
export QDRANT_COLLECTION="personal_kb"
```

### 4. 构建前端

```bash
cd frontend

# 首先安装依赖（必需！）
npm install

# 如果遇到 "vite: command not found" 错误，说明依赖未安装，先运行上面的 npm install

# 然后构建
npm run build

cd ..
```

**注意**：如果遇到 `vite: command not found` 错误，请确保：
1. 已经运行了 `npm install` 安装依赖
2. 在 `frontend` 目录下运行命令
3. 或者使用 `npx vite build` 代替 `npm run build`

### 5. 启动后端服务器

```bash
go run main.go -mode=server -port=8080
```

### 6. 访问Web界面

打开浏览器访问：`http://localhost:8080`

## 使用流程

### 首次使用

1. **管理员登录**：
   - 首次访问会弹出登录对话框
   - 输入管理员token（默认：`admin123`）
   - 点击"登录"按钮
   - 或者点击"跳过（仅搜索）"以普通用户身份使用

2. **上传文档**（仅管理员）：
   - 单个文件上传：点击"选择文件"，选择PDF/Word/TXT文件，点击"上传"
   - 批量上传：点击"选择多个文件"，选择多个文件，点击"批量上传"
   - 上传成功后，文档会被自动切分、向量化并存储到知识库

3. **提问**（所有用户）：
   - 在搜索框中输入问题
   - 点击"提问"按钮
   - 系统会检索相关文档并生成回答

## 权限说明

- **管理员**：可以上传文档和管理知识库
- **普通用户**：只能使用搜索功能提问

## 故障排除

### 前端无法访问

- 确保已运行 `npm run build` 构建前端
- 检查 `frontend/dist` 目录是否存在
- 检查后端服务器是否正常运行

### 上传失败

- 检查管理员token是否正确
- 检查文件格式是否支持（PDF、Word、TXT）
- 查看后端日志获取详细错误信息

### 查询无响应

- 检查Ollama服务是否运行：`ollama list`
- 检查Qdrant服务是否运行：`curl http://localhost:6333/health`
- 检查知识库中是否有文档（需要先上传文档）

### 向量化失败

- 检查 `SILICONFLOW_API_KEY` 是否正确设置
- 检查网络连接（需要访问硅基流动API）
- 查看后端日志获取详细错误信息

## 开发模式

如果需要修改前端代码，可以使用开发模式：

```bash
# 终端1：启动后端
go run main.go -mode=server -port=8080

# 终端2：启动前端开发服务器
cd frontend
npm run dev
```

然后访问 `http://localhost:3000`，前端会自动代理API请求到后端。

