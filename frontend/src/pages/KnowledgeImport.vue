<template>
  <div class="page">
    <section class="hero">
      <div>
        <p class="tag">自然资源和规划行业</p>
        <h1>知识入库</h1>
        <p class="sub">
          上传 PDF / Word / TXT，自动解析并保存至知识库。
        </p>
      </div>
    </section>

    <div class="uploader">
      <form class="upload-form">
        <div
          class="dropzone"
          @dragover.prevent
          @drop.prevent="handleDrop"
          :class="{ 'dragging': isDragging }"
        >
          <input 
            type="file" 
            ref="fileInput" 
            @change="handleFileSelect"
            accept=".pdf,.docx,.doc,.txt"
            multiple
            style="display: none"
          />
          <div class="dropzone-content">
            <p v-if="selectedFiles.length === 0">
              <strong>拖放或点击上传</strong><br>
              <small>支持 PDF / Word(.docx) / TXT</small>
            </p>
            <div v-else class="file-preview">
              <p><strong>已选择 {{ selectedFiles.length }} 个文件</strong></p>
              <div class="file-list-preview">
                <div v-for="(file, index) in selectedFiles" :key="index" class="file-item-preview">
                  {{ file.name }} <span class="file-size">({{ formatFileSize(file.size) }})</span>
                </div>
              </div>
            </div>
          </div>
          <button 
            type="button" 
            @click="$refs.fileInput.click()" 
            class="btn-select"
          >
            {{ selectedFiles.length === 0 ? '选择文件' : '重新选择' }}
          </button>
        </div>

        <div class="actions">
          <button
            type="button"
            class="btn-primary"
            :disabled="selectedFiles.length === 0 || uploading"
            @click="handleUpload"
          >
            {{ uploading ? '上传中...' : '开始上传' }}
          </button>
          <button 
            type="button" 
            class="btn-ghost" 
            @click="resetForm"
            :disabled="uploading"
          >
            清空
          </button>
        </div>
      </form>

      <!-- 进度条 -->
      <div v-if="uploading || uploadResults.length > 0" class="progress-section">
        <h3>上传进度</h3>
        <div class="progress-summary">
          <div class="summary-item">
            <span class="label">总计:</span>
            <span class="value">{{ totalFiles }}</span>
          </div>
          <div class="summary-item success">
            <span class="label">成功:</span>
            <span class="value">{{ successCount }}</span>
          </div>
          <div class="summary-item error">
            <span class="label">失败:</span>
            <span class="value">{{ failCount }}</span>
          </div>
        </div>
        <div class="progress-bar-container">
          <div 
            class="progress-bar" 
            :style="{ width: `${uploadProgress}%` }"
          ></div>
        </div>
        <div class="progress-text">{{ uploadProgress }}%</div>
        
        <!-- 详细结果 -->
        <div v-if="uploadResults.length > 0" class="upload-results">
          <div 
            v-for="(result, index) in uploadResults" 
            :key="index"
            class="result-item"
            :class="{ 'success': result.success, 'error': !result.success }"
          >
            <div class="result-header">
              <span class="result-filename">{{ result.filename }}</span>
              <span class="result-status">
                {{ result.success ? '✓ 成功' : '✗ 失败' }}
              </span>
            </div>
            <div class="result-message">{{ result.message }}</div>
            <div v-if="result.success && result.chunks" class="result-chunks">
              共 {{ result.chunks }} 个文本块
            </div>
          </div>
        </div>
      </div>

      <!-- 文件列表 -->
      <div class="doc-list">
        <header>
          <h3>已上传文件</h3>
          <span>{{ filteredFiles.length }} 个</span>
        </header>
        
        <!-- 文件搜索框 -->
        <div class="file-search-box">
          <input
            type="text"
            v-model="fileSearchQuery"
            placeholder="搜索文件名、标题或内容..."
            class="file-search-input"
          />
          <button
            v-if="fileSearchQuery"
            type="button"
            @click="fileSearchQuery = ''"
            class="file-search-clear"
            title="清空搜索"
          >
            ✕
          </button>
        </div>
        
        <ul v-if="filteredFiles.length > 0">
          <li v-for="file in filteredFiles" :key="file.id">
            <div class="doc-header">
              <div>
                <h4>{{ file.filename }}</h4>
                <small>
                  {{ formatDate(file.uploadedAt) }}
                  <span v-if="file.size" class="file-info">
                    · {{ formatFileSize(file.size) }}
                  </span>
                  <span v-if="file.chunks" class="file-info">
                    · {{ file.chunks }} 个文本块
                  </span>
                </small>
              </div>
              <div class="doc-actions">
                <button 
                  type="button"
                  @click="downloadFile(file.id, file.filename)"
                  class="action-btn download-btn"
                  title="下载原始文件"
                >
                  下载
                </button>
                <button 
                  type="button"
                  @click="deleteFile(file.id, file.filename)"
                  class="action-btn delete-btn"
                  title="删除文件"
                >
                  删除
                </button>
              </div>
            </div>
          </li>
        </ul>
        <div v-else-if="files.length === 0" class="empty-list">
          <p>还没有上传任何文件</p>
        </div>
        <div v-else class="empty-list">
          <p>没有找到匹配的文件</p>
          <small>搜索关键词: "{{ fileSearchQuery }}"</small>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import axios from 'axios'

   const API_BASE = '/api'
 //const API_BASE = '/rest/api'
