package handler

import (
	"camguid/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// MintStreamToken issues a short-lived HMAC token for a device so an
// authenticated user can start relay sessions / capture snapshots without ever
// sending the raw RTSP URL (which may embed credentials) to the relay.
func (h *Handler) MintStreamToken(c *fiber.Ctx) error {
	var req struct {
		DeviceID uuid.UUID `json:"device_id"`
	}
	if err := c.BodyParser(&req); err != nil || req.DeviceID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_BODY", Message: "device_id is required",
		}})
	}

	device, err := h.deviceRepo.GetByID(c.Context(), req.DeviceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "FETCH_FAILED", Message: "Failed to fetch device",
		}})
	}
	if device == nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "DEVICE_NOT_FOUND", Message: "Device not found",
		}})
	}
	if !device.IsEnabled || !device.StreamConfigured {
		return c.Status(fiber.StatusConflict).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "DEVICE_NOT_CONFIGURED", Message: "Device has no stream configured or is disabled",
		}})
	}

	userID, _ := c.Locals("userID").(uuid.UUID)
	token, err := h.streamSigner.Sign(req.DeviceID.String(), userID.String(), h.streamTokenTTL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "TOKEN_FAILED", Message: "Failed to mint stream token",
		}})
	}

	return c.JSON(fiber.Map{
		"token":      token,
		"device_id":  req.DeviceID,
		"expires_in": int(h.streamTokenTTL.Seconds()),
	})
}