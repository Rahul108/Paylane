-- MySQL Initialization Script for Paylane Microservices
-- Creates isolated databases and dedicated service users

-- 1. payment-core
CREATE DATABASE IF NOT EXISTS paylane_payment_core CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'payment_core_user'@'%' IDENTIFIED BY 'payment_core_pass';
GRANT ALL PRIVILEGES ON paylane_payment_core.* TO 'payment_core_user'@'%';

-- 2. orchestrator
CREATE DATABASE IF NOT EXISTS paylane_orchestrator CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'orchestrator_user'@'%' IDENTIFIED BY 'orchestrator_pass';
GRANT ALL PRIVILEGES ON paylane_orchestrator.* TO 'orchestrator_user'@'%';

-- 3. mock-mfs
CREATE DATABASE IF NOT EXISTS paylane_mock_mfs CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'mock_mfs_user'@'%' IDENTIFIED BY 'mock_mfs_pass';
GRANT ALL PRIVILEGES ON paylane_mock_mfs.* TO 'mock_mfs_user'@'%';

-- 4. mock-card
CREATE DATABASE IF NOT EXISTS paylane_mock_card CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'mock_card_user'@'%' IDENTIFIED BY 'mock_card_pass';
GRANT ALL PRIVILEGES ON paylane_mock_card.* TO 'mock_card_user'@'%';

-- 5. mock-downstream
CREATE DATABASE IF NOT EXISTS paylane_mock_downstream CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'mock_downstream_user'@'%' IDENTIFIED BY 'mock_downstream_pass';
GRANT ALL PRIVILEGES ON paylane_mock_downstream.* TO 'mock_downstream_user'@'%';

FLUSH PRIVILEGES;
