CREATE TABLE device_sessions (
     id BIGINT AUTO_INCREMENT PRIMARY KEY,

     public_id CHAR(36) NOT NULL,

     user_id BIGINT NOT NULL,
     device_id BIGINT NOT NULL,

     refresh_token_hash CHAR(64) NOT NULL,

     ip_address VARBINARY(16),
     user_agent TEXT,

     expires_at DATETIME NOT NULL,

     created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
     last_used_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

     UNIQUE KEY uq_user_device (user_id, device_id),
     UNIQUE KEY uq_refresh_token_hash (refresh_token_hash),

     CONSTRAINT fk_ds_user FOREIGN KEY (user_id) REFERENCES users(id),
     CONSTRAINT fk_ds_device FOREIGN KEY (device_id) REFERENCES devices(id)
);