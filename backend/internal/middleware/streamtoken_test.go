package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"camguid/internal/pkg/streamtoken"

	"github.com/gofiber/fiber/v2"
)

func TestStreamTokenMiddleware(t *testing.T) {
	signer, _ := streamtoken.NewSigner("relay-middleware-test-secret-which-is-random")
	app := fiber.New()
	app.Post("/start", StreamTokenMiddleware(signer), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"device_id": GetStreamDeviceID(c)})
	})

	status := func(headers map[string]string) int {
		req := httptest.NewRequest("POST", "/start", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		return resp.StatusCode
	}

	t.Run("missing token rejected", func(t *testing.T) {
		if got := status(nil); got != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	t.Run("valid X-Stream-Token accepted", func(t *testing.T) {
		tok, _ := signer.Sign("device-x", "user-1", time.Minute)
		if got := status(map[string]string{"X-Stream-Token": tok}); got != fiber.StatusOK {
			t.Errorf("status = %d, want 200", got)
		}
	})

	t.Run("valid bearer token accepted", func(t *testing.T) {
		tok, _ := signer.Sign("device-y", "user-1", time.Minute)
		if got := status(map[string]string{"Authorization": "Bearer " + tok}); got != fiber.StatusOK {
			t.Errorf("status = %d, want 200", got)
		}
	})

	t.Run("tampered token rejected", func(t *testing.T) {
		tok, _ := signer.Sign("device-z", "user-1", time.Minute)
		if got := status(map[string]string{"X-Stream-Token": tok + "x"}); got != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		tok, _ := signer.Sign("device-w", "user-1", -time.Minute)
		if got := status(map[string]string{"X-Stream-Token": tok}); got != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})
}