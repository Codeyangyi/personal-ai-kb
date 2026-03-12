# 快速开始指南

## 前置要求

1. Go 1.24 或更高版本
2. Node.js 16 或更高版本
3. MySQL 5.7 或更高版本

## 快速启动步骤

### 1. 创建数据库

登录MySQL，执行以下SQL：

```sql
CREATE DATABASE admin_system CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

### 2. 配置数据库连接

编辑 `backend/config/config.go` 文件，修改数据库连接信息，或设置环境变量：

```bash
# Windows PowerShell
$env:MYSQL_HOST="127.0.0.1"
$env:MYSQL_PORT="3306"
$env:MYSQL_USER="root"
$env:MYSQL_PASSWORD="123456"
$env:MYSQL_DATABASE="admin_system"
$env:SERVER_PORT="8080"

# Linux/Mac
export MYSQL_HOST=127.0.0.1
export MYSQL_PORT=3306
export MYSQL_USER=root
export MYSQL_PASSWORD=123456
export MYSQL_DATABASE=admin_system
export SERVER_PORT=8080
```

### 3. 启动后端服务

#### Windows:
双击运行 `start-backend.bat` 或在命令行执行：

```bash
cd admin-system/backend
go mod download
go run main.go
```

#### Linux/Mac:
```bash
cd admin-system/backend
go mod download
go run main.go
```

后端服务将在 `http://localhost:8080` 启动。

### 4. 启动前端服务

#### Windows:
双击运行 `start-frontend.bat` 或在命令行执行：

```bash
cd admin-system/frontend
npm install
npm run dev
```

#### Linux/Mac:
```bash
cd admin-system/frontend
npm install
npm run dev
```

前端服务将在 `http://localhost:3000` 启动。

### 5. 创建初始管理员用户

在数据库中执行以下SQL创建默认管理员（或使用提供的SQL脚本）：

```sql
-- 默认用户名: admin, 密码: admin123456
INSERT INTO users (username, password, created_at, updated_at) 
VALUES (
  'admin',
  '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
  NOW(),
  NOW()
);
```

或者执行脚本文件：
```bash
mysql -u root -p admin_system < admin-system/backend/scripts/init_admin.sql
```

### 6. 访问系统

打开浏览器访问：`http://localhost:3000`

系统会自动跳转到登录页面，使用默认管理员账号登录：
- 用户名：`admin`
- 密码：`admin123456`

## 功能测试

### 登录功能
1. 访问系统会自动跳转到登录页
2. 输入用户名和密码登录
3. 登录成功后进入用户管理页面
4. 点击右上角"退出登录"按钮可以退出系统

### 用户管理
1. 点击左侧菜单"用户管理"
2. 点击"新增用户"按钮
3. 输入用户名和密码，点击确定
4. 可以编辑、查询、删除用户

### 意见反馈管理
1. 点击左侧菜单"意见反馈管理"
2. 点击"新增意见"按钮
3. 输入姓名、详细描述和图片URL，点击确定
4. 可以编辑、查询、删除意见反馈

### 页面统计管理
1. 点击左侧菜单"页面统计管理"
2. 点击"新增统计"按钮
3. 选择日期，输入UV和PV值，点击确定
4. 可以按日期范围查询统计数据
5. 可以编辑、删除统计记录

## 常见问题

### 1. 数据库连接失败
- 检查MySQL服务是否启动
- 检查数据库连接信息是否正确
- 检查数据库是否已创建

### 2. 前端无法连接后端
- 检查后端服务是否启动（http://localhost:8080）
- 检查前端代理配置（vite.config.js）

### 3. 端口被占用
- 修改 `backend/config/config.go` 中的 `SERVER_PORT`
- 修改 `frontend/vite.config.js` 中的 `server.port`

## 下一步

- 查看 `README.md` 了解详细功能说明
- 根据需要修改配置和样式
- 添加更多功能模块
