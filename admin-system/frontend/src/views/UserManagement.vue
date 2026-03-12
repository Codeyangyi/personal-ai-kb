<template>
  <div class="user-management">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>用户管理</span>
          <el-button type="primary" @click="handleAdd">新增用户</el-button>
        </div>
      </template>

      <!-- 搜索栏 -->
      <el-form :inline="true" class="search-form">
        <el-form-item label="用户名">
          <el-input v-model="searchForm.username" placeholder="请输入用户名" clearable />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">查询</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>

      <!-- 表格 -->
      <el-table :data="tableData" border style="width: 100%">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" />
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
      width="500px"
      @close="handleDialogClose"
    >
      <el-form :model="form" :rules="rules" ref="formRef" label-width="80px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="请输入用户名" />
        </el-form-item>
        <el-form-item label="密码" prop="password" :required="!form.id">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="请输入密码"
            show-password
          />
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
import { ref, reactive, onMounted } from 'vue'
import { ElMessageBox } from 'element-plus'
import api from '../api'
import { formatDate } from '../utils/date'
import { showToast } from '../utils/toast'

const tableData = ref([])
const dialogVisible = ref(false)
const dialogTitle = ref('新增用户')
const formRef = ref(null)

const searchForm = reactive({
  username: ''
})

const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0
})

const form = reactive({
  id: null,
  username: '',
  password: ''
})

const rules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' }
  ],
  password: [
    {
      validator: (rule, value, callback) => {
        // 如果是编辑模式且密码为空，允许通过（不修改密码）
        if (form.id && (!value || !value.trim())) {
          callback()
          return
        }
        // 新增模式或编辑模式提供了密码，必须验证
        if (!value || !value.trim()) {
          callback(new Error('请输入密码'))
          return
        }
        const trimmed = value.trim()
        if (trimmed.length < 6) {
          callback(new Error('密码长度不能少于6位'))
          return
        }
        callback()
      },
      trigger: 'blur'
    }
  ]
}

// 加载数据
const loadData = async () => {
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.pageSize,
      username: searchForm.username
    }
    const response = await api.get('/users', { params })
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
  searchForm.username = ''
  pagination.page = 1
  loadData()
}

// 新增
const handleAdd = () => {
  dialogTitle.value = '新增用户'
  form.id = null
  form.username = ''
  form.password = ''
  dialogVisible.value = true
}

// 编辑
const handleEdit = (row) => {
  dialogTitle.value = '编辑用户'
  form.id = row.id
  form.username = row.username
  form.password = ''
  dialogVisible.value = true
}

// 删除
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除该用户吗？', '提示', {
      type: 'warning'
    })
    await api.delete(`/users/${row.id}`)
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
    // 先检查密码字段的值（调试用）
    console.log('提交前 - form.password:', form.password, '长度:', form.password?.length)
    console.log('提交前 - form.id:', form.id)
    
    // 验证表单
    await formRef.value.validate()
    
    // 再次检查密码字段的值（验证后）
    console.log('验证后 - form.password:', form.password, '长度:', form.password?.length)
    
    // 清理密码（去除前后空格，与后端保持一致）
    const cleanedPassword = form.password ? form.password.trim() : ''
    console.log('清理后 - cleanedPassword:', cleanedPassword, '长度:', cleanedPassword.length)
    
    if (form.id) {
      // 编辑
      const updateData = { username: form.username.trim() }
      if (cleanedPassword) {
        // 编辑时如果提供了密码，验证长度（虽然验证器已经验证过，但这里再确认一次）
        if (cleanedPassword.length < 6) {
          showToast('密码长度不能少于6位', 'error')
          return
        }
        updateData.password = cleanedPassword
      }
      console.log('编辑 - 发送数据:', updateData)
      await api.put(`/users/${form.id}`, updateData)
      showToast('更新成功', 'success')
    } else {
      // 新增 - 必须提供密码（验证器已经验证过）
      if (!cleanedPassword) {
        console.error('新增用户 - 密码为空！form.password:', form.password)
        showToast('密码不能为空', 'error')
        return
      }
      const postData = {
        username: form.username.trim(),
        password: cleanedPassword
      }
      console.log('新增 - 发送数据:', { ...postData, password: '***' }) // 不打印实际密码
      await api.post('/users', postData)
      showToast('创建成功', 'success')
    }
    dialogVisible.value = false
    loadData()
  } catch (error) {
    console.error('提交错误:', error)
    // 表单验证失败，不显示错误（Element Plus 会自动显示）
    if (error !== false && typeof error === 'object') {
      // 如果是 API 错误，显示错误信息
      showToast(error.response?.data?.error || error.message || '操作失败', 'error')
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
.user-management {
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
