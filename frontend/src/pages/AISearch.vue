<template>
  <div class="page">
    <section class="hero">
      <div>
        <p class="tag">自然资源和规划行业</p>
        <h1>AI 知识 · 问答</h1>
        <!-- <p class="sub">基于行业知识库即时检索生成答案。</p> -->
        <p class="sub">知识库已收录政策文件{{ fileCount }}份</p>
      </div>
      <div class="action-buttons">
        <button class="help-btn" @click="showHelp = true" title="查看帮助文档">
          <span class="help-text">帮助文档</span>
        </button>
        <button class="feedback-btn-top" @click="handleOpenFeedback" title="意见反馈">
          <span class="feedback-text">意见反馈</span>
        </button>
      </div>
    </section>
    <div class="search-panel">
      <form @submit.prevent="handleSearch" class="search-box">
        <input
          type="text"
          v-model="query"
          placeholder="输入问题 如：临时用地的期限是多久？"
          class="search-input"
        />
        <button type="submit" :disabled="!query.trim() || searching" class="search-btn">
          {{ searching ? '思考中...' : 'AI 搜索' }}
        </button>
      </form>

      <div class="results" v-if="searchAnswer || searchResults.length > 0">
        <!-- AI生成的答案 -->
        <div v-if="searchAnswer" class="ai-answer">
          <div class="answer-header">
            <h3>AI 答案</h3>
            <span class="ai-badge">基于知识库生成</span>
          </div>
          <div class="answer-content" v-html="formatAnswer(searchAnswer)" @click="handleAnnotationClick"></div>
        </div>

        <!-- 相关文档 - 按文档类型分组 -->
        <div v-if="shouldShowRelatedDocs" class="related-docs">
          <div class="results-header">
            <p v-if="docGroups.length > 0">
              找到 {{ docGroups.length }} 个相关文档，共 {{ totalChunks }} 个相关片段
            </p>
            <p v-else>
              找到 {{ searchResults.length }} 个相关文档片段
            </p>
          </div>
          
          <!-- 按文档类型分组展示（新格式） -->
          <div v-if="docGroups.length > 0" class="doc-groups">
            <div
              v-for="(group, groupIndex) in docGroups"
              :key="groupIndex"
              class="doc-group"
            >
              <div class="doc-group-header">
                <h3 class="doc-group-title">
                  <span class="doc-icon">{{ group.sourceType === 'url' ? '🌐' : '📄' }}</span>
                  {{ group.docTitle || '未命名文档' }}
                </h3>
                <div class="doc-group-actions">
                  <span class="doc-group-count">{{ group.chunks.length }} 个相关片段</span>
                  <button
                    v-if="canDownload(group)"
                    @click="downloadFile(group.fileId, group.docTitle, group)"
                    class="download-btn"
                    title="下载文档"
                  >
                    📥 下载
                  </button>
                </div>
              </div>
              <div class="doc-group-chunks">
                <article
                  v-for="(chunk, chunkIndex) in group.chunks"
                  :key="chunkIndex"
                  :id="`doc-${chunk.index || chunkIndex + 1}`"
                  class="result chunk"
                >
                  <div class="chunk-header">
                    <span class="chunk-index">片段 {{ chunk.index || chunkIndex + 1 }}</span>
                    <span v-if="chunk.score" class="result-score">相关度 {{ chunk.score.toFixed(3) }}</span>
                  </div>
                  <p>{{ chunk.content || chunk.pageContent || chunk.preview }}</p>
                  <div v-if="group.docSource && group.sourceType === 'url'" class="result-source">
                    <small>来源: {{ group.docSource }}</small>
                  </div>
                </article>
              </div>
            </div>
          </div>

          <!-- 平铺格式（兼容旧格式，如果没有docGroups则使用） -->
          <div v-else class="doc-groups">
            <article
              v-for="(item, index) in searchResults"
              :key="index"
              :id="`doc-${item.index || index + 1}`"
              class="result"
            >
              <header>
                <h3>{{ item.title || '未命名文档' }}</h3>
                <span v-if="item.score" class="result-score">相关度 {{ item.score.toFixed(3) }}</span>
              </header>
              <p>{{ item.content || item.pageContent || item.preview }}</p>
              <div v-if="item.source" class="result-source">
                <small>来源: {{ item.source }}</small>
              </div>
            </article>
          </div>
        </div>
      </div>
      <div class="empty" v-else-if="searching">
        <p>正在思考...</p>
      </div>
      <div class="empty error" v-else-if="searchError">
        <p>{{ searchError }}</p>
      </div>
      <div class="empty" v-else>
        <p>还没有搜索结果，试着提问吧。</p>
      </div>
    </div>

    <!-- 帮助文档弹窗 -->
    <div v-if="showHelp" class="help-modal-overlay" @click="showHelp = false">
      <div class="help-modal" @click.stop>
        <div class="help-modal-header">
          <h2>📖 帮助文档</h2>
          <button class="close-btn" @click="showHelp = false">×</button>
        </div>
        <div class="help-modal-content">
          <section class="help-section">
            <h3>🎯 功能介绍</h3>
            <ol>
              <li>本AI知识库包含自然资源和规划、住建、发展改革、林业、水利、生态环境等领域的法律、法规、规章、规范性文件等文件近千份，
                暂时未包含国家、行业及地方标准规范，后续会陆续更新最新政策文件及标准规范，保证知识库的全面性、时效性。</li>
              <li>AI知识问答系统基于RAG（检索增强生成）技术，能够从知识库中检索相关信息并生成精准答案。
                与通用AI问答助手相比，本AI知识问答系统优点在于聚焦行业政策，可辅助规划设计人员高效开展规划编制、政策研究、项目咨询等专业工作。</li>
            </ol>
          </section>

          <section class="help-section">
            <h3>💡 如何使用</h3>
            <ol>
              <li><strong>输入问题</strong>：在搜索框中输入您的问题，例如：
                <ul>
                  <li>"城市更新项目有哪些用地政策？"</li>
                  <li>"土地出让后，如何调整规划条件？"</li>
                  <li>"什么叫多测合一？"</li>
                </ul>
              </li>
              <li><strong>点击搜索</strong>：点击"AI 搜索"按钮，系统会自动检索知识库</li>
              <li><strong>查看答案</strong>：AI会基于检索到的文档生成答案，答案中的标注（①、②、③等）对应下方的文档片段</li>
              <li><strong>查看来源</strong>：点击答案中的标注可以跳转到对应的文档片段，查看详细内容</li>
            </ol>
          </section>

          <!-- <section class="help-section">
            <h3>📚 文档分组</h3>
            <p>检索结果会按照文档来源自动分组显示：</p>
            <ul>
              <li><strong>📄 文件文档</strong>：来自上传的PDF、Word、TXT等文件</li>
              <li><strong>🌐 网页文档</strong>：来自网页URL的内容</li>
            </ul>
            <p>每个文档分组下会显示所有相关的文本片段，方便您查看完整的上下文信息。</p>
          </section> -->

          <section class="help-section">
            <h3>✨ 功能特点</h3>
            <ul>
              <li><strong>智能检索</strong>：基于向量相似度的语义搜索，理解问题意图</li>
              <li><strong>精准答案</strong>：AI基于知识库内容生成答案，确保准确性</li>
              <li><strong>来源标注</strong>：答案中的每个引用都有标注，可快速定位来源</li>
              <!-- <li><strong>文档分组</strong>：按文档类型分组展示，结构清晰</li> -->
              <li><strong>快速跳转</strong>：点击标注即可跳转到对应文档片段</li>
            </ul>
          </section>

          <section class="help-section">
            <h3>💬 提问技巧</h3>
            <ul>
              <li><strong>具体明确</strong>：问题越具体，答案越准确</li>
              <li><strong>使用关键词</strong>：包含关键术语的问题更容易匹配到相关内容</li>
              <li><strong>自然语言</strong>：可以用自然语言提问，无需特殊格式</li>
              <li><strong>多角度提问</strong>：如果第一次没找到答案，可以换个角度重新提问</li>
            </ul>
          </section>

          <section class="help-section">
            <h3>❓ 常见问题</h3>
            <div class="faq-item">
              <strong>Q: 为什么没有找到相关答案？</strong>
              <p>A: 可能是知识库中没有相关内容，或者问题表述不够具体。建议尝试使用不同的关键词重新提问。</p>
            </div>
            <div class="faq-item">
              <strong>Q: 答案中的标注是什么意思？</strong>
              <p>A: 标注（①、②、③等）表示答案引用的文档片段编号，点击标注可以跳转到对应的文档片段查看详细内容。</p>
            </div>
            <div class="faq-item">
              <strong>Q: 如何上传文档到知识库？</strong>
              <p>A: 需要管理员权限。请访问"知识库管理"页面，使用管理员token登录后即可上传文档。</p>
            </div>
             <div class="faq-item">
              <strong>Q: 如何下载知识库中的政策文件？</strong>
              <p>A: 由于部分政策文件涉密、涉敏，本系统暂不支持文件下载，如有需要，请联系管理员。 </p>
            </div>
             <div class="feedback-info">
              <p>意见反馈</p>
              <p>本系统为V1.0版，如有不完善的地方，还请多多包涵!请大家通过[意见反馈]对话框提出宝贵意见和建议，不胜感激！</p>
             </div>
            
          </section>
        </div>
      </div>
    </div>

    <!-- 意见反馈弹窗 -->
    <div v-if="showFeedback" class="feedback-modal-overlay" @click="showFeedback = false">
      <div class="feedback-modal" @click.stop>
        <div class="feedback-modal-header">
          <h2>意见反馈</h2>
          <button class="close-btn" @click="showFeedback = false">×</button>
        </div>
        <div class="feedback-modal-content">
          <form @submit.prevent="handleFeedbackSubmit" class="feedback-form">
            <div class="form-group">
              <label for="feedback-title">标题 <span class="required">*</span></label>
              <input
                id="feedback-title"
                v-model="feedbackForm.title"
                type="text"
                placeholder="请简要描述反馈的主题"
                required
              />
            </div>

            <div class="form-group">
              <label for="feedback-description">详细描述 <span class="required">*</span></label>
              <textarea
                id="feedback-description"
                v-model="feedbackForm.description"
                placeholder="请详细描述您的问题、建议或意见..."
                rows="6"
                required
              ></textarea>
            </div>

            <div class="form-group">
              <label for="feedback-image">图片（可选）</label>
              <div class="image-upload-area">
                <input
                  id="feedback-image"
                  type="file"
                  accept="image/*"
                  @change="handleImageSelect"
                  class="file-input"
                />
                <div v-if="!feedbackForm.imagePreview" class="upload-placeholder">
                  <span class="upload-icon">📷</span>
                  <span class="upload-text">点击选择图片或拖拽图片到此处</span>
                  <span class="upload-hint">支持 JPG、PNG、GIF 格式，最大 5MB</span>
                </div>
                <div v-else class="image-preview">
                  <img :src="feedbackForm.imagePreview" alt="预览图片" />
                  <button type="button" class="remove-image-btn" @click="removeImage">×</button>
                </div>
              </div>
            </div>

            <div class="form-actions">
              <button type="button" class="cancel-btn" @click="closeFeedback">取消</button>
              <button type="submit" class="submit-btn" :disabled="submittingFeedback">
                {{ submittingFeedback ? '提交中...' : '提交反馈' }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </div>

  <!-- 自定义提示框 -->
  <div v-if="showMessage" class="message-overlay" @click="closeMessage">
    <div class="message-modal" @click.stop>
      <div class="message-icon">🔒</div>
      <div class="message-content">{{ messageText }}</div>
      <button class="message-btn" @click="closeMessage">确定</button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, inject } from 'vue'
