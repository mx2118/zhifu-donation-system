# 功德榜系统 (zhifu-server)

## 项目概述

功德榜系统是一个基于Go语言开发的在线捐款平台，支持微信和支付宝支付，提供实时功德榜排名和用户授权功能。系统采用高性能的fasthttp框架，具有良好的并发处理能力和响应速度。

### 接口说明

- **支付接口**：使用收钱吧API接口，同时支持微信和支付宝支付
- **用户信息获取**：分别使用微信公众号和支付宝开放平台的授权接口
- **注意**：请自行申请相应的接口数据并配置到系统中

### 核心功能

- 🔄 实时捐款排行榜
- 💳 微信和支付宝支付集成
- 👤 微信公众号和支付宝用户授权
- 📱 移动端适配
- 🔔 WebSocket实时支付通知
- 📊 分类捐款管理
- 🎨 自定义祝福语
- 📍 多项目支持

## 技术栈

| 类别 | 技术 | 版本 |
|------|------|------|
| 后端 | Go | 1.24.0 |
| HTTP服务器 | fasthttp | v1.58.0 |
| 数据库 | MySQL | - |
| ORM | GORM | v1.25.5 |
| 配置管理 | Viper | v1.18.2 |
| WebSocket | fasthttp/websocket | v1.5.12 |
| 二维码 | skip2/go-qrcode | - |

## 目录结构

```
├── models/          # 数据模型
│   ├── category.go      # 分类模型
│   ├── donation.go      # 捐款模型
│   ├── payment_config.go # 支付配置模型
│   └── user.go          # 用户模型
├── routes/          # 路由处理
│   ├── api.go           # API路由
│   └── websocket.go     # WebSocket处理
├── services/        # 业务服务
│   └── payment.go       # 支付服务
├── static/          # 静态资源
│   ├── combined.min.css # 合并压缩CSS
│   ├── index-script.js  # 首页脚本
│   ├── pay-styles.css   # 支付页样式
│   ├── pay-script.js    # 支付页脚本
│   ├── avatar.jpeg      # 默认头像
│   ├── wechat.png       # 微信图标
│   └── alipay.png       # 支付宝图标
├── templates/       # HTML模板
│   ├── index.html       # 首页模板
│   └── pay.html         # 支付页模板
├── utils/           # 工具函数
│   ├── cache.go         # 缓存管理
│   ├── database.go      # 数据库操作
│   ├── port.go          # 端口管理
│   ├── qrcode.go        # 二维码生成
│   └── websocket.go     # WebSocket工具
├── .gitignore       # Git忽略文件
├── add_indexes.sql  # 索引添加脚本
├── config.yaml      # 配置文件
├── go.mod           # Go模块文件
├── go.sum           # 依赖校验文件
├── main.go          # 主入口文件
├── migrate.sql      # 数据库迁移脚本
└── zhifu-server     # 编译后的可执行文件
```

## 快速开始

### 环境要求

- Go 1.24.0或更高版本
- MySQL 5.7或更高版本
- 微信公众号（用于微信支付和授权）
- 支付宝开发者账号（用于支付宝支付和授权）

### 安装与运行

1. **克隆项目**

```bash
git clone <repository-url>
cd zhifu
```

2. **安装依赖**

```bash
go mod tidy
```

3. **配置数据库**

执行数据库迁移脚本：

```bash
mysql -u username -p database_name < migrate.sql
mysql -u username -p database_name < add_indexes.sql
```

4. **配置文件**

编辑现有的`config.yaml`文件，配置数据库连接和服务器设置：

```yaml
server:
  port: 9090

mysql:
  host: localhost
  user: root
  password: your_password
  dbname: zhifu
  port: 3306
```

5. **编译与运行**

```bash
# 编译
go build -o zhifu-server main.go

# 运行
./zhifu-server
```

服务器默认运行在 `http://localhost:9090`

## API接口文档

### 1. 捐款相关

#### 创建捐款订单
- **URL**: `/api/donate`
- **方法**: `POST`
- **参数**:
  - `amount`: 捐款金额（0.01-10000）
  - `payment`: 支付方式（wechat/alipay）
  - `category`: 捐款类目
  - `blessing`: 祝福语
- **返回**: 订单ID和支付URL

#### 表单提交捐款
- **URL**: `/api/donate/form`
- **方法**: `POST`
- **参数**:
  - `amount`: 捐款金额
  - `payment`: 支付方式
  - `category`: 捐款类目
  - `blessing`: 祝福语
