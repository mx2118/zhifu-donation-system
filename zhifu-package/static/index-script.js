// 从URL获取参数
function getURLParams() {
    const params = new URLSearchParams(window.location.search);
    return {
        payment: params.get('payment'),
        categories: params.get('categories')
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

// 加载分类导航
function loadCategories() {
    const params = getURLParams();
    const categories = dataCache.categories || [];
    
    const navbar = document.getElementById('categories-navbar');
    if (navbar) {
        navbar.innerHTML = '';
        
        // 添加全部分类选项
        const allItem = document.createElement('div');
        allItem.className = 'navbar-item';
        allItem.textContent = '全部分类';
        allItem.dataset.categoryId = '';
        allItem.addEventListener('click', function() {
            // 移除其他项的active类
            document.querySelectorAll('.navbar-item').forEach(item => {
                item.classList.remove('active');
            });
            // 添加当前项的active类
            this.classList.add('active');
            
            // 更新浏览器URL，移除categories参数
            const urlParams = new URLSearchParams(window.location.search);
            urlParams.set('payment', params.payment || '6');
            urlParams.delete('categories');
            
            const newUrl = window.location.pathname + '?' + urlParams.toString();
            window.history.pushState({}, '', newUrl);
            
            // 更新二维码链接
            updateQRCode();
            // 重新加载排行榜
            initLoadMore();
        });
        navbar.appendChild(allItem);
        
        // 添加各个分类选项
        categories.forEach((category, index) => {
            const item = document.createElement('div');
            // 检查是否是URL中指定的分类或默认分类
            if ((index === 0 && !params.categories) || (params.categories && category.id == params.categories)) {
                item.className = 'navbar-item active';
            } else {
                item.className = 'navbar-item';
            }
            item.textContent = category.name;
            item.dataset.categoryId = category.id;
            item.addEventListener('click', function() {
                // 移除其他项的active类
                document.querySelectorAll('.navbar-item').forEach(item => {
                    item.classList.remove('active');
                });
                // 添加当前项的active类
                this.classList.add('active');
                
                // 更新浏览器URL
                const categoryId = this.dataset.categoryId;
                const urlParams = new URLSearchParams(window.location.search);
                urlParams.set('payment', params.payment || '6');
                if (categoryId) {
                    urlParams.set('categories', categoryId);
                } else {
                    urlParams.delete('categories');
                }
                
                const newUrl = window.location.pathname + '?' + urlParams.toString();
                window.history.pushState({}, '', newUrl);
                
                // 更新二维码链接
                updateQRCode();
                // 重新加载排行榜
                initLoadMore();
            });
            navbar.appendChild(item);
        });
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

// 加载真实栏目列表到下拉菜单
function loadRealCategories() {
    const params = getURLParams();
    const payment = params.payment || '1';
    const currentCategory = params.categories || '';
    
    // 获取分类数据
    fetch(`/api/categories?payment=${payment}`)
        .then(response => {
            if (!response.ok) {
                throw new Error(`网络请求失败: ${response.status}`);
            }
            return response.json();
        })
        .then(categories => {
            // 存储分类数据到缓存
            dataCache.categories = categories;
            
            // 找到下拉菜单容器
            const dropdownContent = document.querySelector('.dropdown-content');
            const dropdownBtn = document.querySelector('.dropdown-btn');
            
            if (dropdownContent) {
                // 清空现有的菜单项
                dropdownContent.innerHTML = '';
                
                // 只添加真实的分类菜单项
                if (Array.isArray(categories) && categories.length > 0) {
                    categories.forEach(category => {
                        const categoryItem = document.createElement('a');
                        categoryItem.href = `/?payment=${payment}&categories=${category.id}`;
                        categoryItem.className = `dropdown-item ${currentCategory === category.id.toString() ? 'active' : ''}`;
                        categoryItem.textContent = category.name;
                        dropdownContent.appendChild(categoryItem);
                    });
                } else {
                    // 如果没有真实分类，添加一个默认的首页链接
                    const homeItem = document.createElement('a');
                    homeItem.href = `/?payment=${payment}`;
                    homeItem.className = `dropdown-item ${!currentCategory ? 'active' : ''}`;
                    homeItem.textContent = '首页';
                    dropdownContent.appendChild(homeItem);
                }
            }
            
            // 设置默认的下拉按钮文本
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
                        // 如果没有选择分类，显示第一个分类的名称
                        dropdownBtn.textContent = categories[0].name;
                    }
                } else {
                    // 如果没有真实分类，显示默认文本
                    dropdownBtn.textContent = '栏目列表';
                }
            }
            
            // 分类数据加载完成后，更新页面标题
            updatePageTitle();
        })
        .catch(error => {
            console.error('加载栏目列表失败:', error);
            // 即使加载失败，也不影响页面的其他功能
        });
}

// WebSocket连接管理
let ws;
let reconnectAttempts = 0;
const maxReconnectAttempts = 5;
const reconnectDelay = 3000;

// 连接WebSocket
function connectWebSocket() {
    try {
        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsURL = `${wsProtocol}//${window.location.host}/ws`;
        
        console.log('尝试连接WebSocket:', wsURL);
        
        ws = new WebSocket(wsURL);
        
        ws.onopen = function() {
            console.log('WebSocket连接已建立');
            reconnectAttempts = 0;
        };
        
        ws.onmessage = function(event) {
            try {
                const data = JSON.parse(event.data);
                handleWebSocketMessage(data);
            } catch (error) {
                console.warn('解析WebSocket消息失败:', error);
            }
        };
        
        ws.onclose = function(event) {
            console.log('WebSocket连接已关闭:', event.code, event.reason);
            // 只有在正常关闭之外的情况下才尝试重连
            if (event.code !== 1000) {
                attemptReconnect();
            }
        };
        
        ws.onerror = function(error) {
            // 减少错误信息的显示，使用warn而不是error
            console.warn('WebSocket错误:', error.message || error);
        };
    } catch (error) {
        console.warn('创建WebSocket连接失败:', error.message || error);
        attemptReconnect();
    }
}

// 尝试重连
function attemptReconnect() {
    if (reconnectAttempts < maxReconnectAttempts) {
        reconnectAttempts++;
        console.log(`尝试重连 (${reconnectAttempts}/${maxReconnectAttempts})...`);
        setTimeout(connectWebSocket, reconnectDelay);
    } else {
        console.warn('WebSocket重连失败，已达到最大尝试次数');
        console.log('WebSocket功能不可用，将使用定时刷新作为替代方案');
        // 启用定时刷新作为替代方案
        startPeriodicRefresh();
    }
}

// 定时刷新作为WebSocket的替代方案
function startPeriodicRefresh() {
    console.log('启动定时刷新（30秒一次）');
    setInterval(() => {
        console.log('执行定时刷新');
        initLoadMore();
    }, 30000);
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
            addNewDonation(data.donation);
            break;
            
        default:
            console.log('未知消息类型:', data.type);
    }
}

