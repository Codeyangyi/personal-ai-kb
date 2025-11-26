# 阶段1：编译 Golang 项目（基于官方 Go 镜像，含编译环境）
FROM golang:1.24-alpine AS build-stage
WORKDIR /app
# 复制依赖管理文件（利用 Docker 缓存，加快构建）
COPY go.mod go.sum ./
# 国内加速依赖下载（国外可删除 GOPROXY 配置）
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download
# 复制所有 Golang 源码
COPY . .
# 编译二进制文件（关键参数，适配 Alpine 镜像）
# CGO_ENABLED=0：禁用 CGO，生成静态链接二进制（无系统依赖）
# GOOS=linux：目标系统为 Linux
# -ldflags="-w -s"：去除调试信息，减小二进制文件体积
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o app main.go

# 阶段2：运行二进制文件（基于 Alpine 轻量镜像）
FROM alpine:3.20
WORKDIR /app
# （可选）安装时区数据库（如果后端需要处理时区，如时间格式化）
RUN apk --no-cache add tzdata && ln -snf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo "Asia/Shanghai" > /etc/timezone
# 从构建阶段复制编译好的二进制文件
COPY --from=build-stage /app/app .
# （可选）复制配置文件（如 config.yaml，若用环境变量传递配置可省略）
# COPY --from=build-stage /app/config.yaml .
# 暴露后端服务端口（与 Golang 代码中监听端口一致，如 8080）
EXPOSE 8005
# 启动后端服务（直接运行二进制文件）
CMD ["./app"]