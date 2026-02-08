// 从URL获取参数
function getURLParams() {
    const params = new URLSearchParams(window.location.search);
    return {
        payment_config_id: params.get('payment_config_id') || params.get('payment') || params.get('p') || '',
        category_id: params.get('category_id') || params.get('categories') || params.get('c') || ''
    };
}

// 存储已显示的捐款记录ID，用于去重
var donationIds = new Set();
var lastDonationTime = 0;

// WebSocket连接管理
let ws;
let wsHeartbeatInterval;
let wsReconnectAttempts = 0;
const maxReconnectAttempts = 10; // 增加重连尝试次数
const initialReconnectDelay = 2000; // 增加初始重连延迟
let wsConnected = false;
let wsConnecting = false;
let lastReconnectTime = 0;
const reconnectCooldown = 1000; // 重连冷却时间

// 检测浏览器类型和版本
function getBrowserInfo() {
    const ua = navigator.userAgent.toLowerCase();
    return {
        isWeChat: ua.indexOf('micromessenger') > -1,
        isChrome: ua.indexOf('chrome') > -1 && ua.indexOf('safari') > -1 && ua.indexOf('edg') === -1,
        isEdge: ua.indexOf('edg') > -1,
        isSafari: ua.indexOf('safari') > -1 && ua.indexOf('chrome') === -1,
        isFirefox: ua.indexOf('firefox') > -1,
        isMobile: /android|iphone|ipad|ipod/i.test(ua)
    };
}

// 检测是否是微信浏览器
function isWeChatBrowser() {
    return getBrowserInfo().isWeChat;
}

// 统一日期时间解析函数，确保在所有浏览器中兼容
function parseDateTime(timeStr) {
    let date;
    const browserInfo = getBrowserInfo();
    
    try {
        if (!timeStr) {
            return new Date();
        }
        
        // 尝试直接解析（标准ISO 8601格式）
        date = new Date(timeStr);
        
        // 检查解析是否成功
        if (!isNaN(date.getTime())) {
            return date;
        }
        
        // 尝试处理时间戳格式
        const timestamp = parseInt(timeStr);
        if (!isNaN(timestamp)) {
            // 检查是否是毫秒时间戳（长度大于10）
            if (timeStr.length > 10) {
                // 毫秒时间戳
                date = new Date(timestamp);
            } else {
                // 秒时间戳
                date = new Date(timestamp * 1000);
            }
            
            if (!isNaN(date.getTime())) {
                return date;
            }
        }
        
        // 尝试处理其他格式
        // 1. 处理yyyy-MM-dd HH:mm:ss格式
        if (typeof timeStr === 'string' && /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(timeStr)) {
            const parts = timeStr.match(/(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})/);
            if (parts) {
                date = new Date(
                    parseInt(parts[1]),
                    parseInt(parts[2]) - 1, // 月份从0开始
                    parseInt(parts[3]),
                    parseInt(parts[4]),
                    parseInt(parts[5]),
                    parseInt(parts[6])
                );
                if (!isNaN(date.getTime())) {
                    return date;
                }
            }
        }
        
        // 2. 处理yyyy/MM/dd HH:mm:ss格式
        if (typeof timeStr === 'string' && /^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}$/.test(timeStr)) {
            const parts = timeStr.match(/(\d{4})\/(\d{2})\/(\d{2}) (\d{2}):(\d{2}):(\d{2})/);
            if (parts) {
                date = new Date(
                    parseInt(parts[1]),
                    parseInt(parts[2]) - 1,
                    parseInt(parts[3]),
                    parseInt(parts[4]),
                    parseInt(parts[5]),
                    parseInt(parts[6])
                );
                if (!isNaN(date.getTime())) {
                    return date;
                }
            }
        }
        
    } catch (error) {
        console.error('Error parsing date:', error, 'Time string:', timeStr);
    }
    
    // 如果所有尝试都失败，使用当前时间
    console.warn('Failed to parse date, using current time:', timeStr);
    return new Date();
}

