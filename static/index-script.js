// 从URL获取参数
function getURLParams() {
    const params = new URLSearchParams(window.location.search);
    return {
        payment: params.get('payment') || '',
        categories: params.get('categories') || ''
    };
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
        console.error('获取支付配置失败:', error);
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
            console.error('获取支付配置失败:', error);
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
                console.error('获取分类失败:', error);
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
            console.error('获取配置数据失败:', error);
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
        newTitle = '聖爱安养院 功德榜';
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
        
        console.log('加载排行榜数据，URL:', url);
        console.log('加载排行榜数据，参数:', params);
        
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`网络请求失败: ${response.status}`);
        }
        const data = await response.json();
        console.log('加载排行榜数据，返回:', data);
        
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
        console.error('Error loading rankings:', error);
        
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
        
        console.log('滚动事件触发: scrollTop =', scrollTop, 'scrollHeight =', scrollHeight, 'clientHeight =', clientHeight);
        console.log('滚动事件触发: isLoading =', isLoading, 'hasMoreData =', hasMoreData);
        console.log('滚动事件触发: lastScrollHeight =', lastScrollHeight);
        
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

// WebSocket连接管理
let ws;
let reconnectAttempts = 0;
const maxReconnectAttempts = 99999; // 无限重连
const reconnectDelay = 2000;
let wsConnected = false;
let heartbeatInterval;
const heartbeatIntervalTime = 30000; // 30秒发送一次心跳

// ========== 修改1：向服务端发送客户端参数 ==========
function sendClientParamsToServer() {
    if (ws && ws.readyState === WebSocket.OPEN) {
        const params = getURLParams();
        const paramsMsg = {
            type: 'client_params',
            payment: params.payment,
            categories: params.categories
        };
        ws.send(JSON.stringify(paramsMsg));
        console.log('向服务端发送客户端参数:', paramsMsg);
    }
}

// 连接WebSocket
function connectWebSocket() {
    try {
        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsURL = `${wsProtocol}//${window.location.host}/ws`;
        
        console.log('尝试连接WebSocket:', wsURL);
        
        // 清除之前的连接
        if (ws) {
            try {
                ws.close();
            } catch (e) {
                console.warn('关闭之前的WebSocket连接失败:', e);
            }
        }
        
        // 清除之前的心跳定时器
        if (heartbeatInterval) {
            clearInterval(heartbeatInterval);
        }
        
        ws = new WebSocket(wsURL);
        
        ws.onopen = function() {
            console.log('WebSocket连接已建立');
            reconnectAttempts = 0;
            wsConnected = true;
            // ========== 修改2：连接成功后立即发送客户端参数 ==========
            sendClientParamsToServer();
            console.log('WebSocket连接成功，等待接收实时捐款记录');
            
            // 启动心跳机制
            startHeartbeat();
        };
        
        ws.onmessage = function(event) {
            console.log('收到WebSocket消息:', event.data);
            try {
                const data = JSON.parse(event.data);
                console.log('解析后的消息数据:', data);
                handleWebSocketMessage(data);
            } catch (error) {
                // ========== 修改3：生产环境也打印完整错误日志 ==========
                console.error('解析WebSocket消息失败:', error);
                console.error('原始消息:', event.data);
            }
        };
        
        ws.onclose = function(event) {
            console.log('WebSocket连接已关闭:', event.code, event.reason);
            wsConnected = false;
            
            // 清除心跳定时器
            if (heartbeatInterval) {
                clearInterval(heartbeatInterval);
            }
            
            // 所有情况下都尝试重连，包括正常关闭
            attemptReconnect();
        };
        
        ws.onerror = function(error) {
            // ========== 修改4：生产环境也打印完整错误日志 ==========
            console.error('WebSocket错误:', error);
            
            // 清除心跳定时器
            if (heartbeatInterval) {
                clearInterval(heartbeatInterval);
            }
            
            // 自动尝试重连
            attemptReconnect();
        };
    } catch (error) {
        // ========== 修改5：生产环境也打印完整错误日志 ==========
        console.error('创建WebSocket连接失败:', error);
        
        // 清除心跳定时器
        if (heartbeatInterval) {
            clearInterval(heartbeatInterval);
        }
        
        attemptReconnect();
    }
}