import axios from 'axios'

const API_BASE = '/api'
//const API_BASE = '/rest/api'
const query = ref('')
const searching = ref(false)
const searchAnswer = ref('')
const searchResults = ref([])
const docGroups = ref([])
const searchError = ref('')
const showHelp = ref(false)
const showFeedback = ref(false)
const submittingFeedback = ref(false)
const fileCount = ref(0)
const showMessage = ref(false)
const messageText = ref('')

// 获取登录状态
const isLoggedIn = inject('isLoggedIn', ref(false))

// 打开反馈弹窗前检查登录状态
function handleOpenFeedback() {
  if (!isLoggedIn.value) {
    showMessageDialog('请先登录后再提交意见反馈')
    window.dispatchEvent(new CustomEvent('show-login'))
    return
  }
  showFeedback.value = true
}

// 反馈表单数据（反馈人通过登录 user_id 自动关联，无需手动填写姓名）
const feedbackForm = ref({
  title: '',
  description: '',
  image: null,
  imagePreview: null
})

// 计算总片段数
const totalChunks = computed(() => {
  return docGroups.value.reduce((sum, group) => sum + group.chunks.length, 0)
})

// 判断答案是否表示无法找到信息
const hasNoAnswer = computed(() => {
  if (!searchAnswer.value) return false
  const answerText = searchAnswer.value.toLowerCase()
  return answerText.includes('根据提供的上下文，我无法找到相关信息') ||
         answerText.includes('无法找到相关信息') ||
         answerText.includes('没有找到相关信息') ||
         answerText.includes('抱歉，我在知识库中没有找到相关信息')
})