// 连接WebSocket
function connectWebSocket() {
    // 防止重连风暴
    const now = Date.now();
    if (wsConnecting || (now - lastReconnectTime < reconnectCooldown)) {
        console.log('WebSocket connection already in progress or cooldown period');
        return;
    }
    
    wsConnecting = true;
    lastReconnectTime = now;
    
    const params = getURLParams();
    const browserInfo = getBrowserInfo();
    console.log('WebSocket connect function called');
    console.log('URL params:', params);
    console.log('Browser info:', browserInfo);
    
    // 动态构建WebSocket地址，根据当前页面的协议和主机名
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    let wsUrl = `${protocol}//${host}/ws/pay-notify`;
    console.log('Dynamic WebSocket URL:', wsUrl);
    
    // 添加参数（使用简洁格式）
    const queryParams = [];
    if (params.payment_config_id) queryParams.push(`p=${params.payment_config_id}`);
    if (params.category_id) queryParams.push(`c=${params.category_id}`);
    
    if (queryParams.length > 0) {
        wsUrl += '?' + queryParams.join('&');
    }
    console.log('Final WebSocket URL:', wsUrl);
    
    console.log('Connecting to WebSocket:', wsUrl);
    
    // 清除现有的心跳定时器
    if (wsHeartbeatInterval) {
        clearInterval(wsHeartbeatInterval);
        wsHeartbeatInterval = null;
    }
    
    try {
        // 关闭现有的连接
        if (ws) {
            try {
                ws.close(1000, 'Reconnecting');
            } catch (e) {
                // 忽略关闭错误
            }
            ws = null;
        }
        
        // 根据浏览器类型设置不同的超时时间
        let connectTimeoutMs = 10000; // 默认10秒
        if (browserInfo.isWeChat) {
            connectTimeoutMs = 15000; // 微信浏览器15秒
        } else if (browserInfo.isSafari) {
            connectTimeoutMs = 12000; // Safari浏览器12秒
        }
        
        // 浏览器特殊处理
        if (browserInfo.isWeChat) {
            console.log('Using WeChat browser optimized WebSocket connection');
        } else if (browserInfo.isSafari) {
            console.log('Using Safari browser optimized WebSocket connection');
        } else if (browserInfo.isEdge) {
            console.log('Using Edge browser optimized WebSocket connection');
        } else if (browserInfo.isChrome) {
            console.log('Using Chrome browser optimized WebSocket connection');
        } else if (browserInfo.isFirefox) {
            console.log('Using Firefox browser optimized WebSocket connection');
        }
        
        // 浏览器特殊超时处理
        setTimeout(() => {
            if (wsConnecting) {
                console.log(`${browserInfo.isWeChat ? 'WeChat' : browserInfo.isSafari ? 'Safari' : 'Browser'} connection timeout, retrying...`);
                wsConnecting = false;
                connectWebSocket();
            }
        }, connectTimeoutMs);
        
        ws = new WebSocket(wsUrl);
        
        // 设置WebSocket二进制类型（兼容性处理）
        if (ws.binaryType) {
            ws.binaryType = 'arraybuffer';
        }
        
        // 超时处理
        const connectTimeout = setTimeout(() => {
            if (ws && ws.readyState === WebSocket.CONNECTING) {
                console.log('WebSocket connection timeout');
                try {
                    ws.close(1000, 'Connection timeout');
                } catch (e) {
                    // 忽略错误
                }
                handleWebSocketError(new Error('Connection timeout'));
            }
        }, connectTimeoutMs);
        
        ws.onopen = function() {
            clearTimeout(connectTimeout);
            console.log('WebSocket connected successfully');
            wsConnected = true;
            wsReconnectAttempts = 0; // 重置重连尝试次数
            wsConnecting = false;
            
            // 启动心跳检测
            startWebSocketHeartbeat();
        };
        
        ws.onclose = function(event) {
            clearTimeout(connectTimeout);
            console.log('WebSocket disconnected:', event.code, event.reason);
            wsConnected = false;
            wsConnecting = false;
            
            // 清除心跳定时器
            if (wsHeartbeatInterval) {
                clearInterval(wsHeartbeatInterval);
                wsHeartbeatInterval = null;
            }
            
            // 尝试重连（使用指数退避策略）
            if (wsReconnectAttempts < maxReconnectAttempts) {
                const delay = Math.min(initialReconnectDelay * Math.pow(2, wsReconnectAttempts), 30000); // 最大延迟30秒
                console.log(`Attempting to reconnect in ${delay}ms...`);
                setTimeout(connectWebSocket, delay);
                wsReconnectAttempts++;
            } else {
                console.warn('Max reconnection attempts reached. Will not attempt to reconnect.');
                // 30秒后重置重连计数器，允许再次尝试
                setTimeout(() => {
                    wsReconnectAttempts = 0;
                    console.log('WebSocket reconnection attempts reset');
                }, 30000);
            }
        };
        
        ws.onerror = function(error) {
            clearTimeout(connectTimeout);
            console.error('WebSocket error:', error);
            handleWebSocketError(error);
        };
        
        ws.onmessage = function(event) {
            try {
                // 检查是否是心跳消息
                if (event.data === 'ping') {
                    // 回复pong
                    if (ws && ws.readyState === WebSocket.OPEN) {
                        try {
                            ws.send('pong');
                            console.log('WebSocket pong response sent');
                        } catch (e) {
                            console.error('Error sending pong:', e);
                            console.error('Pong send error details:', { error: e.message, stack: e.stack });
                        }
                    }
                    return;
                }
                
                // 检查是否是pong消息（服务器回复的心跳响应）
                if (event.data === 'pong') {
                    // 心跳响应，不需要处理
                    console.log('WebSocket pong received (heartbeat response)');
                    return;
                }
                
                // 检查是否是字符串类型的消息
                if (typeof event.data === 'string') {
                    console.log('WebSocket message received:', { length: event.data.length, type: typeof event.data });
                    
                    try {
                        const data = JSON.parse(event.data);
                        console.log('Received broadcast:', data);
                        
                        // 处理支付成功消息
                        if (data.type === 'pay_success') {
                            showPaymentSuccessNotification(data);
                            
                            // 检查消息是否包含完整数据
                            if (data.id && data.amount && data.created_at && data.user_name) {
                                // 消息包含完整数据，直接使用
                                console.log('Using broadcast data directly, skipping network request');
                                insertNewPaymentRecord(data);
                            } else {
                                // 消息数据不完整，调用轮询函数获取完整数据
                                console.log('Broadcast data incomplete, using pollForNewDonations for consistent processing');
                                pollForNewDonations();
                            }
                        } else {
                            console.log('Unknown WebSocket message type:', data.type);
                        }
                    } catch (parseError) {
                        console.error('Error parsing WebSocket JSON message:', parseError);
                        console.error('Raw message data:', event.data);
                        console.error('Parse error details:', { error: parseError.message, stack: parseError.stack });
                    }
                } else {
                    console.log('Received non-string WebSocket message:', { type: typeof event.data, data: event.data });
                }
            } catch (error) {
                console.error('Error processing WebSocket message:', error);
                console.error('WebSocket error details:', { error: error.message, stack: error.stack });
                // 即使WebSocket处理失败，也尝试轮询获取数据
                try {
                    console.log('Falling back to polling after WebSocket error');
                    pollForNewDonations();
                } catch (pollError) {
                    console.error('Error in fallback polling:', pollError);
                    console.error('Fallback polling error details:', { error: pollError.message, stack: pollError.stack });
                }
            }
        };
    } catch (error) {
        console.error('WebSocket connection error:', error);
        handleWebSocketError(error);
    }
}

// 处理WebSocket错误
function handleWebSocketError(error) {
    wsConnected = false;
    wsConnecting = false;
    
    // 尝试重连（使用指数退避策略）
    if (wsReconnectAttempts < maxReconnectAttempts) {
        const delay = Math.min(initialReconnectDelay * Math.pow(2, wsReconnectAttempts), 30000); // 最大延迟30秒
        console.log(`Attempting to reconnect in ${delay}ms after error...`);
        setTimeout(connectWebSocket, delay);
        wsReconnectAttempts++;
    }
}

// 启动WebSocket心跳检测
function startWebSocketHeartbeat() {
    // 清除现有的心跳定时器
    if (wsHeartbeatInterval) {
        clearInterval(wsHeartbeatInterval);
        wsHeartbeatInterval = null;
    }
    
    const browserInfo = getBrowserInfo();
    
    // 根据浏览器类型设置不同的心跳间隔
    let heartbeatInterval;
    if (browserInfo.isWeChat) {
        heartbeatInterval = 15000; // 微信浏览器15秒
    } else if (browserInfo.isSafari) {
        heartbeatInterval = 18000; // Safari浏览器18秒
    } else if (browserInfo.isEdge || browserInfo.isChrome) {
        heartbeatInterval = 20000; // Chrome和Edge浏览器20秒
    } else {
        heartbeatInterval = 25000; // 其他浏览器25秒
    }
    
    console.log(`Starting WebSocket heartbeat with interval: ${heartbeatInterval} ms for ${browserInfo.isWeChat ? 'WeChat' : browserInfo.isSafari ? 'Safari' : browserInfo.isEdge ? 'Edge' : browserInfo.isChrome ? 'Chrome' : 'Firefox'} browser`);
    
    wsHeartbeatInterval = setInterval(function() {
        if (ws && ws.readyState === WebSocket.OPEN) {
            try {
                ws.send('ping');
                console.log('WebSocket heartbeat sent');
                
                // 浏览器特殊处理：发送心跳后等待pong响应
                if (browserInfo.isWeChat) {
                    console.log('WeChat browser heartbeat sent, waiting for response...');
                } else if (browserInfo.isSafari) {
                    console.log('Safari browser heartbeat sent, waiting for response...');
                }
            } catch (error) {
                console.error('Error sending heartbeat:', error);
                // 心跳发送失败，可能连接已断开
                if (wsHeartbeatInterval) {
                    clearInterval(wsHeartbeatInterval);
                    wsHeartbeatInterval = null;
                }
                // 触发重连
                if (wsConnected) {
                    wsConnected = false;
                    connectWebSocket();
                }
            }
        } else {
            console.log('WebSocket not open, stopping heartbeat');
            if (wsHeartbeatInterval) {
                clearInterval(wsHeartbeatInterval);
                wsHeartbeatInterval = null;
            }
        }
    }, heartbeatInterval);
}

