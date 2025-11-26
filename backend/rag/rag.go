package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Codeyangyi/personal-ai-kb/embedding"
	"github.com/Codeyangyi/personal-ai-kb/llm"
	"github.com/Codeyangyi/personal-ai-kb/store"
	"github.com/tmc/langchaingo/schema"
)

// RAG RAG系统
type RAG struct {
	embedder *embedding.Embedder
	store    *store.QdrantStore
	llm      llm.LLM
	topK     int
}

// NewRAG 创建新的RAG系统
func NewRAG(embedder *embedding.Embedder, store *store.QdrantStore, llm llm.LLM, topK int) *RAG {
	return &RAG{
		embedder: embedder,
		store:    store,
		llm:      llm,
		topK:     topK,
	}
}

// Query 查询并生成回答
// RAG流程：
// 1. 将问题转换为向量（在Search内部自动完成）
// 2. 在向量数据库中进行相似性搜索，找到最相关的文本块
// 3. 将"原始问题" + "检索到的上下文"组合成增强提示
// 4. 将增强提示发送给LLM（Ollama），生成基于知识库的答案
func (r *RAG) Query(ctx context.Context, question string) (string, error) {
	startTime := time.Now()

	// 步骤1: 向量化查询问题并在向量数据库中进行相似性搜索
	// Search方法内部会自动：
	// - 使用embedder将问题文本转换为向量
	// - 在Qdrant向量数据库中进行相似性搜索
	// - 返回最相关的topK个文本块

	// 混合搜索策略：先搜索更多结果（topK*3），然后进行严格的相关性过滤
	searchTopK := r.topK * 3
	if searchTopK < 15 {
		searchTopK = 15 // 至少搜索15个结果
	}
	if searchTopK > 50 {
		searchTopK = 50 // 最多搜索50个结果
	}

	fmt.Print("正在向量化问题并搜索知识库...")
	embedStart := time.Now()
	allResults, err := r.store.Search(ctx, question, r.embedder.GetEmbedder(), searchTopK)
	embedDuration := time.Since(embedStart)
	if err != nil {
		return "", fmt.Errorf("failed to search: %w", err)
	}
	fmt.Printf(" ✅ (耗时: %v, 检索到 %d 个候选片段)\n", embedDuration.Round(time.Millisecond), len(allResults))

	// 对结果进行严格的重排序和相关性过滤：优先选择真正相关的片段
	results := r.reRankResults(question, allResults, r.topK)

	// 二次验证：确保结果与问题真正相关
	results = r.filterRelevantResults(question, results)

	// 调试：显示重排序后的结果
	fmt.Printf("[调试] 重排序后选择的前 %d 个片段（包含关键词的优先）\n", len(results))

	if len(results) == 0 {
		return "抱歉，我在知识库中没有找到相关信息。", nil
	}

	// 调试：显示检索到的文档片段（完整内容，方便检查是否包含相关信息）
	fmt.Printf("\n[调试] 检索到 %d 个相关文档片段：\n", len(results))
	for i, doc := range results {
		// 显示完整内容（最多1000字符，避免输出过长）
		content := doc.PageContent
		preview := content
		if len(content) > 1000 {
			preview = content[:1000] + "..."
		}
		fmt.Printf("\n  [片段 %d] (长度: %d 字符)\n", i+1, len(content))
		fmt.Printf("  内容: %s\n", preview)

		// 检查是否包含关键词（用于调试）
		lowerContent := strings.ToLower(content)
		lowerQuestion := strings.ToLower(question)
		keywords := strings.Fields(lowerQuestion)
		matchedKeywords := []string{}
		for _, keyword := range keywords {
			if strings.Contains(lowerContent, keyword) {
				matchedKeywords = append(matchedKeywords, keyword)
			}
		}
		if len(matchedKeywords) > 0 {
			fmt.Printf("  匹配的关键词: %v\n", matchedKeywords)
		}

		// 显示元数据信息
		if len(doc.Metadata) > 0 {
			fmt.Printf("  元数据: ")
			metaParts := make([]string, 0)
			if source, ok := doc.Metadata["source"].(string); ok {
				metaParts = append(metaParts, fmt.Sprintf("来源=%s", source))
			}
			if len(metaParts) > 0 {
				fmt.Printf("%s", strings.Join(metaParts, ", "))
			}
			fmt.Println()
		}
	}
	fmt.Println()

	// 步骤2: 构建增强提示（原始问题 + 检索到的上下文）
	prompt := r.buildPrompt(question, results)

	// 步骤3: 将增强提示发送给LLM（通义千问或Ollama），生成精准、基于知识库的答案
	fmt.Print("正在生成回答...")
	llmStart := time.Now()

	// 创建带超时的context（120秒超时，给LLM更多时间生成）
	llmCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	answer, err := r.llm.Generate(llmCtx, prompt)
	llmDuration := time.Since(llmStart)
	if err != nil {
		if llmCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("生成回答超时（超过120秒），请尝试：1) 减少检索文档数量 2) 检查网络连接 3) 检查API服务状态")
		}
		return "", fmt.Errorf("failed to generate answer: %w", err)
	}
	fmt.Printf(" ✅ (耗时: %v)\n", llmDuration.Round(time.Millisecond))

	totalDuration := time.Since(startTime)
	fmt.Printf("\n[性能] 总耗时: %v (向量检索: %v, LLM生成: %v)\n",
		totalDuration.Round(time.Millisecond),
		embedDuration.Round(time.Millisecond),
		llmDuration.Round(time.Millisecond))

	return answer, nil
}

