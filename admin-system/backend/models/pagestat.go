package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// Date 自定义日期类型，只包含日期部分
type Date struct {
	time.Time
}

// UnmarshalJSON 实现JSON反序列化
func (d *Date) UnmarshalJSON(data []byte) error {
	var dateStr string
	if err := json.Unmarshal(data, &dateStr); err != nil {
		return err
	}
	if dateStr == "" {
		d.Time = time.Time{}
		return nil
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return err
	}
	d.Time = t
	return nil
}

// MarshalJSON 实现JSON序列化
func (d Date) MarshalJSON() ([]byte, error) {
	if d.Time.IsZero() {
		return []byte(`""`), nil
	}
	return json.Marshal(d.Time.Format("2006-01-02"))
}

// Value 实现driver.Valuer接口
func (d Date) Value() (driver.Value, error) {
	if d.Time.IsZero() {
		return nil, nil
	}
	return d.Time.Format("2006-01-02"), nil
}

// Scan 实现sql.Scanner接口
func (d *Date) Scan(value interface{}) error {
	if value == nil {
		d.Time = time.Time{}
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		d.Time = v
		return nil
	case []byte:
		return d.scanBytes(v)
	case string:
		return d.scanBytes([]byte(v))
	}
	return fmt.Errorf("无法扫描 %T 到 Date", value)
}

func (d *Date) scanBytes(src []byte) error {
	t, err := time.Parse("2006-01-02", string(src))
	if err != nil {
		return err
	}
	d.Time = t
	return nil
}

// PageStat 页面统计模型
type PageStat struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Date      Date   `gorm:"type:date;not null;uniqueIndex" json:"date"` // 按天统计，日期唯一
	UV        int    `gorm:"type:int;default:0;comment:用户访问人数" json:"uv"`
	PV        int    `gorm:"type:int;default:0;comment:页面浏览总次数" json:"pv"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (PageStat) TableName() string {
	return "page_stats"
}
