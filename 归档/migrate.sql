-- 数据库迁移脚本
-- 用于添加缺失的access_token、refresh_token和expires_at字段

-- 更新wechat_users表
ALTER TABLE wechat_users ADD COLUMN access_token VARCHAR(255) NULL;
ALTER TABLE wechat_users ADD COLUMN refresh_token VARCHAR(255) NULL;
ALTER TABLE wechat_users ADD COLUMN expires_at DATETIME NULL;

-- 更新alipay_users表
ALTER TABLE alipay_users ADD COLUMN refresh_token VARCHAR(255) NULL;
ALTER TABLE alipay_users ADD COLUMN expires_at DATETIME NULL;

-- 查看表结构确认更新
DESCRIBE wechat_users;
DESCRIBE alipay_users;
