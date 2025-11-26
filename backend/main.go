package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Codeyangyi/personal-ai-kb/api"
	"github.com/Codeyangyi/personal-ai-kb/config"
	"github.com/Codeyangyi/personal-ai-kb/embedding"
	"github.com/Codeyangyi/personal-ai-kb/llm"
	"github.com/Codeyangyi/personal-ai-kb/loader"
	"github.com/Codeyangyi/personal-ai-kb/rag"
	"github.com/Codeyangyi/personal-ai-kb/splitter"
	"github.com/Codeyangyi/personal-ai-kb/store"
	"github.com/tmc/langchaingo/schema"
)

func main() {
	var (
		mode      = flag.String("mode", "", "运行模式: load (加载文档), query (查询), load-dir (批量加载), server (启动API服务器)。如果不指定，使用配置文件中的SERVER_MODE或默认server模式")
		filePath  = flag.String("file", "", "要加载的文档路径")
		url       = flag.String("url", "", "要加载的网页URL")
		question  = flag.String("question", "", "要查询的问题")
		topK      = flag.Int("topk", 3, "检索返回的文档数量")
		chunkSize = flag.Int("chunk-size", 1000, "文本块大小")
		overlap   = flag.Int("overlap", 200, "文本块重叠大小")
		fastMode  = flag.Bool("fast", false, "快速模式：使用更大的文本块以减少向量化次数")
		ultraFast = flag.Bool("ultra-fast", false, "极速模式：使用超大文本块（10000字符），大幅减少向量化次数")
		port      = flag.String("port", "", "API服务器端口（仅用于server模式）。如果不指定，使用配置文件中的SERVER_PORT或默认8080")
	)
	flag.Parse()

	// 加载配置
	cfg := config.LoadConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置错误: %v", err)
	}

	// 如果没有指定mode，使用配置文件中的默认值
	if *mode == "" {
		*mode = cfg.ServerMode
		log.Printf("使用配置的默认模式: %s (可通过 -mode 参数或 SERVER_MODE 环境变量修改)", *mode)
	}

	// 如果没有指定port，使用配置文件中的默认值
	if *port == "" {
		*port = cfg.ServerPort
		if *mode == "server" {
			log.Printf("使用配置的默认端口: %s (可通过 -port 参数或 SERVER_PORT 环境变量修改)", *port)
		}
	}

	// 创建嵌入向量生成器
	// 支持硅基流动或Ollama
	embedder, err := embedding.NewEmbedder(
		cfg.EmbeddingProvider,
		cfg.OllamaBaseURL,
		cfg.EmbeddingModelName,
		cfg.SiliconFlowAPIKey,
	)
	if err != nil {
		log.Fatalf("创建嵌入向量生成器失败: %v", err)
	}

	// 创建向量存储（会自动创建集合如果不存在）
	vectorStore, err := store.NewQdrantStore(cfg.QdrantURL, cfg.QdrantAPIKey, cfg.CollectionName, embedder.GetEmbedder(), embedder)
	if err != nil {
		log.Fatalf("创建向量存储失败: %v", err)
	}

	// 创建LLM客户端（根据配置选择Ollama、通义千问或Kimi2）
	var llmClient llm.LLM
	if cfg.LLMProvider == "dashscope" {
		// 使用通义千问
		llmClient, err = llm.NewDashScopeLLM(cfg.DashScopeAPIKey, cfg.DashScopeModel)
		if err != nil {
			log.Fatalf("创建通义千问客户端失败: %v", err)
		}
		log.Printf("使用通义千问模型: %s", cfg.DashScopeModel)
	} else if cfg.LLMProvider == "kimi" {
		// 使用Kimi2
		llmClient, err = llm.NewKimiLLM(cfg.MoonshotAPIKey, cfg.MoonshotModel)
		if err != nil {
			log.Fatalf("创建Kimi2客户端失败: %v", err)
		}
		log.Printf("使用Kimi2模型: %s", cfg.MoonshotModel)
	} else {
		// 使用Ollama
		llmClient, err = llm.NewOllamaLLM(cfg.OllamaBaseURL, cfg.OllamaModel)
		if err != nil {
			log.Fatalf("创建Ollama客户端失败: %v", err)
		}
		log.Printf("使用Ollama模型: %s", cfg.OllamaModel)
	}

	// 创建RAG系统
	ragSystem := rag.NewRAG(embedder, vectorStore, llmClient, *topK)

	ctx := context.Background()

	switch *mode {
	case "load":
		// 加载文档模式
		if *filePath == "" && *url == "" {
			log.Fatal("请指定要加载的文件路径 (-file) 或URL (-url)")
		}

		var docs []schema.Document
		var err error

		if *url != "" {
			// 从URL加载
			fmt.Printf("正在从URL加载文档: %s\n", *url)
			docs, err = loader.LoadFromURL(*url)
		} else {
			// 从文件加载
			fmt.Printf("正在加载文档: %s\n", *filePath)
			fileLoader := loader.NewFileLoader()
			docs, err = fileLoader.Load(*filePath)
		}

		if err != nil {
			log.Fatalf("加载文档失败: %v", err)
		}

		fmt.Printf("已加载 %d 个文档\n", len(docs))

		// 切分文档
		fmt.Println("正在切分文档...")
		// 快速模式：使用更大的文本块以减少向量化次数
		actualChunkSize := *chunkSize
		actualOverlap := *overlap
		if *ultraFast {
			actualChunkSize = 10000 // 极速模式使用10000字符的块
			actualOverlap = 2000
			fmt.Printf("⚡ 极速模式：使用超大文本块 (大小: %d, 重叠: %d) 大幅减少向量化次数\n", actualChunkSize, actualOverlap)
		} else if *fastMode {
			actualChunkSize = 3000 // 快速模式使用3000字符的块
			actualOverlap = 500
			fmt.Printf("快速模式：使用更大的文本块 (大小: %d, 重叠: %d) 以减少向量化次数\n", actualChunkSize, actualOverlap)
		}
		textSplitter := splitter.NewTextSplitter(actualChunkSize, actualOverlap)
		chunks, err := textSplitter.SplitDocuments(docs)
		if err != nil {
			log.Fatalf("切分文档失败: %v", err)
		}

		fmt.Printf("已切分为 %d 个文本块\n", len(chunks))

		// 添加到知识库
		fmt.Printf("正在向量化并存储到知识库... (共 %d 个文本块)\n", len(chunks))
		if err := ragSystem.AddDocuments(ctx, chunks); err != nil {
			log.Fatalf("添加到知识库失败: %v", err)
		}

	case "query":
		// 查询模式
		if *question == "" {
			// 交互式查询
			fmt.Println("个人AI知识库 - 查询模式")
			fmt.Println("输入 'exit' 或 'quit' 退出")
			fmt.Println()

			for {
				fmt.Print("问题: ")
				var input string
				fmt.Scanln(&input)
				input = strings.TrimSpace(input)

				if input == "" {
					continue
				}

				if input == "exit" || input == "quit" {
					fmt.Println("再见！")
					break
				}

				fmt.Println("正在查询...")
				answer, err := ragSystem.Query(ctx, input)
				if err != nil {
					fmt.Printf("查询失败: %v\n", err)
					continue
				}

				fmt.Printf("\n回答: %s\n\n", answer)
			}
		} else {
			// 单次查询
			fmt.Printf("问题: %s\n", *question)
			fmt.Println("正在查询...")
			answer, err := ragSystem.Query(ctx, *question)
			if err != nil {
				log.Fatalf("查询失败: %v", err)
			}
			fmt.Printf("\n回答: %s\n", answer)
		}

	case "load-dir":
		// 批量加载目录中的文档
		if *filePath == "" {
			log.Fatal("请指定要加载的目录路径 (-file)")
		}

		dir := *filePath
		fileInfo, err := os.Stat(dir)
		if err != nil {
			log.Fatalf("无法访问目录: %v", err)
		}
		if !fileInfo.IsDir() {
			log.Fatal("指定的路径不是目录")
		}

		// 支持的文档类型
		supportedExts := map[string]bool{
			".txt":  true,
			".pdf":  true,
			".docx": true,
			".doc":  true,
			".html": true,
			".htm":  true,
		}

		var allChunks []schema.Document
		// 快速模式：使用更大的文本块
		actualChunkSize := *chunkSize
		actualOverlap := *overlap
		if *ultraFast {
			actualChunkSize = 10000
			actualOverlap = 2000
			fmt.Printf("⚡ 极速模式：使用超大文本块 (大小: %d, 重叠: %d)\n", actualChunkSize, actualOverlap)
		} else if *fastMode {
			actualChunkSize = 3000
			actualOverlap = 500
			fmt.Printf("快速模式：使用更大的文本块 (大小: %d, 重叠: %d)\n", actualChunkSize, actualOverlap)
		}
		textSplitter := splitter.NewTextSplitter(actualChunkSize, actualOverlap)

		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if !supportedExts[ext] {
				return nil
			}

			fmt.Printf("正在加载: %s\n", path)
			fileLoader := loader.NewFileLoader()
			docs, err := fileLoader.Load(path)
			if err != nil {
				fmt.Printf("警告: 加载 %s 失败: %v\n", path, err)
				return nil
			}

			chunks, err := textSplitter.SplitDocuments(docs)
			if err != nil {
				fmt.Printf("警告: 切分 %s 失败: %v\n", path, err)
				return nil
			}

			allChunks = append(allChunks, chunks...)
			return nil
		})

		if err != nil {
			log.Fatalf("遍历目录失败: %v", err)
		}

		fmt.Printf("\n共加载 %d 个文本块\n", len(allChunks))
		fmt.Println("正在向量化并存储到知识库...")

		if err := ragSystem.AddDocuments(ctx, allChunks); err != nil {
			log.Fatalf("添加到知识库失败: %v", err)
		}

	case "server":
		// 启动API服务器模式
		server, err := api.NewServer(cfg)
		if err != nil {
			log.Fatalf("创建API服务器失败: %v", err)
		}
		if err := server.Start(*port); err != nil {
			log.Fatalf("启动API服务器失败: %v", err)
		}

	default:
		log.Fatalf("未知模式: %s. 支持的模式: load, query, load-dir, server", *mode)
	}
}
