-- 功德榜系统数据库结构
-- 版本: 1.0.0
-- 日期: 2026-02-09

-- 创建数据库（如果不存在）
CREATE DATABASE IF NOT EXISTS zhifu DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 使用数据库
USE zhifu;

-- 1. 支付配置表
CREATE TABLE IF NOT EXISTS payment_configs (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    vendor_sn VARCHAR(50) UNIQUE COMMENT '商户编号',
    vendor_key VARCHAR(100) COMMENT '商户密钥',
    app_id VARCHAR(50) COMMENT '应用ID',
    terminal_sn VARCHAR(50) UNIQUE COMMENT '终端编号',
    terminal_key VARCHAR(100) COMMENT '终端密钥',
    merchant_sn VARCHAR(50) COMMENT '商户SN',
    merchant_name VARCHAR(255) COMMENT '商户名称',
    store_sn VARCHAR(50) COMMENT '门店SN',
    store_name VARCHAR(255) COMMENT '门店名称',
    device_id VARCHAR(50) COMMENT '设备ID',
    api_url VARCHAR(255) COMMENT 'API地址',
    gateway_url VARCHAR(255) COMMENT '网关地址',
    merchant_id VARCHAR(50) COMMENT '商户ID',
    store_id VARCHAR(50) COMMENT '门店ID',
    logo_url VARCHAR(255) COMMENT 'logo地址',
    title2 VARCHAR(255) COMMENT '标题2',
    title3 VARCHAR(255) COMMENT '标题3',
    wechat_app_id VARCHAR(50) COMMENT '微信AppID',
    wechat_app_secret VARCHAR(100) COMMENT '微信AppSecret',
    wechat_token VARCHAR(100) COMMENT '微信Token',
    wechat_aes_key VARCHAR(100) COMMENT '微信AESKey',
    alipay_app_id VARCHAR(50) COMMENT '支付宝AppID',
    alipay_public_key VARCHAR(500) COMMENT '支付宝公钥',
    alipay_private_key VARCHAR(500) COMMENT '应用私钥',
    is_active BOOLEAN DEFAULT TRUE COMMENT '是否激活',
    last_sign_in_at DATETIME COMMENT '最后签到时间',
    description VARCHAR(255) COMMENT '描述',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_device_id (device_id),
    INDEX idx_is_active (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2. 捐款类目表
CREATE TABLE IF NOT EXISTS categories (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(50) COMMENT '类目名称',
    payment_config_id VARCHAR(20) COMMENT '支付配置ID',
    payment VARCHAR(20) COMMENT '支付参数',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_payment_config_id (payment_config_id),
    INDEX idx_payment (payment)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3. 微信用户表
CREATE TABLE IF NOT EXISTS wechat_users (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    open_id VARCHAR(50) UNIQUE COMMENT '微信OpenID',
    union_id VARCHAR(50) COMMENT '微信UnionID',
    nickname VARCHAR(100) COMMENT '昵称',
    avatar_url VARCHAR(255) COMMENT '头像URL',
    gender INT COMMENT '性别: 0未知, 1男, 2女',
    country VARCHAR(50) COMMENT '国家',
    province VARCHAR(50) COMMENT '省份',
    city VARCHAR(50) COMMENT '城市',
    language VARCHAR(20) COMMENT '语言',
    access_token VARCHAR(255) COMMENT '访问令牌',
    refresh_token VARCHAR(255) COMMENT '刷新令牌',
    expires_at DATETIME COMMENT '令牌过期时间',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 4. 支付宝用户表
CREATE TABLE IF NOT EXISTS alipay_users (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id VARCHAR(50) UNIQUE COMMENT '支付宝用户ID',
    nickname VARCHAR(100) COMMENT '昵称',
    avatar_url VARCHAR(255) COMMENT '头像URL',
    gender VARCHAR(10) COMMENT '性别: F女, M男, UNKNOWN未知',
    province VARCHAR(50) COMMENT '省份',
    city VARCHAR(50) COMMENT '城市',
    access_token VARCHAR(255) COMMENT '访问令牌',
    refresh_token VARCHAR(255) COMMENT '刷新令牌',
    expires_at DATETIME COMMENT '令牌过期时间',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 5. 捐款表
CREATE TABLE IF NOT EXISTS donations (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    openid VARCHAR(50) COMMENT '微信openid或支付宝user_id',
    payer_uid VARCHAR(50) COMMENT '支付回调中的payer_uid',
    amount DECIMAL(10,2) COMMENT '金额',
    payment VARCHAR(20) COMMENT '支付方式: wechat, alipay',
    payment_config_id VARCHAR(20) COMMENT '支付配置ID',
    categories VARCHAR(20) COMMENT '捐款类目',
    blessing VARCHAR(200) COMMENT '祝福语',
    order_id VARCHAR(50) COMMENT '订单ID',
    status VARCHAR(20) COMMENT '状态: pending, completed',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_payment (payment),
    INDEX idx_payment_config_id (payment_config_id),
    INDEX idx_categories (categories),
    INDEX idx_order_id (order_id),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 插入默认数据

-- 1. 默认支付配置
INSERT INTO payment_configs (
    vendor_sn, vendor_key, app_id, terminal_sn, terminal_key, 
    merchant_name, store_name, device_id, 
    api_url, gateway_url, 
    wechat_app_id, wechat_app_secret, 
    alipay_app_id, alipay_public_key, alipay_private_key, 
    is_active, description
) VALUES (
    'default', 'default', 'default', 'default', 'default',
    '功德榜', '功德榜', 'default',
    'http://api.example.com', 'http://gateway.example.com',
    'your_wechat_app_id', 'your_wechat_app_secret',
    'your_alipay_app_id', 'your_alipay_public_key', 'your_alipay_private_key',
    TRUE, '默认支付配置'
);

-- 2. 默认捐款类目
INSERT INTO categories (name, payment_config_id, payment) VALUES
('菜蔬', '1', '1'),
('供灯', '1', '1'),
('放生', '1', '1'),
('印经', '1', '1'),
('建寺', '1', '1');

-- 3. 测试数据（可选）
INSERT INTO donations (
    openid, amount, payment, payment_config_id, categories, blessing, order_id, status
) VALUES
('test_openid_1', 100.00, 'wechat', '1', '1', '阿弥陀佛', 'test_order_1', 'completed'),
('test_openid_2', 50.00, 'alipay', '1', '2', '功德无量', 'test_order_2', 'completed'),
('test_openid_3', 200.00, 'wechat', '1', '3', '福慧双修', 'test_order_3', 'completed');

-- 查看创建的表结构
SHOW TABLES;

-- 查看各表结构
DESCRIBE payment_configs;
DESCRIBE categories;
DESCRIBE wechat_users;
DESCRIBE alipay_users;
DESCRIBE donations;

-- 查看插入的数据
SELECT * FROM payment_configs;
SELECT * FROM categories;
SELECT * FROM donations;