const fileInput = ref(null)
const selectedFiles = ref([])
const isDragging = ref(false)
const uploading = ref(false)
const uploadProgress = ref(0)
const uploadResults = ref([])
const files = ref([])
const fileSearchQuery = ref('')

const totalFiles = computed(() => selectedFiles.value.length)
const successCount = computed(() => uploadResults.value.filter(r => r.success).length)
const failCount = computed(() => uploadResults.value.filter(r => !r.success).length)

// 过滤系统文件
function isSystemFile(filename) {
  if (!filename) return false
  // macOS 系统文件
  if (filename === '.DS_Store') return true
  // macOS 资源分叉文件
  if (filename.startsWith('._')) return true
  // Windows 系统文件
  if (filename.startsWith('~$')) return true
  return false
}

// 过滤文件列表（支持文件名、标题、内容搜索，并排除系统文件）
const filteredFiles = computed(() => {
  // 先过滤掉系统文件
  let validFiles = files.value.filter(file => !isSystemFile(file.filename))
  
  if (!fileSearchQuery.value.trim()) {
    return validFiles
  }
  const query = fileSearchQuery.value.toLowerCase().trim()
  return validFiles.filter(file => {
    // 搜索文件名
    if (file.filename && file.filename.toLowerCase().includes(query)) {
      return true
    }
    // 搜索标题
    if (file.title && file.title.toLowerCase().includes(query)) {
      return true
    }
    // 搜索内容
    if (file.content && file.content.toLowerCase().includes(query)) {
      return true
    }
    return false
  })
})

onMounted(() => {
  fetchFiles()
})

async function fetchFiles() {
  try {
    const token = localStorage.getItem('adminToken')
    if (!token) {
      console.warn('未登录，无法获取文件列表')
      return
    }
    const response = await axios.get(`${API_BASE}/files`, {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })
    if (response.data.success) {
      files.value = response.data.data || []
      console.log('文件列表:', files.value)
    } else {
      console.error('获取文件列表失败:', response.data)
    }
  } catch (error) {
    console.error('获取文件列表失败:', error)
    if (error.response?.status === 401) {
      console.warn('未授权，请重新登录')
    }
  }
}

function handleDrop(e) {
  isDragging.value = false
  if (e.dataTransfer?.files?.length) {
    selectedFiles.value = Array.from(e.dataTransfer.files)
  }
}

function handleFileSelect(e) {
  if (e.target.files?.length) {
    selectedFiles.value = Array.from(e.target.files)
  }
}

