package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Codeyangyi/personal-ai-kb/config"
	"github.com/Codeyangyi/personal-ai-kb/embedding"
	"github.com/Codeyangyi/personal-ai-kb/llm"
	"github.com/Codeyangyi/personal-ai-kb/loader"
	"github.com/Codeyangyi/personal-ai-kb/logger"
	"github.com/Codeyangyi/personal-ai-kb/ocr"
	"github.com/Codeyangyi/personal-ai-kb/rag"
	"github.com/Codeyangyi/personal-ai-kb/splitter"
	"github.com/Codeyangyi/personal-ai-kb/store"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/schema"

	_ "github.com/go-sql-driver/mysql"
)

// FileInfo 文件信息
type FileInfo struct {
	ID         string    `json:"id"`
	Filename   string    `json:"filename"`
	Title      string    `json:"title"`   // 文件标题（从文件名提取，不含扩展名）
	Content    string    `json:"content"` // 文件内容预览（前1000字符）
	Size       int64     `json:"size"`
	UploadedAt time.Time `json:"uploadedAt"`
	Chunks     int       `json:"chunks"`
}

// DocGroup 文档分组信息（用于查询结果和异步检查）
type DocGroup struct {
	DocTitle      string                   `json:"docTitle"`
	DocSource     string                   `json:"docSource"`
	SourceType    string                   `json:"sourceType"`              // "file" 或 "url"
	FileType      string                   `json:"fileType,omitempty"`      // 文件类型，如 "pdf", "docx", "txt" 等
	HasPublicForm bool                     `json:"hasPublicForm,omitempty"` // 是否包含"公开形式"字眼
	FileID        string                   `json:"fileId,omitempty"`        // 文件ID，用于下载
	Chunks        []map[string]interface{} `json:"chunks"`
}

// checkTaskWithResult 包含检查任务和结果channel的结构体
type checkTaskWithResult struct {
	group      *DocGroup
	resultChan chan bool
}

	// Server HTTP API服务器
type Server struct {
	ragSystem      *rag.RAG
	config         *config.Config
	embedder       *embedding.Embedder
	store          *store.QdrantStore
	llm            llm.LLM
	adminToken     string
	filesDir       string
	failedFilesDir string               // 失败文件目录
	files          map[string]*FileInfo // 文件ID -> 文件信息
	db             *sql.DB              // MySQL 连接（用于业务数据，如意见反馈）
	ocrProcessor   *ocr.OCRProcessor    // OCR处理器（纯Go实现，用于处理扫描件PDF）

	// 异步检查相关
	checkQueue   chan *checkTaskWithResult // 检查任务队列（包含结果channel）
	checkWorkers int                       // 检查工作协程数量
}

// NewServer 创建新的API服务器
func NewServer(cfg *config.Config) (*Server, error) {
	// 创建嵌入向量生成器
	embedder, err := embedding.NewEmbedder(
		cfg.EmbeddingProvider,
		cfg.OllamaBaseURL,
		cfg.EmbeddingModelName,
		cfg.SiliconFlowAPIKey,
	)
	if err != nil {
		return nil, fmt.Errorf("创建嵌入向量生成器失败: %v", err)
	}

	// 创建向量存储
	vectorStore, err := store.NewQdrantStore(cfg.QdrantURL, cfg.QdrantAPIKey, cfg.CollectionName, embedder.GetEmbedder(), embedder)
	if err != nil {
		return nil, fmt.Errorf("创建向量存储失败: %v", err)
	}

	// 创建LLM客户端（根据配置选择Ollama、通义千问或Kimi2）
	var llmClient llm.LLM
	if cfg.LLMProvider == "dashscope" {
		// 使用通义千问
		llmClient, err = llm.NewDashScopeLLM(cfg.DashScopeAPIKey, cfg.DashScopeModel)
		if err != nil {
			return nil, fmt.Errorf("创建通义千问客户端失败: %v", err)
		}
		logger.Info("使用通义千问模型: %s", cfg.DashScopeModel)
	} else if cfg.LLMProvider == "kimi" {
		// 使用Kimi2
		llmClient, err = llm.NewKimiLLM(cfg.MoonshotAPIKey, cfg.MoonshotModel)
		if err != nil {
			return nil, fmt.Errorf("创建Kimi2客户端失败: %v", err)
		}
		logger.Info("使用Kimi2模型: %s", cfg.MoonshotModel)
	} else {
		// 使用Ollama
		llmClient, err = llm.NewOllamaLLM(cfg.OllamaBaseURL, cfg.OllamaModel)
		if err != nil {
			return nil, fmt.Errorf("创建Ollama客户端失败: %v", err)
		}
		logger.Info("使用Ollama模型: %s", cfg.OllamaModel)
	}

	// 创建RAG系统
	ragSystem := rag.NewRAG(embedder, vectorStore, llmClient, 3)

	// 初始化 MySQL（可选）
	var db *sql.DB
	if cfg.MySQLDSN != "" {
		var err error
		db, err = sql.Open("mysql", cfg.MySQLDSN)
		if err != nil {
			return nil, fmt.Errorf("连接 MySQL 失败: %v", err)
		}
		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("MySQL 连接测试失败: %v", err)
		}

		// 创建意见反馈表（如果不存在）
		createTableSQL := `CREATE TABLE IF NOT EXISTS feedbacks (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	name VARCHAR(100) NOT NULL,
	title VARCHAR(255) NOT NULL,
	description TEXT NOT NULL,
	image VARCHAR(512) NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;`
		if _, err := db.Exec(createTableSQL); err != nil {
			return nil, fmt.Errorf("创建反馈表失败: %v", err)
		}
		logger.Info("MySQL 已连接，反馈表初始化成功")
	} else {
		logger.Info("未配置 MYSQL_DSN，意见反馈将不会写入数据库")
	}

	// 获取管理员token（从环境变量或配置）
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		adminToken = "Zhzx@666" // 默认token，生产环境应该使用强密码
		logger.Info("警告: 使用默认管理员token，建议设置 ADMIN_TOKEN 环境变量")
	}

	// 创建文件存储目录（在backend目录下）
	filesDir := "./uploads"
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return nil, fmt.Errorf("创建文件存储目录失败: %v", err)
	}

	// 创建失败文件存储目录
	failedFilesDir := filepath.Join(filesDir, "failed")
	if err := os.MkdirAll(failedFilesDir, 0755); err != nil {
		return nil, fmt.Errorf("创建失败文件存储目录失败: %v", err)
	}

	// 创建OCR处理器（纯Go实现，如果配置了DashScope API Key）
	var ocrProcessor *ocr.OCRProcessor
	if cfg.DashScopeAPIKey != "" {
		dashScopeOCR := ocr.NewDashScopeOCR(cfg.DashScopeAPIKey)
		ocrProcessor = ocr.NewOCRProcessor(dashScopeOCR)
		logger.Info("OCR功能已启用（纯Go实现），支持处理扫描版PDF")
	} else {
		logger.Info("未配置DASHSCOPE_API_KEY，OCR功能未启用，无法处理扫描版PDF")
	}

	server := &Server{
		ragSystem:      ragSystem,
		config:         cfg,
		embedder:       embedder,
		store:          vectorStore,
		llm:            llmClient,
		adminToken:     adminToken,
		filesDir:       filesDir,
		failedFilesDir: failedFilesDir,
		files:          make(map[string]*FileInfo),
		db:             db,
		ocrProcessor:   ocrProcessor,
		checkQueue:     make(chan *checkTaskWithResult, 100), // 检查任务队列，缓冲区100
		checkWorkers:   3,                                    // 3个工作协程处理检查任务
	}

	// 从磁盘恢复文件列表
	server.loadFilesFromDisk()

	// 启动异步检查工作协程
	server.startAsyncCheckWorkers()

	return server, nil
}

// Start 启动HTTP服务器
func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// Panic恢复中间件
	recoveryMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("请求处理发生panic: %v, 请求路径: %s, 方法: %s, 堆栈: %s",
						err, r.URL.Path, r.Method, getStackTrace())
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error":   "服务器内部错误",
						"message": "请求处理时发生意外错误，请稍后重试",
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}

	// CORS中间件
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	// API路由
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/upload", s.handleUpload)
	mux.HandleFunc("/api/upload-batch", s.handleBatchUpload)
	mux.HandleFunc("/api/query", s.handleQuery)
	mux.HandleFunc("/api/feedback", s.handleFeedback)
	mux.HandleFunc("/api/check-admin", s.handleCheckAdmin)
	mux.HandleFunc("/api/files/count", s.handleFileCount)
	mux.HandleFunc("/api/files", s.handleFileList)
	mux.HandleFunc("/api/files/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			s.handleFileDownload(w, r)
		} else if r.Method == "DELETE" {
			s.handleFileDelete(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// 静态文件服务（Vue前端，相对于backend目录，需要回到项目根目录）
	staticDir := "./frontend/dist"
	if _, err := os.Stat(staticDir); err == nil {
		fs := http.FileServer(http.Dir(staticDir))
		mux.Handle("/", http.StripPrefix("/", fs))
	} else {
		// 如果前端目录不存在，提供一个简单的提示
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `
			<!DOCTYPE html>
			<html>
			<head>
				<title>个人AI知识库</title>
				<meta charset="utf-8">
			</head>
			<body>
				<h1>个人AI知识库 API 服务器</h1>
				<p>API服务器正在运行</p>
				<p>前端文件未找到，请确保前端已构建并放置在 frontend/dist 目录中</p>
			</body>
			</html>
			`)
		})
	}

	// 先应用panic恢复，再应用CORS
	handler := recoveryMiddleware(corsMiddleware(mux))

	// 创建HTTP服务器并设置超时时间
	// 优化：增加超时时间以支持大文件上传和长时间向量化
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  30 * time.Minute,  // 读取超时：30分钟（用于大文件上传）
		WriteTimeout: 30 * time.Minute,  // 写入超时：30分钟（用于向量化响应）
		IdleTimeout:  120 * time.Second, // 空闲连接超时：2分钟
	}

	logger.Info("服务器启动在 http://localhost%s (超时设置: 读取/写入30分钟)", server.Addr)
	return server.ListenAndServe()
}

