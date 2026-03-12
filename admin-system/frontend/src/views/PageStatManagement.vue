<template>
  <div class="page-stat-management">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>页面统计管理</span>
          <el-button type="primary" @click="handleAdd">新增统计</el-button>
        </div>
      </template>

      <!-- 搜索栏 -->
      <el-form :inline="true" class="search-form">
        <el-form-item label="开始日期">
          <el-date-picker
            v-model="searchForm.startDate"
            type="date"
            placeholder="选择开始日期"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            clearable
          />
        </el-form-item>
        <el-form-item label="结束日期">
          <el-date-picker
            v-model="searchForm.endDate"
            type="date"
            placeholder="选择结束日期"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            clearable
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">查询</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>

      <!-- 表格 -->
      <el-table :data="tableData" border style="width: 100%">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="date" label="日期" width="120" />
        <el-table-column prop="uv" label="用户访问人数(UV)" width="150" />
        <el-table-column prop="pv" label="页面浏览总次数(PV)" width="150" />
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
      <el-form :model="form" :rules="rules" ref="formRef" label-width="140px">
        <el-form-item label="日期" prop="date">
          <el-date-picker
            v-model="form.date"
            type="date"
            placeholder="选择日期"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="用户访问人数(UV)" prop="uv">
          <el-input-number v-model="form.uv" :min="0" style="width: 100%" />
        </el-form-item>
        <el-form-item label="页面浏览总次数(PV)" prop="pv">
          <el-input-number v-model="form.pv" :min="0" style="width: 100%" />
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
const dialogTitle = ref('新增统计')
const formRef = ref(null)

const searchForm = reactive({
  startDate: '',
  endDate: ''
})

const pagination = reactive({
  page: 1,
  pageSize: 30,
  total: 0
})

const form = reactive({
  id: null,
  date: '',
  uv: 0,
  pv: 0
})

const rules = {
  date: [
    { required: true, message: '请选择日期', trigger: 'change' }
  ],
  uv: [
    { required: true, message: '请输入用户访问人数', trigger: 'blur' },
    { type: 'number', min: 0, message: '用户访问人数不能小于0', trigger: 'blur' }
  ],
  pv: [
    { required: true, message: '请输入页面浏览总次数', trigger: 'blur' },
    { type: 'number', min: 0, message: '页面浏览总次数不能小于0', trigger: 'blur' }
  ]
}

// 加载数据
const loadData = async () => {
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.pageSize,
      start_date: searchForm.startDate,
      end_date: searchForm.endDate
    }
    const response = await api.get('/page-stats', { params })
    tableData.value = response.data.list.map(item => {
      let dateStr = ''
      if (item.date) {
        // 处理日期格式，可能是 "2006-01-02" 或 "2006-01-02T00:00:00Z"
        dateStr = item.date.split('T')[0]
      }
      return {
        ...item,
        date: dateStr
      }
    })
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
  searchForm.startDate = ''
  searchForm.endDate = ''
  pagination.page = 1
  loadData()
}

// 新增
const handleAdd = () => {
  dialogTitle.value = '新增统计'
  form.id = null
  form.date = new Date().toISOString().split('T')[0]
  form.uv = 0
  form.pv = 0
  dialogVisible.value = true
}

// 编辑
const handleEdit = (row) => {
  dialogTitle.value = '编辑统计'
  form.id = row.id
  form.date = row.date
  form.uv = row.uv
  form.pv = row.pv
  dialogVisible.value = true
}

// 删除
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除该统计记录吗？', '提示', {
      type: 'warning'
    })
    await api.delete(`/page-stats/${row.id}`)
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
    const submitData = {
      date: form.date,
      uv: form.uv,
      pv: form.pv
    }
    if (form.id) {
      await api.put(`/page-stats/${form.id}`, submitData)
      showToast('更新成功', 'success')
    } else {
      await api.post('/page-stats', submitData)
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
.page-stat-management {
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
