import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../store/auth'
import MainLayout from '../layouts/MainLayout.vue'
import Login from '../views/Login.vue'
import UserManagement from '../views/UserManagement.vue'
import FeedbackManagement from '../views/FeedbackManagement.vue'
import PageStatManagement from '../views/PageStatManagement.vue'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: Login,
    meta: { requiresAuth: false }
  },
  {
    path: '/',
    component: MainLayout,
    redirect: '/users',
    meta: { requiresAuth: true },
    children: [
      {
        path: '/users',
        name: 'UserManagement',
        component: UserManagement
      },
      {
        path: '/feedbacks',
        name: 'FeedbackManagement',
        component: FeedbackManagement
      },
      {
        path: '/page-stats',
        name: 'PageStatManagement',
        component: PageStatManagement
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// 路由守卫
router.beforeEach((to, from, next) => {
  const authStore = useAuthStore()
  const isAuthenticated = authStore.isAuthenticated()

  if (to.meta.requiresAuth === false) {
    // 登录页面，如果已登录则跳转到首页
    if (isAuthenticated && to.path === '/login') {
      next('/users')
    } else {
      next()
    }
  } else {
    // 需要认证的页面
    if (isAuthenticated) {
      next()
    } else {
      next('/login')
    }
  }
})

export default router
