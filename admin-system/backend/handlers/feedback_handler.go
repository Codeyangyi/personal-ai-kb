package handlers

import (
	"admin-system/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// FeedbackHandler 意见反馈处理器
type FeedbackHandler struct {
	DB *gorm.DB
}

// NewFeedbackHandler 创建意见反馈处理器
func NewFeedbackHandler(db *gorm.DB) *FeedbackHandler {
	return &FeedbackHandler{DB: db}
}

// CreateFeedback 创建意见反馈
func (h *FeedbackHandler) CreateFeedback(c *gin.Context) {
	var feedback models.Feedback
	if err := c.ShouldBindJSON(&feedback); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 验证必填字段
	if feedback.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "姓名不能为空"})
		return
	}
	if feedback.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "详细描述不能为空"})
		return
	}

	// 创建意见反馈
	if err := h.DB.Create(&feedback).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建意见反馈失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "意见反馈创建成功", "data": feedback})
}

// GetFeedbacks 获取意见反馈列表
func (h *FeedbackHandler) GetFeedbacks(c *gin.Context) {
	var feedbacks []models.FeedbackWithUser
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	name := c.Query("name")

	query := h.DB.Table("feedbacks").
		Select("feedbacks.*, COALESCE(users.username, '') as username").
		Joins("LEFT JOIN users ON feedbacks.user_id = users.id")

	// 姓名搜索（同时搜索 name 字段和关联的 username）
	if name != "" {
		query = query.Where("feedbacks.name LIKE ? OR users.username LIKE ?", "%"+name+"%", "%"+name+"%")
	}

	// 分页
	var total int64
	query.Count(&total)
	offset := (page - 1) * pageSize
	query.Order("feedbacks.created_at DESC").Offset(offset).Limit(pageSize).Find(&feedbacks)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":      feedbacks,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetFeedback 获取单个意见反馈
func (h *FeedbackHandler) GetFeedback(c *gin.Context) {
	id := c.Param("id")
	var feedback models.Feedback

	if err := h.DB.First(&feedback, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "意见反馈不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询意见反馈失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": feedback})
}

// UpdateFeedback 更新意见反馈
func (h *FeedbackHandler) UpdateFeedback(c *gin.Context) {
	id := c.Param("id")
	var feedback models.Feedback

	if err := h.DB.First(&feedback, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "意见反馈不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询意见反馈失败"})
		return
	}

	if err := c.ShouldBindJSON(&feedback); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 验证必填字段
	if feedback.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "姓名不能为空"})
		return
	}
	if feedback.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题不能为空"})
		return
	}
	if feedback.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "详细描述不能为空"})
		return
	}

	if err := h.DB.Save(&feedback).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新意见反馈失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "意见反馈更新成功", "data": feedback})
}

// DeleteFeedback 删除意见反馈
func (h *FeedbackHandler) DeleteFeedback(c *gin.Context) {
	id := c.Param("id")

	if err := h.DB.Delete(&models.Feedback{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除意见反馈失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "意见反馈删除成功"})
}
