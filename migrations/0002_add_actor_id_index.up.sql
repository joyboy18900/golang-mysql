CREATE INDEX idx_audit_log_actor_id_created_at ON audit_log (actor_id, created_at DESC);