// 显示支付成功通知
function showPaymentSuccessNotification(data) {
    // 去重检查
    if (data.orderNo) {
        const notificationId = `notification_${data.orderNo}`;
        if (document.getElementById(notificationId)) {
            console.log('Notification already exists, skipping:', data.orderNo);
            return;
        }
        
        // 创建通知元素
        const notification = document.createElement('div');
        notification.id = notificationId;
        notification.className = 'payment-notification';
        notification.innerHTML = `
            <div class="notification-content">
                <h4>💰 福生无量</h4>
                <p>订单号: ${data.orderNo}</p>
                <p>金额: ${(() => {
                    // 将分转换成元
                    if (data.amount) {
                        const amount = parseFloat(data.amount);
                        if (!isNaN(amount)) {
                            return (amount / 100).toFixed(2);
                        }
                    }
                    return data.amount || '0.00';
                })()}</p>
                <p>时间: ${data.Time}</p>
            </div>
        `;
        
        // 添加样式
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: #d4edda;
            color: #155724;
            border: 1px solid #c3e6cb;
            border-radius: 8px;
            padding: 15px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            z-index: 10000;
            animation: slideIn 0.3s ease-out;
        `;
        
        // 添加动画
        const style = document.createElement('style');
        style.textContent = `
            @keyframes slideIn {
                from {
                    transform: translateX(100%);
                    opacity: 0;
                }
                to {
                    transform: translateX(0);
                    opacity: 1;
                }
            }
        `;
        document.head.appendChild(style);
        
        // 添加到页面
        document.body.appendChild(notification);
        
        // 3秒后自动移除
        setTimeout(() => {
            notification.style.animation = 'slideIn 0.3s ease-out reverse';
            setTimeout(() => {
                if (notification.parentNode) {
                    notification.parentNode.removeChild(notification);
                }
            }, 300);
        }, 3000);
    } else {
        // 没有订单号，直接显示
        const notification = document.createElement('div');
        notification.className = 'payment-notification';
        notification.innerHTML = `
            <div class="notification-content">
                <h4>💰 福生无量</h4>
                <p>订单号: ${data.orderNo || '未知'}</p>
                <p>金额: ${(() => {
                    // 将分转换成元
                    if (data.amount) {
                        const amount = parseFloat(data.amount);
                        if (!isNaN(amount)) {
                            return (amount / 100).toFixed(2);
                        }
                    }
                    return data.amount || '0.00';
                })()}</p>
                <p>时间: ${data.Time}</p>
            </div>
        `;
        
        // 添加样式
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: #d4edda;
            color: #155724;
            border: 1px solid #c3e6cb;
            border-radius: 8px;
            padding: 15px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            z-index: 10000;
            animation: slideIn 0.3s ease-out;
        `;
        
        // 添加动画
        const style = document.createElement('style');
        style.textContent = `
            @keyframes slideIn {
                from {
                    transform: translateX(100%);
                    opacity: 0;
                }
                to {
                    transform: translateX(0);
                    opacity: 1;
                }
            }
        `;
        document.head.appendChild(style);
        
        // 添加到页面
        document.body.appendChild(notification);
        
        // 3秒后自动移除
        setTimeout(() => {
            notification.style.animation = 'slideIn 0.3s ease-out reverse';
            setTimeout(() => {
                if (notification.parentNode) {
                    notification.parentNode.removeChild(notification);
                }
            }, 300);
        }, 3000);
    }
}

// 数据缓存
const dataCache = {
    paymentConfig: null,
    categories: null,
    rankings: [],
    lastUpdated: 0
};

// 用于去重的捐款记录ID集合 - 已在文件顶部初始化

// 本地存储缓存键
const STORAGE_KEYS = {
    PAYMENT_CONFIG: 'payment_config',
    CATEGORIES: 'categories',
    RANKINGS: 'rankings'
};

// 本地存储缓存过期时间（毫秒）
const CACHE_EXPIRY = {
    PAYMENT_CONFIG: 3600000, // 1小时
    CATEGORIES: 3600000, // 1小时
    RANKINGS: 600000 // 10分钟
};

// 本地存储操作封装
const storage = {
    get: (key) => {
        try {
            const item = localStorage.getItem(key);
            if (item) {
                const parsed = JSON.parse(item);
                if (parsed.expiry && Date.now() < parsed.expiry) {
                    return parsed.data;
                }
            }
            return null;
        } catch (e) {
            return null;
        }
    },
    set: (key, data, expiry) => {
        try {
            localStorage.setItem(key, JSON.stringify({
                data,
                expiry: Date.now() + expiry
            }));
        } catch (e) {
            // 忽略存储错误
        }
    },
    remove: (key) => {
        try {
            localStorage.removeItem(key);
        } catch (e) {
            // 忽略存储错误
        }
    }
};

// 获取支付配置信息
async function getPaymentConfig(paymentConfigId) {
    if (!paymentConfigId) {
        return null;
    }
    
    // 尝试从本地存储获取缓存
    const cacheKey = `${STORAGE_KEYS.PAYMENT_CONFIG}_${paymentConfigId}`;
    const cachedConfig = storage.get(cacheKey);
    if (cachedConfig) {
        return cachedConfig;
    }
    
    try {
        const url = `/api/payment-config/${paymentConfigId}`;
        const response = await fetch(url, {
            headers: {
                'Cache-Control': 'max-age=3600'
            }
        });
        if (!response.ok) {
            throw new Error(`网络请求失败: ${response.status}`);
        }
        const config = await response.json();
        // 缓存到本地存储
        storage.set(cacheKey, config, CACHE_EXPIRY.PAYMENT_CONFIG);
        return config;
    } catch (error) {
        return null;
    }
}

// 并行获取配置数据
async function fetchConfigData() {
    const params = getURLParams();
    const promises = [];
    
    if (params.payment_config_id && !dataCache.paymentConfig) {
        promises.push(getPaymentConfig(params.payment_config_id).then(config => {
            dataCache.paymentConfig = config;
            return config;
        }).catch(error => {
                    return null;
                }));
    }
    
    if (!dataCache.categories) {
        const payment = params.payment_config_id || '6';
        // 尝试从本地存储获取缓存
        const cacheKey = `${STORAGE_KEYS.CATEGORIES}_${payment}`;
        const cachedCategories = storage.get(cacheKey);
        
        if (cachedCategories) {
            dataCache.categories = cachedCategories;
            buildDropdownMenu(cachedCategories, params);
        } else {
            promises.push(fetch(`/api/categories?p=${payment}`, {
                headers: {
                    'Cache-Control': 'max-age=3600'
                }
            })
                .then(response => {
                    if (!response.ok) {
                        throw new Error(`网络请求失败: ${response.status}`);
                    }
                    return response.json();
                })
                .then(categories => {
                    dataCache.categories = categories;
                    // 缓存到本地存储
                    storage.set(cacheKey, categories, CACHE_EXPIRY.CATEGORIES);
                    buildDropdownMenu(categories, params);
                    return categories;
                })
                .catch(error => {
                    return null;
                }));
        }
    }
    
    if (promises.length > 0) {
        try {
            await Promise.all(promises);
            // 配置数据加载完成后，更新logo和标题
            updateLogo();
            updateTitles();
        } catch (error) {
            // 即使失败也继续执行，不阻塞页面加载
        }
    } else {
        // 没有异步操作，直接更新logo和标题
        updateLogo();
        updateTitles();
    }
}