// handleHealth 健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleCheckAdmin 检查管理员权限
func (s *Server) handleCheckAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if req.Token == s.adminToken {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isAdmin": true,
		})
	} else {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isAdmin": false,
		})
	}
}

// handleUpload 处理单个文件上传
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 检查管理员权限
	if !s.checkAdminAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 解析multipart form
	// 优化：统一文件大小限制为500MB，与批量上传保持一致
	err := r.ParseMultipartForm(500 << 20) // 500MB（从32MB增加到500MB，与批量上传保持一致）
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse form: %v (文件可能过大，最大支持500MB)", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 检查文件是否已存在（通过文件名和大小判断）
	if s.isFileDuplicate(header.Filename, header.Size) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  false,
			"message":  fmt.Sprintf("文件 %s 已存在，请勿重复上传", header.Filename),
			"filename": header.Filename,
		})
		return
	}

	// 生成文件ID和保存路径（保留原文件名）
	fileID := uuid.New().String()
	// 清理文件名中的危险字符
	cleanedFilename := strings.ReplaceAll(header.Filename, "/", "_")
	cleanedFilename = strings.ReplaceAll(cleanedFilename, "\\", "_")
	cleanedFilename = strings.ReplaceAll(cleanedFilename, "..", "_")
	// 格式：{fileID}_{原文件名}
	savedPath := filepath.Join(s.filesDir, fileID+"_"+cleanedFilename)

	// 保存上传的文件
	savedFile, err := os.Create(savedPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create file: %v", err), http.StatusInternalServerError)
		return
	}
	defer savedFile.Close()

	fileSize, err := io.Copy(savedFile, file)
	if err != nil {
		os.Remove(savedPath)
		http.Error(w, fmt.Sprintf("Failed to save file: %v", err), http.StatusInternalServerError)
		return
	}

	// 加载文档（使用带OCR支持的文件加载器，纯Go实现）
	var fileLoader *loader.FileLoader
	if s.ocrProcessor != nil {
		fileLoader = loader.NewFileLoaderWithOCR(s.ocrProcessor)
	} else {
		fileLoader = loader.NewFileLoader()
	}
	docs, err := fileLoader.Load(savedPath)
	if err != nil {
		// 优化：提供更友好的错误信息（与批量上传保持一致）
		errMsg := err.Error()
		userFriendlyMsg := errMsg
		if strings.Contains(errMsg, "加密") || strings.Contains(errMsg, "password") {
			userFriendlyMsg = "PDF文件已加密或受密码保护，请先移除密码保护"
		} else if strings.Contains(errMsg, "损坏") || strings.Contains(errMsg, "corrupt") || strings.Contains(errMsg, "格式异常") || strings.Contains(errMsg, "malformed") {
			userFriendlyMsg = "PDF文件可能已损坏或格式不正确，请尝试用PDF阅读器打开并重新保存"
		} else if strings.Contains(errMsg, "stream") || strings.Contains(errMsg, "结构不完整") {
			userFriendlyMsg = "PDF文件格式异常，可能是扫描版PDF（图片格式）或文件结构不完整。请尝试用PDF阅读器打开并重新保存，或使用OCR工具提取文本"
		} else if strings.Contains(errMsg, "扫描版") || strings.Contains(errMsg, "OCR") {
			userFriendlyMsg = "扫描版PDF（纯图片），无法提取文本，请使用OCR工具提取文本"
		} else if strings.Contains(errMsg, "empty") {
			userFriendlyMsg = "PDF文件为空"
		} else if strings.Contains(errMsg, "too large") {
			userFriendlyMsg = "PDF文件过大（最大500MB）"
		}

		// 保存失败文件到失败目录
		failureReason := fmt.Sprintf("加载文档失败: %s", userFriendlyMsg)
		if saveErr := s.saveFailedFile(savedPath, header.Filename, failureReason); saveErr != nil {
			logger.Error("保存失败文件时出错: %v", saveErr)
			os.Remove(savedPath) // 如果保存失败，删除原文件
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  false,
			"message":  failureReason,
			"filename": header.Filename,
		})
		return
	}

	// 提取文件内容预览（前1000字符）
	contentPreview := ""
	title := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	if len(docs) > 0 {
		contentPreview = docs[0].PageContent
		if len(contentPreview) > 1000 {
			contentPreview = contentPreview[:1000] + "..."
		}
		// 尝试从文档元数据获取标题
		if docTitle, ok := docs[0].Metadata["title"].(string); ok && docTitle != "" {
			title = docTitle
		}
	}

	// 切分文档
	textSplitter := splitter.NewTextSplitter(s.config.ChunkSize, s.config.ChunkOverlap)
	chunks, err := textSplitter.SplitDocuments(docs)
	if err != nil {
		// 保存失败文件到失败目录
		failureReason := fmt.Sprintf("切分文档失败: %v", err)
		if saveErr := s.saveFailedFile(savedPath, header.Filename, failureReason); saveErr != nil {
			logger.Error("保存失败文件时出错: %v", saveErr)
			os.Remove(savedPath) // 如果保存失败，删除原文件
		}
		http.Error(w, fmt.Sprintf("Failed to split document: %v", err), http.StatusInternalServerError)
		return
	}

	// 过滤掉内容为空的chunks（只保留有实际文本内容的chunks）
	validChunks := make([]schema.Document, 0, len(chunks))
	for _, chunk := range chunks {
		content := strings.TrimSpace(chunk.PageContent)
		if len(content) > 0 {
			validChunks = append(validChunks, chunk)
		}
	}

	// 检查是否有有效的chunks
	if len(validChunks) == 0 {
		failureReason := "文件加载成功但未提取到任何有效文本内容。可能是扫描版PDF（纯图片）或文件内容为空，请使用OCR工具提取文本后再上传"
		logger.Warn("文件 %s 切分后没有有效内容，原始chunks数量: %d", header.Filename, len(chunks))
		if saveErr := s.saveFailedFile(savedPath, header.Filename, failureReason); saveErr != nil {
			logger.Error("保存失败文件时出错: %v", saveErr)
			os.Remove(savedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  false,
			"message":  failureReason,
			"filename": header.Filename,
		})
		return
	}

	// 如果过滤后chunks数量减少，记录警告
	if len(validChunks) < len(chunks) {
		logger.Warn("文件 %s 过滤掉 %d 个空chunks，保留 %d 个有效chunks", header.Filename, len(chunks)-len(validChunks), len(validChunks))
	}

	// 添加到知识库
	ctx := context.Background()
	if err := s.ragSystem.AddDocuments(ctx, validChunks); err != nil {
		// 向量化失败：保存失败文件到失败目录
		failureReason := fmt.Sprintf("向量化失败: %v", err)
		if saveErr := s.saveFailedFile(savedPath, header.Filename, failureReason); saveErr != nil {
			logger.Error("保存失败文件时出错: %v", saveErr)
			os.Remove(savedPath) // 如果保存失败，删除原文件
		}
		logger.Error("向量化失败，已保存失败文件: %s, 错误: %v", savedPath, err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  false,
			"message":  fmt.Sprintf("文件处理成功，但向量化失败: %v。文件已保存到失败目录，请稍后重试。", err),
			"filename": header.Filename,
		})
		return
	}

	// 保存文件信息
	fileInfo := &FileInfo{
		ID:         fileID,
		Filename:   header.Filename,
		Title:      title,
		Content:    contentPreview,
		Size:       fileSize,
		UploadedAt: time.Now(),
		Chunks:     len(validChunks),
	}
	s.files[fileID] = fileInfo

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("成功上传并处理文件: %s，共 %d 个文本块", header.Filename, len(validChunks)),
		"chunks":   len(validChunks),
		"fileId":   fileID,
		"filename": header.Filename,
	})
}

