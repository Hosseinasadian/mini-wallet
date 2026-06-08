CREATE TABLE IF NOT EXISTS notification_outbox_events (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,

    event_id CHAR(36) NOT NULL,
    event_type VARCHAR(255) NOT NULL,

    payload BLOB NOT NULL,

    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id BIGINT NOT NULL,
    status ENUM('pending', 'processing', 'processed') NOT NULL DEFAULT 'pending',

    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    processed_at DATETIME(3) NULL,
    locked_until DATETIME NULL,

    INDEX idx_outbox_processing (created_at, processed_at),
    INDEX idx_outbox_status (status, created_at),

    UNIQUE KEY uq_event_id (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;