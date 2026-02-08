package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/valyala/fasthttp"
	"github.com/zhifu/donation-rank/utils"
)

// ClientConn WebSocket客户端连接
type ClientConn struct {
	Conn       *websocket.Conn
	LastHeart  time.Time // 最后心跳时间
	ConnID     string    // 连接ID
	IP         string    // 客户端IP
	Payment    string    // 支付方式参数
	Categories string    // 分类参数
}

// PayNotification 支付通知
type PayNotification struct {
	Type      string `json:"type"`       // 通知类型
	OrderNo   string `json:"orderNo"`    // 订单号
	Amount    string `json:"amount"`     // 金额
	Time      string `json:"Time"`       // 时间
	Payment   string `json:"payment"`    // 支付方式
	Blessing  string `json:"blessing"`   // 祝福语
	AvatarURL string `json:"avatar_url"` // 头像URL
	UserName  string `json:"user_name"`  // 用户名
	CreatedAt string `json:"created_at"` // 创建时间
}

// WebSocketManager WebSocket管理器
type WebSocketManager struct {
	Clients           sync.Map      // 线程安全连接池
	HeartbeatInterval time.Duration // 心跳检查间隔
	HeartbeatTimeout  time.Duration // 心跳超时时间
}

// NewWebSocketManager 创建WebSocket管理器
func NewWebSocketManager() *WebSocketManager {
	manager := &WebSocketManager{
		Clients:           sync.Map{},
		HeartbeatInterval: 10 * time.Second, // 10秒检查一次心跳
		HeartbeatTimeout:  30 * time.Second, // 30秒无心跳交互则清理
	}

	// 启动心跳检测
	go manager.startHeartbeatChecker()

	return manager
}

// Upgrader WebSocket升级器
var Upgrader = websocket.FastHTTPUpgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// 生产环境需要限制域名，测试环境返回true
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
		// TODO: 生产必改点1：限制允许的域名
		// 例如：return ctx.Request.Header.Peek("Origin") == []byte("https://yourdomain.com")
		return true // 测试环境允许所有域名
	},
}

// HandleWebSocket 处理WebSocket连接
func (m *WebSocketManager) HandleWebSocket(ctx *fasthttp.RequestCtx) {
	// 获取请求参数（支持别名）
	payment := string(ctx.QueryArgs().Peek("payment"))
	if payment == "" {
		payment = string(ctx.QueryArgs().Peek("p"))
	}
	categories := string(ctx.QueryArgs().Peek("categories"))
	if categories == "" {
		categories = string(ctx.QueryArgs().Peek("c"))
	}

	fmt.Printf("[DEBUG] WebSocket upgrade attempt: payment='%s', categories='%s', IP=%s\n", payment, categories, string(ctx.RemoteIP().String()))

	// 升级HTTP连接为WebSocket
	err := Upgrader.Upgrade(ctx, func(conn *websocket.Conn) {
		// 连接成功后的回调
		connID := utils.GenerateConnID()
		clientIP := string(ctx.RemoteIP().String())

		// 创建客户端连接
		clientConn := &ClientConn{
			Conn:       conn,
			LastHeart:  time.Now(),
			ConnID:     connID,
			IP:         clientIP,
			Payment:    payment,
			Categories: categories,
		}

		// 添加到连接池
		m.Clients.Store(connID, clientConn)
		fmt.Printf("[DEBUG] WebSocket connected: connID=%s, IP=%s, payment='%s', categories='%s'\n", connID, clientIP, payment, categories)

		// 处理连接
		m.handleClientConn(clientConn)
	})

	if err != nil {
		fmt.Printf("[DEBUG] WebSocket upgrade failed: %v, IP=%s\n", err, string(ctx.RemoteIP().String()))
		return
	}
}

