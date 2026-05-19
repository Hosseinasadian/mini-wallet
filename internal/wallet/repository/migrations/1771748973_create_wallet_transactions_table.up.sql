CREATE TABLE wallet_transactions (
     id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
     wallet_id BIGINT UNSIGNED NOT NULL,
     operation_id VARCHAR(64) NOT NULL,

     type ENUM('deposit','withdraw','fee','transfer_in','transfer_out') NOT NULL,
     amount BIGINT NOT NULL,
     reference_id VARCHAR(100) NULL,

     description TEXT NULL,
     status ENUM('pending','completed','failed') NOT NULL DEFAULT 'completed',
     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

     INDEX idx_wallet_id (wallet_id),
     INDEX idx_operation_id (operation_id),
     INDEX idx_reference_id (reference_id),

     CONSTRAINT fk_wallet_transactions_wallet FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE
);