// 判断是否应该显示相关文档和片段
const shouldShowRelatedDocs = computed(() => {
  // 如果答案表示无法找到信息，不显示相关文档和片段
  if (hasNoAnswer.value) return false
  // 否则根据是否有文档和片段来决定
  return docGroups.value.length > 0 || searchResults.value.length > 0
})

// 获取文件数量
async function fetchFileCount() {
  try {
    const response = await axios.get(`${API_BASE}/files/count`)
    if (response.data.success) {
      fileCount.value = response.data.count || 0
    }
  } catch (error) {
    console.error('获取文件数量失败:', error)
    // 如果获取失败，保持默认值0或显示错误提示
  }
}

// 组件挂载时获取文件数量
onMounted(() => {
  fetchFileCount()
  // 每30秒刷新一次文件数量
  setInterval(fetchFileCount, 30000)
})

async function handleSearch() {
  if (!query.value.trim()) return

  // 检查用户是否登录
  if (!isLoggedIn.value) {
    showMessageDialog('请先登录才能使用AI搜索功能')
    // 通过事件通知父组件显示登录弹窗
    window.dispatchEvent(new CustomEvent('show-login'))
    return
  }

  searching.value = true
  searchAnswer.value = ''
  searchResults.value = []
  docGroups.value = []
  searchError.value = ''

  try {
    // 获取用户token
    const token = localStorage.getItem('userToken')
    const headers = {
      'Content-Type': 'application/json'
    }
    if (token && token.trim()) {
      // 确保token是有效的字符串，避免包含特殊字符导致header设置失败
      headers['Authorization'] = `Bearer ${token.trim()}`
    }

    const response = await fetch(`${API_BASE}/query?stream=1`, {
      method: 'POST',
      headers,
      body: JSON.stringify({
        question: query.value.trim(),
        topk: 8
      })
    })

    if (!response.ok) {
      let errorMessage = `请求失败: ${response.status}`
      try {
        const errJson = await response.json()
        errorMessage = errJson?.message || errJson?.error || errorMessage
      } catch (_) {}
      throw new Error(errorMessage)
    }

    if (!response.body) {
      throw new Error('浏览器不支持流式响应')
    }

    const reader = response.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = ''

    const processEventBlock = (block) => {
      const lines = block.split('\n')
      let eventName = ''
      let dataText = ''
      for (const rawLine of lines) {
        const line = rawLine.trim()
        if (line.startsWith('event:')) {
          eventName = line.slice(6).trim()
        } else if (line.startsWith('data:')) {
          dataText += line.slice(5).trim()
        }
      }

      if (!eventName || !dataText) return

      let payload = null
      try {
        payload = JSON.parse(dataText)
      } catch (_) {
        return
      }

      if (eventName === 'chunk' && payload?.text) {
        searchAnswer.value += payload.text
      } else if (eventName === 'result') {
        if (!searchAnswer.value && payload?.answer) {
          searchAnswer.value = payload.answer
        }
        if (payload?.docGroups && payload.docGroups.length > 0) {
          docGroups.value = payload.docGroups
        } else if (payload?.results) {
          searchResults.value = payload.results
        }
      } else if (eventName === 'error') {
        const message = payload?.message || payload?.error || '搜索失败，请稍后重试'
        throw new Error(message)
      }
    }

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const blocks = buffer.split('\n\n')
      buffer = blocks.pop() || ''
      for (const block of blocks) {
        processEventBlock(block)
      }
    }

    if (buffer.trim()) {
      processEventBlock(buffer)
    }
  } catch (error) {
    console.error('搜索失败:', error)
    // 处理未授权错误
    if (error.response?.status === 401) {
      searchError.value = '请先登录才能使用AI搜索功能'
      showMessageDialog('请先登录才能使用AI搜索功能')
      // 通过事件通知父组件显示登录弹窗
      window.dispatchEvent(new CustomEvent('show-login'))
      // 更新登录状态
      if (isLoggedIn.value) {
        isLoggedIn.value = false
        localStorage.removeItem('userToken')
      }
    } else {
    // 优先显示后端返回的详细错误信息（message字段），如果没有则显示error字段，最后才显示通用错误
    const errorData = error.response?.data
    searchError.value = errorData?.message || errorData?.error || error.message || '搜索失败，请稍后重试'
    }
  } finally {
    searching.value = false
  }
}