// QueryResult 查询结果，包含答案和检索到的文档片段
type QueryResult struct {
	Answer  string
	Results []schema.Document
}

// QueryWithResults 查询并生成回答，同时返回检索到的文档片段
// 这样可以避免重复搜索，提高效率
func (r *RAG) QueryWithResults(ctx context.Context, question string) (*QueryResult, error) {
	startTime := time.Now()

	// 步骤1: 向量化查询问题并在向量数据库中进行相似性搜索
	// Search方法内部会自动：
	// - 使用embedder将问题文本转换为向量
	// - 在Qdrant向量数据库中进行相似性搜索
	// - 返回最相关的topK个文本块

	// 混合搜索策略：先搜索更多结果（topK*3），然后进行严格的相关性过滤
	searchTopK := r.topK * 3
	if searchTopK < 15 {
		searchTopK = 15 // 至少搜索15个结果
	}
	if searchTopK > 50 {
		searchTopK = 50 // 最多搜索50个结果
	}

	fmt.Print("正在向量化问题并搜索知识库...")
	embedStart := time.Now()
	allResults, err := r.store.Search(ctx, question, r.embedder.GetEmbedder(), searchTopK)
	embedDuration := time.Since(embedStart)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	fmt.Printf(" ✅ (耗时: %v, 检索到 %d 个候选片段)\n", embedDuration.Round(time.Millisecond), len(allResults))

	// 对结果进行严格的重排序和相关性过滤：优先选择真正相关的片段
	results := r.reRankResults(question, allResults, r.topK)

	// 二次验证：确保结果与问题真正相关
	results = r.filterRelevantResults(question, results)

	// 调试：显示重排序后的结果
	fmt.Printf("[调试] 重排序后选择的前 %d 个片段（包含关键词的优先）\n", len(results))

	if len(results) == 0 {
		return &QueryResult{
			Answer:  "抱歉，我在知识库中没有找到相关信息。",
			Results: []schema.Document{},
		}, nil
	}

	// 调试：显示检索到的文档片段（完整内容，方便检查是否包含相关信息）
	fmt.Printf("\n[调试] 检索到 %d 个相关文档片段：\n", len(results))
	for i, doc := range results {
		// 显示完整内容（最多1000字符，避免输出过长）
		content := doc.PageContent
		preview := content
		if len(content) > 1000 {
			preview = content[:1000] + "..."
		}
		fmt.Printf("\n  [片段 %d] (长度: %d 字符)\n", i+1, len(content))
		fmt.Printf("  内容: %s\n", preview)

		// 检查是否包含关键词（用于调试）
		lowerContent := strings.ToLower(content)
		lowerQuestion := strings.ToLower(question)
		keywords := strings.Fields(lowerQuestion)
		matchedKeywords := []string{}
		for _, keyword := range keywords {
			if strings.Contains(lowerContent, keyword) {
				matchedKeywords = append(matchedKeywords, keyword)
			}
		}
		if len(matchedKeywords) > 0 {
			fmt.Printf("  匹配的关键词: %v\n", matchedKeywords)
		}

		// 显示元数据信息
		if len(doc.Metadata) > 0 {
			fmt.Printf("  元数据: ")
			metaParts := make([]string, 0)
			if source, ok := doc.Metadata["source"].(string); ok {
				metaParts = append(metaParts, fmt.Sprintf("来源=%s", source))
			}
			if len(metaParts) > 0 {
				fmt.Printf("%s", strings.Join(metaParts, ", "))
			}
			fmt.Println()
		}
	}
	fmt.Println()

	// 步骤2: 构建增强提示（原始问题 + 检索到的上下文）
	prompt := r.buildPrompt(question, results)

	// 调试：显示构建的prompt预览（前500字符和后200字符）
	promptPreview := prompt
	if len(prompt) > 700 {
		promptPreview = prompt[:500] + "\n... [中间内容已省略] ...\n" + prompt[len(prompt)-200:]
	}
	fmt.Printf("\n[调试] 构建的Prompt预览 (%d 字符):\n%s\n", len(prompt), promptPreview)

	// 步骤3: 将增强提示发送给LLM（通义千问或Ollama），生成精准、基于知识库的答案
	fmt.Print("正在生成回答...")
	llmStart := time.Now()

	// 创建带超时的context（120秒超时，给LLM更多时间生成）
	llmCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	answer, err := r.llm.Generate(llmCtx, prompt)
	llmDuration := time.Since(llmStart)
	if err != nil {
		if llmCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("生成回答超时（超过120秒），请尝试：1) 减少检索文档数量 2) 检查网络连接 3) 检查API服务状态")
		}
		return nil, fmt.Errorf("failed to generate answer: %w", err)
	}
	fmt.Printf(" ✅ (耗时: %v)\n", llmDuration.Round(time.Millisecond))

	// 调试：显示LLM返回的答案（完整内容）
	fmt.Printf("\n[调试] LLM返回的答案 (%d 字符):\n%s\n", len(answer), answer)

	totalDuration := time.Since(startTime)
	fmt.Printf("\n[性能] 总耗时: %v (向量检索: %v, LLM生成: %v)\n",
		totalDuration.Round(time.Millisecond),
		embedDuration.Round(time.Millisecond),
		llmDuration.Round(time.Millisecond))

	return &QueryResult{
		Answer:  answer,
		Results: results,
	}, nil
}