async function handleUpload() {
  if (selectedFiles.value.length === 0) return

  uploading.value = true
  uploadProgress.value = 0
  uploadResults.value = []

  const token = localStorage.getItem('adminToken')
  const formData = new FormData()
  
  selectedFiles.value.forEach(file => {
    formData.append('files', file)
  })

  try {
    const response = await axios.post(`${API_BASE}/upload-batch`, formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
        'Authorization': `Bearer ${token}`
      },
      timeout: 600000, // 10分钟超时（600000毫秒），用于处理大文件和向量化
      onUploadProgress: (progressEvent) => {
        if (progressEvent.total) {
          uploadProgress.value = Math.round((progressEvent.loaded * 100) / progressEvent.total)
        }
      }
    })

    if (response.data.success) {
      uploadResults.value = response.data.results || []
      uploadProgress.value = 100
      
      // 刷新文件列表
      await fetchFiles()
      
      // 清空选中的文件
      setTimeout(() => {
        resetForm()
      }, 2000)
    }
  } catch (error) {
    console.error('上传失败:', error)
    
    // 提供更友好的错误提示
    let errorMessage = '上传失败: '
    if (error.code === 'ECONNABORTED' || error.message?.includes('timeout')) {
      errorMessage = '上传超时：文件处理时间过长。\n\n建议：\n1. 尝试减小文件大小或分批上传\n2. 检查网络连接\n3. 如果文件很大，向量化可能需要较长时间，请耐心等待'
    } else if (error.response?.status === 413 || error.message?.includes('too large')) {
      errorMessage = '文件过大：单个文件或总大小超过限制（最大500MB）\n\n建议：\n1. 减小文件大小\n2. 分批上传文件'
    } else if (error.response?.data?.error) {
      errorMessage += error.response.data.error
    } else if (error.message) {
      errorMessage += error.message
    } else {
      errorMessage += '未知错误，请检查网络连接或稍后重试'
    }
    
    alert(errorMessage)
  } finally {
    uploading.value = false
  }
}

function resetForm() {
  selectedFiles.value = []
  uploadResults.value = []
  uploadProgress.value = 0
  if (fileInput.value) {
    fileInput.value.value = ''
  }
}

function formatDate(dateString) {
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short'
  }).format(new Date(dateString))
}

function formatFileSize(bytes) {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i]
}

