package handlers

import (
	"admin-system/models"
	"net/http"
	"strconv"
	"strings"
	"log"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserHandler 用户处理器
type UserHandler struct {
	DB *gorm.DB
}

// NewUserHandler 创建用户处理器
func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{DB: db}
}

// CreateUser 创建用户
func (h *UserHandler) CreateUser(c *gin.Context) {
	var user models.User
	if err := c.ShouldBindJSON(&user); err != nil {
		log.Printf("创建用户 - JSON解析失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 添加调试日志
	log.Printf("创建用户 - 接收到的数据: username=%s, password长度=%d, password是否为空=%v", 
		user.Username, len(user.Password), user.Password == "")

	// 验证用户名
	if user.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能为空"})
		return
	}

	// 检查用户名是否已存在
	var existingUser models.User
	if err := h.DB.Where("username = ?", user.Username).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名已存在"})
		return
	}

	// 清理密码（去除前后空格，与登录验证保持一致）
	// 注意：这里trim密码是为了与personal-ai-kb登录验证保持一致
	// 登录验证时会trim密码，所以创建时也应该trim，避免前后空格导致验证失败
	cleanedPassword := strings.TrimSpace(user.Password)
	if cleanedPassword == "" {
		log.Printf("创建用户 - 密码为空或只包含空格: username=%s, 原始密码长度=%d", user.Username, len(user.Password))
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码不能为空"})
		return
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cleanedPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}
	user.Password = string(hashedPassword)
	
	// 记录日志（不记录实际密码，只记录hash前缀）
	if len(user.Password) > 20 {
		log.Printf("创建用户: %s, 密码hash前缀: %s... (长度: %d)", user.Username, user.Password[:20], len(user.Password))
	}

	// 创建用户
	if err := h.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建用户失败: " + err.Error()})
		return
	}

	// 不返回密码
	user.Password = ""
	c.JSON(http.StatusOK, gin.H{"message": "用户创建成功", "data": user})
}

// GetUsers 获取用户列表
func (h *UserHandler) GetUsers(c *gin.Context) {
	var users []models.User
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	username := c.Query("username")

	query := h.DB.Model(&models.User{})

	// 用户名搜索
	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}

	// 分页
	var total int64
	query.Count(&total)
	offset := (page - 1) * pageSize
	query.Offset(offset).Limit(pageSize).Find(&users)

	// 清空所有用户的密码字段（不返回给前端）
	for i := range users {
		users[i].Password = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":     users,
			"total":    total,
			"page":     page,
			"page_size": pageSize,
		},
	})
}

// GetUser 获取单个用户
func (h *UserHandler) GetUser(c *gin.Context) {
	id := c.Param("id")
	var user models.User

	if err := h.DB.First(&user, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}

	user.Password = ""
	c.JSON(http.StatusOK, gin.H{"data": user})
}

// UpdateUser 更新用户
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	var user models.User

	if err := h.DB.First(&user, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}

	var updateData struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 如果更新用户名，检查是否重复
	if updateData.Username != "" && updateData.Username != user.Username {
		var existingUser models.User
		if err := h.DB.Where("username = ? AND id != ?", updateData.Username, id).First(&existingUser).Error; err == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "用户名已存在"})
			return
		}
		user.Username = updateData.Username
	}

	// 如果更新密码，加密新密码
	if updateData.Password != "" {
		// 清理密码（去除前后空格，与创建用户和登录验证保持一致）
		cleanedPassword := strings.TrimSpace(updateData.Password)
		if cleanedPassword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "密码不能为空"})
			return
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cleanedPassword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
			return
		}
		user.Password = string(hashedPassword)
	}

	if err := h.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新用户失败: " + err.Error()})
		return
	}

	user.Password = ""
	c.JSON(http.StatusOK, gin.H{"message": "用户更新成功", "data": user})
}

// DeleteUser 删除用户
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")

	if err := h.DB.Delete(&models.User{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除用户失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "用户删除成功"})
}
