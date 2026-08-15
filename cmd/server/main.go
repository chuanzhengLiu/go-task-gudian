package main

import (
	"ancient-texts-backend/internal/api"
	"ancient-texts-backend/internal/config"
	"ancient-texts-backend/internal/model"
	"ancient-texts-backend/internal/util"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := config.Load(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := os.MkdirAll(config.AppConfig.UPLOAD_DIR, 0755); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}
	if err := os.MkdirAll(config.AppConfig.TILE_DIR, 0755); err != nil {
		log.Fatalf("Failed to create tile directory: %v", err)
	}

	if err := model.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	if err := model.AutoMigrate(); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	if err := seedAdminUser(); err != nil {
		log.Printf("Warning: Failed to seed admin user: %v", err)
	}

	if err := seedVariantData(); err != nil {
		log.Printf("Warning: Failed to seed variant data: %v", err)
	}

	r := gin.Default()

	api.SetupRoutes(r)

	addr := fmt.Sprintf(":%s", config.AppConfig.SERVER_PORT)
	log.Printf("Server starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func seedAdminUser() error {
	var count int64
	model.DB.Model(&model.User{}).Where("role = ?", model.RoleAdmin).Count(&count)
	if count > 0 {
		return nil
	}

	hashedPassword, err := util.HashPassword("Admin12345!")
	if err != nil {
		return err
	}

	admin := model.User{
		Email:                "admin@example.com",
		PasswordHash:         hashedPassword,
		Name:                 "系统管理员",
		Role:                 model.RoleAdmin,
		Status:               model.UserStatusActive,
		LastPasswordChange:   time.Now(),
		TOTPEnabled:          false,
	}

	if err := model.DB.Create(&admin).Error; err != nil {
		return err
	}

	log.Println("Default admin user created: admin@example.com / Admin12345!")
	return nil
}

func seedVariantData() error {
	var count int64
	model.DB.Model(&model.VariantChar{}).Count(&count)
	if count > 0 {
		return nil
	}

	commonVariants := []model.VariantChar{
		{VariantChar: "爲", StandardChar: "为", Source: "通用规范汉字表", Verified: true},
		{VariantChar: "裡", StandardChar: "里", Source: "通用规范汉字表", Verified: true},
		{VariantChar: "後", StandardChar: "后", Source: "通用规范汉字表", Verified: true},
		{VariantChar: "纔", StandardChar: "才", Source: "通用规范汉字表", Verified: true},
		{VariantChar: "礙", StandardChar: "碍", Source: "通用规范汉字表", Verified: true},
		{VariantChar: "巖", StandardChar: "岩", Source: "通用规范汉字表", Verified: true},
		{VariantChar: "灋", StandardChar: "法", Source: "异体字表", Verified: true},
		{VariantChar: "槩", StandardChar: "概", Source: "异体字表", Verified: true},
		{VariantChar: "撡", StandardChar: "操", Source: "异体字表", Verified: true},
		{VariantChar: "栁", StandardChar: "柳", Source: "异体字表", Verified: true},
		{VariantChar: "邨", StandardChar: "村", Source: "异体字表", Verified: true},
		{VariantChar: "聼", StandardChar: "听", Source: "异体字表", Verified: true},
		{VariantChar: "亜", StandardChar: "亚", Source: "异体字表", Verified: true},
		{VariantChar: "悪", StandardChar: "恶", Source: "异体字表", Verified: true},
		{VariantChar: "弐", StandardChar: "二", Source: "异体字表", Verified: true},
		{VariantChar: "壱", StandardChar: "一", Source: "异体字表", Verified: true},
		{VariantChar: "従", StandardChar: "从", Source: "异体字表", Verified: true},
		{VariantChar: "廻", StandardChar: "回", Source: "异体字表", Verified: true},
		{VariantChar: "拠", StandardChar: "据", Source: "异体字表", Verified: true},
		{VariantChar: "挙", StandardChar: "举", Source: "异体字表", Verified: true},
		{VariantChar: "揺", StandardChar: "摇", Source: "异体字表", Verified: true},
		{VariantChar: "摂", StandardChar: "摄", Source: "异体字表", Verified: true},
		{VariantChar: "叙", StandardChar: "叙", Source: "异体字表", Verified: true},
		{VariantChar: "叠", StandardChar: "叠", Source: "异体字表", Verified: true},
		{VariantChar: "悩", StandardChar: "恼", Source: "异体字表", Verified: true},
		{VariantChar: "愼", StandardChar: "慎", Source: "异体字表", Verified: true},
		{VariantChar: "戯", StandardChar: "戏", Source: "异体字表", Verified: true},
		{VariantChar: "戦", StandardChar: "战", Source: "异体字表", Verified: true},
		{VariantChar: "拝", StandardChar: "拜", Source: "异体字表", Verified: true},
		{VariantChar: "攒", StandardChar: "攒", Source: "异体字表", Verified: true},
		{VariantChar: "敍", StandardChar: "叙", Source: "异体字表", Verified: true},
		{VariantChar: "曽", StandardChar: "曾", Source: "异体字表", Verified: true},
		{VariantChar: "札", StandardChar: "扎", Source: "异体字表", Verified: true},
		{VariantChar: "栄", StandardChar: "荣", Source: "异体字表", Verified: true},
		{VariantChar: "桜", StandardChar: "樱", Source: "异体字表", Verified: true},
		{VariantChar: "検", StandardChar: "检", Source: "异体字表", Verified: true},
		{VariantChar: "楽", StandardChar: "乐", Source: "异体字表", Verified: true},
		{VariantChar: "様", StandardChar: "样", Source: "异体字表", Verified: true},
		{VariantChar: "歯", StandardChar: "齿", Source: "异体字表", Verified: true},
		{VariantChar: "歴", StandardChar: "历", Source: "异体字表", Verified: true},
		{VariantChar: "残", StandardChar: "残", Source: "异体字表", Verified: true},
		{VariantChar: "殴", StandardChar: "殴", Source: "异体字表", Verified: true},
		{VariantChar: "殺", StandardChar: "杀", Source: "异体字表", Verified: true},
		{VariantChar: "殻", StandardChar: "壳", Source: "异体字表", Verified: true},
		{VariantChar: "比", StandardChar: "比", Source: "异体字表", Verified: true},
		{VariantChar: "毎", StandardChar: "每", Source: "异体字表", Verified: true},
		{VariantChar: "気", StandardChar: "气", Source: "异体字表", Verified: true},
		{VariantChar: "沢", StandardChar: "泽", Source: "异体字表", Verified: true},
		{VariantChar: "沿", StandardChar: "沿", Source: "异体字表", Verified: true},
		{VariantChar: "泉", StandardChar: "泉", Source: "异体字表", Verified: true},
	}

	for _, v := range commonVariants {
		if err := model.DB.Create(&v).Error; err != nil {
			log.Printf("Failed to create variant %s: %v", v.VariantChar, err)
		}
	}

	log.Println("Seed variant data created with 50 common variant characters")
	return nil
}