function formatAnswer(text) {
  if (!text) return ''
  
  // 检查答案是否表示无法找到信息
  const answerText = text.toLowerCase()
  const isNoAnswer = answerText.includes('根据提供的上下文，我无法找到相关信息') ||
                     answerText.includes('无法找到相关信息') ||
                     answerText.includes('没有找到相关信息') ||
                     answerText.includes('抱歉，我在知识库中没有找到相关信息')
  
  // 格式化答案，将换行转换为<br>
  let formatted = text.replace(/\n/g, '<br>')
  
  // 如果答案表示无法找到信息，移除所有序号标注
  if (isNoAnswer) {
    const circleNumbers = ['①', '②', '③', '④', '⑤', '⑥', '⑦', '⑧', '⑨', '⑩']
    circleNumbers.forEach((num) => {
      const regex = new RegExp(num, 'g')
      formatted = formatted.replace(regex, '')
    })
  } else {
    // 为标注（①、②、③等）添加可点击样式和data属性
    const circleNumbers = ['①', '②', '③', '④', '⑤', '⑥', '⑦', '⑧', '⑨', '⑩']
    circleNumbers.forEach((num, index) => {
      const regex = new RegExp(num, 'g')
      formatted = formatted.replace(regex, `<span class="annotation" data-index="${index + 1}" title="点击跳转到文档片段 ${index + 1}">${num}</span>`)
    })
  }
  
  return formatted
}

function handleAnnotationClick(event) {
  // 处理标注点击事件
  const annotation = event.target.closest('.annotation')
  if (annotation) {
    const index = annotation.getAttribute('data-index')
    if (index) {
      // 滚动到对应的文档片段
      const docElement = document.getElementById(`doc-${index}`)
      if (docElement) {
        docElement.scrollIntoView({ behavior: 'smooth', block: 'center' })
        // 高亮显示文档片段
        docElement.classList.add('highlight')
        setTimeout(() => {
          docElement.classList.remove('highlight')
        }, 2000)
      }
    }
  }
}

// 处理图片选择
function handleImageSelect(event) {
  const file = event.target.files[0]
  if (!file) return

  // 检查文件类型
  if (!file.type.startsWith('image/')) {
    alert('请选择图片文件（JPG、PNG、GIF等）')
    return
  }

  // 检查文件大小（5MB）
  if (file.size > 5 * 1024 * 1024) {
    alert('图片大小不能超过 5MB')
    return
  }

  // 保存文件
  feedbackForm.value.image = file

  // 创建预览
  const reader = new FileReader()
  reader.onload = (e) => {
    feedbackForm.value.imagePreview = e.target.result
  }
  reader.readAsDataURL(file)
}

// 移除图片
function removeImage() {
  feedbackForm.value.image = null
  feedbackForm.value.imagePreview = null
  // 重置文件输入
  const fileInput = document.getElementById('feedback-image')
  if (fileInput) {
    fileInput.value = ''
  }
}

// 关闭反馈弹窗
function closeFeedback() {
  showFeedback.value = false
  // 延迟重置表单，让关闭动画完成
  setTimeout(() => {
    feedbackForm.value = {
      title: '',
      description: '',
      image: null,
      imagePreview: null
    }
  }, 300)
}

// 判断是否可以下载
function canDownload(group) {
  if (!group) {
    return false
  }
  
  // 如果不是文件类型，不允许下载
  if (group.sourceType !== 'file' || !group.fileId) {
    return false
  }
  
  // 对于pdf、word、txt文档，检查是否包含"公开形式"字眼
  if (group.fileType) {
    const fileTypeLower = group.fileType.toLowerCase()
    if (fileTypeLower === 'pdf' || fileTypeLower === 'doc' || fileTypeLower === 'docx' || fileTypeLower === 'txt') {
      // 检查 hasPublicForm 字段（可能是 true、'true'、1 或 undefined）
      const hasPublicForm = group.hasPublicForm === true || group.hasPublicForm === 'true' || group.hasPublicForm === 1
      
      // 如果包含"公开形式"字眼，不允许下载
      if (hasPublicForm) {
        return false
      }
      // 如果没有"公开形式"字眼，允许下载
      return true
    }
  }
  
  // 其他文档类型（非pdf/word/txt）不做限制，允许下载
  return true
}

