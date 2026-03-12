# 后台管理系统

基于 Go + Vue 的后台管理系统，包含用户管理、意见反馈管理和页面统计管理功能。

## 技术栈

### 后端
- Go 1.24
- Gin Web框架
- GORM ORM框架
- MySQL 数据库

### 前端
- Vue 3
- Element Plus UI组件库
- Vue Router 路由管理
- Axios HTTP客户端
- Vite 构建工具

## 项目结构

```
admin-system/
├── backend/          # Go后端
│   ├── config/       # 配置管理
│   ├── database/     # 数据库连接
│   ├── models/       # 数据模型
│   ├── handlers/     # 请求处理器
│   ├── router/       # 路由配置
│   ├── go.mod
│   └── main.go
├── frontend/         # Vue前端
│   ├── src/
│   │   ├── views/    # 页面组件
│   │   ├── router/   # 路由配置
│   │   ├── api/      # API接口
│   │   └── App.vue
│   ├── package.json
│   └── vite.config.js
└── README.md
```

## 数据库表结构

### 用户表 (users)
- id: 用户ID (主键，自增)
- username: 用户名 (唯一索引)
- password: 密码 (加密存储)
- created_at: 创建时间
- updated_at: 更新时间

### 意见反馈表 (feedbacks)
- id: 意见ID (主键，自增)
- name: 姓名
- description: 详细描述
- image: 图片URL
- created_at: 创建时间
- updated_at: 更新时间

### 页面统计表 (page_stats)
- id: 统计ID (主键，自增)
- date: 日期 (唯一索引，按天统计)
- uv: 用户访问人数
- pv: 页面浏览总次数
- created_at: 创建时间
- updated_at: 更新时间

## 安装和运行

### 1. 数据库准备

创建MySQL数据库：

```sql
CREATE DATABASE admin_system CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

### 2. 后端运行

进入后端目录：

```bash
cd admin-system/backend
```

安装依赖：

```bash
go mod download
```

配置环境变量（可选，或直接修改 config/config.go 中的默认值）：

```bash
export MYSQL_HOST=127.0.0.1
export MYSQL_PORT=3306
export MYSQL_USER=root
export MYSQL_PASSWORD=123456
export MYSQL_DATABASE=admin_system
export SERVER_PORT=8080
```

运行后端：

```bash
go run main.go
```

后端服务将在 `http://localhost:8080` 启动。

### 3. 前端运行

进入前端目录：

```bash
cd admin-system/frontend
```

安装依赖：

```bash
npm install
```

运行开发服务器：

```bash
npm run dev
```

前端服务将在 `http://localhost:3000` 启动。

## API接口

### 认证接口（无需token）

- `POST /api/auth/login` - 用户登录
  - 请求体：`{"username": "用户名", "password": "密码"}`
  - 响应：`{"token": "JWT token", "user": {...}, "message": "登录成功"}`
- `POST /api/auth/logout` - 退出登录

### 用户管理（需要token）

- `GET /api/me` - 获取当前登录用户信息
- `POST /api/users` - 创建用户
- `GET /api/users` - 获取用户列表（支持分页和用户名搜索）
- `GET /api/users/:id` - 获取单个用户
- `PUT /api/users/:id` - 更新用户
- `DELETE /api/users/:id` - 删除用户

### 意见反馈管理（需要token）

- `POST /api/feedbacks` - 创建意见反馈
- `GET /api/feedbacks` - 获取意见反馈列表（支持分页和姓名搜索）
- `GET /api/feedbacks/:id` - 获取单个意见反馈
- `PUT /api/feedbacks/:id` - 更新意见反馈
- `DELETE /api/feedbacks/:id` - 删除意见反馈

### 页面统计管理（需要token）

- `POST /api/page-stats` - 创建或更新页面统计（按天，如果日期已存在则更新）
- `GET /api/page-stats` - 获取页面统计列表（支持分页和日期范围筛选）
- `GET /api/page-stats/:id` - 获取单个页面统计
- `PUT /api/page-stats/:id` - 更新页面统计
- `DELETE /api/page-stats/:id` - 删除页面统计

**注意**：所有需要token的接口都需要在请求头中携带 `Authorization: Bearer <token>`

## 功能说明

### 登录认证
- 登录：使用用户名和密码登录系统，登录成功后获得JWT token
- 退出登录：清除本地token并跳转到登录页
- 路由守卫：未登录用户访问受保护页面会自动跳转到登录页
- Token验证：所有API请求都需要在Header中携带有效的JWT token

### 用户管理
- 新增用户：创建新用户，用户名唯一，密码加密存储
- 编辑用户：可以修改用户名和密码
- 查询用户：支持按用户名搜索，支持分页
- 删除用户：删除指定用户

### 意见反馈管理
- 新增意见：创建新的意见反馈，包含姓名、详细描述和图片URL
- 编辑意见：修改意见反馈信息
- 查询意见：支持按姓名搜索，支持分页
- 删除意见：删除指定意见反馈

### 页面统计管理
- 新增统计：按天创建页面统计数据（UV和PV），如果该日期已存在则更新
- 编辑统计：修改指定日期的统计数据
- 查询统计：支持按日期范围筛选，支持分页，按日期倒序排列
- 删除统计：删除指定日期的统计记录

## 注意事项

1. **首次使用**：系统启动后需要先创建管理员才能登录。有两种方式创建管理员：
   - **方式一（推荐）**：使用初始化工具
     ```bash
     cd admin-system/backend
     go run cmd/init_admin/main.go
     # 或指定用户名和密码
     go run cmd/init_admin/main.go admin mypassword
     ```
   - **方式二**：执行SQL脚本
     ```sql
     INSERT INTO admins (username, password, created_at, updated_at) 
     VALUES (
       'admin',
       '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
       NOW(),
       NOW()
     );
     ```
     默认账号：`admin` / `admin123456`
2. 密码使用 bcrypt 加密存储，不会在API响应中返回
3. JWT token有效期为24小时，过期后需要重新登录
4. 页面统计按天统计，同一日期只能有一条记录
5. 所有API都支持CORS跨域请求
6. 前端使用代理转发API请求，开发时无需配置CORS
7. 生产环境请修改 `backend/utils/jwt.go` 中的 `jwtSecret` 为强密钥

## 生产环境部署

### 后端
1. 编译：`go build -o admin-system-backend main.go`
2. 运行：`./admin-system-backend`

### 前端
1. 构建：`npm run build`
2. 将 `dist` 目录部署到Web服务器（如Nginx）
