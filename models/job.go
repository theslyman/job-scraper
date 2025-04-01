package models

import (
	"time"
	"gorm.io/gorm"
)

type Job struct {
	gorm.Model
	Title         string
	Description   string
	Type          string
	Requirements  string
	Email         string
	Phone         string
	DatePublished time.Time
	Source        string
}
