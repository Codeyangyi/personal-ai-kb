<template>
  <el-container>
    <el-header class="header">
      <div class="header-content">
        <h2>后台管理系统</h2>
        <div class="user-info">
          <span class="username">{{ user?.username || '用户' }}</span>
          <el-button type="danger" size="small" @click="handleLogout" :icon="SwitchButton">
            退出登录
          </el-button>
        </div>
      </div>
    </el-header>
    <el-container>
      <el-aside width="200px">
        <el-menu
          :default-active="activeMenu"
          router
          class="el-menu-vertical"
        >
          <el-menu-item index="/users">
            <el-icon><User /></el-icon>
            <span>用户管理</span>
          </el-menu-item>
          <el-menu-item index="/feedbacks">
            <el-icon><ChatLineRound /></el-icon>
            <span>意见反馈管理</span>
          </el-menu-item>
          <el-menu-item index="/page-stats">
            <el-icon><DataAnalysis /></el-icon>
            <span>页面统计管理</span>
          </el-menu-item>
        </el-menu>
      </el-aside>
      <el-main>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { User, ChatLineRound, DataAnalysis, SwitchButton } from '@element-plus/icons-vue'
import { useAuthStore } from '../store/auth'
import api from '../api'
import { showToast } from '../utils/toast'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const activeMenu = computed(() => route.path)
const user = computed(() => authStore.user)

const handleLogout = async () => {
  try {
    await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      type: 'warning'
    })
    
    // 调用退出登录接口
    try {
      await api.post('/auth/logout')
    } catch (error) {
      // 即使接口失败也清除本地token
      console.error('退出登录接口调用失败:', error)
    }
    
    // 清除本地认证信息
    authStore.clearAuth()
    showToast('退出登录成功', 'success')
    router.push('/login')
  } catch (error) {
    if (error !== 'cancel') {
      console.error('退出登录失败:', error)
    }
  }
}
</script>

<style scoped>
.header {
  background-color: #fff;
  border-bottom: 1px solid #e4e7ed;
  padding: 0;
  height: 60px !important;
  line-height: 60px;
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 20px;
  height: 100%;
}

.header-content h2 {
  margin: 0;
  font-size: 20px;
  color: #303133;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 15px;
}

.username {
  color: #606266;
  font-size: 14px;
}

.el-aside {
  background-color: #304156;
}

.el-menu {
  border-right: none;
  background-color: #304156;
}

.el-menu-item {
  color: #bfcbd9;
}

.el-menu-item:hover {
  background-color: #263445;
  color: #409eff;
}

.el-menu-item.is-active {
  background-color: #409eff;
  color: #fff;
}

.el-main {
  background-color: #f0f2f5;
  padding: 20px;
}
</style>