// handleClientConn 处理客户端连接
func (m *WebSocketManager) handleClientConn(clientConn *ClientConn) {
	defer func() {
		// 清理连接
		m.Clients.Delete(clientConn.ConnID)
		clientConn.Conn.Close()
		log.Printf("WebSocket disconnected: connID=%s, IP=%s", clientConn.ConnID, clientConn.IP)
	}()

	for {
		// 读取消息
		messageType, message, err := clientConn.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v, connID=%s, IP=%s", err, clientConn.ConnID, clientConn.IP)
			}
			break
		}

		// 处理ping消息
		if messageType == websocket.PingMessage {
			// 更新心跳时间
			clientConn.LastHeart = time.Now()
			// 回复pong
			if err := clientConn.Conn.WriteMessage(websocket.PongMessage, nil); err != nil {
				log.Printf("WebSocket pong error: %v, connID=%s", err, clientConn.ConnID)
				break
			}
			continue
		}

		// 处理文本类型的ping消息（客户端发送的ping字符串）
		if messageType == websocket.TextMessage && string(message) == "ping" {
			// 更新心跳时间
			clientConn.LastHeart = time.Now()
			// 回复pong
			if err := clientConn.Conn.WriteMessage(websocket.TextMessage, []byte("pong")); err != nil {
				log.Printf("WebSocket text pong error: %v, connID=%s", err, clientConn.ConnID)
				break
			}
			continue
		}

		// 忽略其他类型的消息
		log.Printf("Received message: %s, connID=%s", string(message), clientConn.ConnID)
	}
}

// startHeartbeatChecker 启动心跳检测
func (m *WebSocketManager) startHeartbeatChecker() {
	ticker := time.NewTicker(m.HeartbeatInterval)
	defer ticker.Stop()

	for {
		<-ticker.C
		m.checkHeartbeats()
	}
}

// checkHeartbeats 检查心跳
func (m *WebSocketManager) checkHeartbeats() {
	m.Clients.Range(func(key, value interface{}) bool {
		clientConn, ok := value.(*ClientConn)
		if !ok {
			m.Clients.Delete(key)
			return true
		}

		// 检查心跳是否超时
		if time.Since(clientConn.LastHeart) > m.HeartbeatTimeout {
			log.Printf("WebSocket heartbeat timeout: connID=%s, IP=%s", clientConn.ConnID, clientConn.IP)
			// 关闭连接
			clientConn.Conn.Close()
			// 从连接池删除
			m.Clients.Delete(key)
		}

		return true
	})
}

// Broadcast 广播消息
func (m *WebSocketManager) Broadcast(notification *PayNotification) {
	// 序列化消息
	data, err := json.Marshal(notification)
	if err != nil {
		log.Printf("Broadcast message marshal error: %v", err)
		return
	}

	// 每个连接独立goroutine推送
	m.Clients.Range(func(key, value interface{}) bool {
		go func(clientConn *ClientConn) {
			if err := clientConn.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("Broadcast write error: %v, connID=%s, IP=%s, payment=%s, categories=%s", err, clientConn.ConnID, clientConn.IP, clientConn.Payment, clientConn.Categories)
				// 关闭连接并清理
				clientConn.Conn.Close()
				m.Clients.Delete(key)
			}
		}(value.(*ClientConn))
		return true
	})

	log.Printf("Broadcast pay notification: orderNo=%s, amount=%s", notification.OrderNo, notification.Amount)
}

// BroadcastToSpecific 定向广播消息（根据payment和categories参数）
func (m *WebSocketManager) BroadcastToSpecific(notification *PayNotification, payment, categories string) {
	// 序列化消息
	data, err := json.Marshal(notification)
	if err != nil {
		log.Printf("Broadcast message marshal error: %v", err)
		return
	}

	// 统计发送数量
	sentCount := 0
	failedCount := 0

	// 每个连接独立goroutine推送
	m.Clients.Range(func(key, value interface{}) bool {
		clientConn := value.(*ClientConn)

		// 检查参数匹配
		paymentMatch := (payment == "" || clientConn.Payment == payment)
		categoriesMatch := (categories == "" || clientConn.Categories == categories)

		if paymentMatch && categoriesMatch {
			// 捕获key变量，避免并发问题
			connKey := key
			go func() {
				// 尝试发送消息，最多重试2次
				retryCount := 0
				maxRetries := 2
				
				for retryCount < maxRetries {
					if err := clientConn.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
						retryCount++
						if retryCount >= maxRetries {
							log.Printf("Broadcast write error: %v, connID=%s, IP=%s", err, clientConn.ConnID, clientConn.IP)
							// 关闭连接并清理
							clientConn.Conn.Close()
							m.Clients.Delete(connKey)
							failedCount++
						}
					} else {
						sentCount++
						break
					}
				}
			}()
		}
		return true
	})

	log.Printf("Broadcast pay notification to specific clients: orderNo=%s, amount=%s, payment='%s', categories='%s', sentCount=%d, failedCount=%d", notification.OrderNo, notification.Amount, payment, categories, sentCount, failedCount)
}

// GetConnectionCount 获取连接数
func (m *WebSocketManager) GetConnectionCount() int {
	count := 0
	m.Clients.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
