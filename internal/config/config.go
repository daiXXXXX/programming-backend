package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	CORS     CORSConfig
	JWT      JWTConfig
	OpenAI   OpenAIConfig
	Executor ExecutorConfig
}

type ServerConfig struct {
	Port    string
	GinMode string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

type CORSConfig struct {
	AllowedOrigins []string
}

type JWTConfig struct {
	Secret string
}

type OpenAIConfig struct {
	APIKey           string
	BaseURL          string
	Model            string
	ReasoningEffort  string
	RequestTimeoutMS int
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
	Prefix   string
}

type ExecutorConfig struct {
	Timeout       int
	MaxCodeLength int
}

func Load() *Config {
	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	timeout, _ := strconv.Atoi(getEnv("CODE_EXECUTION_TIMEOUT", "5000"))
	maxCodeLength, _ := strconv.Atoi(getEnv("MAX_CODE_LENGTH", "10000"))
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	openAIRequestTimeoutMS, _ := strconv.Atoi(getEnv("OPENAI_REQUEST_TIMEOUT_MS", "45000"))
	if openAIRequestTimeoutMS <= 0 {
		openAIRequestTimeoutMS = 45000
	}

	return &Config{
		Server: ServerConfig{
			Port:    getEnv("PORT", "8080"),
			GinMode: getEnv("GIN_MODE", "debug"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "root"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "xfy_bs"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       redisDB,
			Prefix:   getEnv("REDIS_PREFIX", "oj"),
		},
		CORS: CORSConfig{
			AllowedOrigins: parseOrigins(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5000")),
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
		},
		OpenAI: OpenAIConfig{
			APIKey:           getEnv("OPENAI_API_KEY", ""),
			BaseURL:          getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			Model:            getEnv("OPENAI_MODEL", "gpt-5-mini"),
			ReasoningEffort:  getEnv("OPENAI_REASONING_EFFORT", ""),
			RequestTimeoutMS: openAIRequestTimeoutMS,
		},
		Executor: ExecutorConfig{
			Timeout:       timeout,
			MaxCodeLength: maxCodeLength,
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseOrigins(origins string) []string {
	result := []string{}
	current := ""
	for _, char := range origins {
		if char == ',' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
