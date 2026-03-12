import { ElMessage } from 'element-plus'

const TOAST_DURATION = 2400

export function showToast(message, type = 'success') {
  return ElMessage({
    message,
    type,
    duration: TOAST_DURATION,
    customClass: `admin-toast admin-toast--${type}`,
    showClose: false
  })
}
