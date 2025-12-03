<template>
  <div class="layout">
    <header class="top">
      <nav>
        <RouterLink to="/" class="nav-btn ghost hide-mobile">AI 搜索 · 问答</RouterLink>
        <!-- 只有管理员才显示"知识入库"入口 -->
        <RouterLink v-if="isAdmin" to="/import" class="nav-btn">知识入库</RouterLink>

        <!-- 管理员登录/退出按钮 -->
        <button
          v-if="!isAdmin"
          type="button"
          class="nav-btn ghost hide-mobile"
          @click="handleAdminLogin"
        >
          管理员登录
        </button>
        <button
          v-else
          type="button"
          class="nav-btn ghost hide-mobile"
          @click="handleLogout"
        >
          退出管理员
        </button>
      </nav>
    </header>

    <main class="content">
      <RouterView />
    </main>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { RouterLink, RouterView, useRouter } from 'vue-router'
import axios from 'axios'

const router = useRouter()
const isAdmin = ref(false)
const API_BASE = '/api'
//const API_BASE = '/rest/api'

onMounted(() => {
  // 检查本地存储的管理员token
  const savedToken = localStorage.getItem('adminToken')
  if (savedToken) {
    checkAdmin(savedToken)
  }
})

async function checkAdmin(token) {
  try {
    const response = await axios.post(`${API_BASE}/check-admin`, { token })
    if (response.data.isAdmin) {
      isAdmin.value = true
      localStorage.setItem('adminToken', token)
    } else {
      isAdmin.value = false
      localStorage.removeItem('adminToken')
    }
  } catch (error) {
    isAdmin.value = false
    localStorage.removeItem('adminToken')
  }
}

function handleAdminLogin() {
  const token = prompt('请输入管理员token（仅限内部人员）：')
  if (!token) return
  checkAdmin(token).then(() => {
    if (isAdmin.value) {
      alert('管理员登录成功')
      router.push('/import')
    } else {
      alert('token错误，无权访问知识入库')
    }
  })
}

function handleLogout() {
  isAdmin.value = false
  localStorage.removeItem('adminToken')
  alert('已退出管理员模式')
  router.push('/')
}
</script>

<style scoped>
.layout {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px;
}

.top {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 24px;
  background: white;
  border-radius: 24px;
  box-shadow: 0 20px 60px rgba(15, 23, 42, 0.08);
}

nav {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.nav-btn {
  border-radius: 999px;
  padding: 12px 20px;
  font-weight: 600;
  text-decoration: none;
  color: white;
  background: linear-gradient(120deg, #6366f1, #8b5cf6);
  font-size: 14px;
  white-space: nowrap;
  border: none;
  cursor: pointer;
}

.nav-btn.ghost {
  background: transparent;
  border: 1px solid #cbd5f5;
  color: #6366f1;
}

.nav-btn.router-link-active {
  box-shadow: 0 10px 25px rgba(99, 102, 241, 0.3);
}

.content {
  margin-top: 32px;
}

/* 移动端适配 */
@media (max-width: 768px) {
  .layout {
    padding: 12px;
    max-width: 100%;
  }

  .top {
    padding: 16px;
    border-radius: 16px;
    flex-direction: column;
    align-items: stretch;
  }

  nav {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .nav-btn {
    width: 100%;
    padding: 12px 16px;
    font-size: 14px;
    text-align: center;
    justify-content: center;
  }

  /* 移动端隐藏 AI 搜索问答和管理员登录 */
  .hide-mobile {
    display: none !important;
  }

  .content {
    margin-top: 16px;
  }
}

@media (max-width: 480px) {
  .layout {
    padding: 8px;
  }

  .top {
    padding: 12px;
  }

  .nav-btn {
    padding: 10px 14px;
    font-size: 13px;
  }
}
</style>