// buildPrompt 构建增强提示
// 将"原始问题" + "检索到的上下文"组合成一个增强的提示
// 这个提示会被发送给LLM（Ollama），让LLM基于上下文信息生成精准、基于知识库的答案
func (r *RAG) buildPrompt(question string, results []schema.Document) string {
	var builder strings.Builder

	builder.WriteString("你是一个专业的AI助手。请基于以下上下文信息，**深入思考和分析**后回答问题。\n\n")
	builder.WriteString("**核心要求**：\n")
	builder.WriteString("1. **严格相关性检查**：只使用与问题真正相关的文档片段。如果某个文档片段与问题无关，请忽略它，不要使用其中的信息\n")
	builder.WriteString("2. **必须进行思考和总结**：不要直接复制粘贴文档片段的内容，而是要对信息进行理解、分析和组织\n")
	builder.WriteString("3. **回答要有逻辑性**：将多个文档片段的信息整合成连贯、有条理的回答\n")
	builder.WriteString("4. **回答要完整**：如果问题涉及多个方面，要全面回答，不要遗漏重要信息\n")
	builder.WriteString("5. **只基于提供的上下文信息回答问题**，不要编造或推测信息\n")
	builder.WriteString("6. **如果上下文中没有相关信息或所有文档片段都与问题无关**，请明确说明\"根据提供的上下文，我无法找到相关信息\"，不要强行使用不相关的片段\n")
	builder.WriteString("6. **必须遵守**：在回答中，每当你引用或提到文档片段中的内容时，必须在引用内容的末尾立即添加对应的文档编号标注\n")
	builder.WriteString("   - 引用文档片段1的内容，在引用内容末尾添加①\n")
	builder.WriteString("   - 引用文档片段2的内容，在引用内容末尾添加②\n")
	builder.WriteString("   - 引用文档片段3的内容，在引用内容末尾添加③\n")
	builder.WriteString("   - 以此类推\n")
	builder.WriteString("   - 如果一段内容来自多个文档片段，可以添加多个编号，如①②\n")
	builder.WriteString("   - **重要**：直接回答问题，不要在答案中添加\"根据文档片段X\"、\"文档片段X提到\"等前缀\n")
	builder.WriteString("   - **示例**：如果文档片段1提到\"培训要求包括3项\"，你的回答应该是：\"培训要求包括3项①\"，而不是\"根据文档片段1，培训要求包括3项①\"\n")
	builder.WriteString("   - **重要**：每个引用都必须有标注，没有标注的引用是不被允许的\n\n")
	builder.WriteString("**回答格式要求**：\n")
	builder.WriteString("- 回答应该是完整的、有逻辑的段落或列表，而不是简单的文档片段拼接\n")
	builder.WriteString("- 如果问题需要多个方面的回答，请分点或分段说明\n")
	builder.WriteString("- 回答要自然流畅，读起来像是一个专业助手在回答问题\n\n")
	builder.WriteString("上下文信息：\n")

	// 限制每个文档片段的长度，避免提示词过长
	maxDocLength := 800 // 增加长度以保留更多上下文
	for i, doc := range results {
		// 使用圆圈数字作为文档编号标记
		docNumber := getCircleNumber(i + 1)
		builder.WriteString(fmt.Sprintf("\n[文档片段 %d] %s\n", i+1, docNumber))
		content := doc.PageContent
		if len(content) > maxDocLength {
			content = content[:maxDocLength] + "..."
		}
		builder.WriteString(content)
		builder.WriteString("\n")

		// 添加来源信息
		if source, ok := doc.Metadata["source"].(string); ok {
			builder.WriteString(fmt.Sprintf("来源: %s\n", source))
		}
	}

	builder.WriteString("\n问题: ")
	builder.WriteString(question)
	builder.WriteString("\n\n**请仔细阅读上述上下文信息，深入思考和分析后，组织成完整、有条理的回答。**\n")
	builder.WriteString("\n**重要提示**：\n")
	builder.WriteString("- **首先检查每个文档片段是否与问题相关**：只使用真正相关的片段，忽略不相关的片段\n")
	builder.WriteString("- 不要直接复制粘贴文档片段，要对信息进行理解和整合\n")
	builder.WriteString("- 回答要完整、有逻辑，读起来像是一个专业助手在回答问题\n")
	builder.WriteString("- 每当你引用文档片段中的任何内容时，必须在引用内容的末尾添加对应的文档编号标注（①、②、③等）\n")
	builder.WriteString("- 如果没有添加标注，你的回答将被视为不完整\n")
	builder.WriteString("- **重要**：直接回答问题，不要添加\"根据文档片段X\"、\"文档片段X提到\"等前缀\n")
	builder.WriteString("- 示例格式：\"培训要求包括：提供系统管理员培训不少于3天①\"（正确）\n")
	builder.WriteString("- 错误示例：\"根据文档片段1，培训要求包括：提供系统管理员培训不少于3天①\"（错误，不要添加前缀）\n")
	builder.WriteString("- **如果所有文档片段都与问题无关，请明确说明\"根据提供的上下文，我无法找到相关信息\"，不要强行使用不相关的信息**\n")
	builder.WriteString("\n现在请**首先检查每个文档片段的相关性**，然后**深入思考和分析**真正相关的上下文信息，最后组织成完整、有条理的回答，确保所有引用都包含文档编号标注：\n")

	return builder.String()
}