function downloadFile(fileId, filename) {
  const token = localStorage.getItem('adminToken')
  const url = `${API_BASE}/files/${fileId}?token=${token}`
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

async function deleteFile(fileId, filename) {
  if (!confirm(`确定要删除文件 "${filename}" 吗？此操作不可恢复。`)) {
    return
  }

  try {
    const token = localStorage.getItem('adminToken')
    const response = await axios.delete(`${API_BASE}/files/${fileId}`, {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })

    if (response.data.success) {
      alert('文件删除成功')
      // 刷新文件列表
      await fetchFiles()
    } else {
      alert('删除失败: ' + (response.data.message || '未知错误'))
    }
  } catch (error) {
    console.error('删除文件失败:', error)
    alert('删除失败: ' + (error.response?.data?.error || error.message || '未知错误'))
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

.uploader {
  display: grid;
  grid-template-columns: 1fr;
  gap: 24px;
}

.upload-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.dropzone {
  border: 2px dashed #c7d2fe;
  border-radius: 18px;
  padding: 40px;
  background: #eef2ff;
  text-align: center;
  position: relative;
  transition: all 0.3s;
}

.dropzone.dragging {
  border-color: #6366f1;
  background: #e0e7ff;
}

.dropzone-content {
  margin-bottom: 20px;
}

.file-preview {
  text-align: left;
}

.file-list-preview {
  margin-top: 12px;
  max-height: 200px;
  overflow-y: auto;
}

.file-item-preview {
  padding: 8px;
  background: white;
  border-radius: 8px;
  margin-bottom: 8px;
  font-size: 14px;
}

.file-size {
  color: #64748b;
  font-size: 12px;
}

.btn-select {
  border-radius: 12px;
  border: none;
  padding: 12px 24px;
  background: linear-gradient(120deg, #6366f1, #a855f7);
  color: white;
  font-weight: 600;
  cursor: pointer;
  font-size: 15px;
}

.actions {
  display: flex;
  gap: 12px;
}

.actions button {
  flex: 1;
  border-radius: 12px;
  border: none;
  padding: 12px 16px;
  cursor: pointer;
  font-size: 15px;
  font-weight: 600;
}

.btn-primary {
  background: linear-gradient(120deg, #6366f1, #a855f7);
  color: white;
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-ghost {
  background: white;
  border: 1px solid #e2e8f0;
}

.progress-section {
  background: white;
  border-radius: 18px;
  padding: 24px;
  box-shadow: 0 15px 45px rgba(15, 23, 42, 0.08);
}

.progress-section h3 {
  margin: 0 0 16px 0;
  color: #333;
}

.progress-summary {
  display: flex;
  gap: 24px;
  margin-bottom: 16px;
}

.summary-item {
  display: flex;
  gap: 8px;
}

.summary-item .label {
  color: #64748b;
}

.summary-item .value {
  font-weight: 600;
  color: #333;
}

.summary-item.success .value {
  color: #10b981;
}

.summary-item.error .value {
  color: #ef4444;
}

.progress-bar-container {
  width: 100%;
  height: 24px;
  background: #e2e8f0;
  border-radius: 12px;
  overflow: hidden;
  margin-bottom: 8px;
}

.progress-bar {
  height: 100%;
  background: linear-gradient(120deg, #6366f1, #a855f7);
  transition: width 0.3s;
}

.progress-text {
  text-align: center;
  font-weight: 600;
  color: #6366f1;
}

.upload-results {
  margin-top: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.result-item {
  padding: 12px;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
}

.result-item.success {
  background: #f0fdf4;
  border-color: #10b981;
}

.result-item.error {
  background: #fef2f2;
  border-color: #ef4444;
}

.result-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.result-filename {
  font-weight: 600;
  color: #333;
}

.result-status {
  font-size: 14px;
  font-weight: 600;
}

.result-item.success .result-status {
  color: #10b981;
}

.result-item.error .result-status {
  color: #ef4444;
}

.result-message {
  font-size: 14px;
  color: #64748b;
  margin-bottom: 4px;
}

.result-chunks {
  font-size: 12px;
  color: #6366f1;
}

.doc-list {
  background: white;
  border-radius: 18px;
  padding: 24px;
  box-shadow: 0 15px 45px rgba(15, 23, 42, 0.08);
  max-height: 600px;
  overflow: auto;
}

.doc-list header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.file-search-box {
  position: relative;
  margin-bottom: 16px;
  display: flex;
  justify-content: center;
  align-items: center;
}

.file-search-box .file-search-input {
  max-width: 400px;
  margin: 0 auto;
}

.file-search-input {
  width: 100%;
  padding: 10px 40px 10px 12px;
  border-radius: 8px;
  border: 1px solid #dbeafe;
  background: #f8fafc;
  font-size: 14px;
  font-family: inherit;
}

.file-search-input:focus {
  outline: none;
  border-color: #6366f1;
  background: white;
}

.file-search-clear {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: #64748b;
  cursor: pointer;
  font-size: 18px;
  padding: 4px 8px;
  border-radius: 4px;
  transition: all 0.2s;
}

.file-search-clear:hover {
  background: #e2e8f0;
  color: #333;
}

.doc-list h3 {
  margin: 0;
  color: #333;
}

.doc-list ul {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.doc-list li {
  padding: 16px;
  border-radius: 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
}

.doc-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.doc-header h4 {
  margin: 0 0 8px 0;
  color: #333;
}

.doc-header small {
  color: #64748b;
  font-size: 13px;
}

.file-info {
  color: #6366f1;
  font-weight: 500;
}

.doc-actions {
  display: flex;
  gap: 8px;
}

.action-btn {
  padding: 6px 12px;
  border-radius: 8px;
  border: none;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}

.download-btn {
  border: 1px solid #6366f1;
  background: white;
  color: #6366f1;
}

.download-btn:hover {
  background: #6366f1;
  color: white;
}

.delete-btn {
  border: 1px solid #ef4444;
  background: white;
  color: #ef4444;
}

.delete-btn:hover {
  background: #ef4444;
  color: white;
}

.empty-list {
  text-align: center;
  padding: 40px;
  color: #64748b;
}

/* 移动端适配 */
@media (max-width: 768px) {
  .page {
    gap: 16px;
  }

  .hero {
    padding: 16px;
    border-radius: 16px;
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

  .dropzone {
    padding: 24px 16px;
    border-radius: 14px;
  }

  .dropzone-content {
    margin-bottom: 16px;
  }

  .dropzone-content p {
    font-size: 14px;
  }

  .dropzone-content small {
    font-size: 12px;
  }

  .file-list-preview {
    max-height: 150px;
  }

  .file-item-preview {
    padding: 6px;
    font-size: 13px;
  }

  .btn-select {
    padding: 10px 20px;
    font-size: 14px;
    border-radius: 10px;
  }

  .actions {
    flex-direction: column;
  }

  .actions button {
    width: 100%;
    padding: 12px 16px;
    font-size: 14px;
  }

  .progress-section {
    padding: 16px;
    border-radius: 14px;
  }

  .progress-section h3 {
    font-size: 18px;
    margin-bottom: 12px;
  }

  .progress-summary {
    flex-direction: column;
    gap: 12px;
  }

  .summary-item {
    justify-content: space-between;
  }

  .summary-item .label {
    font-size: 13px;
  }

  .summary-item .value {
    font-size: 14px;
  }

  .progress-bar-container {
    height: 20px;
    border-radius: 10px;
  }

  .progress-text {
    font-size: 13px;
  }

  .upload-results {
    gap: 10px;
  }

  .result-item {
    padding: 10px;
    border-radius: 10px;
  }

  .result-header {
    flex-wrap: wrap;
    gap: 8px;
  }

  .result-filename {
    font-size: 13px;
    word-break: break-word;
    flex: 1;
    min-width: 0;
  }

  .result-status {
    font-size: 12px;
    white-space: nowrap;
  }

  .result-message {
    font-size: 13px;
  }

  .result-chunks {
    font-size: 11px;
  }

  .doc-list {
    padding: 16px;
    border-radius: 14px;
    max-height: none;
  }

  .doc-list header {
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 12px;
  }

  .doc-list h3 {
    font-size: 18px;
  }

  .file-search-box {
    margin-bottom: 12px;
  }

  .file-search-input {
    padding: 10px 36px 10px 12px;
    font-size: 14px;
    border-radius: 8px;
  }

  .file-search-clear {
    right: 6px;
    font-size: 16px;
    padding: 4px 6px;
  }

  .doc-list ul {
    gap: 10px;
  }

  .doc-list li {
    padding: 12px;
    border-radius: 10px;
  }

  .doc-header {
    flex-direction: column;
    gap: 12px;
    align-items: stretch;
  }

  .doc-header h4 {
    font-size: 15px;
    word-break: break-word;
  }

  .doc-header small {
    font-size: 12px;
  }

  .doc-actions {
    width: 100%;
    justify-content: stretch;
  }

  .action-btn {
    flex: 1;
    padding: 8px 12px;
    font-size: 13px;
  }

  .empty-list {
    padding: 30px 20px;
    font-size: 14px;
  }
}

@media (max-width: 480px) {
  .hero h1 {
    font-size: 20px;
  }

  .dropzone {
    padding: 20px 12px;
  }

  .dropzone-content p {
    font-size: 13px;
  }

  .file-item-preview {
    font-size: 12px;
  }

  .btn-select {
    padding: 8px 16px;
    font-size: 13px;
  }

  .actions button {
    padding: 10px 14px;
    font-size: 13px;
  }

  .progress-section {
    padding: 12px;
  }

  .doc-list {
    padding: 12px;
  }

  .doc-header h4 {
    font-size: 14px;
  }

  .action-btn {
    padding: 6px 10px;
    font-size: 12px;
  }
}
</style>

