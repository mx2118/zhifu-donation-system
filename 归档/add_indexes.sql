-- 为donations表添加索引，优化查询性能

-- 添加状态索引
CREATE INDEX idx_donations_status ON donations(status);

-- 添加支付配置ID索引
CREATE INDEX idx_donations_payment_config_id ON donations(payment_config_id);

-- 添加类目索引
CREATE INDEX idx_donations_categories ON donations(categories);

-- 添加创建时间索引
CREATE INDEX idx_donations_created_at ON donations(created_at);

-- 添加复合索引，优化常用查询
CREATE INDEX idx_donations_status_payment_config_id_categories_created_at ON donations(status, payment_config_id, categories, created_at);

-- 查看索引创建情况
SHOW INDEX FROM donations;