// 构建下拉菜单
function buildDropdownMenu(categories, params) {
    const payment = params.payment_config_id || '6';
    const currentCategory = params.category_id || '';
    const dropdownContent = document.querySelector('.dropdown-content');
    const dropdownBtn = document.querySelector('.dropdown-btn');
    
    if (dropdownContent) {
        dropdownContent.innerHTML = '';
        
        if (Array.isArray(categories) && categories.length > 0) {
            categories.forEach(category => {
                const categoryItem = document.createElement('a');
                categoryItem.href = `/?p=${payment}&c=${category.id}`;
                categoryItem.className = `dropdown-item ${currentCategory === category.id.toString() ? 'active' : ''}`;
                categoryItem.textContent = category.name;
                dropdownContent.appendChild(categoryItem);
            });
        } else {
            const homeItem = document.createElement('a');
            homeItem.href = `/?p=${payment}`;
            homeItem.className = `dropdown-item ${!currentCategory ? 'active' : ''}`;
            homeItem.textContent = '首页';
            dropdownContent.appendChild(homeItem);
        }
    }
    
    if (dropdownBtn) {
        if (Array.isArray(categories) && categories.length > 0) {
            if (currentCategory) {
                const currentCat = categories.find(cat => cat.id.toString() === currentCategory);
                if (currentCat) {
                    dropdownBtn.textContent = currentCat.name;
                } else {
                    dropdownBtn.textContent = '栏目列表';
                }
            } else {
                dropdownBtn.textContent = categories[0].name;
            }
        } else {
            dropdownBtn.textContent = '栏目列表';
        }
    }
}

// 更新页面标题和h1标签
function updatePageTitle() {
    const params = getURLParams();
    let merchantName = '';
    let categoryName = '';
    
    // 使用缓存的支付配置信息
    if (params.payment_config_id && dataCache.paymentConfig && dataCache.paymentConfig.store_name) {
        merchantName = dataCache.paymentConfig.store_name;
    }
    
    // 使用缓存的类目信息
    if (params.category_id && dataCache.categories && Array.isArray(dataCache.categories)) {
        const category = dataCache.categories.find(cat => cat.id == params.category_id);
        if (category && category.name) {
            categoryName = category.name;
        }
    }
    
    // 构建标题
    let newTitle = '';
    if (merchantName && categoryName) {
        newTitle = `${merchantName} ${categoryName} 功德榜`;
    } else if (merchantName) {
        newTitle = `${merchantName} 功德榜`;
    } else if (categoryName) {
        newTitle = `${categoryName} 功德榜`;
    } else {
        newTitle = ' 功德榜';
    }
    
    // 更新页面标题
    document.title = newTitle;
    
    // 只对index1、index2、index6页面更新h1标签
    const payment = params.payment_config_id;
    // 转换为字符串进行比较，确保对数字和字符串类型都有效
    const paymentStr = String(payment);
    if (paymentStr === '1' || paymentStr === '2' || paymentStr === '6') {
        // 使用更具体的选择器
        const h1Element = document.querySelector('.header-content h1');
        if (h1Element) {
            h1Element.textContent = newTitle;
        } else {
            // 如果找不到，尝试其他选择器
            const allH1Elements = document.getElementsByTagName('h1');
            if (allH1Elements.length > 0) {
                allH1Elements[0].textContent = newTitle;
            }
        }
    }
}

// 更新logo地址
function updateLogo() {
    const params = getURLParams();
    
    // 使用缓存的支付配置信息
    if (params.payment_config_id && dataCache.paymentConfig && dataCache.paymentConfig.logo_url) {
        const logoElement = document.querySelector('.header-logo');
        if (logoElement) {
            logoElement.src = dataCache.paymentConfig.logo_url;
        }
    }
}

// 更新标题2和标题3
function updateTitles() {
    const params = getURLParams();
    
    // 使用缓存的支付配置信息
    if (params.payment_config_id && dataCache.paymentConfig) {
        // 转换为字符串进行比较，确保对数字和字符串类型都有效
        const paymentStr = String(params.payment_config_id);
        if (paymentStr === '1' || paymentStr === '2' || paymentStr === '6') {
            // 获取包含所有标题的容器
            const titleContainer = document.querySelector('.header-content > div');
            
            if (titleContainer) {
                // 更新标题2
                if (dataCache.paymentConfig.title2) {
                    const title2Element = titleContainer.querySelector('div:nth-child(2)');
                    if (title2Element) {
                        title2Element.textContent = dataCache.paymentConfig.title2;
                    }
                }
                
                // 更新标题3
                if (dataCache.paymentConfig.title3) {
                    const title3Element = titleContainer.querySelector('.header-info');
                    if (title3Element) {
                        title3Element.textContent = dataCache.paymentConfig.title3;
                    }
                }
            }
        }
    }
}

// 加载功德记录
let currentPage = 1;
let isLoading = false;
const pageSize = 50;
let hasMoreData = true;

// 显示加载状态
function showLoadingState(append = false) {
    if (!append) {
        const rankingsList = document.getElementById('rankings-list');
        rankingsList.innerHTML = `
            <div style="grid-column: 1 / -1; text-align: center; padding: 40px; color: #666;">
                <div style="margin-bottom: 10px;">加载中...</div>
                <div style="width: 40px; height: 40px; margin: 0 auto; border: 3px solid #f3f3f3; border-top: 3px solid #8FD39F; border-radius: 50%; animation: spin 1s linear infinite;"></div>
            </div>
        `;
    } else {
        // 滚动加载时在底部显示小型加载指示器
        const rankingsList = document.getElementById('rankings-list');
        const loadMoreIndicator = document.createElement('div');
        loadMoreIndicator.id = 'load-more-indicator';
        loadMoreIndicator.innerHTML = `
            <div style="grid-column: 1 / -1; text-align: center; padding: 1rem;">
                <div class="loading-spinner" style="width: 16px; height: 16px;"></div>
                <span style="font-size: 0.875rem; color: #666;">加载更多...</span>
            </div>
        `;
        rankingsList.appendChild(loadMoreIndicator);
    }
}

// 隐藏加载状态
function hideLoadingState() {
    const loadMoreIndicator = document.getElementById('load-more-indicator');
    if (loadMoreIndicator) {
        loadMoreIndicator.remove();
    }
    // 确保isLoading状态被重置
    isLoading = false;
}

// 显示无数据状态
function showNoDataState() {
    const rankingsList = document.getElementById('rankings-list');
    rankingsList.innerHTML = `
        <div style="grid-column: 1 / -1; text-align: center; padding: 60px; color: #666;">
            <div style="margin-bottom: 10px;">暂无功德记录</div>
        </div>
    `;
}

// 显示错误状态
function showErrorState(retryCallback) {
    const rankingsList = document.getElementById('rankings-list');
    rankingsList.innerHTML = `
        <div style="grid-column: 1 / -1; text-align: center; padding: 60px; color: #e74c3c;">
            <div style="margin-bottom: 10px;">加载失败</div>
            <button onclick="${retryCallback}" style="padding: 8px 16px; background-color: #8FD39F; color: white; border: none; border-radius: 4px; cursor: pointer;">
                重新加载
            </button>
        </div>
    `;
}

