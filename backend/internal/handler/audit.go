package handler

import (
	"encoding/csv"
	"strconv"

	"camguid/internal/models"
	"camguid/internal/repository"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ListAuditLogs returns paged audit entries. Admin/operator only.
func (h *Handler) ListAuditLogs(c *fiber.Ctx) error {
	filter := repository.AuditLogFilter{
		Action: c.Query("action"),
		Result: c.Query("result"),
		Page:   c.QueryInt("page", 1),
		Limit:  c.QueryInt("limit", 50),
	}
	if uid := c.Query("user_id"); uid != "" {
		if parsed, err := uuid.Parse(uid); err == nil {
			filter.UserID = parsed
		}
	}

	logs, total, err := h.auditRepo.List(c.Context(), filter)
	if err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "AUDIT_LIST_FAILED", Message: "Failed to list audit logs",
		}})
	}
	if logs == nil {
		logs = []models.AuditLog{}
	}

	totalPages := (total + filter.Limit - 1) / filter.Limit
	return c.JSON(models.PaginatedResponse{
		Data:       logs,
		Total:      total,
		Page:       filter.Page,
		Limit:      filter.Limit,
		TotalPages: totalPages,
	})
}

// ExportAuditLogs streams the full (filtered) audit trail as CSV for offline
// analysis. Admin/operator only.
func (h *Handler) ExportAuditLogs(c *fiber.Ctx) error {
	filter := repository.AuditLogFilter{
		Action: c.Query("action"),
		Result: c.Query("result"),
	}
	if uid := c.Query("user_id"); uid != "" {
		if parsed, err := uuid.Parse(uid); err == nil {
			filter.UserID = parsed
		}
	}

	logs, _, err := h.auditRepo.List(c.Context(), filter)
	if err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "AUDIT_LIST_FAILED", Message: "Failed to export audit logs",
		}})
	}

	c.Set(fiber.HeaderContentType, "text/csv")
	c.Set("Content-Disposition", `attachment; filename="audit-logs.csv"`)
	c.Set("Cache-Control", "no-store")

	w := csv.NewWriter(c)
	_ = w.Write([]string{"id", "created_at", "actor", "action", "resource", "resource_id", "result", "ip_address"})
	for _, l := range logs {
		actor := ""
		if l.Actor != nil {
			actor = *l.Actor
		}
		resource := ""
		if l.Resource != nil {
			resource = *l.Resource
		}
		resourceID := ""
		if l.ResourceID != nil {
			resourceID = l.ResourceID.String()
		}
		ip := ""
		if l.IPAddress != nil {
			ip = *l.IPAddress
		}
		_ = w.Write([]string{
			strconv.FormatInt(l.ID, 10),
			l.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			actor,
			l.Action,
			resource,
			resourceID,
			l.Result,
			ip,
		})
	}
	w.Flush()
	return nil
}