// reRankResults 对搜索结果进行重排序，优先选择包含查询关键词的片段
func (r *RAG) reRankResults(question string, allResults []schema.Document, topK int) []schema.Document {
	if len(allResults) <= topK {
		return allResults
	}

	// 提取查询关键词（去除常见停用词）
	lowerQuestion := strings.ToLower(question)
	// 先尝试匹配完整短语
	fullPhrase := lowerQuestion

	// 提取关键词（去除"的"、"有"、"几"、"条"等常见词）
	stopWords := map[string]bool{
		"的": true, "有": true, "几": true, "条": true, "是": true,
		"在": true, "和": true, "或": true, "与": true,
	}
	keywords := []string{}

	// 对于中文，需要按字符处理
	// 先尝试提取2-4字的短语（如"培训要求"）
	runes := []rune(lowerQuestion)
	for i := 0; i < len(runes)-1; i++ {
		for length := 2; length <= 4 && i+length <= len(runes); length++ {
			phrase := string(runes[i : i+length])
			// 检查是否包含停用词
			hasStopWord := false
			for _, r := range phrase {
				if stopWords[string(r)] {
					hasStopWord = true
					break
				}
			}
			if !hasStopWord {
				keywords = append(keywords, phrase)
			}
		}
	}

	// 如果关键词为空，使用完整短语
	if len(keywords) == 0 {
		keywords = []string{fullPhrase}
	}

	// 调试：显示提取的关键词
	fmt.Printf("[调试] 提取的关键词: %v, 完整短语: %s\n", keywords, fullPhrase)

	// 计算每个片段的关键词匹配分数
	type scoredDoc struct {
		doc   schema.Document
		score int
		index int
	}

	scoredDocs := make([]scoredDoc, len(allResults))
	for i, doc := range allResults {
		lowerContent := strings.ToLower(doc.PageContent)
		score := 0

		// 优先匹配完整短语
		if strings.Contains(lowerContent, fullPhrase) {
			score += 100 // 完整短语匹配给高分
		}

		// 尝试匹配去除停用词后的短语（如"培训要求"）
		// 对于中文，需要按字符处理
		phraseWithoutStopWords := ""
		runes := []rune(lowerQuestion)
		for _, r := range runes {
			char := string(r)
			if !stopWords[char] {
				phraseWithoutStopWords += char
			}
		}
		// 支持空格分隔的匹配
		contentNoSpace := strings.ReplaceAll(lowerContent, " ", "")
		if phraseWithoutStopWords != "" && (strings.Contains(lowerContent, phraseWithoutStopWords) || strings.Contains(contentNoSpace, phraseWithoutStopWords)) {
			score += 80 // 去除停用词后的短语匹配给高分
		}

		// 计算关键词匹配分数
		matchedKeywords := 0
		for _, keyword := range keywords {
			// 支持短语匹配（即使被空格分隔）
			keywordPattern := strings.ReplaceAll(keyword, " ", "")
			contentNoSpace := strings.ReplaceAll(lowerContent, " ", "")
			if strings.Contains(contentNoSpace, keywordPattern) || strings.Contains(lowerContent, keyword) {
				score += 20 // 每个匹配的关键词加20分
				matchedKeywords++
				// 如果关键词在标题或重要位置，额外加分
				if strings.HasPrefix(lowerContent, keyword) || strings.Contains(lowerContent, keyword+" ") {
					score += 10
				}
			}
		}

		// 如果匹配了所有关键词，额外加分
		if matchedKeywords == len(keywords) && len(keywords) > 0 {
			score += 50
		}

		// 保持原始顺序作为次要排序（索引越小，分数越高）
		scoredDocs[i] = scoredDoc{
			doc:   doc,
			score: score - i, // 减去索引，保持原始顺序作为次要排序
			index: i,
		}
	}

	// 按分数排序（分数高的在前）
	for i := 0; i < len(scoredDocs)-1; i++ {
		for j := i + 1; j < len(scoredDocs); j++ {
			if scoredDocs[j].score > scoredDocs[i].score {
				scoredDocs[i], scoredDocs[j] = scoredDocs[j], scoredDocs[i]
			}
		}
	}

	// 调试：显示排序后的前几个片段（按分数从高到低）
	if len(scoredDocs) > 0 {
		fmt.Printf("[调试] 重排序后（按分数从高到低，前5个）: ")
		for i := 0; i < 5 && i < len(scoredDocs); i++ {
			// 计算原始分数（加上索引）
			originalScore := scoredDocs[i].score + scoredDocs[i].index
			fmt.Printf("片段%d(原始分数:%d,最终分数:%d) ", scoredDocs[i].index+1, originalScore, scoredDocs[i].score)
		}
		fmt.Println()
	}

	// 选择前topK个结果（排序后的前topK个），但只选择分数大于0的结果
	// 分数为0表示完全不相关，应该被过滤掉
	result := make([]schema.Document, 0, topK)
	for i := 0; i < len(scoredDocs) && len(result) < topK; i++ {
		// 计算原始分数（加上索引）
		originalScore := scoredDocs[i].score + scoredDocs[i].index
		// 只选择分数大于0的结果（至少匹配了一些关键词）
		if originalScore > 0 {
			result = append(result, scoredDocs[i].doc)
		}
	}

	// 调试：显示选择的结果
	if len(result) > 0 {
		fmt.Printf("[调试] 选择的结果（前%d个，已过滤不相关片段）: ", len(result))
		for i := 0; i < len(result) && i < 3; i++ {
			// 找到这个文档在原始结果中的索引
			originalIndex := -1
			for j, doc := range allResults {
				if doc.PageContent == result[i].PageContent {
					originalIndex = j
					break
				}
			}
			fmt.Printf("结果%d(原始索引:%d) ", i+1, originalIndex+1)
		}
		fmt.Println()
	} else {
		fmt.Printf("[警告] 重排序后没有找到相关片段，将使用原始结果的前%d个\n", topK)
		// 如果过滤后没有结果，至少返回前topK个（即使相关性不高）
		for i := 0; i < topK && i < len(allResults); i++ {
			result = append(result, allResults[i])
		}
	}

	return result
}

