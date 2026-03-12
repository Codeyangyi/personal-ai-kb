package main

import (
	"admin-system/config"
	"admin-system/database"
	"admin-system/router"
	"log"
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

	// 初始化路由
	r := router.SetupRouter(db)

	// 启动服务器
	log.Printf("服务器启动在端口 %s", cfg.ServerPort)
	if err := r.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
