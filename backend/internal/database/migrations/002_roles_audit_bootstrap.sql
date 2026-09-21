-- 002_roles_audit_bootstrap.sql
-- RBAC roles, first-boot password enforcement, and audit-log hardening.

-- 1) RBAC: allow the 'operator' role.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users
    ADD CONSTRAINT users_role_check
    CHECK (role IN ('admin', 'operator', 'viewer'));

-- 2) First-boot setup: force the initial admin to set a password on login.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT false;

-- 3) Audit logs: capture result (success/fail/denied) and a denormalized actor
--    username so entries survive user deletion and are filterable.
ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS result VARCHAR(16) NOT NULL DEFAULT 'success'
        CHECK (result IN ('success', 'fail', 'denied'));
ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS actor VARCHAR(64);
CREATE INDEX IF NOT EXISTS idx_audit_actor_created ON audit_logs(actor, created_at);