// 启动心跳机制
function startHeartbeat() {
    // 清除之前的定时器
    if (heartbeatInterval) {
        clearInterval(heartbeatInterval);
    }
    
    // 每30秒发送一次心跳
    heartbeatInterval = setInterval(function() {
        if (ws && ws.readyState === WebSocket.OPEN) {
            try {
                ws.send(JSON.stringify({ type: 'heartbeat' }));
                console.log('发送WebSocket心跳');
            } catch (error) {
                console.error('发送心跳失败:', error);
                // 发送失败，可能连接已断开，尝试重连
                if (heartbeatInterval) {
                    clearInterval(heartbeatInterval);
                }
                attemptReconnect();
            }
        }
    }, heartbeatIntervalTime);
}

// 尝试重连
function attemptReconnect() {
    reconnectAttempts++;
    console.log(`尝试重连 (${reconnectAttempts})...`);
    // 使用指数退避策略，增加重连延迟
    const delay = reconnectDelay * Math.min(Math.pow(1.5, reconnectAttempts - 1), 30000);
    setTimeout(connectWebSocket, delay);
}

// 处理WebSocket消息
function handleWebSocketMessage(data) {
    switch (data.type) {
        case 'initial_data':
            // 处理初始数据（如果需要）
            console.log('收到初始数据:', data.rankings.length, '条记录');
            break;
            
        case 'new_donation':
            // 处理新的捐款记录
            console.log('收到新的捐款记录:', data.donation);
            
            // 检查捐款记录是否与当前页面参数匹配
            const params = getURLParams();
            if (checkDonationMatch(data.donation, params)) {
                console.log('捐款记录匹配当前页面参数，添加到页面');
                addNewDonation(data.donation);
            } else {
                // ========== 修改6：打印详细的不匹配原因 ==========
                console.error('捐款记录不匹配当前页面参数，跳过添加');
                console.error('当前页面参数:', params);
                console.error('捐款记录参数:', {
                    payment: data.donation.payment || data.donation.Payment,
                    payment_config_id: data.donation.payment_config_id || data.donation.PaymentConfigID,
                    category_id: data.donation.category_id || data.donation.CategoryID,
                    categories: data.donation.categories || data.donation.Categories
                });
            }
            break;
            
        default:
            console.log('未知消息类型:', data.type);
    }
}

// ========== 修改7：优化参数匹配逻辑（兼容更多字段） ==========
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
        paymentMatch = (donationPayment === paymentParam) || 
                       (donationPaymentText === 'wechat' && paymentParam === '2') || 
                       (donationPaymentText === 'alipay' && paymentParam === '1');
        
        if (!paymentMatch) {
            console.error('Payment不匹配:', donationPayment, '/', donationPaymentText, '!==', paymentParam);
        }
    }
    
    // 检查categories参数（兼容多种字段名）
    let categoryMatch = true;
    if (categoryParam) {
        const donationCategory = (donation.category_id || donation.CategoryID || donation.categories || donation.Categories || '').toString().trim();
        categoryMatch = donationCategory === categoryParam;
        
        if (!categoryMatch) {
            console.error('Categories不匹配:', donationCategory, '!==', categoryParam);
        }
    }
    
    return paymentMatch && categoryMatch;
}

