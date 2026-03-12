<template>
  <router-view />
</template>

<script setup>
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

#app {
  height: 100vh;
}

.admin-toast {
  border-radius: 14px !important;
  border: none !important;
  color: #fff !important;
  font-weight: 600 !important;
  padding: 12px 16px !important;
  box-shadow: 0 14px 35px rgba(15, 23, 42, 0.22) !important;
  backdrop-filter: blur(6px);
}

.admin-toast .el-message__content {
  color: #fff !important;
}

.admin-toast .el-message__icon {
  color: rgba(255, 255, 255, 0.92) !important;
}

.admin-toast--success {
  background: linear-gradient(135deg, #10b981, #059669) !important;
}

.admin-toast--error {
  background: linear-gradient(135deg, #ef4444, #dc2626) !important;
}

.admin-toast--info {
  background: linear-gradient(135deg, #6366f1, #4f46e5) !important;
}

.admin-toast--warning {
  background: linear-gradient(135deg, #f59e0b, #d97706) !important;
}
</style>
