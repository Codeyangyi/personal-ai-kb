package models

import "time"

// User 用户模型（对应users表）
type User struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Username  string    `gorm:"column:username;uniqueIndex;not null;size:100" json:"username"` // 用户名
	Password  string    `gorm:"column:password;not null;size:255" json:"-"`                     // 密码（不返回给前端）
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`                           // 创建时间
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`                           // 更新时间
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}
