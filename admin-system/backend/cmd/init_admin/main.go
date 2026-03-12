package main

import (
	"admin-system/config"
	"admin-system/database"
	"admin-system/models"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	// 加载配置
	cfg := config.LoadConfig()

	// 初始化数据库
	db, err := database.InitDB(cfg)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	// 自动迁移数据库表
	err = database.AutoMigrate(db)
	if err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	// 从命令行参数获取用户名和密码，如果没有则使用默认值
	username := "admin"
	password := "admin123456"

	if len(os.Args) > 1 {
		username = os.Args[1]
	}
	if len(os.Args) > 2 {
		password = os.Args[2]
	}

	// 检查是否已存在
	var existingAdmin models.Admin
	result := db.Where("username = ?", username).First(&existingAdmin)
	if result.Error == nil {
		log.Printf("管理员 %s 已存在，跳过创建", username)
		return
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("密码加密失败: %v", err)
	}

	// 创建管理员
	admin := models.Admin{
		Username: username,
		Password: string(hashedPassword),
	}

	if err := db.Create(&admin).Error; err != nil {
		log.Fatalf("创建管理员失败: %v", err)
	}

	log.Printf("管理员创建成功！")
	log.Printf("用户名: %s", username)
	log.Printf("密码: %s", password)
	log.Printf("\n使用方法:")
	log.Printf("  go run cmd/init_admin/main.go [用户名] [密码]")
	log.Printf("  例如: go run cmd/init_admin/main.go admin mypassword")
}
