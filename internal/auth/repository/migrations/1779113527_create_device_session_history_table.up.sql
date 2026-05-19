CREATE TABLE device_session_history (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,

    session_public_id CHAR(36) NOT NULL,
    user_id BIGINT NOT NULL,
    device_id BIGINT NOT NULL,

    refresh_token_hash CHAR(64),

    ip_address VARBINARY(16),
    user_agent TEXT,

    created_at DATETIME NOT NULL,
    last_used_at DATETIME NOT NULL,

    revoked_at DATETIME NOT NULL,
    revoke_reason VARCHAR(100),
    revoked_by VARCHAR(50), -- user/admin/system

    INDEX idx_user_time (user_id, revoked_at),
    INDEX idx_device_time (device_id, revoked_at)
);