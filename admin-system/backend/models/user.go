package models

import (
	"time"
)

// User 用户模型
type User struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Username string `gorm:"type:varchar(100);not null;uniqueIndex" json:"username"`
	Password string `gorm:"type:varchar(255);not null" json:"password"` // 允许接收密码字段，但在返回时手动清空
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}
