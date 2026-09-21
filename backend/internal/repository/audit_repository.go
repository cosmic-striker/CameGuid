package repository

import (
	"context"
	"time"

	"camguid/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogRepository persists and queries audit trail entries.
type AuditLogRepository struct {
	pool *pgxpool.Pool
}

func NewAuditLogRepository(pool *pgxpool.Pool) *AuditLogRepository {
	return &AuditLogRepository{pool: pool}
}

// Log inserts a single audit entry. userID may be nil for anonymous actions
// (e.g. failed logins). actor is a username snapshot so entries survive user
// deletion.
func (r *AuditLogRepository) Log(ctx context.Context, entry *models.AuditLog) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_logs (user_id, actor, action, resource, resource_id, details, result, ip_address)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		entry.UserID, entry.Actor, entry.Action, entry.Resource, entry.ResourceID,
		jsonOrNil(entry.Details), entry.Result, entry.IPAddress,
	)
	return err
}

type AuditLogFilter struct {
	Action   string
	UserID   uuid.UUID
	Resource string
	Result   string
	Page     int
	Limit    int
}

func (r *AuditLogRepository) List(ctx context.Context, f AuditLogFilter) ([]models.AuditLog, int, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	argN := 1

	if f.Action != "" {
		where += " AND action = $1"
		args = append(args, f.Action)
		argN++
	}
	if f.Result != "" {
		where += " AND result = $1"
		args = append(args, f.Result)
		argN++
	}
	if f.UserID != uuid.Nil {
		where += " AND user_id = $1"
		args = append(args, f.UserID)
		argN++
	}

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 200 {
		f.Limit = 50
	}
	offset := (f.Page - 1) * f.Limit

	query := "SELECT id, user_id, actor, action, resource, resource_id, details, result, ip_address, created_at" +
		" FROM audit_logs " + where + " ORDER BY created_at DESC LIMIT $1 OFFSET $2"
	args = append(args, f.Limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Actor, &l.Action, &l.Resource, &l.ResourceID,
			&l.Details, &l.Result, &l.IPAddress, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, nil
}

// DeleteOlderThan removes audit entries older than the cutoff, returning the
// number of rows deleted. Enforces AUDIT_RETENTION_DAYS.
func (r *AuditLogRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM audit_logs WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func jsonOrNil(raw []byte) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}