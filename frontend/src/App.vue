<template>
  <div class="layout">
    <header class="top">
      <nav>
        <RouterLink to="/" class="nav-btn ghost hide-mobile">AI 搜索 · 问答</RouterLink>
        <!-- 只有管理员才显示"知识入库"入口 -->
        <RouterLink v-if="isAdmin" to="/import" class="nav-btn">知识入库</RouterLink>

        <!-- 用户登录/退出按钮 -->
        <div v-if="!isLoggedIn" class="user-actions">
          <button
            type="button"
            class="nav-btn ghost hide-mobile"
            @click="showLoginModal = true"
          >
            用户登录
          </button>
        </div>
        <div v-else class="user-actions">
          <span class="user-info hide-mobile">{{ username }}</span>
          <button
            type="button"
            class="nav-btn ghost hide-mobile"
            @click="handleLogout"
          >
            退出登录
          </button>
        </div>
      </nav>
    </header>

    <main class="content">
      <RouterView />
    </main>

    <!-- 登录弹窗 -->
    <div v-if="showLoginModal" class="login-modal-overlay" @click="showLoginModal = false">
      <div class="login-modal" @click.stop>
        <div class="login-modal-header">
          <h2>用户登录</h2>
          <button class="close-btn" @click="showLoginModal = false">×</button>
        </div>
        <div class="login-modal-content">
          <form @submit.prevent="handleLogin" class="login-form">
            <div class="form-group">
              <label for="username">用户名</label>
              <input
                id="username"
                v-model="loginForm.username"
                type="text"
                placeholder="请输入用户名"
                required
              />
            </div>
            <div class="form-group">
              <label for="password">密码</label>
              <input
                id="password"
                v-model="loginForm.password"
                type="password"
                placeholder="请输入密码"
                required
              />
            </div>
            <div class="form-actions">
              <button type="button" class="cancel-btn" @click="showLoginModal = false">取消</button>
              <button type="submit" class="submit-btn" :disabled="loggingIn">
                {{ loggingIn ? '登录中...' : '登录' }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>

    <transition name="toast-pop">
      <div v-if="toastVisible" :class="['app-toast', `app-toast--${toastType}`]">
        <span class="app-toast__icon">{{ toastType === 'success' ? 'OK' : '!' }}</span>
        <span>{{ toastMessage }}</span>
      </div>
    </transition>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, provide } from 'vue'
import { RouterLink, RouterView, useRouter } from 'vue-router'
import axios from 'axios'

const router = useRouter()
const isAdmin = ref(false)
const isLoggedIn = ref(false)
const username = ref('')
const showLoginModal = ref(false)
const loggingIn = ref(false)
const toastVisible = ref(false)
const toastType = ref('success')
const toastMessage = ref('')
let toastTimer = null
const API_BASE = '/api'
//const API_BASE = '/rest/api'
const handleShowLogin = () => {
  showLoginModal.value = true
}

// 登录表单
const loginForm = ref({
  username: '',
  password: ''
})

// 提供登录状态给子组件
provide('isLoggedIn', isLoggedIn)
provide('isAdmin', isAdmin)
provide('username', username)
provide('showLoginModal', showLoginModal)

onMounted(() => {
  // 检查本地存储的用户token
  const savedToken = localStorage.getItem('userToken')
  if (savedToken) {
    checkUserStatus(savedToken)
  }
  
  // 监听显示登录弹窗事件
  window.addEventListener('show-login', handleShowLogin)
})

onBeforeUnmount(() => {
  window.removeEventListener('show-login', handleShowLogin)
  if (toastTimer) {
    clearTimeout(toastTimer)
  }
})

function showToast(message, type = 'success') {
  if (toastTimer) {
    clearTimeout(toastTimer)
  }
  toastMessage.value = message
  toastType.value = type
  toastVisible.value = true
  toastTimer = setTimeout(() => {
    toastVisible.value = false
  }, 2400)
}

