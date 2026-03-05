// Package logger fornece logging estruturado via zap para todo o ETL BUFFS.
// Nunca use fmt.Println — sempre passe o *zap.Logger por injeção.
package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New cria um *zap.Logger configurado para o ambiente.
// Em produção (BUFFS_ETL_ENV=production), usa JSON. Caso contrário, console colorido.
func New() *zap.Logger {
	env := os.Getenv("BUFFS_ETL_ENV")

	var cfg zap.Config
	if env == "production" {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logger, err := cfg.Build(zap.AddCallerSkip(0))
	if err != nil {
		// Fallback extremo — nunca deveria acontecer
		fallback, _ := zap.NewProduction()
		return fallback
	}

	return logger
}
