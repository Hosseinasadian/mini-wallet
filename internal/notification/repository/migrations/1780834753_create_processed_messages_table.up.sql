CREATE TABLE IF NOT EXISTS notification_processed_messages (
     message_id VARCHAR(36) NOT NULL,
     status ENUM(
        'processing',
        'completed',
        'failed'
    ) NOT NULL DEFAULT 'processing',

     event_payload BLOB NULL,

     processing_started_at DATETIME NOT NULL DEFAULT NOW(),
     completed_at DATETIME NULL,

     PRIMARY KEY (message_id)
);