// 添加新的捐款记录到页面
function addNewDonation(donation) {
    console.log('====================================');
    console.log('开始添加新捐款记录');
    console.log('当前时间:', new Date().toISOString());
    console.log('捐款记录数据:', donation);
    console.log('====================================');
    
    const rankingsList = document.getElementById('rankings-list');
    if (!rankingsList) {
        console.error('未找到rankings-list元素');
        return;
    }
    
    try {
        // 兼容处理：获取ID字段（支持驼峰和蛇形命名）
        const donationId = (donation.id || donation.ID || '').toString().trim();
        if (donationId) {
            if (donationIds.has(donationId)) {
                console.log('捐款记录已存在，跳过重复添加:', donationId);
                return;
            }
            // 添加到已存在的ID集合
            donationIds.add(donationId);
            console.log('捐款记录ID已添加到去重集合:', donationId);
        } else {
            console.warn('捐款记录缺少ID字段，无法进行去重检查');
        }
        
        // 兼容处理：获取时间字段（支持驼峰和蛇形命名，处理不同格式）
        let date;
        const createdAt = donation.created_at || donation.CreatedAt;
        if (createdAt) {
            if (typeof createdAt === 'string') {
                date = new Date(createdAt);
            } else if (createdAt instanceof Date) {
                date = createdAt;
            } else {
                // 尝试其他时间格式
                date = new Date(createdAt);
            }
        } else {
            date = new Date();
        }
        const formattedDate = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
        const formattedTime = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`;
        console.log('捐款时间:', formattedDate, formattedTime);
        
        // 创建新的功德项
        const meritItem = document.createElement('div');
        meritItem.className = 'merit-item';
        
        // 兼容处理：获取其他字段（支持驼峰和蛇形命名）
        const amount = donation.amount || donation.Amount || 0;
        const payment = donation.payment || donation.Payment || '';
        const blessing = donation.blessing || donation.Blessing || '';
        const avatarUrl = donation.avatar_url || donation.AvatarURL || './static/avatar.jpeg';
        const userName = donation.user_name || donation.UserName || '匿名施主';
        
        // 构建HTML内容
        const paymentIcon = payment === 'wechat' ? '/static/wechat.png' : '/static/alipay.png';
        const paymentText = payment === 'wechat' ? '微信支付' : '支付宝';
        
        console.log('构建功德项HTML，金额: ¥' + amount.toFixed(2) + ', 支付方式: ' + paymentText + ', 用户名: ' + userName);
        
        meritItem.innerHTML = `
            <div style="display: flex; align-items: center; justify-content: space-between; height: 36px;">
                <div class="merit-amount">¥${amount.toFixed(2)}</div>
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
        
        // ========== 修改8：如果列表为空（无初始数据），直接替换而非插入顶部 ==========
        if (rankingsList.children.length === 0 || (rankingsList.children[0].textContent && rankingsList.children[0].textContent.includes('暂无功德记录'))) {
            rankingsList.innerHTML = '';
            rankingsList.appendChild(meritItem);
        } else {
            // 添加到列表顶部
            rankingsList.insertBefore(meritItem, rankingsList.firstChild);
        }
        console.log('功德项添加成功！');
    } catch (error) {
        console.error('添加新捐款记录失败:', error);
        console.error('错误堆栈:', error.stack);
    }
}

// 初始化WebSocket连接
function initWebSocket() {
    try {
        console.log('====================================');
        console.log('开始初始化WebSocket连接');
        console.log('当前时间:', new Date().toISOString());
        console.log('当前页面参数:', getURLParams());
        console.log('====================================');
        connectWebSocket();
    } catch (error) {
        console.error('初始化WebSocket连接失败:', error);
        // 即使初始化失败，也会在connectWebSocket中自动尝试重连
    }
}

// 初始加载
function init() {
    // 检查URL参数
    const params = getURLParams();
    console.log('页面初始化，URL参数:', params);
    
    // 处理默认图片容器
    const defaultImageContainer = document.getElementById('default-image-container');
    if (!params.payment) {
        if (defaultImageContainer) {
            defaultImageContainer.style.display = 'flex';
        }
        // 没有payment参数，只初始化WebSocket连接和必要的功能
        initWebSocket();
        initLazyLoading();
        return;
    }
    
    // 有payment参数，确保默认图片容器隐藏
    if (defaultImageContainer) {
        defaultImageContainer.style.display = 'none';
    }
    
    // 立即初始化模态窗口，让页面快速显示
    initModal();
    
    // ========== 修改9：优先初始化WebSocket，避免错过早期广播 ==========
    initWebSocket();
    
    // 异步加载配置数据（包含分类数据和下拉菜单构建），不阻塞页面显示
    fetchConfigData().then(() => {
        // 配置数据加载完成后，更新页面标题、二维码和其他标题
        updatePageTitle();
        updateTitles();
        updateQRCode();
        // 配置加载完成后，重新发送参数给服务端（防止参数更新）
        sendClientParamsToServer();
    }).catch(error => {
        console.error('加载配置数据失败:', error);
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