// handleBatchUpload 处理批量文件上传
func (s *Server) handleBatchUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 检查管理员权限
	if !s.checkAdminAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 解析multipart form
	// 优化：增加文件大小限制到500MB，支持更大的文件上传
	err := r.ParseMultipartForm(500 << 20) // 500MB（从100MB增加到500MB）
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse form: %v (文件可能过大，最大支持500MB)", err), http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		http.Error(w, "No files uploaded", http.StatusBadRequest)
		return
	}

	// 创建文件加载器（使用带OCR支持的文件加载器，纯Go实现）
	var fileLoader *loader.FileLoader
	if s.ocrProcessor != nil {
		fileLoader = loader.NewFileLoaderWithOCR(s.ocrProcessor)
	} else {
		fileLoader = loader.NewFileLoader()
	}
	textSplitter := splitter.NewTextSplitter(s.config.ChunkSize, s.config.ChunkOverlap)

	type FileResult struct {
		Filename string `json:"filename"`
		Success  bool   `json:"success"`
		Message  string `json:"message"`
		Chunks   int    `json:"chunks"`
		FileID   string `json:"fileId,omitempty"`
	}

	var results []FileResult
	var allChunks []schema.Document
	successCount := 0
	failCount := 0

	// 处理每个文件
	for _, fileHeader := range files {
		// 检查文件是否已存在（通过文件名和大小判断）
		if s.isFileDuplicate(fileHeader.Filename, fileHeader.Size) {
			results = append(results, FileResult{
				Filename: fileHeader.Filename,
				Success:  false,
				Message:  "文件已存在，请勿重复上传",
			})
			failCount++
			continue
		}

		file, err := fileHeader.Open()
		if err != nil {
			logger.Error("Failed to open file %s: %v", fileHeader.Filename, err)
			results = append(results, FileResult{
				Filename: fileHeader.Filename,
				Success:  false,
				Message:  fmt.Sprintf("打开文件失败: %v", err),
			})
			failCount++
			continue
		}

		// 生成文件ID和保存路径（保留原文件名）
		fileID := uuid.New().String()
		// 清理文件名中的危险字符
		cleanedFilename := strings.ReplaceAll(fileHeader.Filename, "/", "_")
		cleanedFilename = strings.ReplaceAll(cleanedFilename, "\\", "_")
		cleanedFilename = strings.ReplaceAll(cleanedFilename, "..", "_")
		// 格式：{fileID}_{原文件名}
		savedPath := filepath.Join(s.filesDir, fileID+"_"+cleanedFilename)

		// 保存文件
		savedFile, err := os.Create(savedPath)
		if err != nil {
			file.Close()
			logger.Error("Failed to create file for %s: %v", fileHeader.Filename, err)
			results = append(results, FileResult{
				Filename: fileHeader.Filename,
				Success:  false,
				Message:  fmt.Sprintf("创建文件失败: %v", err),
			})
			failCount++
			continue
		}

		fileSize, err := io.Copy(savedFile, file)
		file.Close()
		savedFile.Close()

		if err != nil {
			// 保存失败文件到失败目录
			failureReason := fmt.Sprintf("保存文件失败: %v", err)
			if saveErr := s.saveFailedFile(savedPath, fileHeader.Filename, failureReason); saveErr != nil {
				logger.Error("保存失败文件时出错: %v", saveErr)
				os.Remove(savedPath) // 如果保存失败，删除原文件
			}
			logger.Error("Failed to save file %s: %v", fileHeader.Filename, err)
			results = append(results, FileResult{
				Filename: fileHeader.Filename,
				Success:  false,
				Message:  failureReason,
			})
			failCount++
			continue
		}

		// 加载文档
		docs, err := fileLoader.Load(savedPath)
		if err != nil {
			logger.Error("Failed to load document %s: %v", fileHeader.Filename, err)
			// 提取更友好的错误信息
			errMsg := err.Error()
			userFriendlyMsg := errMsg
			if strings.Contains(errMsg, "加密") || strings.Contains(errMsg, "password") {
				userFriendlyMsg = "PDF文件已加密或受密码保护，请先移除密码保护"
			} else if strings.Contains(errMsg, "损坏") || strings.Contains(errMsg, "corrupt") || strings.Contains(errMsg, "格式异常") || strings.Contains(errMsg, "malformed") {
				userFriendlyMsg = "PDF文件可能已损坏或格式不正确，请尝试用PDF阅读器打开并重新保存"
			} else if strings.Contains(errMsg, "stream") || strings.Contains(errMsg, "结构不完整") {
				userFriendlyMsg = "PDF文件格式异常，可能是扫描版PDF（图片格式）或文件结构不完整。请尝试用PDF阅读器打开并重新保存，或使用OCR工具提取文本"
			} else if strings.Contains(errMsg, "扫描版") || strings.Contains(errMsg, "OCR") {
				userFriendlyMsg = "扫描版PDF（纯图片），无法提取文本，请使用OCR工具提取文本"
			} else if strings.Contains(errMsg, "empty") {
				userFriendlyMsg = "PDF文件为空"
			} else if strings.Contains(errMsg, "too large") {
				userFriendlyMsg = "PDF文件过大（最大100MB）"
			}

			// 保存失败文件到失败目录
			failureReason := fmt.Sprintf("加载文档失败: %s", userFriendlyMsg)
			if saveErr := s.saveFailedFile(savedPath, fileHeader.Filename, failureReason); saveErr != nil {
				logger.Error("保存失败文件时出错: %v", saveErr)
				os.Remove(savedPath) // 如果保存失败，删除原文件
			}

			results = append(results, FileResult{
				Filename: fileHeader.Filename,
				Success:  false,
				Message:  failureReason,
			})
			failCount++
			continue
		}

		// 提取文件内容预览（前1000字符）
		contentPreview := ""
		title := strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename))
		if len(docs) > 0 {
			contentPreview = docs[0].PageContent
			if len(contentPreview) > 1000 {
				contentPreview = contentPreview[:1000] + "..."
			}
			// 尝试从文档元数据获取标题
			if docTitle, ok := docs[0].Metadata["title"].(string); ok && docTitle != "" {
				title = docTitle
			}
		}

		// 切分文档
		chunks, err := textSplitter.SplitDocuments(docs)
		if err != nil {
			// 保存失败文件到失败目录
			failureReason := fmt.Sprintf("切分文档失败: %v", err)
			if saveErr := s.saveFailedFile(savedPath, fileHeader.Filename, failureReason); saveErr != nil {
				logger.Error("保存失败文件时出错: %v", saveErr)
				os.Remove(savedPath) // 如果保存失败，删除原文件
			}
			logger.Error("Failed to split document %s: %v", fileHeader.Filename, err)
			results = append(results, FileResult{
				Filename: fileHeader.Filename,
				Success:  false,
				Message:  failureReason,
			})
			failCount++
			continue
		}

		// 过滤掉内容为空的chunks（只保留有实际文本内容的chunks）
		validChunks := make([]schema.Document, 0, len(chunks))
		for _, chunk := range chunks {
			content := strings.TrimSpace(chunk.PageContent)
			if len(content) > 0 {
				validChunks = append(validChunks, chunk)
			}
		}

		// 检查是否有有效的chunks
		if len(validChunks) == 0 {
			failureReason := "文件加载成功但未提取到任何有效文本内容。可能是扫描版PDF（纯图片）或文件内容为空，请使用OCR工具提取文本后再上传"
			logger.Warn("文件 %s 切分后没有有效内容，原始chunks数量: %d", fileHeader.Filename, len(chunks))
			if saveErr := s.saveFailedFile(savedPath, fileHeader.Filename, failureReason); saveErr != nil {
				logger.Error("保存失败文件时出错: %v", saveErr)
				os.Remove(savedPath)
			}
			results = append(results, FileResult{
				Filename: fileHeader.Filename,
				Success:  false,
				Message:  failureReason,
			})
			failCount++
			continue
		}

		// 如果过滤后chunks数量减少，记录警告
		if len(validChunks) < len(chunks) {
			logger.Warn("文件 %s 过滤掉 %d 个空chunks，保留 %d 个有效chunks", fileHeader.Filename, len(chunks)-len(validChunks), len(validChunks))
		}

		allChunks = append(allChunks, validChunks...)
		logger.Info("文件 %s 处理成功，生成 %d 个有效文本块（原始 %d 个），累计 %d 个文本块", fileHeader.Filename, len(validChunks), len(chunks), len(allChunks))

		// 保存文件信息
		fileInfo := &FileInfo{
			ID:         fileID,
			Filename:   fileHeader.Filename,
			Title:      title,
			Content:    contentPreview,
			Size:       fileSize,
			UploadedAt: time.Now(),
			Chunks:     len(validChunks),
		}
		s.files[fileID] = fileInfo

		results = append(results, FileResult{
			Filename: fileHeader.Filename,
			Success:  true,
			Message:  fmt.Sprintf("成功处理，共 %d 个文本块", len(validChunks)),
			Chunks:   len(validChunks),
			FileID:   fileID,
		})
		successCount++
	}

	// 添加到知识库（如果有成功的文件）
	var vectorizationError error
	var vectorizedChunks int
	if len(allChunks) > 0 {
		ctx := context.Background()
		logger.Info("开始向量化 %d 个文本块...", len(allChunks))
		if err := s.ragSystem.AddDocuments(ctx, allChunks); err != nil {
			logger.Error("向量化失败: %v", err)
			vectorizationError = err

			// 向量化失败时，将所有成功处理的文件移动到失败目录
			failureReason := fmt.Sprintf("向量化失败: %v", err)
			for i := range results {
				result := &results[i]
				if result.Success && result.FileID != "" {
					// 查找对应的文件路径
					if fileInfo, exists := s.files[result.FileID]; exists {
						// 构建文件路径
						cleanedFilename := strings.ReplaceAll(fileInfo.Filename, "/", "_")
						cleanedFilename = strings.ReplaceAll(cleanedFilename, "\\", "_")
						cleanedFilename = strings.ReplaceAll(cleanedFilename, "..", "_")
						filePath := filepath.Join(s.filesDir, result.FileID+"_"+cleanedFilename)

						// 保存失败文件
						if saveErr := s.saveFailedFile(filePath, fileInfo.Filename, failureReason); saveErr != nil {
							logger.Error("保存失败文件时出错: %v", saveErr)
						} else {
							// 从文件列表中删除
							delete(s.files, result.FileID)
							// 更新结果状态
							result.Success = false
							result.Message = failureReason
							successCount--
							failCount++
						}
					}
				}
			}
		} else {
			logger.Info("向量化成功，共处理 %d 个文本块", len(allChunks))
			vectorizedChunks = len(allChunks)
		}
	} else {
		logger.Info("没有需要向量化的文本块")
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":          true,
		"message":          fmt.Sprintf("处理完成：成功 %d 个，失败 %d 个", successCount, failCount),
		"totalFiles":       len(files),
		"successCount":     successCount,
		"failCount":        failCount,
		"results":          results,
		"totalChunks":      len(allChunks),
		"vectorizedChunks": vectorizedChunks,
	}

	// 如果向量化失败，添加错误信息
	if vectorizationError != nil {
		response["vectorizationError"] = vectorizationError.Error()
		response["message"] = fmt.Sprintf("处理完成：成功 %d 个，失败 %d 个。⚠️ 向量化失败: %v", successCount, failCount, vectorizationError)
	}

	json.NewEncoder(w).Encode(response)
}