// filterRelevantResults 二次验证：过滤掉与问题不真正相关的文档片段
// 通过检查文档内容是否真正包含问题的核心信息来判断相关性
func (r *RAG) filterRelevantResults(question string, results []schema.Document) []schema.Document {
	if len(results) == 0 {
		return results
	}

	// 提取问题的核心关键词（去除停用词）
	lowerQuestion := strings.ToLower(question)
	stopWords := map[string]bool{
		"的": true, "有": true, "几": true, "条": true, "是": true,
		"在": true, "和": true, "或": true, "与": true, "什么": true,
		"怎么": true, "如何": true, "哪些": true, "？": true,
		"?": true, "：": true, ":": true, "，": true, ",": true,
		"。": true, ".": true, "！": true, "!": true,
	}

	// 提取核心关键词（去除停用词后的连续字符）
	coreKeywords := []string{}
	runes := []rune(lowerQuestion)
	currentKeyword := ""
	for _, r := range runes {
		char := string(r)
		if !stopWords[char] && char != " " {
			currentKeyword += char
		} else {
			if len(currentKeyword) >= 2 {
				coreKeywords = append(coreKeywords, currentKeyword)
			}
			currentKeyword = ""
		}
	}
	if len(currentKeyword) >= 2 {
		coreKeywords = append(coreKeywords, currentKeyword)
	}

	// 如果无法提取关键词，返回所有结果
	if len(coreKeywords) == 0 {
		fmt.Printf("[调试] 无法提取核心关键词，保留所有结果\n")
		return results
	}

	fmt.Printf("[调试] 提取的核心关键词: %v\n", coreKeywords)

	// 过滤结果：只保留包含至少一个核心关键词的片段
	filtered := make([]schema.Document, 0, len(results))
	for i, doc := range results {
		lowerContent := strings.ToLower(doc.PageContent)
		contentNoSpace := strings.ReplaceAll(lowerContent, " ", "")

		// 检查是否包含至少一个核心关键词
		hasRelevantKeyword := false
		for _, keyword := range coreKeywords {
			keywordNoSpace := strings.ReplaceAll(keyword, " ", "")
			if strings.Contains(lowerContent, keyword) || strings.Contains(contentNoSpace, keywordNoSpace) {
				hasRelevantKeyword = true
				break
			}
		}

		if hasRelevantKeyword {
			filtered = append(filtered, doc)
		} else {
			fmt.Printf("[调试] 过滤掉片段 %d（不包含核心关键词）\n", i+1)
		}
	}

	// 如果过滤后没有结果，至少保留前3个（避免完全无结果）
	if len(filtered) == 0 && len(results) > 0 {
		fmt.Printf("[警告] 相关性过滤后没有结果，保留前3个原始结果\n")
		maxKeep := 3
		if len(results) < maxKeep {
			maxKeep = len(results)
		}
		return results[:maxKeep]
	}

	fmt.Printf("[调试] 相关性过滤：从 %d 个结果过滤到 %d 个相关结果\n", len(results), len(filtered))
	return filtered
}