// 添加新的捐款记录到页面
function addNewDonation(donation) {
    const rankingsList = document.getElementById('rankings-list');
    if (!rankingsList) {
        console.error('未找到rankings-list元素');
        return;
    }
    
    try {
        // 检查是否有ID字段用于去重
        if (donation.id) {
            const donationId = donation.id.toString();
            if (donationIds.has(donationId)) {
                console.log('捐款记录已存在，跳过重复添加:', donationId);
                return;
            }
            // 添加到已存在的ID集合
            donationIds.add(donationId);
        } else {
            console.warn('捐款记录缺少ID字段，无法进行去重检查');
        }
        
        // 格式化时间显示
        const date = new Date(donation.created_at);
        const formattedDate = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
        const formattedTime = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`;
        
        // 创建新的功德项
        const meritItem = document.createElement('div');
        meritItem.className = 'merit-item';
        meritItem.innerHTML = `
            <div style="display: flex; align-items: center; justify-content: space-between; height: 36px;">
                <div class="merit-amount">¥${donation.amount.toFixed(2)}</div>
                <img src="${donation.payment === 'wechat' ? '/static/wechat.png' : '/static/alipay.png'}" alt="${donation.payment === 'wechat' ? '微信支付' : '支付宝'}" style="width: 24px; height: 24px; border-radius: 4px; vertical-align: middle;">
            </div>
            ${donation.blessing ? `<div style="font-size: 14px; color: #666; margin: 8px 0;">${donation.blessing}</div>` : ''}
            <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; margin-top: 8px;">
                <div style="display: flex; align-items: center; gap: 10px;">
                    <img src="${donation.avatar_url || './static/avatar.jpeg'}" alt="头像" style="width: 32px; height: 32px; border-radius: 8px;">
                    <span style="font-size: 14px; font-weight: bold;">${donation.user_name || '匿名施主'}</span>
                </div>
                <div class="merit-time">${formattedDate} ${formattedTime}</div>
            </div>
        `;
        
        // 添加到列表顶部
        rankingsList.insertBefore(meritItem, rankingsList.firstChild);
    } catch (error) {
        console.error('添加新捐款记录失败:', error);
    }
}

// 初始化WebSocket连接
function initWebSocket() {
    try {
        console.log('初始化WebSocket连接');
        connectWebSocket();
    } catch (error) {
        console.error('初始化WebSocket连接失败:', error);
        // 即使WebSocket连接失败，也不影响页面的其他功能
        // 直接启动定时刷新作为备用方案
        console.log('WebSocket初始化失败，直接启动定时刷新');
        startPeriodicRefresh();
    }
}

// 初始加载
function init() {
    // 检查URL参数
    const params = getURLParams();
    
    // 处理默认图片容器
    const defaultImageContainer = document.getElementById('default-image-container');
    if (!params.payment) {
        if (defaultImageContainer) {
            defaultImageContainer.style.display = 'flex';
        }
        // 没有payment参数，直接返回，不执行初始化操作
        return;
    }
    
    // 有payment参数，确保默认图片容器隐藏
    if (defaultImageContainer) {
        defaultImageContainer.style.display = 'none';
    }
    
    // 立即初始化模态窗口，让页面快速显示
    initModal();
    
    // 加载真实栏目列表
    loadRealCategories();
    
    // 异步加载配置数据，不阻塞页面显示
    fetchConfigData().then(() => {
        // 配置数据加载完成后，更新页面标题、二维码和其他标题
        updatePageTitle();
        updateTitles();
        updateQRCode();
    }).catch(error => {
        console.error('加载配置数据失败:', error);
        // 即使失败也更新页面标题，使用默认值
        updatePageTitle();
        updateTitles();
    });
    
    // 异步加载排名数据，不阻塞页面显示
    initLoadMore();
    
    // 异步初始化WebSocket连接，不阻塞页面显示
    initWebSocket();
}

// 初始加载
init();
