package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// LoginRateLimit enforces a per (IP + username) failed-login budget using a
// window stored in Redis. Once the budget is exhausted, login attempts for
// that key are rejected with 429 until the window elapses. This mitigates
// online brute force without affecting legit traffic on other accounts.
func LoginRateLimit(rdb *redis.Client, limit int, window time.Duration) fiber.Handler {
	if rdb == nil || limit <= 0 {
		return func(c *fiber.Ctx) error { return c.Next() }
	}

	return func(c *fiber.Ctx) error {
		var body struct {
			Username string `json:"username"`
		}
		// Parse errors are reported by the login handler itself.
		_ = c.BodyParser(&body)

		ctx := context.Background()
		key := LoginFailKey(c.IP(), body.Username)

		count, err := rdb.Get(ctx, key).Int64()
		if err == nil && count >= int64(limit) {
			ttl, _ := rdb.TTL(ctx, key).Result()
			if ttl < 0 {
				ttl = window
			}
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "LOGIN_RATE_LIMITED",
					"message": fmt.Sprintf("Too many failed login attempts. Try again in %ds.", int(ttl.Seconds())),
				},
			})
		}

		return c.Next()
	}
}

// LoginFailKey is the redis key for failed-login counting of an IP+username
// pair. Shared between the gateway middleware and the login handler so the
// count and the check reference the same bucket.
func LoginFailKey(ip, username string) string {
	return "login:fail:" + ip + ":" + username
}

// RecordLoginFailure increments the failure counter for a key and starts the
// window on first failure.
func RecordLoginFailure(rdb *redis.Client, key string, window time.Duration) {
	if rdb == nil {
		return
	}
	ctx := context.Background()
	if err := rdb.Incr(ctx, key).Err(); err != nil {
		return
	}
	rdb.Expire(ctx, key, window)
}