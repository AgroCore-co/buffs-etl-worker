package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jaobarreto/buffs-etl-worker/internal/dto"
)

// RateLimiter implementa rate limiting in-memory por propriedade.
// Máximo de N imports por hora por propriedade.
type RateLimiter struct {
	mu       sync.RWMutex
	counters map[string][]time.Time // property_id → timestamps dos imports
	max      int
	window   time.Duration
}

// NewRateLimiter cria um rate limiter com o máximo de requests por hora.
func NewRateLimiter(maxPerHour int) *RateLimiter {
	return &RateLimiter{
		counters: make(map[string][]time.Time),
		max:      maxPerHour,
		window:   time.Hour,
	}
}

// CheckImportLimit verifica se a propriedade pode fazer mais imports.
// Extrai property_id do query param ou path param.
func (rl *RateLimiter) CheckImportLimit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		propertyID := c.Query("property_id")
		if propertyID == "" {
			return c.Next()
		}

		rl.mu.Lock()
		defer rl.mu.Unlock()

		now := time.Now()
		cutoff := now.Add(-rl.window)

		// Remove entradas fora da janela
		timestamps := rl.counters[propertyID]
		valid := timestamps[:0]
		for _, t := range timestamps {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= rl.max {
			return c.Status(fiber.StatusTooManyRequests).JSON(dto.ErrorResponse{
				Code:    "RATE_LIMIT_EXCEEDED",
				Message: "Limite de imports por hora excedido. Tente novamente mais tarde.",
			})
		}

		valid = append(valid, now)
		rl.counters[propertyID] = valid

		return c.Next()
	}
}