// 下载文件
function downloadFile(fileId, filename, group) {
  if (!fileId) {
    showMessageDialog('文件ID不存在，无法下载')
    return
  }
  
  // 检查是否可以下载（使用canDownload函数统一检查）
  if (group && !canDownload(group)) {
    showMessageDialog('此文件涉密文件 不提供下载按钮')
    return
  }
  
  const url = `${API_BASE}/files/${fileId}`
  const link = document.createElement('a')
  link.href = url
  link.download = filename || 'document'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

// 显示提示框
function showMessageDialog(text) {
  messageText.value = text
  showMessage.value = true
}

// 关闭提示框
function closeMessage() {
  showMessage.value = false
  setTimeout(() => {
    messageText.value = ''
  }, 300)
}

// 提交反馈
async function handleFeedbackSubmit() {
  if (!feedbackForm.value.title.trim() || !feedbackForm.value.description.trim()) {
    alert('请填写完整的反馈信息（标题、详细描述为必填项）')
    return
  }

  submittingFeedback.value = true

  try {
    // 创建FormData以支持文件上传
    const formData = new FormData()
    // name 由后端根据 token 自动填充当前登录用户名
    formData.append('name', '')
    formData.append('title', feedbackForm.value.title)
    formData.append('description', feedbackForm.value.description)
    if (feedbackForm.value.image) {
      formData.append('image', feedbackForm.value.image)
    }

    // 提交到后端API（带上 token 以便后端识别当前登录用户）
    const userToken = localStorage.getItem('userToken')
    const requestHeaders = {
          'Content-Type': 'multipart/form-data'
        }
    if (userToken) {
      requestHeaders['Authorization'] = `Bearer ${userToken}`
    }

    try {
      await axios.post(`${API_BASE}/feedback`, formData, {
        headers: requestHeaders
      })
      alert('感谢您的反馈！我们会认真对待每一条建议。')
    } catch (apiError) {
      console.log('反馈内容:', {
        title: feedbackForm.value.title,
        description: feedbackForm.value.description,
        image: feedbackForm.value.image ? feedbackForm.value.image.name : null
      })
      alert('感谢您的反馈！我们会认真对待每一条建议。\n（提示：后端反馈接口尚未配置，反馈内容已记录在控制台）')
    }
    
    // 关闭弹窗并重置表单
    closeFeedback()
  } catch (error) {
    console.error('提交反馈失败:', error)
    alert('提交失败，请稍后重试')
  } finally {
    submittingFeedback.value = false
  }
}
</script>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.hero {
  background: white;
  border-radius: 20px;
  padding: 24px;
  box-shadow: 0 15px 45px rgba(15, 23, 42, 0.08);
  position: relative;
}

.action-buttons {
  position: absolute;
  top: 0;
  right: 24px;
  width: auto;
  height: 100%;
}

.help-btn {
  position: absolute;
  top: 24px;
  right: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 8px 12px;
  min-width: 85px;
  height: 36px;
  background: #f1f5f9;
  color: #1e293b;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  cursor: pointer;
  font-size: 14px;
  line-height: 1.2;
  font-weight: 500;
  transition: all 0.3s;
  white-space: nowrap;
}

.help-btn:hover {
  background: #e2e8f0;
}

.feedback-btn-top {
  position: absolute;
  bottom: 24px;
  right: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 8px 12px;
  min-width: 85px;
  height: 36px;
  background: #f0f4f7;
  color: #1e293b;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  cursor: pointer;
  font-size: 14px;
  line-height: 1.2;
  font-weight: 500;
  transition: all 0.3s;
  white-space: nowrap;
}

.feedback-btn-top:hover {
  background: #e2e8f0;
}

.feedback-icon {
  font-size: 14px;
}

.feedback-text {
  font-size: 14px;
}

.help-icon {
  font-size: 14px;
}

.help-text {
  font-size: 14px;
}

.tag {
  color: #6366f1;
  letter-spacing: 0.2em;
  font-size: 12px;
  text-transform: uppercase;
}

.hero h1 {
  margin: 12px 0 8px;
  font-size: 32px;
}

.sub {
  color: #475569;
  margin: 0;
}

.search-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.search-box {
  display: flex;
  gap: 12px;
}

.search-input {
  flex: 1;
  border-radius: 14px;
  padding: 16px;
  border: 1px solid #dbeafe;
  background: #f8fafc;
  font-size: 16px;
  font-family: inherit;
}

.search-input:focus {
  outline: none;
  border-color: #6366f1;
}

.search-btn {
  min-width: 140px;
  border-radius: 14px;
  border: none;
  background: linear-gradient(120deg, #0ea5e9, #6366f1);
  color: white;
  font-weight: 600;
  cursor: pointer;
  font-size: 16px;
  padding: 16px 24px;
}

.search-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.results {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.result {
  border-radius: 16px;
  border: 1px solid #e2e8f0;
  padding: 16px;
  background: white;
  transition: all 0.3s;
}

.result.highlight {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2);
  background: #f0f4ff;
}

.result header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.result h3 {
  margin: 0;
  color: #333;
  font-size: 16px;
}

.result-score {
  font-size: 12px;
  color: #6366f1;
}

.result p {
  color: #475569;
  line-height: 1.6;
  margin: 8px 0;
}

.result-source {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid #e2e8f0;
}

.result-source small {
  color: #64748b;
  font-size: 12px;
}

.empty {
  padding: 32px;
  border: 1px dashed #cbd5f5;
  border-radius: 16px;
  text-align: center;
  color: #475569;
  background: white;
}

.empty.error {
  border-color: #fca5a5;
  background: #fef2f2;
  color: #991b1b;
}

.results-header {
  margin-bottom: 12px;
  padding: 8px 12px;
  background: #eef2ff;
  border-radius: 8px;
  font-size: 14px;
  color: #6366f1;
  font-weight: 500;
}

.ai-answer {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 16px;
  padding: 24px;
  margin-bottom: 24px;
  color: white;
  box-shadow: 0 10px 30px rgba(102, 126, 234, 0.3);
}

.answer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.answer-header h3 {
  margin: 0;
  font-size: 20px;
  color: white;
}

.ai-badge {
  background: rgba(255, 255, 255, 0.2);
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}

.answer-content {
  line-height: 1.8;
  font-size: 15px;
  color: rgba(255, 255, 255, 0.95);
}

.answer-content :deep(.annotation) {
  cursor: pointer;
  background: rgba(255, 255, 255, 0.3);
  padding: 2px 6px;
  border-radius: 4px;
  margin: 0 2px;
  transition: all 0.2s;
  display: inline-block;
}

.answer-content :deep(.annotation:hover) {
  background: rgba(255, 255, 255, 0.5);
  transform: scale(1.1);
}

.related-docs {
  margin-top: 24px;
}

.doc-groups {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.doc-group {
  border-radius: 16px;
  border: 1px solid #e2e8f0;
  background: white;
  overflow: hidden;
}

.doc-group-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  background: linear-gradient(135deg, #f0f4ff 0%, #e0e7ff 100%);
  border-bottom: 2px solid #c7d2fe;
}

.doc-group-title {
  margin: 0;
  color: #333;
  font-size: 18px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 8px;
}

.doc-icon {
  font-size: 20px;
}

.doc-group-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.doc-group-count {
  font-size: 13px;
  color: #6366f1;
  background: rgba(99, 102, 241, 0.1);
  padding: 4px 12px;
  border-radius: 12px;
  font-weight: 500;
}

.download-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 8px 12px;
  min-width: 85px;
  height: 36px;
  background: #f1f5f9;
  color: #1e293b;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  cursor: pointer;
  font-size: 14px;
  line-height: 1.2;
  font-weight: 500;
  transition: all 0.3s;
  white-space: nowrap;
}

.download-btn:hover {
  background: #e2e8f0;
}

.download-restriction {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 8px 12px;
  min-width: fit-content;
  height: 36px;
  background: #fee2e2;
  color: #991b1b;
  border: 1px solid #fca5a5;
  border-radius: 10px;
  font-size: 14px;
  line-height: 1.2;
  font-weight: 500;
  white-space: nowrap;
  cursor: help;
  box-shadow: 0 2px 8px rgba(252, 165, 165, 0.3);
}

.doc-group-chunks {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
}

.result.chunk {
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 14px;
  background: #f8fafc;
  transition: all 0.3s;
}

.result.chunk:hover {
  border-color: #c7d2fe;
  background: #f0f4ff;
}

.result.chunk.highlight {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.2);
  background: #eef2ff;
}

.chunk-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.chunk-index {
  font-size: 13px;
  color: #6366f1;
  font-weight: 600;
  background: rgba(99, 102, 241, 0.1);
  padding: 2px 8px;
  border-radius: 6px;
}

/* 移动端适配 */
@media (max-width: 768px) {
  .page {
    gap: 16px;
  }

  .hero {
    padding: 16px;
    border-radius: 16px;
    position: relative;
  }

  .hero h1 {
    font-size: 24px;
    margin: 8px 0 6px;
  }

  .tag {
    font-size: 11px;
  }

  .sub {
    font-size: 14px;
  }

  .search-box {
    flex-direction: column;
    gap: 12px;
  }

  .search-input {
    padding: 14px;
    font-size: 16px; /* 防止iOS自动缩放 */
    border-radius: 12px;
  }

  .search-btn {
    width: 100%;
    min-width: auto;
    padding: 14px 20px;
    font-size: 15px;
    /* 帮助文档按钮的背景色 */
    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%) !important;
    box-shadow: 0 4px 12px rgba(102, 126, 234, 0.3) !important;
  }

  .search-btn:hover:not(:disabled) {
    box-shadow: 0 6px 16px rgba(102, 126, 234, 0.4) !important;
  }

  .action-buttons {
    top: 0;
    right: 16px;
    height: 100%;
  }

  .help-btn {
    top: 16px;
    padding: 0 8px;
    min-width: 75px;
    height: auto;
    font-size: 12px;
    line-height: 1.2;
    align-items: center;
    justify-content: center;
    background: #f1f5f9;
    color: #1e293b;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
  }

  .help-btn:hover {
    background: #e2e8f0;
  }

  .feedback-btn-top {
    bottom: 5px;
    padding: 0 8px;
    min-width: 75px;
    height: auto;
    font-size: 12px;
    line-height: 1.2;
    align-items: center;
    justify-content: center;
    background: #f0f4f7;
    color: #1e293b;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
  }

  .feedback-btn-top:hover {
    background: #e2e8f0;
  }

  .feedback-icon {
    font-size: 11px;
  }

  .feedback-text {
    font-size: 12px;
  }

  .help-text {
    font-size: 12px;
  }

  .ai-answer {
    padding: 16px;
    border-radius: 12px;
    margin-bottom: 16px;
  }

  .answer-header h3 {
    font-size: 18px;
  }

  .answer-content {
    font-size: 14px;
    line-height: 1.7;
  }

  .doc-group-header {
    padding: 12px 16px;
    flex-wrap: wrap;
    gap: 8px;
  }

  .doc-group-title {
    font-size: 16px;
    width: 100%;
  }

  .doc-group-actions {
    flex-wrap: wrap;
    gap: 8px;
    width: 100%;
    justify-content: flex-end;
  }

  .doc-group-count {
    font-size: 12px;
    padding: 3px 10px;
  }

  .download-btn {
    padding: 0 8px;
    min-width: 75px;
    height: auto;
    font-size: 12px;
    line-height: 1.2;
    align-items: center;
    justify-content: center;
    background: #f1f5f9;
    color: #1e293b;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
  }

  .download-btn:hover {
    background: #e2e8f0;
  }

  .download-restriction {
    padding: 0 8px;
    min-width: 75px;
    height: auto;
    font-size: 12px;
    line-height: 1.2;
    align-items: center;
    justify-content: center;
    background: #fee2e2;
    color: #991b1b;
    border: 1px solid #fca5a5;
    border-radius: 10px;
  }

  .doc-group-chunks {
    padding: 12px;
  }

  .result.chunk {
    padding: 12px;
  }

  .chunk-header {
    flex-wrap: wrap;
    gap: 8px;
  }

  .chunk-index {
    font-size: 12px;
  }

  .result-score {
    font-size: 11px;
  }

  .results-header {
    padding: 6px 10px;
    font-size: 13px;
  }
}

