package handler

import (
	"camguid/internal/middleware"
	"camguid/internal/models"
	"camguid/internal/repository"
	auth "camguid/internal/service/auth"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type Handler struct {
	authService *auth.Service
	deviceRepo  *repository.DeviceRepository
	userRepo    *repository.UserRepository
}

func New(authSvc *auth.Service, deviceRepo *repository.DeviceRepository, userRepo *repository.UserRepository) *Handler {
	return &Handler{
		authService: authSvc,
		deviceRepo:  deviceRepo,
		userRepo:    userRepo,
	}
}

// --- Auth Handlers ---

func (h *Handler) Login(c *fiber.Ctx) error {
	var req models.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_BODY", Message: "Invalid request body",
		}})
	}
	if req.Username == "" || req.Password == "" {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "MISSING_FIELDS", Message: "Username and password are required",
		}})
	}

	tokens, user, err := h.authService.Login(c.Context(), req)
	if err != nil {
		return c.Status(401).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "AUTH_FAILED", Message: err.Error(),
		}})
	}

	return c.JSON(fiber.Map{
		"tokens": tokens,
		"user": fiber.Map{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

func (h *Handler) RefreshToken(c *fiber.Ctx) error {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&body); err != nil || body.RefreshToken == "" {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "MISSING_TOKEN", Message: "Refresh token is required",
		}})
	}

	tokens, err := h.authService.RefreshToken(c.Context(), body.RefreshToken)
	if err != nil {
		return c.Status(401).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "REFRESH_FAILED", Message: err.Error(),
		}})
	}
	return c.JSON(tokens)
}

func (h *Handler) GetMe(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	user, err := h.userRepo.GetByID(c.Context(), userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "USER_NOT_FOUND", Message: "User not found",
		}})
	}
	return c.JSON(user)
}

func (h *Handler) Register(c *fiber.Ctx) error {
	var req models.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_BODY", Message: "Invalid request body",
		}})
	}
	if req.Username == "" || req.Password == "" || req.Role == "" {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "MISSING_FIELDS", Message: "Username, password, and role are required",
		}})
	}

	user, err := h.authService.Register(c.Context(), req)
	if err != nil {
		return c.Status(409).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "REGISTER_FAILED", Message: err.Error(),
		}})
	}
	return c.Status(201).JSON(user)
}

func (h *Handler) ChangePassword(c *fiber.Ctx) error {
	var req models.ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_BODY", Message: "Invalid request body",
		}})
	}

	userID := middleware.GetUserID(c)
	if err := h.authService.ChangePassword(c.Context(), userID, req); err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "PASSWORD_CHANGE_FAILED", Message: err.Error(),
		}})
	}
	return c.JSON(fiber.Map{"message": "Password changed successfully"})
}

// --- Device Handlers ---

func (h *Handler) ListDevices(c *fiber.Ctx) error {
	params := models.DeviceListParams{
		Type:   c.Query("type"),
		Search: c.Query("search"),
		Page:   c.QueryInt("page", 1),
		Limit:  c.QueryInt("limit", 20),
	}
	if enabled := c.Query("enabled"); enabled != "" {
		val := enabled == "true"
		params.Enabled = &val
	}

	devices, total, err := h.deviceRepo.List(c.Context(), params)
	if err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "LIST_FAILED", Message: "Failed to list devices",
		}})
	}

	if devices == nil {
		devices = []models.Device{}
	}

	totalPages := (total + params.Limit - 1) / params.Limit
	return c.JSON(models.PaginatedResponse{
		Data:       devices,
		Total:      total,
		Page:       params.Page,
		Limit:      params.Limit,
		TotalPages: totalPages,
	})
}

func (h *Handler) GetDevice(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_ID", Message: "Invalid device ID",
		}})
	}

	device, err := h.deviceRepo.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "FETCH_FAILED", Message: "Failed to fetch device",
		}})
	}
	if device == nil {
		return c.Status(404).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "DEVICE_NOT_FOUND", Message: "Device not found",
		}})
	}
	return c.JSON(device)
}

func (h *Handler) CreateDevice(c *fiber.Ctx) error {
	var req models.CreateDeviceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_BODY", Message: "Invalid request body",
		}})
	}
	if req.Name == "" || req.DeviceType == "" || req.IPAddress == "" {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "MISSING_FIELDS", Message: "Name, device_type, and ip_address are required",
		}})
	}
	if req.Port == 0 {
		req.Port = 554
	}
	if req.ChannelCount == 0 {
		req.ChannelCount = 1
	}

	device := &models.Device{
		ID:           uuid.New(),
		Name:         req.Name,
		DeviceType:   req.DeviceType,
		IPAddress:    req.IPAddress,
		Port:         req.Port,
		MACAddress:   req.MACAddress,
		Manufacturer: req.Manufacturer,
		Model:        req.Model,
		Firmware:     req.Firmware,
		Location:     req.Location,
		Tags:         req.Tags,
		Notes:        req.Notes,
		IsEnabled:    true,
		StreamURL:    req.StreamURL,
		SecondaryURL: req.SecondaryURL,
		SnapshotURL:  req.SnapshotURL,
		WebInterface: req.WebInterface,
		OnvifPath:    req.OnvifPath,
		ChannelCount: req.ChannelCount,
		Metadata:     []byte("{}"),
	}

	if err := h.deviceRepo.Create(c.Context(), device); err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "CREATE_FAILED", Message: "Failed to create device: " + err.Error(),
		}})
	}

	return c.Status(201).JSON(device)
}

