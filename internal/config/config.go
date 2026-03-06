// Package config centraliza configurações do ETL BUFFS via .env e variáveis de ambiente.
// Todas as variáveis são prefixadas com BUFFS_ETL_ (ex: BUFFS_ETL_DB_URL).
package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config agrupa todas as configurações do ETL.
type Config struct {
	Server      ServerConfig
	DB          DBConfig
	Upload      UploadConfig
	Export      ExportConfig
	Worker      WorkerConfig
	InternalKey string // X-Internal-Key para autenticação inter-serviço
}

type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DBConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
}

type UploadConfig struct {
	MaxFileSize int64  // bytes
	TempDir     string // diretório temporário para uploads
}

type ExportConfig struct {
	IncludeRefSheet bool // incluir aba ANIMAIS_REF por padrão
}

type WorkerConfig struct {
	AsyncThreshold int // linhas acima desse valor → processamento assíncrono via goroutine
}

// Load carrega a configuração a partir de variáveis de ambiente e .env.
func Load() *Config {
	// Tenta ler o .env (ignora erro se não existir)
	_ = godotenv.Load()

	readTimeout, _ := time.ParseDuration(envOrDefault("BUFFS_ETL_READ_TIMEOUT", "30s"))
	writeTimeout, _ := time.ParseDuration(envOrDefault("BUFFS_ETL_WRITE_TIMEOUT", "60s"))
	connLifetime, _ := time.ParseDuration(envOrDefault("BUFFS_ETL_DB_MAX_CONN_LIFETIME", "30m"))

	return &Config{
		Server: ServerConfig{
			Port:         envInt("BUFFS_ETL_PORT", 3001),
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
		},
		DB: DBConfig{
			URL:             envOrDefault("BUFFS_ETL_DB_URL", "postgresql://postgres:postgres@localhost:5432/buffs_db"),
			MaxConns:        int32(envInt("BUFFS_ETL_DB_MAX_CONNS", 20)),
			MinConns:        int32(envInt("BUFFS_ETL_DB_MIN_CONNS", 5)),
			MaxConnLifetime: connLifetime,
		},
		Upload: UploadConfig{
			MaxFileSize: int64(envInt("BUFFS_ETL_MAX_FILE_SIZE", 50*1024*1024)),
			TempDir:     envOrDefault("BUFFS_ETL_TEMP_DIR", "./temp/uploads"),
		},
		Export: ExportConfig{
			IncludeRefSheet: envOrDefault("BUFFS_ETL_INCLUDE_REF_SHEET", "true") == "true",
		},
		Worker: WorkerConfig{
			AsyncThreshold: envInt("BUFFS_ETL_ASYNC_THRESHOLD", 1000),
		},
		InternalKey: envOrDefault("BUFFS_ETL_INTERNAL_KEY", ""),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