// handleQuery 处理查询请求
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	// 添加panic恢复，确保即使发生panic也不会导致服务崩溃
	defer func() {
		if r := recover(); r != nil {
			logger.Error("⚠️ handleQuery发生panic: %v, 堆栈: %s", r, getStackTrace())
			// 尝试返回错误响应
			if w.Header().Get("Content-Type") == "" {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "服务器内部错误",
					"message": "查询处理时发生意外错误",
				})
			}
		}
	}()

	// 提前设置响应头，确保即使发生错误也能正确返回
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Method not allowed",
		})
		return
	}

	var req struct {
		Question string `json:"question"`
		TopK     int    `json:"topk"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("解析请求体失败: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Invalid request",
			"message": "无法解析请求体",
		})
		return
	}

	if req.Question == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Question is required",
			"message": "问题不能为空",
		})
		return
	}

	if req.TopK == 0 {
		req.TopK = 3
	}

	// 创建临时RAG实例用于查询（使用指定的topK）
	tempRAG := rag.NewRAG(s.embedder, s.store, s.llm, req.TopK)

	logger.Info("收到查询请求: %s (topK=%d), 客户端: %s", req.Question, req.TopK, r.RemoteAddr)

	// 优化：使用请求的context，并添加超时控制（50秒），确保请求可以取消
	// 减少超时时间，避免LLM调用时间过长导致服务被停止
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()

	// 使用 QueryWithResults 方法，避免重复搜索
	// 添加panic恢复，确保LLM调用失败不会导致服务崩溃
	var queryResult *rag.QueryResult
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("⚠️ QueryWithResults发生panic: %v, 堆栈: %s", r, getStackTrace())
				err = fmt.Errorf("查询处理时发生panic: %v", r)
			}
		}()
		queryResult, err = tempRAG.QueryWithResults(ctx, req.Question)
	}()
	if err != nil {
		logger.Error("查询失败 - 问题: %s, 错误: %v, 错误类型: %T, 客户端: %s", req.Question, err, err, r.RemoteAddr)
		// 返回更详细的错误信息
		w.WriteHeader(http.StatusInternalServerError)
		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "查询失败",
			"message": err.Error(),
		}); encodeErr != nil {
			logger.Error("编码错误响应失败: %v", encodeErr)
		}
		return
	}
	logger.Info("查询成功，答案长度: %d 字符, 结果数量: %d", len(queryResult.Answer), len(queryResult.Results))

	// 分析答案中的标注，找出被使用的文档片段编号
	usedIndices := extractUsedAnnotations(queryResult.Answer)

	// 按文档来源分组，只返回被标注使用的文档片段
	// 使用 map 来按文档来源分组
	// DocGroup 类型已在包级别定义

	// 优化：使用sync.Map和并发处理文档分组，提升性能
	type docProcessResult struct {
		index    int
		result   map[string]interface{}
		groupKey string
		group    *DocGroup
	}

	// 使用带缓冲的channel收集处理结果
	// 限制缓冲区大小，避免大结果集导致内存问题（最多1000个结果）
	const maxChannelBuffer = 1000
	bufferSize := len(queryResult.Results)
	if bufferSize > maxChannelBuffer {
		bufferSize = maxChannelBuffer
	}
	resultChan := make(chan docProcessResult, bufferSize)

	// 并发处理所有文档片段
	var wg sync.WaitGroup
	for i, doc := range queryResult.Results {
		// 检查这个文档片段是否在答案中被标注使用（索引从1开始，所以i+1）
		if !usedIndices[i+1] {
			continue
		}

		wg.Add(1)
		go func(idx int, d schema.Document) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error("⚠️ 处理文档片段时发生panic: %v, 索引: %d", r, idx)
				}
			}()

			// 使用原始索引（idx+1），与AI答案中的标注保持一致
			originalIndex := idx + 1

			// 获取文档来源信息
			var docTitle, docSource, sourceType, fileType, fileID string
			if source, ok := d.Metadata["source"].(string); ok {
				docSource = source
				// 判断是文件还是URL
				if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
					sourceType = "url"
					docTitle = source // URL直接使用完整URL作为标题
				} else {
					sourceType = "file"
					// 从文件路径中提取原始文件名（去除UUID前缀）
					docTitle = extractOriginalFilename(filepath.Base(source))
					// 从文件路径中提取fileID（格式：{fileID}_{原文件名}）
					baseName := filepath.Base(source)
					if idx := strings.Index(baseName, "_"); idx > 0 {
						fileID = baseName[:idx]
					}
					// 判断文件类型
					ext := strings.ToLower(filepath.Ext(docTitle))
					if ext != "" {
						fileType = ext[1:] // 去掉点号
					}
				}
			}
			// 优先使用file_name元数据（如果存在且不包含UUID）
			if fileName, ok := d.Metadata["file_name"].(string); ok && fileName != "" {
				// 从file_name中提取原始文件名（去除UUID前缀）
				originalFileName := extractOriginalFilename(fileName)
				if originalFileName != "" {
					docTitle = originalFileName
				}
				// 从file_name中提取fileID
				if idx := strings.Index(fileName, "_"); idx > 0 {
					fileID = fileName[:idx]
				}
				// 判断文件类型
				ext := strings.ToLower(filepath.Ext(originalFileName))
				if ext != "" {
					fileType = ext[1:] // 去掉点号
				}
			}
			if docTitle == "" {
				docTitle = "未命名文档"
			}

			// 生成预览（前200字符）
			preview := d.PageContent
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}

			// 创建文档片段结果
			result := map[string]interface{}{
				"content":     d.PageContent,
				"pageContent": d.PageContent,
				"index":       originalIndex, // 使用原始索引，与AI答案中的标注保持一致
				"source":      docSource,
				"title":       docTitle,
				"preview":     preview,
			}

			// 按文档来源分组
			groupKey := docSource
			if groupKey == "" {
				groupKey = docTitle // 如果没有source，使用title作为分组key
			}

			// 创建文档组
			group := &DocGroup{
				DocTitle:   docTitle,
				DocSource:  docSource,
				SourceType: sourceType,
				FileType:   fileType,
				FileID:     fileID,
				Chunks:     []map[string]interface{}{result},
			}

			resultChan <- docProcessResult{
				index:    originalIndex,
				result:   result,
				groupKey: groupKey,
				group:    group,
			}
		}(i, doc)
	}

	// 等待所有goroutine完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果并分组
	docGroupsMap := make(map[string]*DocGroup)
	var searchResults []map[string]interface{} // 保留平铺格式以兼容旧前端

	// 使用sync.Map确保并发安全
	var mu sync.Mutex

	// 收集所有结果
	for res := range resultChan {
		mu.Lock()
		// 添加到平铺格式（兼容旧前端）
		searchResults = append(searchResults, res.result)

		// 按文档来源分组
		if existingGroup, exists := docGroupsMap[res.groupKey]; exists {
			// 如果组已存在，更新文件类型和文件ID（如果当前文档片段有这些信息）
			if res.group.FileType != "" && existingGroup.FileType == "" {
				existingGroup.FileType = res.group.FileType
			}
			if res.group.FileID != "" && existingGroup.FileID == "" {
				existingGroup.FileID = res.group.FileID
			}
			existingGroup.Chunks = append(existingGroup.Chunks, res.result)
		} else {
			// 创建新组
			docGroupsMap[res.groupKey] = res.group
		}
		mu.Unlock()
	}

	// 对searchResults按index排序，确保顺序正确
	sort.Slice(searchResults, func(i, j int) bool {
		idxI, _ := searchResults[i]["index"].(int)
		idxJ, _ := searchResults[j]["index"].(int)
		return idxI < idxJ
	})

	// 将 map 转换为 slice，并异步检查pdf、word、txt文档中是否包含"公开形式"字眼
	// 完全异步：主请求立即返回，检查在后台进行
	docGroups := make([]DocGroup, 0, len(docGroupsMap))

	// 先将所有文档放入异步检查队列，等待一小段时间看是否能快速完成
	checkTasks := make([]*checkTaskWithResult, 0)
	for _, group := range docGroupsMap {
		// 只对pdf、word、txt文档检查
		fileTypeLower := strings.ToLower(group.FileType)
		if (fileTypeLower == "pdf" || fileTypeLower == "doc" || fileTypeLower == "docx" || fileTypeLower == "txt") && group.FileID != "" {
			// 创建结果channel，用于等待检查结果
			resultChan := make(chan bool, 1)

			// 创建检查任务，放入异步队列
			checkTask := &checkTaskWithResult{
				group:      group,
				resultChan: resultChan,
			}

			// 尝试放入队列（非阻塞）
			select {
			case s.checkQueue <- checkTask:
				logger.Info("📋 文档 %s 已加入异步检查队列", group.DocTitle)
				checkTasks = append(checkTasks, checkTask)
			default:
				// 队列已满，记录警告，使用更安全的默认值（不允许下载）
				logger.Info("⚠️ 检查队列已满，跳过异步检查: %s（使用安全默认值：不允许下载）", group.DocTitle)
				group.HasPublicForm = true // 改为true，不允许下载（更安全）
			}
		} else {
			// 非pdf/word/txt文档，不需要检查，允许下载
			group.HasPublicForm = false
		}
	}

	// 异步检查：快速检查已完成的检查结果（非阻塞，等待足够时间确保检查完成）
	// 平衡：既要避免502错误，又要确保检查完成
	if len(checkTasks) > 0 {
		// 使用map跟踪已处理的task，避免重复处理
		processedTasks := make(map[*DocGroup]bool)

		// 先立即检查一次（可能检查已经完成）
		completedCount := 0
		for _, task := range checkTasks {
			select {
			case hasPublicForm := <-task.resultChan:
				task.group.HasPublicForm = hasPublicForm
				processedTasks[task.group] = true
				completedCount++
				if hasPublicForm {
					logger.Info("✅ 文档 %s 检查完成，包含'公开形式'（不允许下载）", task.group.DocTitle)
				} else {
					logger.Info("✅ 文档 %s 检查完成，不包含'公开形式'（允许下载）", task.group.DocTitle)
				}
			default:
				// 检查未完成，稍后处理
			}
		}

		// 如果还有未完成的检查，等待足够的时间（500ms，确保检查能完成）
		if completedCount < len(checkTasks) {
			maxWaitTime := 500 * time.Millisecond // 增加到500ms，确保检查完成
			if len(checkTasks) > 10 {
				maxWaitTime = 300 * time.Millisecond // 文档多时300ms
			}

			logger.Info("等待 %d 个文档的检查结果（最多等待%v）...", len(checkTasks)-completedCount, maxWaitTime)

			// 使用带超时的select，非阻塞等待
			timeout := time.NewTimer(maxWaitTime)
			defer timeout.Stop()

			// 每50ms检查一次，直到超时或全部完成
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()

		waitLoop:
			for completedCount < len(checkTasks) {
				select {
				case <-timeout.C:
					// 超时，停止等待
					logger.Info("等待超时，已收集 %d/%d 个检查结果", completedCount, len(checkTasks))
					break waitLoop
				case <-ticker.C:
					// 检查是否有新的完成
					for _, task := range checkTasks {
						if processedTasks[task.group] {
							continue // 已处理
						}
						select {
						case hasPublicForm := <-task.resultChan:
							task.group.HasPublicForm = hasPublicForm
							processedTasks[task.group] = true
							completedCount++
							if hasPublicForm {
								logger.Info("✅ 文档 %s 检查完成，包含'公开形式'（不允许下载）", task.group.DocTitle)
							} else {
								logger.Info("✅ 文档 %s 检查完成，不包含'公开形式'（允许下载）", task.group.DocTitle)
							}
						default:
						}
					}
				}
			}
		}

		// 处理未完成的检查，使用更安全的默认值（不允许下载，更安全）
		for _, task := range checkTasks {
			if processedTasks[task.group] {
				continue // 已处理
			}

			// 尝试最后一次读取
			select {
			case hasPublicForm := <-task.resultChan:
				task.group.HasPublicForm = hasPublicForm
				processedTasks[task.group] = true
				if hasPublicForm {
					logger.Info("✅ 文档 %s 检查完成（最后读取），包含'公开形式'（不允许下载）", task.group.DocTitle)
				} else {
					logger.Info("✅ 文档 %s 检查完成（最后读取），不包含'公开形式'（允许下载）", task.group.DocTitle)
				}
			default:
				// 检查未完成，使用更安全的默认值（不允许下载）
				// 这样即使检查失败，也不会误允许下载包含"公开形式"的文档
				task.group.HasPublicForm = true // 改为true，不允许下载（更安全）
				logger.Info("⏳ 文档 %s 检查未完成，使用安全默认值：不允许下载（检查在后台继续）", task.group.DocTitle)
			}
		}

		logger.Info("检查结果收集完成，完成: %d/%d（异步检查，不阻塞主请求）", completedCount, len(checkTasks))
	}

	logger.Info("所有文档检查处理完成，立即返回响应")

	// 按原始顺序添加到docGroups（完全异步，不等待检查结果）
	logger.Info("开始构建响应数据，docGroupsMap数量: %d", len(docGroupsMap))
	defer func() {
		if r := recover(); r != nil {
			logger.Error("⚠️ 构建响应数据时发生panic: %v, 堆栈: %s", r, getStackTrace())
		}
	}()

	// 直接使用docGroupsMap构建响应（检查在后台异步进行）
	for _, group := range docGroupsMap {
		docGroups = append(docGroups, *group)
	}
	logger.Info("docGroups构建完成，共 %d 个文档组（检查在后台异步进行）", len(docGroups))

	// 构建响应数据
	// 限制响应大小，避免内存溢出和502错误
	// 如果docGroups太大，只返回前50个
	const maxDocGroups = 50
	limitedDocGroups := docGroups
	if len(docGroups) > maxDocGroups {
		logger.Info("⚠️ 文档组数量过多 (%d > %d)，只返回前 %d 个", len(docGroups), maxDocGroups, maxDocGroups)
		limitedDocGroups = docGroups[:maxDocGroups]
	}

	// 限制每个文档组的chunks数量，避免响应过大
	// 同时限制每个chunk的内容长度，避免单个chunk过大
	const maxChunksPerGroup = 20
	const maxChunkContentLength = 2000 // 每个chunk最多2000字符

	totalChunksBefore := 0
	for i := range limitedDocGroups {
		totalChunksBefore += len(limitedDocGroups[i].Chunks)

		// 限制chunks数量
		if len(limitedDocGroups[i].Chunks) > maxChunksPerGroup {
			logger.Info("⚠️ 文档 %s 的chunks数量过多 (%d > %d)，只返回前 %d 个", limitedDocGroups[i].DocTitle, len(limitedDocGroups[i].Chunks), maxChunksPerGroup, maxChunksPerGroup)
			limitedDocGroups[i].Chunks = limitedDocGroups[i].Chunks[:maxChunksPerGroup]
		}

		// 限制每个chunk的内容长度
		for j := range limitedDocGroups[i].Chunks {
			chunk := limitedDocGroups[i].Chunks[j]
			if content, ok := chunk["content"].(string); ok && len(content) > maxChunkContentLength {
				chunk["content"] = content[:maxChunkContentLength] + "..."
			}
			if pageContent, ok := chunk["pageContent"].(string); ok && len(pageContent) > maxChunkContentLength {
				chunk["pageContent"] = pageContent[:maxChunkContentLength] + "..."
			}
		}
	}
	totalChunksAfter := 0
	for _, g := range limitedDocGroups {
		totalChunksAfter += len(g.Chunks)
	}
	logger.Info("响应数据限制完成，文档组数: %d, 总chunks数: %d -> %d", len(limitedDocGroups), totalChunksBefore, totalChunksAfter)

	// 构建响应数据，添加错误处理
	var response map[string]interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("⚠️ 构建response map时发生panic: %v, 堆栈: %s", r, getStackTrace())
				// 使用简化的响应
				response = map[string]interface{}{
					"answer":    queryResult.Answer,
					"results":   []map[string]interface{}{}, // 空结果
					"docGroups": []DocGroup{},               // 空文档组
				}
			}
		}()
		response = map[string]interface{}{
			"answer":    queryResult.Answer,
			"results":   searchResults,    // 平铺格式（兼容旧前端）
			"docGroups": limitedDocGroups, // 按文档分组的格式（新格式）
		}
	}()
	logger.Info("响应数据构建完成，准备编码JSON，answer长度: %d, results数量: %d, docGroups数量: %d", len(queryResult.Answer), len(searchResults), len(limitedDocGroups))

	// 设置响应头，确保即使编码失败也能正确返回
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// 提前发送响应头，避免502错误（在Ubuntu/Nginx环境下很重要）
	// 这样即使后续处理出现问题，客户端也能知道请求已收到
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
		logger.Info("✅ 响应头已提前刷新，避免502错误")
	}

	// 检查context是否已取消（超时）
	if ctx.Err() != nil {
		logger.Info("⚠️ 请求context已取消: %v, 问题: %s", ctx.Err(), req.Question)
		// 如果context已取消，尝试返回错误响应
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		w.WriteHeader(http.StatusRequestTimeout)
		fmt.Fprintf(w, `{"error":"请求超时","message":"处理时间过长，请求已超时"}`)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}

	// 记录响应大小，用于监控
	responseSize := len(queryResult.Answer) + len(limitedDocGroups)*100 // 粗略估算
	logger.Info("准备发送响应，答案长度: %d 字符, 文档组数: %d, 估算响应大小: %d 字节", len(queryResult.Answer), len(limitedDocGroups), responseSize)

	// 检查客户端连接是否已关闭
	if r.Context().Err() != nil {
		logger.Info("⚠️ 客户端连接已关闭: %v, 问题: %s", r.Context().Err(), req.Question)
		return
	}

	// 编码响应，确保错误处理
	// 使用缓冲写入，避免大响应导致问题
	logger.Info("开始编码JSON响应...")
	defer func() {
		if r := recover(); r != nil {
			logger.Error("⚠️ 编码响应时发生panic: %v, 堆栈: %s", r, getStackTrace())
			// 尝试返回错误响应
			if w.Header().Get("Content-Type") == "" {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
			}
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error":"响应编码失败","message":"服务器处理响应时出错"}`)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}()

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "") // 不格式化，减少响应大小

	if err := encoder.Encode(response); err != nil {
		logger.Error("⚠️ 编码查询响应失败: %v, 问题: %s, 错误类型: %T", err, req.Question, err)
		// 如果编码失败，尝试返回一个简单的错误响应
		// 注意：此时响应头可能已经部分写入，但这是最后的尝试
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		// 检查是否已经写入状态码（避免重复写入）
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"响应编码失败","message":"服务器处理响应时出错"}`)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}

	logger.Info("JSON编码完成，准备刷新响应...")

	// 尝试刷新响应（如果支持），确保数据及时发送，避免超时导致502
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
		logger.Info("✅ 响应已刷新，确保数据及时发送")
	}

	logger.Info("✅ 查询响应已成功发送，答案长度: %d 字符, 文档组数: %d", len(queryResult.Answer), len(limitedDocGroups))
}

// loadDocumentLastPart 加载PDF或Word文档的最后部分（只加载最后几个字符）
// 避免加载整个文档，节省内存和CPU
// 注意：虽然我们只保留最后几个字符，但底层的fileLoader.Load()仍会解析整个文档
// 这是PDF/Word解析库的限制，无法避免。但我们已经限制了内存使用
// maxChars: 最多加载的字符数（默认100）
func loadDocumentLastPart(filePath string, fileType string, maxChars int) (string, error) {
	if maxChars <= 0 {
		maxChars = 100 // 默认只加载最后100个字符
	}

	// 创建带超时的context（1.5秒），避免大文件加载时间过长
	// 进一步减少超时时间，最小化CPU占用
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	// 在goroutine中加载文档，以便可以超时取消
	type loadResult struct {
		docs []schema.Document
		err  error
	}
	resultChan := make(chan loadResult, 1)

	// 使用goroutine加载，避免阻塞
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("⚠️ loadDocumentLastPart加载文档时发生panic: %v", r)
				resultChan <- loadResult{err: fmt.Errorf("加载文档时发生panic: %v", r)}
			}
		}()
		fileLoader := loader.NewFileLoader()
		docs, err := fileLoader.Load(filePath)
		resultChan <- loadResult{docs: docs, err: err}
	}()

	var docs []schema.Document
	var err error

	select {
	case result := <-resultChan:
		docs = result.docs
		err = result.err
	case <-ctx.Done():
		// 超时，返回错误，避免继续占用内存和CPU
		logger.Info("⚠️ 加载文档超时（超过1.5秒）: %s", filePath)
		return "", fmt.Errorf("加载文档超时（超过1.5秒）")
	}

	if err != nil {
		return "", fmt.Errorf("加载文档失败: %w", err)
	}

	if len(docs) == 0 {
		return "", fmt.Errorf("文档为空")
	}

	// 只取最后一页/最后一个文档的最后部分
	// 对于PDF，通常每个文档代表一页，我们只取最后一页
	// 对于Word，通常只有一个文档，我们只取最后部分
	lastDoc := docs[len(docs)-1]
	content := lastDoc.PageContent

	// 只取最后maxChars个字符
	if len(content) > maxChars {
		content = content[len(content)-maxChars:]
	}

	return content, nil
}

// readFileLastBytes 读取文件的最后N个字节（尝试按UTF-8解码）
func readFileLastBytes(filePath string, maxBytes int) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 获取文件大小
	fileInfo, err := file.Stat()
	if err != nil {
		return "", err
	}

	fileSize := fileInfo.Size()
	if fileSize == 0 {
		return "", nil
	}

	// 计算要读取的字节数
	bytesToRead := int64(maxBytes)
	if fileSize < bytesToRead {
		bytesToRead = fileSize
	}

	// 定位到文件末尾
	_, err = file.Seek(fileSize-bytesToRead, 0)
	if err != nil {
		return "", err
	}

	// 读取最后N个字节
	buffer := make([]byte, bytesToRead)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	// 尝试按UTF-8解码
	content := string(buffer[:n])
	return content, nil
}

// checkPublicFormInContent 检查内容中是否包含"公开形式"相关字样
// 支持全角冒号（：）和半角冒号（:），以及可能的空格和换行
// 如果内容中包含"公开形式"四个字，就认为包含（因为用户需求是检查是否有"公开形式"）
func checkPublicFormInContent(content string) bool {
	if content == "" {
		return false
	}

	// 首先检查是否包含"公开形式"四个字（这是最基本的检查）
	if strings.Contains(content, "公开形式") {
		return true
	}

	// 如果直接包含"公开形式"四个字，已经返回true
	// 下面的代码是为了更精确的匹配，但上面的检查已经足够了
	// 先尝试精确匹配
	containsNotPublicFull := strings.Contains(content, "公开形式：不予公开")
	containsApplyPublicFull := strings.Contains(content, "公开形式：依申请公开")
	containsNotPublicFull2 := strings.Contains(content, "公开形式：不公开")
	containsNotPublicHalf := strings.Contains(content, "公开形式:不予公开")
	containsApplyPublicHalf := strings.Contains(content, "公开形式:依申请公开")
	containsNotPublicHalf2 := strings.Contains(content, "公开形式:不公开")

	if containsNotPublicFull || containsApplyPublicFull || containsNotPublicFull2 ||
		containsNotPublicHalf || containsApplyPublicHalf || containsNotPublicHalf2 {
		return true
	}

	// 如果精确匹配失败，尝试模糊匹配（允许冒号前后有空格）
	normalizedContent := strings.ReplaceAll(content, " ", "")
	normalizedContent = strings.ReplaceAll(normalizedContent, "\n", "")
	normalizedContent = strings.ReplaceAll(normalizedContent, "\r", "")
	normalizedContent = strings.ReplaceAll(normalizedContent, "\t", "")

	// 在规范化后的内容中也检查"公开形式"四个字
	if strings.Contains(normalizedContent, "公开形式") {
		return true
	}

	containsNotPublicFull = strings.Contains(normalizedContent, "公开形式：不予公开")
	containsApplyPublicFull = strings.Contains(normalizedContent, "公开形式：依申请公开")
	containsNotPublicFull2 = strings.Contains(normalizedContent, "公开形式：不公开")
	containsNotPublicHalf = strings.Contains(normalizedContent, "公开形式:不予公开")
	containsApplyPublicHalf = strings.Contains(normalizedContent, "公开形式:依申请公开")
	containsNotPublicHalf2 = strings.Contains(normalizedContent, "公开形式:不公开")

	return containsNotPublicFull || containsApplyPublicFull || containsNotPublicFull2 ||
		containsNotPublicHalf || containsApplyPublicHalf || containsNotPublicHalf2
}

// extractOriginalFilename 从文件名中提取原始文件名，去除UUID前缀
// 格式：{UUID}_{原文件名} -> {原文件名}
func extractOriginalFilename(filename string) string {
	if filename == "" {
		return ""
	}

	// UUID格式：xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	// 查找第一个下划线，如果下划线前是UUID格式，则提取下划线后的部分
	// UUID长度：36个字符（32个十六进制字符 + 4个连字符）
	underscoreIndex := strings.Index(filename, "_")
	if underscoreIndex > 0 {
		// 检查下划线前的部分是否是UUID格式（36个字符）
		prefix := filename[:underscoreIndex]
		if len(prefix) == 36 && strings.Contains(prefix, "-") {
			// 验证是否是UUID格式（包含4个连字符）
			parts := strings.Split(prefix, "-")
			if len(parts) == 5 {
				// 提取下划线后的部分作为原始文件名
				originalName := filename[underscoreIndex+1:]
				if originalName != "" {
					return originalName
				}
			}
		}
	}

	// 如果没有UUID前缀，直接返回原文件名
	return filename
}

// extractUsedAnnotations 从答案中提取使用的标注编号
// 返回一个map，key是文档片段编号（从1开始），value表示是否被使用
func extractUsedAnnotations(answer string) map[int]bool {
	used := make(map[int]bool)

	// 圆圈数字映射：①=1, ②=2, ③=3, ...
	circleNumbers := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩"}

	// 检查答案中是否包含这些标注
	for i, circleNum := range circleNumbers {
		if strings.Contains(answer, circleNum) {
			used[i+1] = true
		}
	}

	return used
}

// isSystemFile 检查是否是系统文件
func isSystemFile(filename string) bool {
	// macOS 系统文件
	if filename == ".DS_Store" {
		return true
	}
	// macOS 资源分叉文件
	if strings.HasPrefix(filename, "._") {
		return true
	}
	// Windows 系统文件
	if strings.HasPrefix(filename, "~$") {
		return true
	}
	// Linux/Unix 隐藏文件（但允许 .开头的正常文件，只过滤系统文件）
	if filename == ".gitkeep" || filename == ".gitignore" {
		return true
	}
	return false
}

// isFileDuplicate 检查文件是否已存在（通过文件名和大小判断）
func (s *Server) isFileDuplicate(filename string, size int64) bool {
	// 重新加载文件列表以确保数据最新
	s.loadFilesFromDisk()

	for _, file := range s.files {
		if file.Filename == filename && file.Size == size {
			return true
		}
	}
	return false
}

// loadFilesFromDisk 从磁盘加载文件列表
func (s *Server) loadFilesFromDisk() {
	entries, err := os.ReadDir(s.filesDir)
	if err != nil {
		logger.Error("读取文件目录失败: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// 过滤系统文件
		filename := entry.Name()
		if isSystemFile(filename) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 提取文件ID和原文件名
		// 文件名格式可能是：{fileID}{扩展名} 或 {fileID}_{原文件名}
		ext := filepath.Ext(entry.Name())
		nameWithoutExt := strings.TrimSuffix(entry.Name(), ext)

		var fileID, originalFilename string
		// 检查是否包含下划线（新格式：{fileID}_{原文件名}）
		idx := strings.Index(nameWithoutExt, "_")
		if idx > 0 {
			// 新格式：{fileID}_{原文件名}
			fileID = nameWithoutExt[:idx]
			originalFilename = nameWithoutExt[idx+1:] + ext
		} else {
			// 旧格式：{fileID}{扩展名}，无法恢复原文件名，使用默认名称
			fileID = nameWithoutExt
			originalFilename = "文件" + ext // 使用默认名称
		}

		// 如果文件信息不存在，创建它
		if _, exists := s.files[fileID]; !exists {
			title := strings.TrimSuffix(originalFilename, ext)
			s.files[fileID] = &FileInfo{
				ID:         fileID,
				Filename:   originalFilename, // 使用原文件名，而不是保存的文件名
				Title:      title,
				Content:    "", // 无法从文件系统获取内容预览
				Size:       info.Size(),
				UploadedAt: info.ModTime(),
				Chunks:     0, // 无法从文件系统获取，设为0
			}
		}
	}

	logger.Info("从磁盘加载了 %d 个文件", len(s.files))
}

// handleFileList 获取文件列表
func (s *Server) handleFileList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 检查管理员权限
	if !s.checkAdminAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 重新从磁盘加载文件列表（确保数据最新）
	s.loadFilesFromDisk()

	var fileList []*FileInfo
	for _, file := range s.files {
		fileList = append(fileList, file)
	}

	// 按上传时间倒序排列（最新的在前面）
	sort.Slice(fileList, func(i, j int) bool {
		return fileList[i].UploadedAt.After(fileList[j].UploadedAt)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    fileList,
		"total":   len(fileList),
	})
}

// handleFileCount 获取文件数量（无需管理员权限，公开接口）
func (s *Server) handleFileCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 重新从磁盘加载文件列表（确保数据最新）
	s.loadFilesFromDisk()

	// 过滤系统文件
	var count int
	for _, file := range s.files {
		if !isSystemFile(file.Filename) {
			count++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   count,
	})
}

// handleFileDownload 下载文件
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从URL提取文件ID
	path := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if path == "" {
		http.Error(w, "File ID required", http.StatusBadRequest)
		return
	}

	// 查找文件信息
	fileInfo, exists := s.files[path]
	if !exists {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// 构建文件路径
	// 新格式：{fileID}_{原文件名}
	// 旧格式：{fileID}{扩展名}（兼容处理）
	var filePath string
	newFormatPath := filepath.Join(s.filesDir, path+"_"+fileInfo.Filename)
	oldFormatPath := filepath.Join(s.filesDir, path+filepath.Ext(fileInfo.Filename))

	// 优先尝试新格式
	if _, err := os.Stat(newFormatPath); err == nil {
		filePath = newFormatPath
	} else if _, err := os.Stat(oldFormatPath); err == nil {
		// 兼容旧格式
		filePath = oldFormatPath
	} else {
		http.Error(w, "File not found on disk", http.StatusNotFound)
		return
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// 设置响应头
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileInfo.Filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size))

	// 复制文件内容到响应
	_, err = io.Copy(w, file)
	if err != nil {
		logger.Info("Failed to send file: %v", err)
	}
}

// handleFileDelete 删除文件
func (s *Server) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 检查管理员权限
	if !s.checkAdminAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 从URL提取文件ID
	path := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if path == "" {
		http.Error(w, "File ID required", http.StatusBadRequest)
		return
	}

	// 查找文件信息
	fileInfo, exists := s.files[path]
	if !exists {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// 构建文件路径
	var filePath string
	newFormatPath := filepath.Join(s.filesDir, path+"_"+fileInfo.Filename)
	oldFormatPath := filepath.Join(s.filesDir, path+filepath.Ext(fileInfo.Filename))

	// 优先尝试新格式
	if _, err := os.Stat(newFormatPath); err == nil {
		filePath = newFormatPath
	} else if _, err := os.Stat(oldFormatPath); err == nil {
		// 兼容旧格式
		filePath = oldFormatPath
	} else {
		// 文件不存在，但仍然从列表中删除
		logger.Info("文件 %s 在磁盘上不存在，仅从列表中删除", path)
	}

	// 删除磁盘上的文件
	if filePath != "" {
		if err := os.Remove(filePath); err != nil {
			logger.Error("删除文件失败: %v", err)
			// 继续执行，即使删除文件失败也继续删除记录
		}
	}

	// 从内存中的文件列表删除
	delete(s.files, path)

	// 从Qdrant向量数据库中删除相关文档
	// 通过metadata中的source字段匹配文件路径
	ctx := context.Background()

	// 构建待匹配的所有可能路径（无论磁盘上是否仍存在文件，都需要尝试删除向量数据）
	pathSet := make(map[string]struct{})
	addPath := func(p string) {
		if p == "" {
			return
		}
		if _, exists := pathSet[p]; exists {
			return
		}
		pathSet[p] = struct{}{}
	}

	// 原始保存路径（新旧两种命名格式）
	addPath(newFormatPath)
	addPath(oldFormatPath)

	// 绝对路径 & 相对路径（相对 filesDir）
	for _, p := range []string{newFormatPath, oldFormatPath} {
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			addPath(abs)
		} else {
			addPath(p)
		}
		if rel, err := filepath.Rel(s.filesDir, p); err == nil {
			addPath(rel)
		}

		// 同一路径的正斜杠版本，避免路径分隔符差异导致匹配失败
		addPath(filepath.ToSlash(p))
		if abs, err := filepath.Abs(p); err == nil {
			addPath(filepath.ToSlash(abs))
		}
		if rel, err := filepath.Rel(s.filesDir, p); err == nil {
			addPath(filepath.ToSlash(rel))
		}
	}

	// 基础文件名和原始文件名（兼容仅存储文件名的情况）
	addPath(filepath.Base(newFormatPath))
	addPath(filepath.Base(oldFormatPath))
	addPath(fileInfo.Filename)

	var deleteErr error
	successfulPath := ""
	for p := range pathSet {
		deleteErr = s.store.DeleteDocumentsBySource(ctx, s.config.QdrantURL, s.config.QdrantAPIKey, s.config.CollectionName, p)
		if deleteErr == nil {
			successfulPath = p
			// 继续尝试其他形式，确保不同存储格式的残留也被清理
			logger.Info("已从向量数据库删除文件相关文档，匹配路径: %s", p)
		}
	}

	if successfulPath == "" && deleteErr != nil {
		logger.Error("从向量数据库删除文档失败（已尝试多种路径格式）: %v", deleteErr)
		// 即使删除向量数据库中的文档失败，也返回成功（因为文件已删除）
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("文件 %s 已删除", fileInfo.Filename),
	})
}

// handleFeedback 处理意见反馈提交，将数据写入 MySQL
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 必须配置 MySQL 才能使用反馈功能
	if s.db == nil {
		http.Error(w, "Feedback database not configured (缺少 MYSQL_DSN)", http.StatusInternalServerError)
		return
	}

	// 解析表单（包括可选图片）
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB
		http.Error(w, fmt.Sprintf("解析表单失败: %v", err), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	title := strings.TrimSpace(r.FormValue("title"))
	description := strings.TrimSpace(r.FormValue("description"))

	if name == "" || title == "" || description == "" {
		http.Error(w, "姓名、标题、详细描述为必填项", http.StatusBadRequest)
		return
	}

	// 图片（可选）：保存到本地目录，并在数据库中记录相对路径
	var imagePath sql.NullString
	file, header, err := r.FormFile("image")
	if err == nil && header != nil {
		defer file.Close()

		// 创建图片保存目录：./uploads/feedback-images
		imageDir := filepath.Join(s.filesDir, "feedback-images")
		if err := os.MkdirAll(imageDir, 0755); err != nil {
			logger.Error("创建反馈图片目录失败: %v", err)
		} else {
			// 使用时间戳+原始文件名，避免重名
			ext := filepath.Ext(header.Filename)
			nameWithoutExt := strings.TrimSuffix(header.Filename, ext)
			nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "/", "_")
			nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "\\", "_")
			nameWithoutExt = strings.ReplaceAll(nameWithoutExt, "..", "_")
			timestamp := time.Now().Format("20060102_150405")
			savedName := fmt.Sprintf("%s_%s%s", timestamp, nameWithoutExt, ext)

			fullPath := filepath.Join(imageDir, savedName)
			out, err := os.Create(fullPath)
			if err != nil {
				logger.Error("保存反馈图片失败: %v", err)
			} else {
				if _, err := io.Copy(out, file); err != nil {
					logger.Error("写入反馈图片失败: %v", err)
				} else {
					// 在数据库中记录相对路径（相对于 backend 根目录）
					relPath := filepath.ToSlash(filepath.Join("uploads", "feedback-images", savedName))
					imagePath.String = relPath
					imagePath.Valid = true
				}
				out.Close()
			}
		}
	}

	// 写入 MySQL
	query := `INSERT INTO feedbacks (name, title, description, image, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err = s.db.Exec(query, name, title, description, imagePath, time.Now())
	if err != nil {
		logger.Error("保存反馈失败: %v", err)
		http.Error(w, fmt.Sprintf("保存反馈失败: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "感谢您的反馈！已成功保存。",
	})
}

// saveFailedFile 保存失败的文件到失败目录，并记录失败原因
func (s *Server) saveFailedFile(filePath, originalFilename, reason string) error {
	// 确保失败目录存在
	if err := os.MkdirAll(s.failedFilesDir, 0755); err != nil {
		return fmt.Errorf("创建失败文件目录失败: %v", err)
	}

	ext := filepath.Ext(originalFilename)
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)

	// 清理文件名中的危险字符
	cleanedName := strings.ReplaceAll(nameWithoutExt, "/", "_")
	cleanedName = strings.ReplaceAll(cleanedName, "\\", "_")
	cleanedName = strings.ReplaceAll(cleanedName, "..", "_")

	// 使用原文件名（清理危险字符后）
	failedFilename := cleanedName + ext
	failedPath := filepath.Join(s.failedFilesDir, failedFilename)

	// 如果文件名已存在，添加序号避免冲突
	counter := 1
	for {
		if _, err := os.Stat(failedPath); os.IsNotExist(err) {
			break // 文件不存在，可以使用这个文件名
		}
		// 文件已存在，添加序号
		failedFilename = fmt.Sprintf("%s_%d%s", cleanedName, counter, ext)
		failedPath = filepath.Join(s.failedFilesDir, failedFilename)
		counter++
	}

	// 如果文件不存在，直接返回（可能已经被删除）
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("源文件不存在: %s", filePath)
	}

	// 移动文件到失败目录
	if err := os.Rename(filePath, failedPath); err != nil {
		// 如果重命名失败（可能跨文件系统），尝试复制后删除
		if err := s.copyFile(filePath, failedPath); err != nil {
			return fmt.Errorf("移动失败文件失败: %v", err)
		}
		os.Remove(filePath) // 删除原文件
	}

	logger.Info("失败文件已保存: %s, 原因: %s", failedPath, reason)
	return nil
}