@media (max-width: 480px) {
  .hero h1 {
    font-size: 20px;
  }

  .search-input {
    padding: 12px;
    font-size: 16px;
  }

  .search-btn {
    padding: 12px 18px;
    font-size: 14px;
  }

  .ai-answer {
    padding: 12px;
  }

  .answer-header h3 {
    font-size: 16px;
  }

  .answer-content {
    font-size: 13px;
  }

  .doc-group-title {
    font-size: 14px;
  }
}

/* 帮助文档弹窗样式 */
.help-modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
  animation: fadeIn 0.3s;
}

@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

.help-modal {
  background: white;
  border-radius: 20px;
  max-width: 800px;
  width: 100%;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  animation: slideUp 0.3s;
}

@keyframes slideUp {
  from {
    transform: translateY(20px);
    opacity: 0;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}

.help-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 24px;
  border-bottom: 2px solid #e2e8f0;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border-radius: 20px 20px 0 0;
}

.help-modal-header h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}

.close-btn {
  background: rgba(255, 255, 255, 0.2);
  border: none;
  color: white;
  font-size: 32px;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.3s;
  line-height: 1;
}

.close-btn:hover {
  background: rgba(255, 255, 255, 0.3);
  transform: rotate(90deg);
}

.help-modal-content {
  padding: 24px;
  overflow-y: auto;
  flex: 1;
}

