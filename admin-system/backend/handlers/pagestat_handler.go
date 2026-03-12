package handlers

import (
	"admin-system/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PageStatHandler 页面统计处理器
type PageStatHandler struct {
	DB *gorm.DB
}

// NewPageStatHandler 创建页面统计处理器
func NewPageStatHandler(db *gorm.DB) *PageStatHandler {
	return &PageStatHandler{DB: db}
}

// CreatePageStat 创建或更新页面统计
func (h *PageStatHandler) CreatePageStat(c *gin.Context) {
	var stat models.PageStat
	if err := c.ShouldBindJSON(&stat); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 如果没有指定日期，使用今天
	if stat.Date.IsZero() {
		stat.Date = models.Date{Time: time.Now()}
	}

	// 只保留日期部分（去掉时间）
	stat.Date.Time = time.Date(stat.Date.Year(), stat.Date.Month(), stat.Date.Day(), 0, 0, 0, 0, stat.Date.Location())

	// 检查该日期是否已存在
	var existingStat models.PageStat
	err := h.DB.Where("date = ?", stat.Date.Time.Format("2006-01-02")).First(&existingStat).Error

	if err == nil {
		// 如果存在，更新
		existingStat.UV = stat.UV
		existingStat.PV = stat.PV
		if err := h.DB.Save(&existingStat).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新页面统计失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "页面统计更新成功", "data": existingStat})
	} else if err == gorm.ErrRecordNotFound {
		// 如果不存在，创建
		if err := h.DB.Create(&stat).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建页面统计失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "页面统计创建成功", "data": stat})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询页面统计失败"})
		return
	}
}

// GetPageStats 获取页面统计列表（按天统计）
func (h *PageStatHandler) GetPageStats(c *gin.Context) {
	var stats []models.PageStat
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := h.DB.Model(&models.PageStat{})

	// 日期范围筛选
	if startDate != "" {
		if start, err := time.Parse("2006-01-02", startDate); err == nil {
			query = query.Where("date >= ?", start.Format("2006-01-02"))
		}
	}
	if endDate != "" {
		if end, err := time.Parse("2006-01-02", endDate); err == nil {
			query = query.Where("date <= ?", end.Format("2006-01-02"))
		}
	}

	// 分页
	var total int64
	query.Count(&total)
	offset := (page - 1) * pageSize
	query.Order("date DESC").Offset(offset).Limit(pageSize).Find(&stats)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":      stats,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetPageStat 获取单个页面统计
func (h *PageStatHandler) GetPageStat(c *gin.Context) {
	id := c.Param("id")
	var stat models.PageStat

	if err := h.DB.First(&stat, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "页面统计不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询页面统计失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stat})
}

// UpdatePageStat 更新页面统计
func (h *PageStatHandler) UpdatePageStat(c *gin.Context) {
	id := c.Param("id")
	var stat models.PageStat

	if err := h.DB.First(&stat, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "页面统计不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询页面统计失败"})
		return
	}

	var updateData struct {
		UV int `json:"uv"`
		PV int `json:"pv"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	stat.UV = updateData.UV
	stat.PV = updateData.PV

	if err := h.DB.Save(&stat).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新页面统计失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "页面统计更新成功", "data": stat})
}

// DeletePageStat 删除页面统计
func (h *PageStatHandler) DeletePageStat(c *gin.Context) {
	id := c.Param("id")

	if err := h.DB.Delete(&models.PageStat{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除页面统计失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "页面统计删除成功"})
}
