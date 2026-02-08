// 从URL获取参数
function getURLParams() {
    const params = new URLSearchParams(window.location.search);
    return {
        payment: params.get('payment') || '',
        categories: params.get('categories') || ''
    };
}

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

// 检测是否是微信浏览器
function isWeChatBrowser() {
    const ua = navigator.userAgent.toLowerCase();
    return ua.indexOf('micromessenger') > -1;
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
    console.log('WebSocket connect function called');
    console.log('URL params:', params);
    console.log('Is WeChat browser:', isWeChatBrowser());
    
    // 动态构建WebSocket地址，根据当前页面的协议和主机名
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    let wsUrl = `${protocol}//${host}/ws/pay-notify`;
    console.log('Dynamic WebSocket URL:', wsUrl);
    
    // 添加参数
    const queryParams = [];
    if (params.payment) queryParams.push(`payment=${params.payment}`);
    if (params.categories) queryParams.push(`categories=${params.categories}`);
    
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
        
        // 微信浏览器特殊处理：使用更可靠的连接方式
        if (isWeChatBrowser()) {
            console.log('Using WeChat browser optimized WebSocket connection');
            // 微信浏览器可能需要更长的超时时间
            setTimeout(() => {
                if (wsConnecting) {
                    console.log('WeChat browser connection timeout, retrying...');
                    wsConnecting = false;
                    connectWebSocket();
                }
            }, 15000); // 微信浏览器使用15秒超时
        }
        
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
        }, isWeChatBrowser() ? 15000 : 10000); // 微信浏览器使用15秒超时
        
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
                        } catch (e) {
                            console.error('Error sending pong:', e);
                        }
                    }
                    return;
                }
                
                // 检查是否是pong消息（服务器回复的心跳响应）
                if (event.data === 'pong') {
                    // 心跳响应，不需要处理
                    return;
                }
                
                // 检查是否是字符串类型的消息
                if (typeof event.data === 'string') {
                    const data = JSON.parse(event.data);
                    console.log('Received broadcast:', data);
                    
                    // 处理支付成功消息
                    if (data.type === 'pay_success') {
                        showPaymentSuccessNotification(data);
                        
                        // 不直接使用广播数据，而是从 /api/rankings?limit=1 获取最新数据
                        console.log('Broadcast received, fetching latest data from /api/rankings?limit=1');
                        
                        // 构建API请求URL
                        const params = getURLParams();
                        let apiUrl = '/api/rankings?limit=1';
                        
                        // 添加参数
                        if (params.payment) {
                            apiUrl += `&payment=${encodeURIComponent(params.payment)}`;
                        }
                        if (params.categories) {
                            apiUrl += `&categories=${encodeURIComponent(params.categories)}`;
                        }
                        
                        // 发起请求获取最新数据
                        fetch(apiUrl)
                            .then(response => {
                                if (!response.ok) {
                                    throw new Error(`HTTP error! status: ${response.status}`);
                                }
                                return response.json();
                            })
                            .then(rankingsData => {
                                console.log('Received latest rankings data:', rankingsData);
                                
                                // 使用获取到的数据更新页面
                                if (rankingsData && rankingsData.rankings && Array.isArray(rankingsData.rankings)) {
                                    rankingsData.rankings.forEach(donation => {
                                        // 直接使用API返回的数据，不做任何处理
                                        console.log('Using API data for broadcast:', donation);
                                        insertNewPaymentRecord(donation);
                                    });
                                }
                            })
                            .catch(error => {
                                console.error('Error fetching latest rankings:', error);
                                // 如果API请求失败，回退到使用广播数据
                                console.log('Falling back to broadcast data:', data);
                                insertNewPaymentRecord(data);
                            });
                    }
                } else {
                    console.log('Received non-string WebSocket message:', event.data);
                }
            } catch (error) {
                console.error('Error parsing WebSocket message:', error);
                // 忽略解析错误，继续运行
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
    
    // 微信浏览器使用更频繁的心跳检测
    const heartbeatInterval = isWeChatBrowser() ? 15000 : 20000; // 微信浏览器15秒，其他浏览器20秒
    console.log('Starting WebSocket heartbeat with interval:', heartbeatInterval, 'ms');
    
    wsHeartbeatInterval = setInterval(function() {
        if (ws && ws.readyState === WebSocket.OPEN) {
            try {
                ws.send('ping');
                console.log('WebSocket heartbeat sent');
                
                // 微信浏览器特殊处理：发送心跳后等待pong响应
                if (isWeChatBrowser()) {
                    console.log('WeChat browser heartbeat sent, waiting for response...');
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
    rankings: []
};

// 用于去重的捐款记录ID集合
const donationIds = new Set();

// 获取支付配置信息
async function getPaymentConfig(paymentConfigId) {
    if (!paymentConfigId) {
        return null;
    }
    
    try {
        const url = `/api/payment-config/${paymentConfigId}`;
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`网络请求失败: ${response.status}`);
        }
        return await response.json();
    } catch (error) {
        return null;
    }
}

// 并行获取配置数据
async function fetchConfigData() {
    const params = getURLParams();
    const promises = [];
    
    if (params.payment && !dataCache.paymentConfig) {
        promises.push(getPaymentConfig(params.payment).then(config => {
            dataCache.paymentConfig = config;
            return config;
        }).catch(error => {
                    return null;
                }));
    }
    
    if (!dataCache.categories) {
        const payment = params.payment || '6';
        promises.push(fetch(`/api/categories?payment=${payment}`)
            .then(response => {
                if (!response.ok) {
                    throw new Error(`网络请求失败: ${response.status}`);
                }
                return response.json();
            })
            .then(categories => {
                dataCache.categories = categories;
                
                // 构建下拉菜单
                const currentCategory = params.categories || '';
                const dropdownContent = document.querySelector('.dropdown-content');
                const dropdownBtn = document.querySelector('.dropdown-btn');
                
                if (dropdownContent) {
                    dropdownContent.innerHTML = '';
                    
                    if (Array.isArray(categories) && categories.length > 0) {
                        categories.forEach(category => {
                            const categoryItem = document.createElement('a');
                            categoryItem.href = `/?payment=${payment}&categories=${category.id}`;
                            categoryItem.className = `dropdown-item ${currentCategory === category.id.toString() ? 'active' : ''}`;
                            categoryItem.textContent = category.name;
                            dropdownContent.appendChild(categoryItem);
                        });
                    } else {
                        const homeItem = document.createElement('a');
                        homeItem.href = `/?payment=${payment}`;
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
                
                return categories;
            })
            .catch(error => {
                return null;
            }));
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
    }
}

// 更新页面标题和h1标签
function updatePageTitle() {
    const params = getURLParams();
    let merchantName = '';
    let categoryName = '';
    
    // 使用缓存的支付配置信息
    if (params.payment && dataCache.paymentConfig && dataCache.paymentConfig.store_name) {
        merchantName = dataCache.paymentConfig.store_name;
    }
    
    // 使用缓存的类目信息
    if (params.categories && dataCache.categories && Array.isArray(dataCache.categories)) {
        const category = dataCache.categories.find(cat => cat.id == params.categories);
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
    const payment = params.payment;
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
    if (params.payment && dataCache.paymentConfig && dataCache.paymentConfig.logo_url) {
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
    if (params.payment && dataCache.paymentConfig) {
        // 转换为字符串进行比较，确保对数字和字符串类型都有效
        const paymentStr = String(params.payment);
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
        if (params.payment) {
            url += `&payment=${params.payment}`;
        }
        
        // 直接使用URL中的分类参数（如果有）
        if (params.categories) {
            url += `&categories=${params.categories}`;
        }
        
        const response = await fetch(url);
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
        data.rankings.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));
        
        // 缓存数据并添加到去重集合
        data.rankings.forEach(ranking => {
            if (ranking.id) {
                donationIds.add(ranking.id.toString());
            }
        });
        dataCache.rankings = [...dataCache.rankings, ...data.rankings];
        
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
                const date = new Date(item.created_at);
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
                            <img src="${item.avatar_url}" alt="头像" style="width: 32px; height: 32px; border-radius: 8px;">
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
    
    // 添加参数
    if (params.payment) {
        url += `?payment=${params.payment}`;
        
        // 直接使用URL中的分类参数（如果有）
        if (params.categories) {
            url += `&categories=${params.categories}`;
        }
    } else if (params.categories) {
        url += `?categories=${params.categories}`;
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
const pollingIntervalTime = 5000; // 5秒轮询一次
let lastDonationTime = 0;

// 启动HTTP轮询
function startPolling() {
    // 清除之前的定时器
    if (pollingInterval) {
        clearInterval(pollingInterval);
    }
    
    // 立即执行一次轮询
    pollForNewDonations();
    
    // 设置轮询定时器
    pollingInterval = setInterval(pollForNewDonations, pollingIntervalTime);
}

// 轮询获取新的捐款记录
function pollForNewDonations() {
    const params = getURLParams();
    
    // 构建API请求URL
    let apiUrl = '/api/rankings?limit=1';
    
    // 添加参数
    if (params.payment) {
        apiUrl += `&payment=${encodeURIComponent(params.payment)}`;
    }
    if (params.categories) {
        apiUrl += `&categories=${encodeURIComponent(params.categories)}`;
    }
    
    console.log('Polling for new donations from:', apiUrl);
    
    // 暂时关闭HTTP轮询获取数据的功能，避免与WebSocket广播重复
    console.log('HTTP polling disabled to avoid duplicate data with WebSocket broadcast');
    /*
    // 发起HTTP请求
    fetch(apiUrl)
        .then(response => {
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
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
                    
                    // 检查捐款记录是否与当前页面参数匹配
                    if (checkDonationMatch(donation, params)) {
                        console.log('Donation matches current page parameters');
                        
                        // 检查是否是新的捐款记录（通过ID判断）
                        const donationId = (donation.id || donation.ID || '').toString().trim();
                        console.log('Donation ID:', donationId);
                        
                        if (donationId && !donationIds.has(donationId)) {
                            console.log('New donation found by ID, adding to page:', donation);
                            
                            // 直接使用API返回的数据，不做任何处理
                            addNewDonation(donation);
                            
                            // 同时更新时间戳，作为备用去重机制
                            const donationTime = new Date(donation.created_at || donation.CreatedAt || Date.now()).getTime();
                            if (donationTime > lastDonationTime) {
                                lastDonationTime = donationTime;
                            }
                        } else if (!donationId) {
                            console.log('Donation has no ID, using time-based check:', donation);
                            
                            // 如果没有ID，使用时间判断
                            const donationTime = new Date(donation.created_at || donation.CreatedAt || Date.now()).getTime();
                            if (donationTime > lastDonationTime) {
                                console.log('New donation found by time, adding to page:', donation);
                                addNewDonation(donation);
                                lastDonationTime = donationTime;
                            }
                        } else {
                            console.log('Donation already exists, skipping:', donationId);
                        }
                    } else {
                        console.log('Donation does not match current page parameters, skipping:', donation);
                    }
                });
            } else {
                console.log('No rankings data received:', data);
            }
        })
        .catch(error => {
            console.error('Error polling for donations:', error);
            // 静默处理网络错误，避免日志混乱
        });
    */
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
    const paymentParam = params.payment.toString().trim();
    const categoryParam = params.categories.toString().trim();
    
    // 情况1：无任何参数，接收所有广播
    if (!paymentParam && !categoryParam) {
        return true;
    }
    
    // 检查payment参数（兼容多种字段名）
    let paymentMatch = true;
    if (paymentParam) {
        const donationPayment = (donation.payment || donation.Payment || donation.payment_config_id || donation.PaymentConfigID || '').toString().trim();
        const donationPaymentText = donation.payment || donation.Payment || '';
        
        // 支持ID匹配（如2）或文本匹配（如wechat/alipay）
        paymentMatch = false;
        
        // 情况1：直接匹配（如ID或文本完全相同）
        if (donationPayment === paymentParam) {
            paymentMatch = true;
        }
        // 情况2：微信支付匹配
        else if ((donationPaymentText === 'wechat' || donationPayment === '2') && 
                 (paymentParam === '2' || paymentParam === 'wechat')) {
            paymentMatch = true;
        }
        // 情况3：支付宝匹配
        else if ((donationPaymentText === 'alipay' || donationPayment === '1') && 
                 (paymentParam === '1' || paymentParam === 'alipay')) {
            paymentMatch = true;
        }
        

    }
    
    // 检查categories参数（兼容多种字段名）
    let categoryMatch = true;
    if (categoryParam) {
        const donationCategory = (donation.category_id || donation.CategoryID || donation.categories || donation.Categories || '').toString().trim();
        categoryMatch = donationCategory === categoryParam;
        

    }
    
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
        
        // 尝试多种时间格式解析
        if (timeStr) {
            // 首先尝试直接解析
            date = new Date(timeStr);
            
            // 如果解析失败，尝试其他格式
            if (isNaN(date.getTime())) {
                // 尝试处理时间戳格式（毫秒）
                const timestamp = parseInt(timeStr);
                if (!isNaN(timestamp)) {
                    // 检查是否是毫秒时间戳（长度大于10）
                    if (timeStr.length > 10) {
                        date = new Date(timestamp);
                    } else {
                        // 秒时间戳
                        date = new Date(timestamp * 1000);
                    }
                }
            }
        }
        
        // 如果所有尝试都失败，使用当前时间
        if (!date || isNaN(date.getTime())) {
            console.error('Invalid date format, using current time:', timeStr);
            date = new Date();
        }
        
        const formattedDate = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
        const formattedTime = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`;
        
        // 直接使用API返回的数据，不做任何处理
        const amount = donation.amount || donation.Amount || '0';
        console.log('Using API amount:', amount);
        
        const payment = donation.payment || donation.Payment || '';
        console.log('Using API payment:', payment);
        
        const blessing = donation.blessing || donation.Blessing || '';
        console.log('Using API blessing:', blessing);
        
        const avatarUrl = donation.avatar_url || donation.AvatarURL || './static/avatar.jpeg';
        console.log('Using API avatar_url:', avatarUrl);
        
        const userName = donation.user_name || donation.UserName || donation.username || donation.Username || '匿名施主';
        console.log('Using API user_name:', userName);
        
        // 构建HTML内容
        const paymentIcon = payment === 'wechat' ? '/static/wechat.png' : '/static/alipay.png';
        const paymentText = payment === 'wechat' ? '微信支付' : '支付宝';
        
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
        
        if (donationId && donationIds.has(donationId)) {
            console.log('Payment record already exists, skipping:', donationId);
            return;
        }
        
        if (donationId) {
            donationIds.add(donationId);
        }
        
        // 构建新的支付记录元素
        const meritItem = document.createElement('div');
        meritItem.className = 'merit-item';
        
        // 为新记录添加特殊背景色（浅红色）
        meritItem.style.backgroundColor = '#fff0f0';
        meritItem.style.transition = 'background-color 0.3s ease';
        
        // 格式化时间
        let date;
        let timeStr = data.created_at || data.CreatedAt || data.Time || '';
        
        // 尝试多种时间格式解析
        if (timeStr) {
            // 首先尝试直接解析
            date = new Date(timeStr);
            
            // 如果解析失败，尝试其他格式
            if (isNaN(date.getTime())) {
                // 尝试处理时间戳格式（毫秒）
                const timestamp = parseInt(timeStr);
                if (!isNaN(timestamp)) {
                    // 检查是否是毫秒时间戳（长度大于10）
                    if (timeStr.length > 10) {
                        date = new Date(timestamp);
                    } else {
                        // 秒时间戳
                        date = new Date(timestamp * 1000);
                    }
                }
            }
        }
        
        // 如果所有尝试都失败，使用当前时间
        if (!date || isNaN(date.getTime())) {
            console.error('Invalid date format, using current time:', timeStr);
            date = new Date();
        }
        
        const formattedDate = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
        const formattedTime = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`;
        
        // 确定支付方式图标和文本
        let payment = data.payment || '';
        // 直接使用API返回的支付方式，不做任何处理
        console.log('Using API payment:', payment);
        const paymentIcon = payment === 'wechat' ? '/static/wechat.png' : '/static/alipay.png';
        const paymentText = payment === 'wechat' ? '微信支付' : '支付宝';
        
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
        
        // 构建HTML内容（与现有样式保持一致）
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
        
        // 插入到数据列最前面
        if (rankingsList.children.length === 0 || (rankingsList.children[0].textContent && rankingsList.children[0].textContent.includes('暂无功德记录'))) {
            rankingsList.innerHTML = '';
            rankingsList.appendChild(meritItem);
        } else {
            rankingsList.insertBefore(meritItem, rankingsList.firstChild);
        }
        
        // 5秒钟后恢复与数据列表相同的背景色
        setTimeout(() => {
            meritItem.style.backgroundColor = '';
        }, 5000);
        
    } catch (error) {
        console.error('Error inserting new payment record:', error);
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
    if (!params.payment) {
        console.log('No payment parameter, skipping WebSocket connection');
        if (defaultImageContainer) {
            defaultImageContainer.style.display = 'flex';
        }
        // 没有payment参数，只初始化HTTP轮询和必要的功能
        initPolling();
        initLazyLoading();
        return;
    }
    
    console.log('Payment parameter found:', params.payment);
    
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