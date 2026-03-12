package models

import (
	"time"
)

// Feedback 意见反馈模型
type Feedback struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint   `gorm:"type:int unsigned;default:0;index" json:"user_id"`
	Name        string `gorm:"type:varchar(100);not null" json:"name"`
	Title       string `gorm:"type:varchar(255);not null" json:"title"`
	Description string `gorm:"type:text;not null" json:"description"`
	Image       string `gorm:"type:varchar(512)" json:"image"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FeedbackWithUser 带用户名的意见反馈（用于列表查询）
type FeedbackWithUser struct {
	Feedback
	Username string `json:"username" gorm:"->"` // 只读字段，来自 JOIN 查询
}

// TableName 指定表名
func (Feedback) TableName() string {
	return "feedbacks"
}
