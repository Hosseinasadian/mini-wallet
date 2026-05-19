CREATE TABLE IF NOT EXISTS devices (
   id BIGINT AUTO_INCREMENT PRIMARY KEY,

    public_id CHAR(36) NOT NULL,
    installation_id CHAR(36) NOT NULL,

    platform ENUM('android','ios','web') NOT NULL,

    device_name VARCHAR(255) DEFAULT NULL,
    app_version VARCHAR(50) DEFAULT NULL,

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY uq_public_id (public_id),
    UNIQUE KEY uq_installation_platform (installation_id, platform)
);