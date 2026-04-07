package repository

import (
	"context"
	"fmt"

	"camguid/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash, role, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		user.ID, user.Username, user.Email, user.PasswordHash, user.Role, user.IsActive,
	)
	return err
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user := &models.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, role, is_active, last_login_at, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role,
		&user.IsActive, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	user := &models.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, role, is_active, last_login_at, created_at, updated_at
		 FROM users WHERE username = $1`, username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role,
		&user.IsActive, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (r *UserRepository) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, id)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`, hash, id)
	return err
}

func (r *UserRepository) List(ctx context.Context) ([]models.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, username, email, role, is_active, last_login_at, created_at, updated_at
		 FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role,
			&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *UserRepository) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count)
	return count, err
}

// DeviceRepository handles device CRUD operations.
type DeviceRepository struct {
	pool *pgxpool.Pool
}

func NewDeviceRepository(pool *pgxpool.Pool) *DeviceRepository {
	return &DeviceRepository{pool: pool}
}

func (r *DeviceRepository) Create(ctx context.Context, d *models.Device) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO devices (id, name, device_type, ip_address, port, mac_address, manufacturer, model,
			firmware, location, tags, notes, is_enabled, stream_url, secondary_url, snapshot_url,
			web_interface, onvif_path, channel_count, metadata)
		 VALUES ($1,$2,$3,$4::inet,$5,$6::macaddr,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		d.ID, d.Name, d.DeviceType, d.IPAddress, d.Port, d.MACAddress, d.Manufacturer, d.Model,
		d.Firmware, d.Location, d.Tags, d.Notes, d.IsEnabled, d.StreamURL, d.SecondaryURL,
		d.SnapshotURL, d.WebInterface, d.OnvifPath, d.ChannelCount, d.Metadata,
	)
	return err
}

func (r *DeviceRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Device, error) {
	d := &models.Device{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, device_type, host(ip_address), port, mac_address::text, manufacturer, model,
			firmware, location, tags, notes, is_enabled, stream_url, secondary_url, snapshot_url,
			web_interface, onvif_path, channel_count, metadata, last_seen_at, created_at, updated_at
		 FROM devices WHERE id = $1`, id,
	).Scan(&d.ID, &d.Name, &d.DeviceType, &d.IPAddress, &d.Port, &d.MACAddress, &d.Manufacturer,
		&d.Model, &d.Firmware, &d.Location, &d.Tags, &d.Notes, &d.IsEnabled, &d.StreamURL,
		&d.SecondaryURL, &d.SnapshotURL, &d.WebInterface, &d.OnvifPath, &d.ChannelCount,
		&d.Metadata, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return d, err
}

func (r *DeviceRepository) List(ctx context.Context, params models.DeviceListParams) ([]models.Device, int, error) {
	// Build dynamic WHERE clause
	where := "WHERE 1=1"
	args := []interface{}{}
	argN := 1

	if params.Type != "" {
		where += fmt.Sprintf(" AND device_type = $%d", argN)
		args = append(args, params.Type)
		argN++
	}
	if params.Enabled != nil {
		where += fmt.Sprintf(" AND is_enabled = $%d", argN)
		args = append(args, *params.Enabled)
		argN++
	}
	if params.Search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR host(ip_address) ILIKE $%d OR manufacturer ILIKE $%d)", argN, argN, argN)
		args = append(args, "%"+params.Search+"%")
		argN++
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM devices " + where
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Pagination defaults
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Limit < 1 || params.Limit > 100 {
		params.Limit = 20
	}
	offset := (params.Page - 1) * params.Limit

	query := fmt.Sprintf(
		`SELECT id, name, device_type, host(ip_address), port, mac_address::text, manufacturer, model,
			firmware, location, tags, notes, is_enabled, stream_url, secondary_url, snapshot_url,
			web_interface, onvif_path, channel_count, metadata, last_seen_at, created_at, updated_at
		 FROM devices %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argN, argN+1)
	args = append(args, params.Limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.Name, &d.DeviceType, &d.IPAddress, &d.Port, &d.MACAddress,
			&d.Manufacturer, &d.Model, &d.Firmware, &d.Location, &d.Tags, &d.Notes, &d.IsEnabled,
			&d.StreamURL, &d.SecondaryURL, &d.SnapshotURL, &d.WebInterface, &d.OnvifPath,
			&d.ChannelCount, &d.Metadata, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, 0, err
		}
		devices = append(devices, d)
	}
	return devices, total, nil
}

func (r *DeviceRepository) Update(ctx context.Context, id uuid.UUID, req models.UpdateDeviceRequest) error {
	// Dynamic update query
	sets := []string{}
	args := []interface{}{}
	argN := 1

	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argN)); args = append(args, *req.Name); argN++
	}
	if req.IPAddress != nil {
		sets = append(sets, fmt.Sprintf("ip_address = $%d::inet", argN)); args = append(args, *req.IPAddress); argN++
	}
	if req.Port != nil {
		sets = append(sets, fmt.Sprintf("port = $%d", argN)); args = append(args, *req.Port); argN++
	}
	if req.Manufacturer != nil {
		sets = append(sets, fmt.Sprintf("manufacturer = $%d", argN)); args = append(args, *req.Manufacturer); argN++
	}
	if req.Model != nil {
		sets = append(sets, fmt.Sprintf("model = $%d", argN)); args = append(args, *req.Model); argN++
	}
	if req.Location != nil {
		sets = append(sets, fmt.Sprintf("location = $%d", argN)); args = append(args, *req.Location); argN++
	}
	if req.Tags != nil {
		sets = append(sets, fmt.Sprintf("tags = $%d", argN)); args = append(args, req.Tags); argN++
	}
	if req.StreamURL != nil {
		sets = append(sets, fmt.Sprintf("stream_url = $%d", argN)); args = append(args, *req.StreamURL); argN++
	}

	if len(sets) == 0 {
		return nil // Nothing to update
	}

	sets = append(sets, "updated_at = now()")
	query := fmt.Sprintf("UPDATE devices SET %s WHERE id = $%d",
		joinStrings(sets, ", "), argN)
	args = append(args, id)

	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

func (r *DeviceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	return err
}

func (r *DeviceRepository) ToggleEnabled(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE devices SET is_enabled = NOT is_enabled, updated_at = now() WHERE id = $1`, id)
	return err
}

func (r *DeviceRepository) GetStats(ctx context.Context) (*models.DashboardStats, error) {
	stats := &models.DashboardStats{}
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE device_type = 'camera'),
			COUNT(*) FILTER (WHERE device_type = 'nvr'),
			COUNT(*) FILTER (WHERE is_enabled = true)
		FROM devices
	`).Scan(&stats.TotalDevices, &stats.TotalCameras, &stats.TotalNVRs, &stats.EnabledDevices)
	if err != nil {
		return nil, err
	}

	// Active streams count
	r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM stream_sessions WHERE status = 'active'`).Scan(&stats.ActiveStreams)

	// Groups count
	r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM device_groups`).Scan(&stats.TotalGroups)

	// Last scan
	var lastScan *string
	r.pool.QueryRow(ctx,
		`SELECT to_char(MAX(completed_at), 'YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM scan_jobs WHERE status = 'completed'`).Scan(&lastScan)
	stats.LastScanAt = lastScan

	return stats, nil
}

// --- Helpers ---

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
