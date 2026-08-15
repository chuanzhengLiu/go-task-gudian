package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DB_DSN            string
	JWT_SECRET        string
	JWT_REFRESH_SECRET string
	UPLOAD_DIR        string
	TILE_DIR          string
	SERVER_PORT       string
	BCRYPT_COST       int
	TOKEN_TTL         int
	REFRESH_TOKEN_TTL int
	IMAGE_SIGN_SECRET string
	IMAGE_SIGN_TTL    int
}

var AppConfig Config

func Load() error {
	_ = godotenv.Load()

	AppConfig = Config{
		DB_DSN:             getEnv("DB_DSN", "root:ancientroot@tcp(localhost:3306)/ancient_texts_db?charset=utf8mb4&parseTime=true"),
		JWT_SECRET:         getEnv("JWT_SECRET", "development-secret-key"),
		JWT_REFRESH_SECRET: getEnv("JWT_REFRESH_SECRET", "development-refresh-secret-key"),
		UPLOAD_DIR:         getEnv("UPLOAD_DIR", "./uploads"),
		TILE_DIR:           getEnv("TILE_DIR", "./tiles"),
		SERVER_PORT:        getEnv("SERVER_PORT", "8080"),
		BCRYPT_COST:        getEnvInt("BCRYPT_COST", 12),
		TOKEN_TTL:          getEnvInt("TOKEN_TTL", 7200),
		REFRESH_TOKEN_TTL:  getEnvInt("REFRESH_TOKEN_TTL", 604800),
		IMAGE_SIGN_SECRET:  getEnv("IMAGE_SIGN_SECRET", "image-sign-secret"),
		IMAGE_SIGN_TTL:     getEnvInt("IMAGE_SIGN_TTL", 3600),
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