async function loadRankings(append = false) {
    if (isLoading || !hasMoreData) return;
    
    try {
        isLoading = true;
        showLoadingState(append);
        
        const params = getURLParams();
        let url = `/api/rankings?limit=${pageSize}&page=${currentPage}`;
        
        // 添加参数
        if (params.payment_config_id) {
            url += `&payment=${params.payment_config_id}`;
        }
        
        // 直接使用URL中的分类参数（如果有）
        if (params.category_id) {
            url += `&categories=${params.category_id}`;
        }
        
        // 发起请求，添加缓存控制头
        const response = await fetch(url, {
            headers: {
                'Cache-Control': append ? 'no-cache' : 'max-age=60'
            }
        });
        
        if (!response.ok) {
            throw new Error(`网络请求失败: ${response.status}`);
        }
        const data = await response.json();
        
        const rankingsList = document.getElementById('rankings-list');
        
        // 如果不是追加模式，清空列表
        if (!append) {
            rankingsList.innerHTML = '';
            dataCache.rankings = [];
        }
        
        // 按时间倒序排序（最新的在前）
        data.rankings.sort((a, b) => parseDateTime(b.created_at) - parseDateTime(a.created_at));
        
        // 缓存数据并添加到去重集合
        data.rankings.forEach(ranking => {
            if (ranking.id) {
                donationIds.add(ranking.id.toString());
            }
        });
        dataCache.rankings = [...dataCache.rankings, ...data.rankings];
        dataCache.lastUpdated = Date.now();
        
        if (data.rankings.length === 0) {
            if (!append) {
                showNoDataState();
            } else {
                // 显示结束消息
                const endMessage = document.createElement('div');
                endMessage.style.gridColumn = '1 / -1';
                endMessage.style.textAlign = 'center';
                endMessage.style.padding = '2rem';
                endMessage.style.color = '#666';
                endMessage.style.fontSize = '0.875rem';
                endMessage.textContent = '没有更多数据了';
                rankingsList.appendChild(endMessage);
            }
            hasMoreData = false;
            window.removeEventListener('scroll', handleScroll);
        } else {
            // 使用DocumentFragment批量处理DOM操作，减少重排和重绘
            const fragment = document.createDocumentFragment();
            
            data.rankings.forEach((item) => {
                const meritItem = document.createElement('div');
                meritItem.className = 'merit-item';
                
                // 格式化时间显示
                const date = parseDateTime(item.created_at);
                const formattedDate = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
                const formattedTime = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`;
                
                meritItem.innerHTML = `
                    <div style="display: flex; align-items: center; justify-content: space-between; height: 36px;">
                        <div class="merit-amount">¥${item.amount.toFixed(2)}</div>
                        <img src="${item.payment === 'wechat' ? '/static/wechat.png' : '/static/alipay.png'}" alt="${item.payment === 'wechat' ? '微信支付' : '支付宝'}" style="width: 24px; height: 24px; border-radius: 4px; vertical-align: middle;">
                    </div>
                    ${item.blessing ? `<div style="font-size: 14px; color: #666; margin: 8px 0;">${item.blessing}</div>` : ''}
                    <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; margin-top: 8px;">
                        <div style="display: flex; align-items: center; gap: 10px;">
                            <img src="${item.avatar_url}" alt="头像" style="width: 32px; height: 32px; border-radius: 8px;" loading="lazy">
                            <span style="font-size: 14px; font-weight: bold;">${item.user_name || '匿名施主'}</span>
                        </div>
                        <div class="merit-time">${formattedDate} ${formattedTime}</div>
                    </div>
                `;
                
                fragment.appendChild(meritItem);
            });
            
            // 一次性将所有元素添加到DOM中
            rankingsList.appendChild(fragment);
            
            // 如果数据不足一页，说明已经加载完所有数据
            if (data.rankings.length < pageSize) {
                hasMoreData = false;
                window.removeEventListener('scroll', handleScroll);
                
                // 显示结束消息
                const endMessage = document.createElement('div');
                endMessage.style.gridColumn = '1 / -1';
                endMessage.style.textAlign = 'center';
                endMessage.style.padding = '2rem';
                endMessage.style.color = '#666';
                endMessage.style.fontSize = '0.875rem';
                endMessage.textContent = '没有更多数据了';
                rankingsList.appendChild(endMessage);
            }
        }
        
        currentPage++;
    } catch (error) {
        if (!append) {
            showErrorState('initLoadMore()');
        }
    } finally {
        // 无论成功还是失败，都要重置加载状态
        isLoading = false;
        hideLoadingState();
    }
}

// 处理滚动事件，实现下拉加载更多
let scrollTimeout;
let lastScrollHeight = 0;
function handleScroll() {
    // 防抖处理，避免频繁触发
    clearTimeout(scrollTimeout);
    scrollTimeout = setTimeout(() => {
        const scrollTop = document.documentElement.scrollTop || document.body.scrollTop;
        const scrollHeight = document.documentElement.scrollHeight || document.body.scrollHeight;
        const clientHeight = document.documentElement.clientHeight || window.innerHeight;
        

        
        // 当滚动到距离底部100px时加载更多
        // 同时确保页面高度确实增加了，避免因为加载指示器的显示/隐藏导致的无限循环
        if (scrollTop + clientHeight >= scrollHeight - 100 && hasMoreData && !isLoading && scrollHeight > lastScrollHeight) {
            lastScrollHeight = scrollHeight;
            loadRankings(true);
        }
    }, 300); // 增加防抖时间到300ms，进一步减少触发频率
}

// 初始化页面时加载数据并添加滚动事件监听器
function initLoadMore() {
    // 重置状态
    currentPage = 1;
    hasMoreData = true;
    lastScrollHeight = 0; // 重置滚动高度记录
    
    // 首次加载数据
    loadRankings();
    
    // 添加滚动事件监听器
    window.removeEventListener('scroll', handleScroll);
    window.addEventListener('scroll', handleScroll);
}

// 更新二维码链接
function updateQRCode() {
    const params = getURLParams();
    let url = '/qrcode';
    
    // 添加参数（使用简洁格式）
    if (params.payment_config_id) {
        url += `?p=${params.payment_config_id}`;
        
        // 直接使用URL中的分类参数（如果有）
        if (params.category_id) {
            url += `&c=${params.category_id}`;
        }
    } else if (params.category_id) {
        url += `?c=${params.category_id}`;
    }
    
    // 更新模态窗口中的二维码
    const modalQRCodeImg = document.getElementById('modal-qrcode');
    if (modalQRCodeImg) {
        modalQRCodeImg.src = url;
    }
}

// 模态窗口功能
function initModal() {
    const meritBtn = document.getElementById('merit-btn');
    const meritModal = document.getElementById('merit-modal');
    const closeModal = document.getElementById('close-modal');
    
    if (meritBtn && meritModal && closeModal) {
        // 打开模态窗口
        meritBtn.addEventListener('click', function() {
            meritModal.style.display = 'flex';
        });
        
        // 关闭模态窗口
        closeModal.addEventListener('click', function() {
            meritModal.style.display = 'none';
        });
        
        // 点击模态窗口外部关闭
        meritModal.addEventListener('click', function(e) {
            if (e.target === meritModal) {
                meritModal.style.display = 'none';
            }
        });
    }
}

// HTTP轮询管理
let pollingInterval;
let currentPollingInterval = 2000; // 当前轮询间隔（毫秒）
const minPollingInterval = 1000; // 最小轮询间隔（毫秒）
const maxPollingInterval = 5000; // 最大轮询间隔（毫秒）
const defaultPollingInterval = 2000; // 默认轮询间隔（毫秒）
const pollingBackoffFactor = 1.2; // 轮询间隔增加因子
const pollingRecoveryFactor = 0.9; // 轮询间隔减少因子
// lastDonationTime 已在文件顶部初始化

// 启动HTTP轮询
function startPolling() {
    // 清除之前的定时器
    if (pollingInterval) {
        clearInterval(pollingInterval);
        pollingInterval = null;
    }
    
    console.log(`Starting polling with interval: ${currentPollingInterval} ms`);
    
    // 立即执行一次轮询
    pollForNewDonations();
    
    // 设置轮询定时器（使用动态间隔）
    pollingInterval = setInterval(pollForNewDonations, currentPollingInterval);
}

// 调整轮询间隔
function adjustPollingInterval(success) {
    if (success) {
        // 请求成功，减少轮询间隔（恢复）
        currentPollingInterval = Math.max(minPollingInterval, Math.floor(currentPollingInterval * pollingRecoveryFactor));
        console.log(`Polling interval adjusted (success): ${currentPollingInterval} ms`);
    } else {
        // 请求失败，增加轮询间隔（退避）
        currentPollingInterval = Math.min(maxPollingInterval, Math.floor(currentPollingInterval * pollingBackoffFactor));
        console.log(`Polling interval adjusted (failure): ${currentPollingInterval} ms`);
    }
    
    // 直接更新定时器，避免立即执行轮询
    if (pollingInterval) {
        clearInterval(pollingInterval);
        pollingInterval = setInterval(pollForNewDonations, currentPollingInterval);
        console.log(`Polling interval updated: ${currentPollingInterval} ms`);
    }
}

// 轮询获取新的捐款记录
function pollForNewDonations() {
    const params = getURLParams();
    
    // 构建API请求URL
    let apiUrl = '/api/rankings?limit=1';
    
    // 添加参数（使用简洁格式）
    if (params.payment_config_id) {
        apiUrl += `&p=${encodeURIComponent(params.payment_config_id)}`;
    }
    if (params.category_id) {
        apiUrl += `&c=${encodeURIComponent(params.category_id)}`;
    }
    
    console.log('Polling for new donations from:', apiUrl);
    
    // 启用HTTP轮询作为备用机制，确保WebSocket有问题时数据也能更新
    console.log('HTTP polling enabled as backup');
    
    // 发起HTTP请求（添加缓存控制）
    console.log('Initiating poll request:', {
        url: apiUrl,
        timestamp: new Date().toISOString(),
        browser: getBrowserInfo()
    });
    
    fetch(apiUrl, {
        method: 'GET',
        headers: {
            'Cache-Control': 'no-cache, no-store, must-revalidate',
            'Pragma': 'no-cache',
            'Expires': '0'
        },
        cache: 'no-cache'
    })
        .then(response => {
            console.log('Poll response received:', {
                status: response.status,
                statusText: response.statusText,
                url: response.url
            });
            
            if (!response.ok) {
                const errorMsg = `HTTP error! status: ${response.status}, statusText: ${response.statusText}`;
                console.error('Poll response error:', errorMsg);
                throw new Error(errorMsg);
            }
            
            return response.json();
        })
        .then(data => {
            console.log('Received rankings data:', data);
            
            // 处理响应数据
            if (data && data.rankings && Array.isArray(data.rankings)) {
                console.log('Processing', data.rankings.length, 'rankings');
                data.rankings.forEach(donation => {
                    console.log('Processing donation:', donation);
                    
                    try {
                        // 检查捐款记录是否与当前页面参数匹配
                        if (checkDonationMatch(donation, params)) {
                            console.log('Donation matches current page parameters');
                            
                            // 检查是否是新的捐款记录（通过ID判断）
                            const donationId = (donation.id || donation.ID || donation.order_id || donation.OrderID || '').toString().trim();
                            console.log('Donation ID:', donationId);
                            
                            if (donationId && !donationIds.has(donationId)) {
                                console.log('New donation found by ID, adding to page:', donation);
                                
                                // 直接使用API返回的数据，不做任何处理
                                insertNewPaymentRecord(donation);
                                
                                // 同时更新时间戳，作为备用去重机制
                                const donationTime = parseDateTime(donation.created_at || donation.CreatedAt || Date.now()).getTime();
                                if (donationTime > lastDonationTime) {
                                    lastDonationTime = donationTime;
                                    console.log('Updated last donation time:', new Date(lastDonationTime).toISOString());
                                }
                            } else if (!donationId) {
                                console.log('Donation has no ID, using time-based check:', donation);
                                
                                // 如果没有ID，使用时间判断
                                const donationTime = parseDateTime(donation.created_at || donation.CreatedAt || Date.now()).getTime();
                                if (donationTime > lastDonationTime) {
                                    console.log('New donation found by time, adding to page:', donation);
                                    insertNewPaymentRecord(donation);
                                    lastDonationTime = donationTime;
                                    console.log('Updated last donation time:', new Date(lastDonationTime).toISOString());
                                }
                            } else {
                                console.log('Donation already exists, skipping:', donationId);
                            }
                        } else {
                            console.log('Donation does not match current page parameters, skipping:', donation);
                        }
                    } catch (processError) {
                        console.error('Error processing donation:', processError);
                        console.error('Donation processing error details:', { 
                            error: processError.message, 
                            stack: processError.stack,
                            donation: donation 
                        });
                    }
                });
            } else {
                console.log('No rankings data received:', data);
            }
            
            // 请求成功，调整轮询间隔
            adjustPollingInterval(true);
        })
        .catch(error => {
            console.error('Error polling for donations:', error);
            console.error('Poll error details:', { 
                error: error.message, 
                stack: error.stack,
                url: apiUrl
            });
            // 请求失败，调整轮询间隔
            adjustPollingInterval(false);
        });
}

// 停止HTTP轮询
function stopPolling() {
    if (pollingInterval) {
        clearInterval(pollingInterval);
        pollingInterval = null;
    }
}


function checkDonationMatch(donation, params) {
    // 兼容处理：统一转换为字符串
    const paymentParam = params.payment_config_id.toString().trim();
    const categoryParam = params.category_id.toString().trim();
    
    // 情况1：无任何参数，接收所有广播
    if (!paymentParam && !categoryParam) {
        return true;
    }
    
    // 检查payment参数（兼容多种字段名）
    let paymentMatch = true;
    if (paymentParam) {
        // 分别获取payment和payment_config_id
        const donationPayment = (donation.payment || donation.Payment || '').toString().trim().toLowerCase();
        const donationPaymentConfigId = (donation.payment_config_id || donation.PaymentConfigID || '').toString().trim();
        const normalizedPaymentParam = paymentParam.toString().trim().toLowerCase();
        
        // 浏览器兼容性处理
        const browserInfo = getBrowserInfo();
        console.log(`Checking payment match for ${browserInfo.isWeChat ? 'WeChat' : browserInfo.isSafari ? 'Safari' : browserInfo.isEdge ? 'Edge' : browserInfo.isChrome ? 'Chrome' : 'Firefox'} browser`);
        console.log(`Payment param: ${paymentParam} (normalized: ${normalizedPaymentParam})`);
        console.log(`Donation payment: ${donationPayment}, Config ID: ${donationPaymentConfigId}`);
        
        // 支持ID匹配（如2）或文本匹配（如wechat/alipay）
        paymentMatch = false;
        
        // 情况1：直接匹配payment_config_id
        if (donationPaymentConfigId === paymentParam) {
            paymentMatch = true;
            console.log('Payment match by config ID:', donationPaymentConfigId, '=', paymentParam);
        }
        // 情况2：直接匹配payment（使用规范化值）
        else if (donationPayment === normalizedPaymentParam) {
            paymentMatch = true;
            console.log('Payment match by payment (normalized):', donationPayment, '=', normalizedPaymentParam);
        }
        // 情况3：微信支付匹配（增强兼容性）
        else if ((donationPayment === 'wechat' || donationPaymentConfigId === '2' || donationPaymentConfigId === '1') && 
                 (normalizedPaymentParam === '2' || normalizedPaymentParam === '1' || normalizedPaymentParam === 'wechat')) {
            paymentMatch = true;
            console.log('Payment match by wechat rule (enhanced compatibility)');
        }
        // 情况4：支付宝匹配（增强兼容性）
        else if ((donationPayment === 'alipay' || donationPaymentConfigId === '1') && 
                 (normalizedPaymentParam === '1' || normalizedPaymentParam === 'alipay')) {
            paymentMatch = true;
            console.log('Payment match by alipay rule');
        }
        else {
            console.log('Payment mismatch:');
            console.log('  Param:', paymentParam);
            console.log('  Normalized param:', normalizedPaymentParam);
            console.log('  Payment:', donationPayment);
            console.log('  Config ID:', donationPaymentConfigId);
        }
    }
    
    // 检查categories参数（兼容多种字段名）
    let categoryMatch = true;
    if (categoryParam) {
        const donationCategory = (donation.category_id || donation.CategoryID || donation.categories || donation.Categories || '').toString().trim();
        categoryMatch = donationCategory === categoryParam;
        if (!categoryMatch) {
            console.log('Category mismatch:');
            console.log('  Param:', categoryParam);
            console.log('  Donation:', donationCategory);
        }
    }
    
    console.log('Match result:', paymentMatch && categoryMatch);
    return paymentMatch && categoryMatch;
}

// 添加新的捐款记录到页面
function addNewDonation(donation) {
    console.log('Adding new donation using API data:', donation);
    const rankingsList = document.getElementById('rankings-list');
    if (!rankingsList) {
        return;
    }
    
    try {
        // 兼容处理：获取ID字段（支持驼峰和蛇形命名）
        const donationId = (donation.id || donation.ID || '').toString().trim();
        if (donationId) {
            if (donationIds.has(donationId)) {
                console.log('Donation already exists, skipping:', donationId);
                return;
            }
            // 添加到已存在的ID集合
            donationIds.add(donationId);
        }
        
        // 兼容处理：获取时间字段（支持驼峰和蛇形命名，处理不同格式）
        let date;
        let timeStr = donation.created_at || donation.CreatedAt || '';
        
        // 使用统一的日期时间解析函数
        date = parseDateTime(timeStr);
        
        const formattedDate = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
        const formattedTime = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`;
        
        // 直接使用API返回的数据，不做任何处理
        const amount = donation.amount || donation.Amount || '0';
        console.log('Using API amount:', amount);
        
        const payment = donation.payment || donation.Payment || '';
        console.log('Using API payment:', payment);
        
        // 浏览器兼容性处理：确保支付方式值在所有浏览器中一致
        const browserInfo = getBrowserInfo();
        const normalizedPayment = payment.toLowerCase().trim();
        console.log(`Normalized payment: ${normalizedPayment} for ${browserInfo.isWeChat ? 'WeChat' : browserInfo.isSafari ? 'Safari' : browserInfo.isEdge ? 'Edge' : browserInfo.isChrome ? 'Chrome' : 'Firefox'} browser`);
        
        const blessing = donation.blessing || donation.Blessing || '';
        console.log('Using API blessing:', blessing);
        
        const avatarUrl = donation.avatar_url || donation.AvatarURL || './static/avatar.jpeg';
        console.log('Using API avatar_url:', avatarUrl);
        
        const userName = donation.user_name || donation.UserName || donation.username || donation.Username || '匿名施主';
        console.log('Using API user_name:', userName);
        
        // 构建HTML内容
        const paymentIcon = normalizedPayment === 'wechat' ? '/static/wechat.png' : '/static/alipay.png';
        const paymentText = normalizedPayment === 'wechat' ? '微信支付' : '支付宝';
        
        // 创建新的功德项
        const meritItem = document.createElement('div');
        meritItem.className = 'merit-item';
        
        meritItem.innerHTML = `
            <div style="display: flex; align-items: center; justify-content: space-between; height: 36px;">
                <div class="merit-amount">¥${amount}</div>
                <img src="${paymentIcon}" alt="${paymentText}" style="width: 24px; height: 24px; border-radius: 4px; vertical-align: middle;">
            </div>
            ${blessing ? `<div style="font-size: 14px; color: #666; margin: 8px 0;">${blessing}</div>` : ''}
            <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; margin-top: 8px;">
                <div style="display: flex; align-items: center; gap: 10px;">
                    <img src="${avatarUrl}" alt="头像" style="width: 32px; height: 32px; border-radius: 8px;">
                    <span style="font-size: 14px; font-weight: bold;">${userName}</span>
                </div>
                <div class="merit-time">${formattedDate} ${formattedTime}</div>
            </div>
        `;
        
        if (rankingsList.children.length === 0 || (rankingsList.children[0].textContent && rankingsList.children[0].textContent.includes('暂无功德记录'))) {
            rankingsList.innerHTML = '';
            rankingsList.appendChild(meritItem);
        } else {
            // 添加到列表顶部
            rankingsList.insertBefore(meritItem, rankingsList.firstChild);
        }
    } catch (error) {
        console.error('Error adding new donation:', error);
        // 静默处理错误，避免日志混乱
    }
}

// 刷新排行榜数据
function refreshRankings() {
    console.log('Refreshing rankings...');
    // 重置状态并重新加载排行榜
    currentPage = 1;
    hasMoreData = true;
    loadRankings(false);
}

// 插入新的支付记录到数据列最前面
function insertNewPaymentRecord(data) {
    console.log('Inserting new payment record:', data);
    
    const rankingsList = document.getElementById('rankings-list');
    if (!rankingsList) {
        console.error('rankings-list element not found!');
        return;
    }
    
    try {
        // 去重检查（与addNewDonation函数保持一致）
        let donationId = '';
        // 优先使用id字段
        if (data.id) {
            donationId = data.id.toString().trim();
        } else if (data.ID) {
            donationId = data.ID.toString().trim();
        } else if (data.orderNo) {
            donationId = data.orderNo.toString().trim();
        } else if (data.OrderNo) {
            donationId = data.OrderNo.toString().trim();
        } else if (data.order_id) {
            donationId = data.order_id.toString().trim();
        } else if (data.OrderID) {
            donationId = data.OrderID.toString().trim();
        }
        
        console.log('Processing donation with ID:', donationId);
        
        if (donationId && donationIds.has(donationId)) {
            console.log('Payment record already exists, skipping:', donationId);
            return;
        }
        
        if (donationId) {
            donationIds.add(donationId);
            console.log('Added donation ID to set:', donationId);
        }
        
        // 格式化时间
        let timeStr = data.created_at || data.CreatedAt || data.Time || '';
        
        // 使用统一的日期时间解析函数
        let date = parseDateTime(timeStr);
        
        const formattedDate = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
        const formattedTime = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`;
        
        // 确定支付方式图标和文本
        let payment = data.payment || data.Payment || '';
        // 直接使用API返回的支付方式，不做任何处理
        console.log('Using API payment:', payment);
        
        // 浏览器兼容性处理：确保支付方式值在所有浏览器中一致
        const browserInfo = getBrowserInfo();
        const normalizedPayment = payment.toLowerCase().trim();
        console.log(`Normalized payment: ${normalizedPayment} for ${browserInfo.isWeChat ? 'WeChat' : browserInfo.isSafari ? 'Safari' : browserInfo.isEdge ? 'Edge' : browserInfo.isChrome ? 'Chrome' : 'Firefox'} browser`);
        
        const paymentIcon = normalizedPayment === 'wechat' ? '/static/wechat.png' : '/static/alipay.png';
        const paymentText = normalizedPayment === 'wechat' ? '微信支付' : '支付宝';
        
        // 确定头像URL（支持多种字段名格式）
        const avatarUrl = data.avatar_url || data.AvatarURL || './static/avatar.jpeg';
        console.log('Using API avatar_url:', avatarUrl);
        
        // 确定用户名（支持多种字段名格式）
        const userName = data.user_name || data.UserName || data.username || data.Username || '匿名施主';
        console.log('Using API user_name:', userName);
        
        // 确定祝福语（支持多种字段名格式）
        const blessing = data.blessing || data.Blessing || '';
        console.log('Using API blessing:', blessing);
        
        // 确定金额
        // 直接使用API返回的金额，不做任何处理
        let amount = data.amount || data.Amount || '0';
        console.log('Using API amount:', amount);
        
        console.log('Building HTML content...');
        
        // 构建HTML内容（与现有样式保持一致）
        const meritItem = document.createElement('div');
        meritItem.className = 'merit-item';
        
        // 为新记录添加特殊背景色（浅红色）
        meritItem.style.backgroundColor = '#fff0f0';
        meritItem.style.transition = 'background-color 0.3s ease';
        meritItem.style.padding = '10px';
        meritItem.style.margin = '5px 0';
        meritItem.style.border = '1px solid #ddd';
        meritItem.style.borderRadius = '4px';
        
        // 简化HTML构建，避免可能的语法错误
        meritItem.innerHTML = `
            <div style="display: flex; align-items: center; justify-content: space-between; height: 36px;">
                <div class="merit-amount">¥${amount}</div>
                <img src="${paymentIcon}" alt="${paymentText}" style="width: 24px; height: 24px; border-radius: 4px; vertical-align: middle;">
            </div>
            ${blessing ? `<div style="font-size: 14px; color: #666; margin: 8px 0;">${blessing}</div>` : ''}
            <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; margin-top: 8px;">
                <div style="display: flex; align-items: center; gap: 10px;">
                    <img src="${avatarUrl}" alt="头像" style="width: 32px; height: 32px; border-radius: 8px;">
                    <span style="font-size: 14px; font-weight: bold;">${userName}</span>
                </div>
                <div class="merit-time">${formattedDate} ${formattedTime}</div>
            </div>
        `;
        
        console.log('HTML content built successfully');
        console.log('Merit item created:', meritItem);
        
        // 插入到数据列最前面
        console.log('Inserting item to rankings-list...');
        console.log('Rankings list children count:', rankingsList.children.length);
        
        if (rankingsList.children.length === 0 || (rankingsList.children[0].textContent && rankingsList.children[0].textContent.includes('暂无功德记录'))) {
            console.log('Empty list, clearing and adding first item');
            rankingsList.innerHTML = '';
            rankingsList.appendChild(meritItem);
            console.log('Inserted as first record (empty list)');
        } else {
            console.log('Adding to existing list as first child');
            rankingsList.insertBefore(meritItem, rankingsList.firstChild);
            console.log('Inserted as first child');
        }
        
        // 验证插入是否成功
        console.log('After insertion, number of children:', rankingsList.children.length);
        console.log('First child:', rankingsList.firstChild);
        
        // 5秒钟后恢复与数据列表相同的背景色
        setTimeout(() => {
            meritItem.style.backgroundColor = '';
        }, 5000);
        
        console.log('InsertNewPaymentRecord completed successfully');
        
    } catch (error) {
        console.error('Error inserting new payment record:', error);
        console.error('Error stack:', error.stack);
    }
}

// 初始化HTTP轮询
function initPolling() {
    try {
        startPolling();
    } catch (error) {
        // 静默处理错误，避免日志混乱
    }
}

// 初始加载
function init() {
    // 检查URL参数
    const params = getURLParams();
    console.log('Init function called with params:', params);
    
    // 处理默认图片容器
    const defaultImageContainer = document.getElementById('default-image-container');
    if (!params.payment_config_id) {
        console.log('No payment_config_id parameter, skipping WebSocket connection');
        if (defaultImageContainer) {
            defaultImageContainer.style.display = 'flex';
        }
        // 没有payment_config_id参数，只初始化HTTP轮询和必要的功能
        initPolling();
        initLazyLoading();
        return;
    }
    
    console.log('Payment parameter found:', params.payment_config_id);
    
    // 有payment参数，确保默认图片容器隐藏
    if (defaultImageContainer) {
        defaultImageContainer.style.display = 'none';
    }
    
    // 立即初始化模态窗口，让页面快速显示
    initModal();
    
    // 优先初始化HTTP轮询，避免错过早期广播
    initPolling();
    
    // 初始化WebSocket连接 - 移到前面，确保优先建立连接
    console.log('Initializing WebSocket connection...');
    connectWebSocket();
    
    // 异步加载配置数据（包含分类数据和下拉菜单构建），不阻塞页面显示
    fetchConfigData().then(() => {
        // 配置加载完成后，更新页面标题、二维码和其他标题
        updatePageTitle();
        updateTitles();
        updateQRCode();
    }).catch(error => {
        // 即使失败也更新页面标题，使用默认值
        updatePageTitle();
        updateTitles();
    });
    
    // 异步加载排名数据，不阻塞页面显示
    initLoadMore();
    
    // 初始化图片懒加载
    initLazyLoading();
}

// 初始化图片懒加载
function initLazyLoading() {
    const lazyImages = document.querySelectorAll('img[data-src]');
    
    if ('IntersectionObserver' in window) {
        // 使用Intersection Observer API
        const imageObserver = new IntersectionObserver((entries, observer) => {
            entries.forEach(entry => {
                if (entry.isIntersecting) {
                    const image = entry.target;
                    image.src = image.dataset.src;
                    image.removeAttribute('data-src');
                    imageObserver.unobserve(image);
                }
            });
        });
        
        lazyImages.forEach(image => {
            imageObserver.observe(image);
        });
    } else {
        // 回退到传统方法
        lazyLoadFallback(lazyImages);
    }
}

// 懒加载的回退方法
function lazyLoadFallback(images) {
    const imageInView = (image) => {
        const rect = image.getBoundingClientRect();
        return (
            rect.top <= (window.innerHeight || document.documentElement.clientHeight) &&
            rect.left <= (window.innerWidth || document.documentElement.clientWidth)
        );
    };
    
    const loadImages = (images) => {
        images.forEach(image => {
            if (imageInView(image)) {
                image.src = image.dataset.src;
                image.removeAttribute('data-src');
            }
        });
        
        // 过滤掉已经加载的图片
        const remainingImages = document.querySelectorAll('img[data-src]');
        if (remainingImages.length > 0) {
            setTimeout(() => {
                lazyLoadFallback(remainingImages);
            }, 200);
        }
    };
    
    loadImages(images);
}

// 初始加载
init();