// getCircleNumber 获取圆圈数字（①、②、③等）
func getCircleNumber(n int) string {
	circleNumbers := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩"}
	if n >= 1 && n <= len(circleNumbers) {
		return circleNumbers[n-1]
	}
	// 如果超过10，使用数字加圆圈
	return fmt.Sprintf("(%d)", n)
}

// AddDocuments 添加文档到知识库（并发优化版本）
func (r *RAG) AddDocuments(ctx context.Context, docs []schema.Document) error {
	if len(docs) == 0 {
		return nil
	}

	// 根据文档数量自动调整批次大小
	// 注意：硅基流动API最大批次大小为32，但为了避免触发TPM（每分钟token数）限制
	// 需要减小批次大小并增加延迟
	// 少量文档使用较小批次，大量文档使用更小的批次以避免速率限制
	var batchSize int
	if len(docs) < 50 {
		batchSize = 10 // 少量文档：10个/批（减小批次避免速率限制）
	} else if len(docs) < 200 {
		batchSize = 15 // 中等文档：15个/批（减小批次避免速率限制）
	} else {
		batchSize = 10 // 大量文档：10个/批（减小批次避免触发TPM限制）
	}

	totalBatches := (len(docs) + batchSize - 1) / batchSize
	startTime := time.Now()

	fmt.Printf("使用批次大小: %d，共 %d 批\n", batchSize, totalBatches)

	for i := 0; i < len(docs); i += batchSize {
		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}

		batch := docs[i:end]
		batchNum := (i / batchSize) + 1
		batchStartTime := time.Now()

		fmt.Printf("正在处理第 %d/%d 批 (%d 个文档)...", batchNum, totalBatches, len(batch))

		// 存储到向量数据库（会自动批量向量化）
		// 添加重试机制，处理速率限制错误
		var err error
		maxRetries := 3
		retryDelay := 2 * time.Second

		for retry := 0; retry < maxRetries; retry++ {
			err = r.store.AddDocuments(ctx, batch, r.embedder.GetEmbedder())

			if err == nil {
				break // 成功，退出重试循环
			}

			// 检查是否是速率限制错误
			errMsg := err.Error()
			isRateLimit := strings.Contains(errMsg, "rate limiting") ||
				strings.Contains(errMsg, "rate limit") ||
				strings.Contains(errMsg, "TPM limit") ||
				strings.Contains(errMsg, "tokens per minute")

			if isRateLimit && retry < maxRetries-1 {
				// 速率限制错误，等待后重试（指数退避）
				waitTime := retryDelay * time.Duration(1<<uint(retry)) // 2秒, 4秒, 8秒
				fmt.Printf(" ⚠️  遇到速率限制，等待 %v 后重试 (第 %d/%d 次重试)...\n", waitTime.Round(time.Second), retry+1, maxRetries)
				time.Sleep(waitTime)
				continue
			}

			// 其他错误或重试次数用完，直接返回错误
			break
		}

		if err != nil {
			return fmt.Errorf("failed to add batch %d to store: %w", batchNum, err)
		}

		// 批次之间添加延迟，避免触发速率限制
		// 根据批次大小计算延迟时间（每个文档约0.1秒延迟）
		if batchNum < totalBatches {
			delay := time.Duration(len(batch)) * 100 * time.Millisecond // 每个文档100ms延迟
			if delay > 2*time.Second {
				delay = 2 * time.Second // 最大延迟2秒
			}
			if delay < 500*time.Millisecond {
				delay = 500 * time.Millisecond // 最小延迟500ms
			}
			time.Sleep(delay)
		}

		batchDuration := time.Since(batchStartTime)
		processedCount := i + len(batch)
		elapsed := time.Since(startTime)
		avgTimePerDoc := elapsed / time.Duration(processedCount)
		remainingDocs := len(docs) - processedCount
		estimatedRemaining := time.Duration(remainingDocs) * avgTimePerDoc

		fmt.Printf(" ✅ 完成 (耗时: %v, 已处理: %d/%d, 预计剩余: %v, 速度: %.1f 文档/秒)\n",
			batchDuration.Round(time.Second), processedCount, len(docs), estimatedRemaining.Round(time.Second),
			float64(len(batch))/batchDuration.Seconds())
	}

	totalDuration := time.Since(startTime)
	fmt.Printf("\n🎉 全部完成！共处理 %d 个文档，总耗时: %v，平均: %v/文档，速度: %.1f 文档/秒\n",
		len(docs), totalDuration.Round(time.Second),
		(totalDuration / time.Duration(len(docs))).Round(time.Millisecond),
		float64(len(docs))/totalDuration.Seconds())

	return nil
}