- **返回**: 302重定向到支付页面

### 2. 排行榜相关

#### 获取排行榜
- **URL**: `/api/rankings`
- **方法**: `GET`
- **参数**:
  - `limit`: 每页数量（默认10）
  - `page`: 页码（默认1）
  - `payment`/`p`: 项目ID
  - `categories`/`c`: 分类ID
- **返回**: 排行榜数据和分页信息

### 3. 用户授权

#### 微信授权
- **URL**: `/api/wechat/auth`
- **方法**: `GET`
- **参数**:
  - `redirect_url`: 授权后重定向URL
  - `payment`/`p`: 项目ID
  - `categories`/`c`: 分类ID

#### 微信授权回调
- **URL**: `/api/wechat/callback`
- **方法**: `GET`
- **参数**:
  - `code`: 授权码

#### 支付宝授权
- **URL**: `/api/alipay/auth`
- **方法**: `GET`
- **参数**:
  - `redirect_url`: 授权后重定向URL
  - `payment`/`p`: 项目ID
  - `categories`/`c`: 分类ID

#### 支付宝授权回调
- **URL**: `/api/alipay/callback`
- **方法**: `GET`
- **参数**:
  - `auth_code`: 授权码

### 4. 支付回调

#### 支付回调
- **URL**: `/api/callback` 或 `/api/pay/callback`
- **方法**: `POST`
- **参数**: 支付平台回调参数
- **返回**: "success"表示成功

### 5. 其他接口

#### 生成支付二维码
- **URL**: `/qrcode`
- **方法**: `GET`
- **参数**:
  - `payment`/`p`: 项目ID
  - `categories`/`c`: 分类ID
- **返回**: PNG格式二维码图片

#### 获取支付配置
- **URL**: `/api/payment-config/{id}`
- **方法**: `GET`
- **返回**: 支付配置信息

#### 获取分类信息
- **URL**: `/api/category/{id}`
- **方法**: `GET`
- **返回**: 分类信息

#### 获取所有分类
- **URL**: `/api/categories`
- **方法**: `GET`
- **参数**:
  - `payment`/`p`: 项目ID
- **返回**: 分类列表

## 前端页面

### 1. 首页 (`/`)

- 功德榜展示
- 栏目列表下拉菜单
- 做功德按钮
- 模态窗口二维码

### 2. 支付页 (`/pay`)

- 金额输入
- 快捷金额选择
- 祝福语输入
- 用户信息展示
- 立即支付按钮

## 配置说明

### 支付配置

系统支持多个支付配置，通过数据库中的`payment_configs`表管理。配置包括：

- 商户信息（VendorSN, VendorKey）
- 应用信息（AppID, DeviceID）
- 支付平台配置（微信、支付宝）
- 终端信息（TerminalSN, TerminalKey）

### 分类配置

通过`categories`表管理捐款分类，支持按项目分组。

## 部署建议

### 生产环境

1. **使用反向代理**
   - Nginx或Caddy作为前端代理
   - 配置HTTPS

2. **数据库优化**
   - 启用连接池
   - 定期备份
   - 优化查询索引

3. **性能优化**
   - 调整服务器并发参数
   - 启用Gzip压缩
   - 使用CDN加速静态资源

4. **监控与日志**
   - 配置日志轮换
   - 实现监控告警
   - 定期分析访问日志

### 开发环境

- 使用`go run main.go`快速启动
- 配置`config.yaml`中的数据库连接
- 启用调试日志

## 常见问题

### 1. 支付回调失败

**解决方案**:
- 检查支付平台的回调地址配置
- 确保服务器可以被外部访问
- 查看日志中的回调处理信息

### 2. 二维码生成失败

**解决方案**:
- 检查项目ID和分类ID是否存在
- 确保服务器可以访问支付平台

### 3. 排行榜不更新

**解决方案**:
- 检查WebSocket连接
- 查看支付回调是否成功
- 检查缓存是否正常

## 技术支持

- **问题反馈**: 提交Issue或联系开发团队
- **功能建议**: 欢迎提出改进建议
- **代码贡献**: 欢迎提交Pull Request

## 许可证

本项目采用MIT许可证，详见LICENSE文件。

---

**乘风不问鲲鹏路，独与天地共悠然。**

© 2026 功德榜系统 版权所有