.help-section {
  margin-bottom: 32px;
}

.help-section:last-child {
  margin-bottom: 0;
}

.help-section h3 {
  color: #6366f1;
  font-size: 20px;
  font-weight: 600;
  margin: 0 0 12px 0;
  padding-bottom: 8px;
  border-bottom: 2px solid #e2e8f0;
}

.help-section p {
  color: #475569;
  line-height: 1.8;
  margin: 8px 0;
}


/* 意见反馈部分的段落缩进 */
.help-section .feedback-info p {
  text-indent: 2em;
}

/* 意见反馈标题不缩进 */
.help-section .feedback-info p:first-child {
  text-indent: 0;
}

.help-section ul,
.help-section ol {
  color: #475569;
  line-height: 1.8;
  margin: 12px 0;
  padding-left: 24px;
  list-style-position: outside;
}

.help-section li {
  margin: 8px 0;
}

.help-section ul ul {
  margin-top: 8px;
  margin-bottom: 8px;
}

.help-section strong {
  color: #333;
  font-weight: 600;
}

.faq-item {
  margin-bottom: 20px;
  padding: 16px;
  background: #f8fafc;
  border-radius: 12px;
  border-left: 4px solid #6366f1;
}

.faq-item strong {
  display: block;
  color: #6366f1;
  margin-bottom: 8px;
  font-size: 15px;
}

.faq-item p {
  margin: 0;
  color: #475569;
  line-height: 1.6;
}

/* 移动端适配 */
@media (max-width: 768px) {
  .help-modal-overlay {
    padding: 10px;
    align-items: flex-end;
  }

  .help-modal {
    max-height: 95vh;
    border-radius: 16px 16px 0 0;
    max-width: 100%;
    width: 100%;
  }

  .help-modal-header {
    padding: 16px;
    border-radius: 16px 16px 0 0;
  }

  .help-modal-header h2 {
    font-size: 20px;
  }

  .close-btn {
    width: 36px;
    height: 36px;
    font-size: 28px;
  }

  .help-modal-content {
    padding: 16px;
    max-height: calc(95vh - 80px);
  }

  .help-section {
    margin-bottom: 24px;
  }

  .help-section h3 {
    font-size: 18px;
    margin-bottom: 10px;
  }

  .help-section p,
  .help-section ul,
  .help-section ol {
    font-size: 14px;
    line-height: 1.6;
  }

  .faq-item {
    padding: 12px;
    margin-bottom: 16px;
  }

  .faq-item strong {
    font-size: 14px;
  }

  .faq-item p {
    font-size: 13px;
  }

  .feedback-section {
    padding: 16px;
    margin-top: 24px;
  }

  .feedback-btn {
    padding: 10px 20px;
    font-size: 14px;
  }
}

@media (max-width: 480px) {
  .help-modal-header h2 {
    font-size: 18px;
  }

  .help-section h3 {
    font-size: 16px;
  }

  .help-section p,
  .help-section ul,
  .help-section ol {
    font-size: 13px;
  }
}

/* 意见反馈区域样式 */
.feedback-section {
  margin-top: 32px;
  padding: 24px;
  background: linear-gradient(135deg, #f0f4ff 0%, #e0e7ff 100%);
  border-radius: 16px;
  text-align: center;
  border: 2px solid #c7d2fe;
}

.feedback-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 12px 24px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border: none;
  border-radius: 24px;
  cursor: pointer;
  font-size: 16px;
  font-weight: 600;
  transition: all 0.3s;
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.3);
}

.feedback-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(102, 126, 234, 0.4);
}

.feedback-icon {
  font-size: 18px;
}

.feedback-hint {
  margin-top: 12px;
  color: #6366f1;
  font-size: 14px;
  margin-bottom: 0;
}

/* 意见反馈弹窗样式 */
.feedback-modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1001;
  padding: 20px;
  animation: fadeIn 0.3s;
}

@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

.feedback-modal {
  background: white;
  border-radius: 20px;
  max-width: 700px;
  width: 100%;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  animation: slideUp 0.3s;
}

@keyframes slideUp {
  from {
    transform: translateY(20px);
    opacity: 0;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}

.feedback-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 24px;
  border-bottom: 2px solid #e2e8f0;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border-radius: 20px 20px 0 0;
}

