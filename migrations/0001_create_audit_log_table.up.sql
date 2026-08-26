CREATE TABLE audit_log (
    id          BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    actor_id    BIGINT NOT NULL,
    action      VARCHAR(64) NOT NULL,
    entity_type VARCHAR(64) NOT NULL,
    entity_id   BIGINT,
    metadata    JSON NOT NULL DEFAULT (JSON_OBJECT()),
    created_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB;
