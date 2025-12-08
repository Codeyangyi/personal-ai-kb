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
	"sync"
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
		log.Printf("解析请求体失败: %v", err)
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

	log.Printf("收到查询请求: %s (topK=%d), 客户端: %s", req.Question, req.TopK, r.RemoteAddr)

	// 优化：使用请求的context，并添加超时控制（60秒），确保请求可以取消
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// 使用 QueryWithResults 方法，避免重复搜索
	queryResult, err := tempRAG.QueryWithResults(ctx, req.Question)
	if err != nil {
		log.Printf("查询失败 - 问题: %s, 错误: %v, 错误类型: %T, 客户端: %s", req.Question, err, err, r.RemoteAddr)
		// 返回更详细的错误信息
		w.WriteHeader(http.StatusInternalServerError)
		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "查询失败",
			"message": err.Error(),
		}); encodeErr != nil {
			log.Printf("编码错误响应失败: %v", encodeErr)
		}
		return
	}
	log.Printf("查询成功，答案长度: %d 字符, 结果数量: %d", len(queryResult.Answer), len(queryResult.Results))

	// 分析答案中的标注，找出被使用的文档片段编号
	usedIndices := extractUsedAnnotations(queryResult.Answer)

	// 按文档来源分组，只返回被标注使用的文档片段
	// 使用 map 来按文档来源分组
	type DocGroup struct {
		DocTitle      string                   `json:"docTitle"`
		DocSource     string                   `json:"docSource"`
		SourceType    string                   `json:"sourceType"`              // "file" 或 "url"
		FileType      string                   `json:"fileType,omitempty"`      // 文件类型，如 "pdf", "docx", "txt" 等
		HasPublicForm bool                     `json:"hasPublicForm,omitempty"` // 是否包含"公开形式"字眼
		FileID        string                   `json:"fileId,omitempty"`        // 文件ID，用于下载
		Chunks        []map[string]interface{} `json:"chunks"`
	}

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
					log.Printf("⚠️ 处理文档片段时发生panic: %v, 索引: %d", r, idx)
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

	// 将 map 转换为 slice，并检查pdf、word、txt文档中是否包含"公开形式"字眼
	// 优化：并行处理文档检查，避免串行阻塞
	docGroups := make([]DocGroup, 0, len(docGroupsMap))

	// 创建一个辅助函数来检查单个文档
	checkPublicForm := func(group *DocGroup) {
		// 只对pdf、word、txt文档检查是否包含"公开形式：不予公开"或"公开形式：依申请公开"
		fileTypeLower := strings.ToLower(group.FileType)
		if fileTypeLower != "pdf" && fileTypeLower != "doc" && fileTypeLower != "docx" && fileTypeLower != "txt" {
			// 对于非pdf/word/txt文档，不设置HasPublicForm字段
			log.Printf("文档 %s (类型: %s) 不是PDF/Word/TXT，不检查'公开形式'", group.DocTitle, group.FileType)
			return
		}

		// 先检查返回的chunks内容，收集所有内容
		// 限制总大小，避免内存溢出（最多收集100KB的内容）
		const maxContentSize = 100 * 1024 // 100KB
		allContent := strings.Builder{}
		allContent.Grow(2000) // 预分配2KB
		totalSize := 0
		
		for _, chunk := range group.Chunks {
			if totalSize >= maxContentSize {
				break // 达到限制，停止收集
			}
			
			// 尝试多个可能的字段名
			var content string
			if c, ok := chunk["content"].(string); ok && c != "" {
				content = c
			} else if c, ok = chunk["pageContent"].(string); ok && c != "" {
				content = c
			} else if c, ok = chunk["preview"].(string); ok && c != "" {
				content = c
			}
			
			if content != "" {
				// 如果加上这个内容会超过限制，只取部分
				if totalSize+len(content) > maxContentSize {
					remaining := maxContentSize - totalSize
					if remaining > 0 {
						allContent.WriteString(content[:remaining])
						totalSize = maxContentSize
					}
					break
				}
				allContent.WriteString(content)
				allContent.WriteString("\n")
				totalSize += len(content) + 1
			}
		}

		// 如果文件信息中有内容预览，也加入检查（但不超过限制）
		if group.FileID != "" && totalSize < maxContentSize {
			if fileInfo, exists := s.files[group.FileID]; exists && fileInfo.Content != "" {
				preview := fileInfo.Content
				remaining := maxContentSize - totalSize
				if len(preview) > remaining {
					preview = preview[:remaining]
				}
				allContent.WriteString(preview)
				allContent.WriteString("\n")
				totalSize += len(preview) + 1
			}
		}

		allContentStr := allContent.String()
		// 只检查文档内容的最后部分（最后2000个字符）
		// 因为"公开形式"通常在文档末尾
		contentToCheck := allContentStr
		if len(contentToCheck) > 2000 {
			contentToCheck = contentToCheck[len(contentToCheck)-2000:]
		}

		log.Printf("检查文档 %s (类型: %s, FileID: %s) 最后部分是否包含'公开形式'，检查内容长度: %d (总长度: %d)", group.DocTitle, group.FileType, group.FileID, len(contentToCheck), len(allContentStr))

		// 如果chunks中没有找到，尝试重新加载文档并检查最后部分
		if len(contentToCheck) == 0 || !checkPublicFormInContent(contentToCheck) {
			if group.FileID != "" {
				if fileInfo, exists := s.files[group.FileID]; exists {
					// 构建文件路径
					var filePath string
					newFormatPath := filepath.Join(s.filesDir, group.FileID+"_"+fileInfo.Filename)
					oldFormatPath := filepath.Join(s.filesDir, group.FileID+filepath.Ext(fileInfo.Filename))

					// 优先尝试新格式
					if _, err := os.Stat(newFormatPath); err == nil {
						filePath = newFormatPath
					} else if _, err := os.Stat(oldFormatPath); err == nil {
						filePath = oldFormatPath
					}

					if filePath != "" {
						// 对于TXT文件，直接读取文件的最后部分
						if fileTypeLower == "txt" {
							if fileContent, err := readFileLastBytes(filePath, 2000); err == nil {
								contentToCheck = fileContent
								log.Printf("从TXT文件读取最后部分，长度: %d", len(contentToCheck))
							}
						} else if fileTypeLower == "pdf" || fileTypeLower == "doc" || fileTypeLower == "docx" {
							// 对于PDF和Word文件，只加载文档的最后部分
							lastContent, err := loadDocumentLastPart(filePath, fileTypeLower)
							if err == nil && lastContent != "" {
								// 只检查最后2000个字符
								if len(lastContent) > 2000 {
									contentToCheck = lastContent[len(lastContent)-2000:]
								} else {
									contentToCheck = lastContent
								}
								log.Printf("加载文档最后部分并检查，检查内容长度: %d (总长度: %d)", len(contentToCheck), len(lastContent))
							} else {
								log.Printf("加载文档最后部分失败: %v", err)
							}
						}
					}
				}
			}
		}

		// 检查最后部分是否包含"公开形式"
		hasPublicForm := checkPublicFormInContent(contentToCheck)

		if hasPublicForm {
			group.HasPublicForm = true
			log.Printf("✅ 检测到文档 %s (类型: %s, FileID: %s) 最后部分包含'公开形式' - 将禁止下载", group.DocTitle, group.FileType, group.FileID)
			// 输出检测到的具体内容片段用于确认
			idx := strings.Index(contentToCheck, "公开形式")
			if idx >= 0 {
				start := idx - 20
				if start < 0 {
					start = 0
				}
				end := idx + 50
				if end > len(contentToCheck) {
					end = len(contentToCheck)
				}
				log.Printf("检测到的内容片段: ...%s...", contentToCheck[start:end])
			}
		} else {
			// 明确设置为false，确保JSON序列化时包含该字段
			group.HasPublicForm = false
			log.Printf("❌ 文档 %s (类型: %s, FileID: %s) 最后部分未检测到'公开形式' - 允许下载", group.DocTitle, group.FileType, group.FileID)
		}
	}

	// 并行处理所有文档检查
	type checkResult struct {
		groupKey string // 使用docSource作为唯一标识
		group    *DocGroup
	}
	
	// 限制channel缓冲区大小，避免内存问题
	docGroupsCount := len(docGroupsMap)
	const maxCheckBuffer = 100
	checkBufferSize := docGroupsCount
	if checkBufferSize > maxCheckBuffer {
		checkBufferSize = maxCheckBuffer
	}
	checkChan := make(chan checkResult, checkBufferSize)

	// 使用WaitGroup确保所有goroutine完成
	var checkWg sync.WaitGroup
	
	// 启动goroutine并行检查所有文档
	// 为每个文档检查添加超时控制（10秒），避免单个文档检查时间过长导致整体超时
	checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
	defer checkCancel()
	
	for groupKey, group := range docGroupsMap {
		checkWg.Add(1)
		go func(key string, g *DocGroup) {
			defer checkWg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️ 检查文档时发生panic: %v, 文档: %s", r, g.DocTitle)
					// 发生panic时，设置默认值并继续
					g.HasPublicForm = false
				}
			}()
			
			// 使用带超时的context检查文档
			done := make(chan bool, 1)
			go func() {
				checkPublicForm(g)
				done <- true
			}()
			
			// 等待完成或超时
			select {
			case <-done:
				// 检查完成
			case <-checkCtx.Done():
				// 超时，设置默认值
				log.Printf("⚠️ 文档检查超时: %s", g.DocTitle)
				g.HasPublicForm = false
			}
			
			// 发送结果（使用select避免阻塞）
			select {
			case checkChan <- checkResult{groupKey: key, group: g}:
			default:
				// channel已满，记录警告但继续（不会阻塞）
				log.Printf("⚠️ 检查结果channel已满，跳过文档: %s", g.DocTitle)
			}
		}(groupKey, group)
	}

	// 等待所有检查完成，然后关闭channel
	go func() {
		checkWg.Wait()
		close(checkChan)
	}()
	
	// 设置收集结果的超时时间（15秒），确保不会无限等待
	collectCtx, collectCancel := context.WithTimeout(ctx, 15*time.Second)
	defer collectCancel()
	
	// 收集所有检查结果（带超时控制）
	checkedGroups := make(map[string]*DocGroup, docGroupsCount)
	collectDone := make(chan bool, 1)
	
	go func() {
		for result := range checkChan {
			checkedGroups[result.groupKey] = result.group
		}
		collectDone <- true
	}()
	
	// 等待收集完成或超时
	select {
	case <-collectDone:
		// 收集完成
	case <-collectCtx.Done():
		// 超时，记录警告但继续处理已收集的结果
		log.Printf("⚠️ 文档检查结果收集超时，已收集 %d/%d 个结果", len(checkedGroups), docGroupsCount)
	}
	
	// 如果有些文档没有收到结果（可能因为channel满了），使用原始group
	if len(checkedGroups) < docGroupsCount {
		log.Printf("⚠️ 警告：只收到 %d/%d 个文档的检查结果", len(checkedGroups), docGroupsCount)
	}

	// 按原始顺序添加到docGroups
	for groupKey, group := range docGroupsMap {
		if checkedGroup, exists := checkedGroups[groupKey]; exists {
			docGroups = append(docGroups, *checkedGroup)
		} else {
			// 如果检查失败，使用原始group
			docGroups = append(docGroups, *group)
		}
	}

	// 构建响应数据
	response := map[string]interface{}{
		"answer":    queryResult.Answer,
		"results":   searchResults, // 平铺格式（兼容旧前端）
		"docGroups": docGroups,     // 按文档分组的格式（新格式）
	}

	// 设置响应头，确保即使编码失败也能正确返回
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// 编码响应，确保错误处理
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("编码查询响应失败: %v, 问题: %s", err, req.Question)
		// 如果编码失败，尝试返回一个简单的错误响应
		// 注意：此时响应头可能已经部分写入，但这是最后的尝试
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"响应编码失败"}`)
		return
	}

	log.Printf("查询响应已成功发送，答案长度: %d 字符, 文档组数: %d", len(queryResult.Answer), len(docGroups))
}

// loadDocumentLastPart 加载PDF或Word文档的最后部分（只加载最后几页/最后部分内容）
// 避免加载整个文档，节省内存
// 添加超时控制，避免大文件加载时间过长
func loadDocumentLastPart(filePath string, fileType string) (string, error) {
	// 创建带超时的context（5秒），避免大文件加载时间过长
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// 在goroutine中加载文档，以便可以超时取消
	type loadResult struct {
		docs []schema.Document
		err  error
	}
	resultChan := make(chan loadResult, 1)
	
	go func() {
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
		return "", fmt.Errorf("加载文档超时（超过5秒）")
	}
	
	if err != nil {
		return "", fmt.Errorf("加载文档失败: %w", err)
	}
	
	if len(docs) == 0 {
		return "", fmt.Errorf("文档为空")
	}
	
	// 对于PDF，通常每个文档代表一页，我们只取最后几页
	// 对于Word，通常只有一个文档，我们只取最后部分
	const maxPagesToLoad = 2        // 最多加载最后2页
	const maxContentSize = 500 * 1024 // 最多500KB内容
	
	var lastContent strings.Builder
	lastContent.Grow(2000) // 预分配2KB
	
	totalSize := 0
	
	// 从后往前收集内容
	startIdx := 0
	if len(docs) > maxPagesToLoad {
		startIdx = len(docs) - maxPagesToLoad
	}
	
	// 为了保持顺序（从后往前），我们需要先收集，然后反转
	// 但为了简单，我们直接收集最后几页的内容
	for i := startIdx; i < len(docs) && totalSize < maxContentSize; i++ {
		content := docs[i].PageContent
		
		// 如果加上这个内容会超过限制，只取部分
		if totalSize+len(content) > maxContentSize {
			remaining := maxContentSize - totalSize
			if remaining > 0 {
				// 对于最后一页，只取内容的最后部分
				if i == len(docs)-1 {
					contentStart := len(content) - remaining
					if contentStart < 0 {
						contentStart = 0
					}
					lastContent.WriteString(content[contentStart:])
				} else {
					// 对于前面的页，跳过
					break
				}
			}
			totalSize = maxContentSize
			break
		}
		
		lastContent.WriteString(content)
		lastContent.WriteString("\n")
		totalSize += len(content) + 1
	}
	
	return lastContent.String(), nil
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
