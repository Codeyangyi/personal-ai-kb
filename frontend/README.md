# 前端项目说明

## 安装依赖

首次使用前，必须先安装依赖：

```bash
npm install
```

这会安装所有必需的依赖包，包括：
- Vue 3
- Vite
- Axios
- @vitejs/plugin-vue

## 常见问题

### vite: command not found

**原因**：依赖没有安装或安装不完整。

**解决方法**：

1. **确保已安装依赖**：
   ```bash
   npm install
   ```

2. **如果仍然报错，尝试使用 npx**：
   ```bash
   # 开发模式
   npx vite
   
   # 构建
   npx vite build
   ```

3. **或者使用 npm scripts**（推荐）：
   ```bash
   # 开发模式
   npm run dev
   
   # 构建
   npm run build
   ```

4. **清理并重新安装**：
   ```bash
   rm -rf node_modules package-lock.json
   npm install
   ```

## 开发命令

```bash
# 开发模式（热重载）
npm run dev
# 或
npx vite

# 构建生产版本
npm run build
# 或
npx vite build

# 预览构建结果
npm run preview
# 或
npx vite preview
```

## 目录结构

```
frontend/
├── src/
│   ├── App.vue      # 主组件
│   └── main.js      # 入口文件
├── index.html       # HTML模板
├── package.json     # 依赖配置
├── vite.config.js   # Vite配置
└── dist/            # 构建输出目录（运行 build 后生成）
```

## 注意事项

- 确保 Node.js 版本 >= 16
- 首次安装可能需要几分钟时间
- 如果网络较慢，可以使用国内镜像：
  ```bash
  npm config set registry https://registry.npmmirror.com
  ```