func (h *Handler) UpdateDevice(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_ID", Message: "Invalid device ID",
		}})
	}

	var req models.UpdateDeviceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_BODY", Message: "Invalid request body",
		}})
	}

	if err := h.deviceRepo.Update(c.Context(), id, req); err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "UPDATE_FAILED", Message: "Failed to update device",
		}})
	}

	device, _ := h.deviceRepo.GetByID(c.Context(), id)
	return c.JSON(device)
}

func (h *Handler) DeleteDevice(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_ID", Message: "Invalid device ID",
		}})
	}

	if err := h.deviceRepo.Delete(c.Context(), id); err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "DELETE_FAILED", Message: "Failed to delete device",
		}})
	}
	return c.SendStatus(204)
}

func (h *Handler) ToggleDevice(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_ID", Message: "Invalid device ID",
		}})
	}

	if err := h.deviceRepo.ToggleEnabled(c.Context(), id); err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "TOGGLE_FAILED", Message: "Failed to toggle device",
		}})
	}

	device, _ := h.deviceRepo.GetByID(c.Context(), id)
	return c.JSON(device)
}

func (h *Handler) GenerateChannels(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_ID", Message: "Invalid device ID",
		}})
	}

	nvr, err := h.deviceRepo.GetByID(c.Context(), id)
	if err != nil || nvr == nil {
		return c.Status(404).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "DEVICE_NOT_FOUND", Message: "NVR not found",
		}})
	}

	if nvr.DeviceType != "nvr" {
		return c.Status(400).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "INVALID_TYPE", Message: "Device is not an NVR",
		}})
	}

	channels := nvr.ChannelCount
	if channels < 1 {
		channels = 1
	}

	baseIP := nvr.IPAddress
	mfg := ""
	if nvr.Manufacturer != nil {
		mfg = *nvr.Manufacturer
	}

	// Simple heuristic to grab credentials if they were put in the NVR's StreamURL

	// Avoid inline imports, extract auth properly
	authStr := ""
	if nvr.StreamURL != nil {
		sUrl := *nvr.StreamURL
		start := 0
		if len(sUrl) > 7 && sUrl[:7] == "rtsp://" {
			start = 7
		}
		atIdx := -1
		for i := start; i < len(sUrl); i++ {
			if sUrl[i] == '@' {
				atIdx = i
				break
			}
		}
		if atIdx != -1 {
			authStr = sUrl[start:atIdx]
		}
	}
	
	if authStr == "" {
		authStr = "admin:admin"
	}

	createdCount := 0
	for i := 1; i <= channels; i++ {
		camName := nvr.Name + " - CH" + strconv.Itoa(i)
		
		// Dahua specific logic (or default)
		streamUrl := "rtsp://" + authStr + "@" + baseIP + ":554/cam/realmonitor?channel=" + strconv.Itoa(i) + "&subtype=0"
		if mfg == "hikvision" {
			streamUrl = "rtsp://" + authStr + "@" + baseIP + ":554/Streaming/Channels/" + strconv.Itoa(i) + "01"
		}

		newCam := &models.Device{
			ID:           uuid.New(),
			Name:         camName,
			DeviceType:   "camera",
			IPAddress:    baseIP,
			Port:         554,
			Manufacturer: nvr.Manufacturer,
			IsEnabled:    true,
			StreamURL:    &streamUrl,
			ChannelCount: 1,
			Metadata:     []byte("{}"),
		}

		if err := h.deviceRepo.Create(c.Context(), newCam); err == nil {
			createdCount++
		}
	}

	return c.JSON(fiber.Map{
		"message": "Generated channels",
		"count":   createdCount,
	})
}

// --- Dashboard ---

func (h *Handler) GetDashboardStats(c *fiber.Ctx) error {
	stats, err := h.deviceRepo.GetStats(c.Context())
	if err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: models.ErrorDetail{
			Code: "STATS_FAILED", Message: "Failed to get dashboard stats",
		}})
	}
	return c.JSON(stats)
}

// --- Health ---

func (h *Handler) HealthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"service": "camguid-api",
	})
}
