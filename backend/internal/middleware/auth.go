package middleware

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AuthMiddleware validates JWT tokens and extracts user info into context.
func AuthMiddleware(jwtSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "AUTH_REQUIRED",
					"message": "Authorization header is required",
				},
			})
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(401).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "INVALID_AUTH_FORMAT",
					"message": "Authorization header must be 'Bearer <token>'",
				},
			})
		}

		token, err := jwt.Parse(parts[1], func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			return c.Status(401).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "INVALID_TOKEN",
					"message": "Invalid or expired token",
				},
			})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(401).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "INVALID_CLAIMS",
					"message": "Invalid token claims",
				},
			})
		}

		// Ensure it's an access token
		tokenType, _ := claims["type"].(string)
		if tokenType != "access" {
			return c.Status(401).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "WRONG_TOKEN_TYPE",
					"message": "Access token required",
				},
			})
		}

		// Extract and store user info in locals
		sub, _ := claims["sub"].(string)
		userID, _ := uuid.Parse(sub)
		role, _ := claims["role"].(string)

		c.Locals("userID", userID)
		c.Locals("userRole", role)

		return c.Next()
	}
}

// RequireRole checks if the authenticated user has the required role.
func RequireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRole, ok := c.Locals("userRole").(string)
		if !ok {
			return c.Status(403).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "FORBIDDEN",
					"message": "Access denied",
				},
			})
		}

		for _, r := range roles {
			if userRole == r {
				return c.Next()
			}
		}

		return c.Status(403).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "INSUFFICIENT_ROLE",
				"message": fmt.Sprintf("This action requires one of: %s", strings.Join(roles, ", ")),
			},
		})
	}
}

// GetUserID extracts the user ID from context.
func GetUserID(c *fiber.Ctx) uuid.UUID {
	id, _ := c.Locals("userID").(uuid.UUID)
	return id
}

// GetUserRole extracts the user role from context.
func GetUserRole(c *fiber.Ctx) string {
	role, _ := c.Locals("userRole").(string)
	return role
}
