package config

import (
	"os"
)

// Config 系统配置
type Config struct {
	// MySQL 配置
	MySQLHost     string
	MySQLPort     string
	MySQLUser     string
	MySQLPassword string
	MySQLDatabase string

	// 服务器配置
	ServerPort string
}

// LoadConfig 从环境变量加载配置
func LoadConfig() *Config {
	return &Config{
		//MySQLDSN: getEnv("MYSQL_DSN", "personal-ai-kb:6mcETznRjwdmK7XN@tcp(127.0.0.1:3306)/ai_kb?charset=utf8mb4&parseTime=true&loc=Local"),
		MySQLHost: getEnv("MYSQL_HOST", "127.0.0.1"),
		MySQLPort: getEnv("MYSQL_PORT", "3306"),
		// MySQLUser:     getEnv("MYSQL_USER", "root"),
		// MySQLPassword: getEnv("MYSQL_PASSWORD", "123456"),
		// MySQLDatabase: getEnv("MYSQL_DATABASE", "ai_kb"),
		// ServerPort:    getEnv("SERVER_PORT", "8007"),
		MySQLUser:     getEnv("MYSQL_USER", "personal-ai-kb"),
		MySQLPassword: getEnv("MYSQL_PASSWORD", "6mcETznRjwdmK7XN"),
		MySQLDatabase: getEnv("MYSQL_DATABASE", "ai_kb"),
		ServerPort:    getEnv("SERVER_PORT", "8007"),
	}
}

// GetDSN 获取MySQL连接字符串
func (c *Config) GetDSN() string {
	return c.MySQLUser + ":" + c.MySQLPassword + "@tcp(" + c.MySQLHost + ":" + c.MySQLPort + ")/" + c.MySQLDatabase + "?charset=utf8mb4&parseTime=True&loc=Local"
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
