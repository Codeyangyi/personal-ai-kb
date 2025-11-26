# 阶段1：构建 Vue3 项目（基于 Node 镜像编译打包）
FROM node:18-alpine AS build-stage
WORKDIR /app  
# 复制依赖文件（利用 Docker 缓存，避免每次改代码都重新装依赖）
COPY package*.json ./
# 安装依赖（国内加速用淘宝镜像，国外可删除 --registry 参数）
RUN npm install --registry=https://registry.npm.taobao.org
# 复制所有 Vue3 源码
COPY . .
# 打包生成静态资源（输出到 dist 目录）
RUN npm run build

# 阶段2：部署静态资源（基于 Nginx 轻量镜像，体积仅 5MB 左右）
FROM nginx:alpine
# 从构建阶段复制打包后的 dist 目录到 Nginx 静态文件目录
COPY --from=build-stage /app/dist /usr/share/nginx/html
# 替换 Nginx 默认配置（解决跨域和 Vue3 SPA 路由刷新 404 问题）
COPY nginx.conf /etc/nginx/conf.d/default.conf
# 暴露 Nginx 端口（容器内端口 80）
EXPOSE 8006
# 启动 Nginx（前台运行，确保容器不退出）
CMD ["nginx", "-g", "daemon off;"]