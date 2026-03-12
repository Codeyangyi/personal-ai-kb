package handlers

import (
	"admin-system/models"
	"admin-system/utils"
	"net/http"

	"log"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	DB *gorm.DB
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{DB: db}
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token   string      `json:"token"`
	User    interface{} `json:"user"`
	Message string      `json:"message"`
}

// Login 登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 查询管理员
	var admin models.Admin
	if err := h.DB.Where("username = ?", req.Username).First(&admin).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("登录失败: 用户名 %s 不存在", req.Username)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
			return
		}
		log.Printf("查询管理员失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询管理员失败: " + err.Error()})
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(req.Password)); err != nil {
		log.Printf("登录失败: 用户 %s 密码错误", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	// 生成token
	token, err := utils.GenerateToken(admin.ID, admin.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成token失败: " + err.Error()})
		return
	}

	// 返回结果（不返回密码）
	admin.Password = ""
	c.JSON(http.StatusOK, LoginResponse{
		Token:   token,
		User:    admin,
		Message: "登录成功",
	})
}

// Logout 退出登录
func (h *AuthHandler) Logout(c *gin.Context) {
	// JWT是无状态的，退出登录主要是前端删除token
	// 如果需要实现token黑名单，可以在这里添加逻辑
	c.JSON(http.StatusOK, gin.H{"message": "退出登录成功"})
}

// GetCurrentUser 获取当前管理员信息
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	// 从中间件获取用户信息
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	var admin models.Admin
	if err := h.DB.First(&admin, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询管理员失败"})
		return
	}

	admin.Password = ""
	c.JSON(http.StatusOK, gin.H{"data": admin})
}
