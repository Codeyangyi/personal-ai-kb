-- 初始化管理员用户脚本
-- 默认用户名: admin
-- 默认密码: admin123456
-- 密码已使用bcrypt加密（cost=10）

INSERT INTO admins (username, password, created_at, updated_at) 
VALUES (
  'admin',
  '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
  NOW(),
  NOW()
) ON DUPLICATE KEY UPDATE username=username;
