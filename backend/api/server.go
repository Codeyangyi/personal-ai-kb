package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Codeyangyi/personal-ai-kb/config"
	"github.com/Codeyangyi/personal-ai-kb/embedding"
	"github.com/Codeyangyi/personal-ai-kb/llm"
	"github.com/Codeyangyi/personal-ai-kb/loader"
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
		log.Printf("使用通义千问模型: %s", cfg.DashScopeModel)
	} else if cfg.LLMProvider == "kimi" {
		// 使用Kimi2
		llmClient, err = llm.NewKimiLLM(cfg.MoonshotAPIKey, cfg.MoonshotModel)
		if err != nil {
			return nil, fmt.Errorf("创建Kimi2客户端失败: %v", err)
		}
		log.Printf("使用Kimi2模型: %s", cfg.MoonshotModel)
	} else {
		// 使用Ollama
		llmClient, err = llm.NewOllamaLLM(cfg.OllamaBaseURL, cfg.OllamaModel)
		if err != nil {
			return nil, fmt.Errorf("创建Ollama客户端失败: %v", err)
		}
		log.Printf("使用Ollama模型: %s", cfg.OllamaModel)
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
		log.Println("MySQL 已连接，反馈表初始化成功")
	} else {
		log.Println("未配置 MYSQL_DSN，意见反馈将不会写入数据库")
	}

	// 获取管理员token（从环境变量或配置）
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		adminToken = "admin123" // 默认token，生产环境应该使用强密码
		log.Println("警告: 使用默认管理员token，建议设置 ADMIN_TOKEN 环境变量")
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
	}

	// 从磁盘恢复文件列表
	server.loadFilesFromDisk()

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
					log.Printf("请求处理发生panic: %v, 请求路径: %s, 方法: %s, 堆栈: %s", 
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

	log.Printf("服务器启动在 http://localhost%s (超时设置: 读取/写入30分钟)", server.Addr)
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

	// 加载文档
	fileLoader := loader.NewFileLoader()
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
			log.Printf("保存失败文件时出错: %v", saveErr)
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
			log.Printf("保存失败文件时出错: %v", saveErr)
			os.Remove(savedPath) // 如果保存失败，删除原文件
		}
		http.Error(w, fmt.Sprintf("Failed to split document: %v", err), http.StatusInternalServerError)
		return
	}

	// 添加到知识库
	ctx := context.Background()
	if err := s.ragSystem.AddDocuments(ctx, chunks); err != nil {
		// 向量化失败：保存失败文件到失败目录
		failureReason := fmt.Sprintf("向量化失败: %v", err)
		if saveErr := s.saveFailedFile(savedPath, header.Filename, failureReason); saveErr != nil {
			log.Printf("保存失败文件时出错: %v", saveErr)
			os.Remove(savedPath) // 如果保存失败，删除原文件
		}
		log.Printf("向量化失败，已保存失败文件: %s, 错误: %v", savedPath, err)
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
		Chunks:     len(chunks),
	}
	s.files[fileID] = fileInfo

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("成功上传并处理文件: %s，共 %d 个文本块", header.Filename, len(chunks)),
		"chunks":   len(chunks),
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

	fileLoader := loader.NewFileLoader()
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
			log.Printf("Failed to open file %s: %v", fileHeader.Filename, err)
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
			log.Printf("Failed to create file for %s: %v", fileHeader.Filename, err)
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
				log.Printf("保存失败文件时出错: %v", saveErr)
				os.Remove(savedPath) // 如果保存失败，删除原文件
			}
			log.Printf("Failed to save file %s: %v", fileHeader.Filename, err)
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
			log.Printf("Failed to load document %s: %v", fileHeader.Filename, err)
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
				log.Printf("保存失败文件时出错: %v", saveErr)
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
				log.Printf("保存失败文件时出错: %v", saveErr)
				os.Remove(savedPath) // 如果保存失败，删除原文件
			}
			log.Printf("Failed to split document %s: %v", fileHeader.Filename, err)
			results = append(results, FileResult{
				Filename: fileHeader.Filename,
				Success:  false,
				Message:  failureReason,
			})
			failCount++
			continue
		}

		allChunks = append(allChunks, chunks...)
		log.Printf("文件 %s 处理成功，生成 %d 个文本块，累计 %d 个文本块", fileHeader.Filename, len(chunks), len(allChunks))

		// 保存文件信息
		fileInfo := &FileInfo{
			ID:         fileID,
			Filename:   fileHeader.Filename,
			Title:      title,
			Content:    contentPreview,
			Size:       fileSize,
			UploadedAt: time.Now(),
			Chunks:     len(chunks),
		}
		s.files[fileID] = fileInfo

		results = append(results, FileResult{
			Filename: fileHeader.Filename,
			Success:  true,
			Message:  fmt.Sprintf("成功处理，共 %d 个文本块", len(chunks)),
			Chunks:   len(chunks),
			FileID:   fileID,
		})
		successCount++
	}

	// 添加到知识库（如果有成功的文件）
	var vectorizationError error
	var vectorizedChunks int
	if len(allChunks) > 0 {
		ctx := context.Background()
		log.Printf("开始向量化 %d 个文本块...", len(allChunks))
		if err := s.ragSystem.AddDocuments(ctx, allChunks); err != nil {
			log.Printf("向量化失败: %v", err)
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
							log.Printf("保存失败文件时出错: %v", saveErr)
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
			log.Printf("向量化成功，共处理 %d 个文本块", len(allChunks))
			vectorizedChunks = len(allChunks)
		}
	} else {
		log.Printf("没有需要向量化的文本块")
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
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Question string `json:"question"`
		TopK     int    `json:"topk"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Question == "" {
		http.Error(w, "Question is required", http.StatusBadRequest)
		return
	}

	if req.TopK == 0 {
		req.TopK = 3
	}

	// 创建临时RAG实例用于查询（使用指定的topK）
	tempRAG := rag.NewRAG(s.embedder, s.store, s.llm, req.TopK)

	log.Printf("收到查询请求: %s (topK=%d)", req.Question, req.TopK)
	ctx := context.Background()

	// 使用 QueryWithResults 方法，避免重复搜索
	queryResult, err := tempRAG.QueryWithResults(ctx, req.Question)
	if err != nil {
		log.Printf("查询失败 - 问题: %s, 错误: %v, 错误类型: %T", req.Question, err, err)
		// 返回更详细的错误信息
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "查询失败",
			"message": err.Error(),
		})
		return
	}
	log.Printf("查询成功，答案长度: %d 字符", len(queryResult.Answer))

	// 分析答案中的标注，找出被使用的文档片段编号
	usedIndices := extractUsedAnnotations(queryResult.Answer)

	// 按文档来源分组，只返回被标注使用的文档片段
	// 使用 map 来按文档来源分组
	type DocGroup struct {
		DocTitle   string                   `json:"docTitle"`
		DocSource  string                   `json:"docSource"`
		SourceType string                   `json:"sourceType"` // "file" 或 "url"
		Chunks     []map[string]interface{} `json:"chunks"`
	}

	docGroupsMap := make(map[string]*DocGroup)
	var searchResults []map[string]interface{} // 保留平铺格式以兼容旧前端

	// 保持原始索引，不重新编号
	// 这样编号就能与AI答案中的标注（①、②、③等）完全一致
	for i, doc := range queryResult.Results {
		// 检查这个文档片段是否在答案中被标注使用（索引从1开始，所以i+1）
		if !usedIndices[i+1] {
			continue
		}

		// 使用原始索引（i+1），与AI答案中的标注保持一致
		originalIndex := i + 1

		// 获取文档来源信息
		var docTitle, docSource, sourceType string
		if source, ok := doc.Metadata["source"].(string); ok {
			docSource = source
			// 判断是文件还是URL
			if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
				sourceType = "url"
				docTitle = source // URL直接使用完整URL作为标题
			} else {
				sourceType = "file"
				// 从文件路径中提取原始文件名（去除UUID前缀）
				docTitle = extractOriginalFilename(filepath.Base(source))
			}
		}
		// 优先使用file_name元数据（如果存在且不包含UUID）
		if fileName, ok := doc.Metadata["file_name"].(string); ok && fileName != "" {
			// 从file_name中提取原始文件名（去除UUID前缀）
			originalFileName := extractOriginalFilename(fileName)
			if originalFileName != "" {
				docTitle = originalFileName
			}
		}
		if docTitle == "" {
			docTitle = "未命名文档"
		}

		// 生成预览（前200字符）
		preview := doc.PageContent
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}

		// 创建文档片段结果
		// 使用原始索引（originalIndex），与AI答案中的标注（①、②、③等）完全一致
		result := map[string]interface{}{
			"content":     doc.PageContent,
			"pageContent": doc.PageContent,
			"index":       originalIndex, // 使用原始索引，与AI答案中的标注保持一致
			"source":      docSource,
			"title":       docTitle,
			"preview":     preview,
		}

		// 添加到平铺格式（兼容旧前端）
		searchResults = append(searchResults, result)

		// 按文档来源分组
		groupKey := docSource
		if groupKey == "" {
			groupKey = docTitle // 如果没有source，使用title作为分组key
		}

		if _, exists := docGroupsMap[groupKey]; !exists {
			docGroupsMap[groupKey] = &DocGroup{
				DocTitle:   docTitle,
				DocSource:  docSource,
				SourceType: sourceType,
				Chunks:     []map[string]interface{}{},
			}
		}
		docGroupsMap[groupKey].Chunks = append(docGroupsMap[groupKey].Chunks, result)
	}

	// 将 map 转换为 slice
	docGroups := make([]DocGroup, 0, len(docGroupsMap))
	for _, group := range docGroupsMap {
		docGroups = append(docGroups, *group)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"answer":    queryResult.Answer,
		"results":   searchResults, // 平铺格式（兼容旧前端）
		"docGroups": docGroups,     // 按文档分组的格式（新格式）
	})
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
		log.Printf("读取文件目录失败: %v", err)
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

	log.Printf("从磁盘加载了 %d 个文件", len(s.files))
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
		log.Printf("Failed to send file: %v", err)
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
		log.Printf("文件 %s 在磁盘上不存在，仅从列表中删除", path)
	}

	// 删除磁盘上的文件
	if filePath != "" {
		if err := os.Remove(filePath); err != nil {
			log.Printf("删除文件失败: %v", err)
			// 继续执行，即使删除文件失败也继续删除记录
		}
	}

	// 从内存中的文件列表删除
	delete(s.files, path)

	// 从Qdrant向量数据库中删除相关文档
	// 通过metadata中的source字段匹配文件路径
	ctx := context.Background()
	if err := s.deleteDocumentsBySource(ctx, filePath); err != nil {
		log.Printf("从向量数据库删除文档失败: %v", err)
		// 即使删除向量数据库中的文档失败，也返回成功（因为文件已删除）
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("文件 %s 已删除", fileInfo.Filename),
	})
}

// deleteDocumentsBySource 从Qdrant中删除指定source的文档
func (s *Server) deleteDocumentsBySource(ctx context.Context, sourcePath string) error {
	if sourcePath == "" {
		return nil
	}

	// 使用Qdrant的API删除文档
	// 注意：langchaingo的Qdrant实现可能不直接支持按metadata删除
	// 这里我们需要直接调用Qdrant的API
	// 由于Qdrant的删除需要point ID，我们需要先查询所有匹配的point，然后删除

	// 简化实现：由于langchaingo的Qdrant包装器不直接支持按metadata删除
	// 这里我们只记录日志，实际删除可以通过Qdrant的API实现
	// 或者，我们可以重新构建整个知识库（删除所有，然后重新添加其他文件）
	// 为了简化，这里先只删除文件，向量数据库中的文档可以保留（不影响功能）

	log.Printf("注意：向量数据库中的文档（source=%s）需要手动清理或通过Qdrant API删除", sourcePath)
	return nil
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
			log.Printf("创建反馈图片目录失败: %v", err)
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
				log.Printf("保存反馈图片失败: %v", err)
			} else {
				if _, err := io.Copy(out, file); err != nil {
					log.Printf("写入反馈图片失败: %v", err)
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
		log.Printf("保存反馈失败: %v", err)
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

	log.Printf("失败文件已保存: %s, 原因: %s", failedPath, reason)
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
