package middleware

import (
	"strings"

	"camguid/internal/pkg/streamtoken"

	"github.com/gofiber/fiber/v2"
)

// StreamTokenMiddleware authenticates relay requests using a short-lived HMAC
// stream token (minted by the API). The token may travel in an Authorization
// bearer header, an X-Stream-Token header, or a query-string ?token=. On
// success the target device and user ids are placed in locals.
func StreamTokenMiddleware(signer *streamtoken.Signer) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := extractStreamToken(c)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "STREAM_TOKEN_REQUIRED",
					"message": "A valid stream token is required",
				},
			})
		}

		payload, err := signer.Verify(token)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "INVALID_STREAM_TOKEN",
					"message": "Invalid or expired stream token",
				},
			})
		}

		c.Locals("streamDeviceID", payload.DeviceID)
		c.Locals("streamUserID", payload.UserID)
		return c.Next()
	}
}

func extractStreamToken(c *fiber.Ctx) string {
	if auth := c.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if tok := c.Get("X-Stream-Token"); tok != "" {
		return tok
	}
	return c.Query("token")
}

// GetStreamDeviceID returns the authenticated token's target device.
func GetStreamDeviceID(c *fiber.Ctx) string {
	v, _ := c.Locals("streamDeviceID").(string)
	return v
}