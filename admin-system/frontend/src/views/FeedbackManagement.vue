<template>
  <div class="feedback-management">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>意见反馈管理</span>
          <el-button type="primary" @click="handleAdd">新增意见</el-button>
        </div>
      </template>

      <!-- 搜索栏 -->
      <el-form :inline="true" class="search-form">
        <el-form-item label="用户名">
          <el-input v-model="searchForm.name" placeholder="请输入用户名" clearable />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">查询</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>

      <!-- 表格 -->
      <el-table :data="tableData" border style="width: 100%">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column label="反馈用户" width="120">
          <template #default="scope">
            <span>{{ scope.row.username || scope.row.name || '匿名用户' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="title" label="标题" width="200" show-overflow-tooltip />
        <el-table-column prop="description" label="详细描述" show-overflow-tooltip />
        <el-table-column prop="image" label="图片" width="120">
          <template #default="scope">
            <el-image
              v-if="scope.row.image"
              :src="getImageUrl(scope.row.image)"
              :preview-src-list="[getImageUrl(scope.row.image)]"
              style="width: 60px; height: 60px"
              fit="cover"
            />
            <span v-else>无图片</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="scope">
            {{ formatDate(scope.row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="scope">
            <el-button size="small" @click="handleEdit(scope.row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(scope.row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="handleSizeChange"
        @current-change="handlePageChange"
        style="margin-top: 20px"
      />
    </el-card>

    <!-- 新增/编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="600px"
      @close="handleDialogClose"
    >
      <el-form :model="form" :rules="rules" ref="formRef" label-width="100px">
        <el-form-item label="姓名" prop="name">
          <el-input v-model="form.name" placeholder="请输入姓名" />
        </el-form-item>
        <el-form-item label="标题" prop="title">
          <el-input v-model="form.title" placeholder="请输入标题" />
        </el-form-item>
        <el-form-item label="详细描述" prop="description">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="4"
            placeholder="请输入详细描述"
          />
        </el-form-item>
        <el-form-item label="图片">
          <div class="image-upload">
            <el-upload
              :action="uploadUrl"
              :headers="uploadHeaders"
              :on-success="handleUploadSuccess"
              :on-error="handleUploadError"
              :before-upload="beforeUpload"
              :show-file-list="false"
              accept="image/*"
            >
              <el-button type="primary" :icon="Upload">上传图片</el-button>
            </el-upload>
            <div v-if="form.image" class="image-preview">
              <el-image
                :src="getImageUrl(form.image)"
                style="width: 100px; height: 100px; margin-top: 10px"
                fit="cover"
                :preview-src-list="[getImageUrl(form.image)]"
              />
              <el-button
                type="danger"
                size="small"
                @click="form.image = ''"
                style="margin-left: 10px"
              >
                删除
              </el-button>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessageBox } from 'element-plus'
import { Upload } from '@element-plus/icons-vue'
import api from '../api'
import { useAuthStore } from '../store/auth'
import { formatDate } from '../utils/date'
import { showToast } from '../utils/toast'

const tableData = ref([])
const dialogVisible = ref(false)
const dialogTitle = ref('新增意见')
const formRef = ref(null)

const searchForm = reactive({
  name: ''
})

const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0
})

const form = reactive({
  id: null,
  name: '',
  title: '',
  description: '',
  image: ''
})

const rules = {
  name: [
    { required: true, message: '请输入姓名', trigger: 'blur' }
  ],
  title: [
    { required: true, message: '请输入标题', trigger: 'blur' }
  ],
  description: [
    { required: true, message: '请输入详细描述', trigger: 'blur' }
  ]
}

const authStore = useAuthStore()

// 上传配置
const uploadUrl = computed(() => {
  return '/api/upload'
})

const uploadHeaders = computed(() => {
  return {
    Authorization: `Bearer ${authStore.token}`
  }
})

// 上传前验证
const beforeUpload = (file) => {
  const isImage = file.type.startsWith('image/')
  const isLt5M = file.size / 1024 / 1024 < 5

  if (!isImage) {
    showToast('只能上传图片文件!', 'error')
    return false
  }
  if (!isLt5M) {
    showToast('图片大小不能超过5MB!', 'error')
    return false
  }
  return true
}

// 上传成功
const handleUploadSuccess = (response) => {
  if (response.url) {
    // 使用返回的URL（已经是相对路径，前端代理会处理）
    form.image = response.url
    showToast('图片上传成功', 'success')
  } else {
    showToast('上传失败，请重试', 'error')
  }
}

// 上传失败
const handleUploadError = () => {
  showToast('图片上传失败', 'error')
}

// 获取图片完整URL
const getImageUrl = (url) => {
  if (!url) return ''
  // 如果是完整URL（http/https开头），直接返回
  if (url.startsWith('http://') || url.startsWith('https://')) {
    return url
  }
  // 如果是相对路径，直接返回（Vite代理会处理）
  return url
}

// 加载数据
const loadData = async () => {
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.pageSize,
      name: searchForm.name
    }
    const response = await api.get('/feedbacks', { params })
    tableData.value = response.data.list
    pagination.total = response.data.total
  } catch (error) {
    showToast(error.message || '加载数据失败', 'error')
  }
}

// 搜索
const handleSearch = () => {
  pagination.page = 1
  loadData()
}

// 重置
const handleReset = () => {
  searchForm.name = ''
  pagination.page = 1
  loadData()
}

// 新增
const handleAdd = () => {
  dialogTitle.value = '新增意见'
  form.id = null
  form.name = ''
  form.title = ''
  form.description = ''
  form.image = ''
  dialogVisible.value = true
}

// 编辑
const handleEdit = (row) => {
  dialogTitle.value = '编辑意见'
  form.id = row.id
  form.name = row.name
  form.title = row.title || ''
  form.description = row.description
  form.image = row.image || ''
  dialogVisible.value = true
}

// 删除
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除该意见反馈吗？', '提示', {
      type: 'warning'
    })
    await api.delete(`/feedbacks/${row.id}`)
    showToast('删除成功', 'success')
    loadData()
  } catch (error) {
    if (error !== 'cancel') {
      showToast(error.message || '删除失败', 'error')
    }
  }
}

// 提交
const handleSubmit = async () => {
  try {
    await formRef.value.validate()
    if (form.id) {
      await api.put(`/feedbacks/${form.id}`, form)
      showToast('更新成功', 'success')
    } else {
      await api.post('/feedbacks', form)
      showToast('创建成功', 'success')
    }
    dialogVisible.value = false
    loadData()
  } catch (error) {
    if (error !== false) {
      showToast(error.message || '操作失败', 'error')
    }
  }
}

// 对话框关闭
const handleDialogClose = () => {
  formRef.value?.resetFields()
}

// 分页
const handleSizeChange = () => {
  loadData()
}

const handlePageChange = () => {
  loadData()
}

onMounted(() => {
  loadData()
})
</script>

<style scoped>
.feedback-management {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.search-form {
  margin-bottom: 20px;
}
</style>