// copyFile 复制文件（用于跨文件系统移动）
func (s *Server) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// checkAdminAuth 检查管理员权限
func (s *Server) checkAdminAuth(r *http.Request) bool {
	// 从Header获取token
	token := r.Header.Get("Authorization")
	if token != "" {
		// 支持 "Bearer token" 格式
		token = strings.TrimPrefix(token, "Bearer ")
		return token == s.adminToken
	}

	// 从Query参数获取token
	token = r.URL.Query().Get("token")
	return token == s.adminToken
}

// getStackTrace 获取当前goroutine的堆栈跟踪信息
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// startAsyncCheckWorkers 启动异步检查工作协程
// 这些协程会从队列中取出文档检查任务，在后台异步执行
func (s *Server) startAsyncCheckWorkers() {
	for i := 0; i < s.checkWorkers; i++ {
		go func(workerID int) {
			logger.Info("启动异步检查工作协程 #%d", workerID)
			for task := range s.checkQueue {
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("⚠️ 异步检查工作协程 #%d 发生panic: %v, 文档: %s", workerID, r, task.group.DocTitle)
							// panic时发送默认结果（如果resultChan存在）
							if task.resultChan != nil {
								select {
								case task.resultChan <- false:
								default:
								}
							}
						}
					}()

					// 执行检查
					logger.Info("[工作协程 #%d] 开始检查文档: %s (FileID: %s)", workerID, task.group.DocTitle, task.group.FileID)
					s.checkPublicFormAsync(task.group)

					// 发送结果（如果resultChan存在，完全异步模式下为nil）
					if task.resultChan != nil {
						select {
						case task.resultChan <- task.group.HasPublicForm:
							if task.group.HasPublicForm {
								logger.Info("[工作协程 #%d] ✅ 文档 %s 检查完成，包含'公开形式'", workerID, task.group.DocTitle)
							} else {
								logger.Info("[工作协程 #%d] ✅ 文档 %s 检查完成，不包含'公开形式'", workerID, task.group.DocTitle)
							}
						default:
							// channel已关闭或已满，记录警告
							logger.Info("⚠️ [工作协程 #%d] 无法发送检查结果: %s", workerID, task.group.DocTitle)
						}
					} else {
						// 完全异步模式，不发送结果，只记录日志
						if task.group.HasPublicForm {
							logger.Info("[工作协程 #%d] ✅ 文档 %s 异步检查完成，包含'公开形式'（完全异步模式）", workerID, task.group.DocTitle)
						} else {
							logger.Info("[工作协程 #%d] ✅ 文档 %s 异步检查完成，不包含'公开形式'（完全异步模式）", workerID, task.group.DocTitle)
						}
					}
				}()
			}
			logger.Info("异步检查工作协程 #%d 已退出", workerID)
		}(i)
	}
	logger.Info("已启动 %d 个异步检查工作协程", s.checkWorkers)
}