async function checkUserStatus(token) {
  try {
    const response = await axios.post(`${API_BASE}/check-admin`, { token })
    if (response.data.isLoggedIn) {
      isLoggedIn.value = true
      isAdmin.value = response.data.isAdmin || false
      username.value = response.data.username || ''
      localStorage.setItem('userToken', token)
    } else {
      isLoggedIn.value = false
      isAdmin.value = false
      username.value = ''
      localStorage.removeItem('userToken')
    }
  } catch (error) {
    isLoggedIn.value = false
    isAdmin.value = false
    username.value = ''
    localStorage.removeItem('userToken')
  }
}

async function handleLogin() {
  if (!loginForm.value.username || !loginForm.value.password) {
    showToast('请输入用户名和密码', 'error')
    return
  }

  loggingIn.value = true
  try {
    const response = await axios.post(`${API_BASE}/login`, {
      username: loginForm.value.username,
      password: loginForm.value.password
    })

    if (response.data.success) {
      const token = response.data.token
      isAdmin.value = response.data.isAdmin || false
      username.value = response.data.username || ''
      isLoggedIn.value = true
      localStorage.setItem('userToken', token)
      
      showLoginModal.value = false
      loginForm.value = { username: '', password: '' }

      if (isAdmin.value) {
        showToast('登录成功，欢迎管理员', 'success')
      } else {
        showToast('登录成功，欢迎回来', 'success')
      }
    } else {
      showToast(response.data.message || '登录失败', 'error')
    }
  } catch (error) {
    console.error('登录失败:', error)
    showToast(error.response?.data?.message || '登录失败，请检查用户名和密码', 'error')
  } finally {
    loggingIn.value = false
  }
}

function handleLogout() {
  isLoggedIn.value = false
  isAdmin.value = false
  username.value = ''
  localStorage.removeItem('userToken')
  showToast('已退出登录', 'info')
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

.user-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.user-info {
  color: #6366f1;
  font-weight: 500;
  font-size: 14px;
}

/* 登录弹窗样式 */
.login-modal-overlay {
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

.login-modal {
  background: white;
  border-radius: 20px;
  max-width: 400px;
  width: 100%;
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

.login-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 24px;
  border-bottom: 2px solid #e2e8f0;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border-radius: 20px 20px 0 0;
}

.login-modal-header h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}

.login-modal-content {
  padding: 24px;
}

.login-form {
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

.form-group input {
  padding: 12px;
  border: 2px solid #e2e8f0;
  border-radius: 12px;
  font-size: 14px;
  font-family: inherit;
  transition: all 0.3s;
  background: #f8fafc;
}

.form-group input:focus {
  outline: none;
  border-color: #6366f1;
  background: white;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
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

  .hide-mobile {
    display: none !important;
  }

  .content {
    margin-top: 16px;
  }

  .login-modal {
    max-width: 100%;
    border-radius: 16px 16px 0 0;
  }

  .login-modal-header {
    padding: 16px;
    border-radius: 16px 16px 0 0;
  }

  .login-modal-header h2 {
    font-size: 20px;
  }

  .login-modal-content {
    padding: 16px;
  }

  .form-actions {
    flex-direction: column;
    gap: 10px;
  }

  .cancel-btn,
  .submit-btn {
    width: 100%;
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

.app-toast {
  position: fixed;
  right: 24px;
  top: 24px;
  z-index: 1200;
  min-width: 240px;
  max-width: 320px;
  padding: 12px 16px;
  border-radius: 14px;
  box-shadow: 0 14px 35px rgba(15, 23, 42, 0.22);
  color: #fff;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 10px;
  backdrop-filter: blur(6px);
}

.app-toast__icon {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
  background: rgba(255, 255, 255, 0.26);
}

.app-toast--success {
  background: linear-gradient(135deg, #10b981, #059669);
}

.app-toast--error {
  background: linear-gradient(135deg, #ef4444, #dc2626);
}

.app-toast--info {
  background: linear-gradient(135deg, #6366f1, #4f46e5);
}

.toast-pop-enter-active,
.toast-pop-leave-active {
  transition: all 0.28s ease;
}

.toast-pop-enter-from,
.toast-pop-leave-to {
  opacity: 0;
  transform: translateY(-12px) scale(0.96);
}
</style>