.feedback-modal-header h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}

.feedback-modal-content {
  padding: 24px;
  overflow-y: auto;
  flex: 1;
}

.feedback-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-group label {
  color: #333;
  font-weight: 600;
  font-size: 14px;
}

.required {
  color: #ef4444;
}

.form-group input,
.form-group textarea {
  padding: 12px;
  border: 2px solid #e2e8f0;
  border-radius: 12px;
  font-size: 14px;
  font-family: inherit;
  transition: all 0.3s;
  background: #f8fafc;
}

.form-group input:focus,
.form-group textarea:focus {
  outline: none;
  border-color: #6366f1;
  background: white;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.form-group textarea {
  resize: vertical;
  min-height: 120px;
  line-height: 1.6;
}

/* 图片上传区域 */
.image-upload-area {
  position: relative;
  border: 2px dashed #c7d2fe;
  border-radius: 12px;
  background: #f8fafc;
  transition: all 0.3s;
}

.image-upload-area:hover {
  border-color: #6366f1;
  background: #f0f4ff;
}

.file-input {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  opacity: 0;
  cursor: pointer;
  z-index: 1;
}

.upload-placeholder {
  padding: 40px 20px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.upload-icon {
  font-size: 48px;
  opacity: 0.6;
}

.upload-text {
  color: #6366f1;
  font-weight: 500;
  font-size: 16px;
}

.upload-hint {
  color: #94a3b8;
  font-size: 12px;
}

.image-preview {
  position: relative;
  padding: 12px;
  display: flex;
  justify-content: center;
  align-items: center;
}

.image-preview img {
  max-width: 100%;
  max-height: 300px;
  border-radius: 8px;
  object-fit: contain;
}

.remove-image-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: rgba(239, 68, 68, 0.9);
  color: white;
  border: none;
  font-size: 20px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.3s;
  z-index: 2;
}

.remove-image-btn:hover {
  background: rgba(239, 68, 68, 1);
  transform: scale(1.1);
}

.form-actions {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  margin-top: 8px;
}

.cancel-btn,
.submit-btn {
  padding: 12px 24px;
  border: none;
  border-radius: 12px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s;
}

.cancel-btn {
  background: #f1f5f9;
  color: #475569;
}

.cancel-btn:hover {
  background: #e2e8f0;
}

.submit-btn {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.3);
}

.submit-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(102, 126, 234, 0.4);
}

.submit-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* 移动端适配 */
@media (max-width: 768px) {
  .feedback-modal-overlay {
    padding: 10px;
    align-items: flex-end;
  }

  .feedback-modal {
    max-height: 95vh;
    border-radius: 16px 16px 0 0;
    max-width: 100%;
    width: 100%;
  }

  .feedback-modal-header {
    padding: 16px;
    border-radius: 16px 16px 0 0;
  }

  .feedback-modal-header h2 {
    font-size: 20px;
  }

  .feedback-modal-content {
    padding: 16px;
    max-height: calc(95vh - 80px);
  }

  .feedback-form {
    gap: 16px;
  }

  .form-group label {
    font-size: 13px;
  }

  .form-group input,
  .form-group textarea {
    padding: 10px;
    font-size: 14px;
    border-radius: 10px;
  }

  .form-group textarea {
    min-height: 100px;
  }

  .form-actions {
    flex-direction: column;
    gap: 10px;
    margin-top: 4px;
  }

  .cancel-btn,
  .submit-btn {
    width: 100%;
    padding: 12px 20px;
    font-size: 14px;
  }

  .image-upload-area {
    border-radius: 10px;
  }

  .upload-placeholder {
    padding: 30px 16px;
  }

  .upload-icon {
    font-size: 40px;
  }

  .upload-text {
    font-size: 14px;
  }

  .upload-hint {
    font-size: 11px;
  }

  .image-preview {
    padding: 10px;
  }

  .image-preview img {
    max-height: 200px;
  }

  .remove-image-btn {
    width: 28px;
    height: 28px;
    font-size: 18px;
  }
}

@media (max-width: 480px) {
  .feedback-modal-header h2 {
    font-size: 18px;
  }

  .form-group input,
  .form-group textarea {
    font-size: 16px; /* 防止iOS自动缩放 */
  }
}

/* 自定义提示框样式 */
.message-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10000;
  padding: 20px;
  animation: fadeIn 0.3s ease;
}

@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

.message-modal {
  background: white;
  border-radius: 16px;
  padding: 32px 24px;
  max-width: 400px;
  width: 100%;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.2);
  animation: slideUp 0.3s ease;
  text-align: center;
}

@keyframes slideUp {
  from {
    transform: translateY(20px);
    opacity: 0;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}

.message-icon {
  font-size: 48px;
  margin-bottom: 16px;
}

.message-content {
  font-size: 16px;
  color: #333;
  line-height: 1.6;
  margin-bottom: 24px;
  word-wrap: break-word;
}

.message-btn {
  width: 100%;
  padding: 12px 24px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border: none;
  border-radius: 8px;
  font-size: 16px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.3s;
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.3);
}

.message-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(102, 126, 234, 0.4);
}

.message-btn:active {
  transform: translateY(0);
}

/* 移动端适配 */
@media (max-width: 768px) {
  .message-overlay {
    padding: 16px;
  }

  .message-modal {
    padding: 24px 20px;
    border-radius: 12px;
    max-width: 100%;
  }

  .message-icon {
    font-size: 40px;
    margin-bottom: 12px;
  }

  .message-content {
    font-size: 15px;
    margin-bottom: 20px;
  }

  .message-btn {
    padding: 14px 24px;
    font-size: 15px;
  }
}
</style>