// checkPublicFormSync 同步检查文档是否包含"公开形式"（实时检查，不使用缓存）
// 只读取文档最后100个字符进行检查
func (s *Server) checkPublicFormSync(group *DocGroup) {
	fileTypeLower := strings.ToLower(group.FileType)
	if fileTypeLower != "pdf" && fileTypeLower != "doc" && fileTypeLower != "docx" && fileTypeLower != "txt" {
		group.HasPublicForm = false
		return
	}

	// 检查文件路径
	if group.FileID == "" {
		group.HasPublicForm = false
		return
	}

	fileInfo, exists := s.files[group.FileID]
	if !exists {
		group.HasPublicForm = false
		return
	}

	// 构建文件路径
	var filePath string
	newFormatPath := filepath.Join(s.filesDir, group.FileID+"_"+fileInfo.Filename)
	oldFormatPath := filepath.Join(s.filesDir, group.FileID+filepath.Ext(fileInfo.Filename))

	if _, err := os.Stat(newFormatPath); err == nil {
		filePath = newFormatPath
	} else if _, err := os.Stat(oldFormatPath); err == nil {
		filePath = oldFormatPath
	} else {
		group.HasPublicForm = false
		return
	}

	// 只读取最后100个字符进行检查（检查文档内容的最后一页）
	const maxCheckLength = 100
	var contentToCheck string

	if fileTypeLower == "txt" {
		// TXT文件：读取最后100字节
		if fileContent, err := readFileLastBytes(filePath, maxCheckLength); err == nil {
			contentToCheck = fileContent
			logger.Info("[检查] TXT文件 %s 读取的最后%d个字符，实际长度: %d", group.DocTitle, maxCheckLength, len(contentToCheck))
		} else {
			logger.Error("[检查] TXT文件 %s 读取失败: %v", group.DocTitle, err)
		}
	} else if fileTypeLower == "pdf" || fileTypeLower == "doc" || fileTypeLower == "docx" {
		// PDF/Word文档：加载最后一页的内容（最多100字符）
		lastContent, err := loadDocumentLastPart(filePath, fileTypeLower, maxCheckLength)
		if err == nil && lastContent != "" {
			if len(lastContent) > maxCheckLength {
				contentToCheck = lastContent[len(lastContent)-maxCheckLength:]
			} else {
				contentToCheck = lastContent
			}
			logger.Info("[检查] %s文件 %s 读取最后一页的最后%d个字符，实际长度: %d", strings.ToUpper(fileTypeLower), group.DocTitle, maxCheckLength, len(contentToCheck))
		} else {
			logger.Error("[检查] %s文件 %s 读取失败: %v", strings.ToUpper(fileTypeLower), group.DocTitle, err)
		}
	}

	// 检查是否包含"公开形式"
	hasPublicForm := checkPublicFormInContent(contentToCheck)
	group.HasPublicForm = hasPublicForm

	// 记录检查结果，方便调试
	if hasPublicForm {
		logger.Info("[检查结果] ✅ 文档 %s 包含'公开形式'，不允许下载", group.DocTitle)
	} else {
		logger.Info("[检查结果] ✅ 文档 %s 不包含'公开形式'，允许下载", group.DocTitle)
	}
}

// checkPublicFormAsync 异步检查文档是否包含"公开形式"（保留用于兼容，但不再使用）
// 只读取文档最后100个字符进行检查
func (s *Server) checkPublicFormAsync(group *DocGroup) {
	s.checkPublicFormSync(group)
}
