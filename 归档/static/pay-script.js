// 快速金额选择（仅作为快捷选项，不设置默认值）
function initQuickAmounts(elements) {
    // 使用事件委托，减少内存占用
    if (elements.quickAmountsContainer) {
        elements.quickAmountsContainer.addEventListener('click', (e) => {
            if (e.target.classList.contains('quick-amount')) {
                elements.amountInput.value = e.target.dataset.amount;
            }
        });
    }
}

// 祝福语输入框折叠功能
function initBlessingToggle(elements) {
    if (elements.toggleBtn && elements.blessingContainer) {
        elements.toggleBtn.addEventListener('click', function() {
            const container = elements.blessingContainer;
            const button = this;
            
            // 检查当前显示状态
            if (container.style.display === 'none' || container.style.display === '') {
                container.style.display = 'block';
                button.textContent = '- 收起祝福语';
            } else {
                container.style.display = 'none';
                button.textContent = '+ 祝福语（选填）';
            }
        });
    }
}

// 防抖函数
function debounce(func, wait) {
    let timeout;
    return function() {
        const context = this;
        const args = arguments;
        clearTimeout(timeout);
        timeout = setTimeout(() => {
            func.apply(context, args);
        }, wait);
    };
}

// 祝福语字数限制
function initBlessingCounter(elements) {
    const MAX_BLESSING_LENGTH = 50;
    if (elements.blessingTextarea && elements.blessingCounter) {
        // 添加防抖处理，避免频繁触发事件
        const handleInput = debounce(function() {
            const currentLength = this.value.length;
            
            if (currentLength > MAX_BLESSING_LENGTH) {
                this.value = this.value.substring(0, MAX_BLESSING_LENGTH);
                elements.blessingCounter.textContent = `${MAX_BLESSING_LENGTH}/${MAX_BLESSING_LENGTH}`;
                elements.blessingCounter.style.color = '#e74c3c';
            } else {
                elements.blessingCounter.textContent = `${currentLength}/${MAX_BLESSING_LENGTH}`;
                elements.blessingCounter.style.color = currentLength >= MAX_BLESSING_LENGTH ? '#e74c3c' : '#666';
            }
        }, 200);
        
        elements.blessingTextarea.addEventListener('input', handleInput);
    }
}

// 自动检测支付方式
let selectedPayment = 'wechat'; // 默认支付方式

// 前端缓存管理
const FrontendCache = {
    // 缓存键前缀
    prefix: 'zhifu_',
    
    // 缓存过期时间（5分钟）
    expiry: 5 * 60 * 1000,
    
    // 设置缓存
    set(key, value) {
        try {
            const item = {
                value: value,
                expiry: Date.now() + this.expiry
            };
            localStorage.setItem(this.prefix + key, JSON.stringify(item));
        } catch (error) {
            console.error('设置缓存失败:', error);
        }
    },
    
    // 获取缓存
    get(key) {
        try {
            const itemStr = localStorage.getItem(this.prefix + key);
            if (!itemStr) {
                return null;
            }
            
            const item = JSON.parse(itemStr);
            if (Date.now() > item.expiry) {
                localStorage.removeItem(this.prefix + key);
                return null;
            }
            
            return item.value;
        } catch (error) {
            console.error('获取缓存失败:', error);
            return null;
        }
    },
    
    // 清除缓存
    clear(key) {
        try {
            if (key) {
                localStorage.removeItem(this.prefix + key);
            } else {
                // 清除所有本应用的缓存
                for (let i = 0; i < localStorage.length; i++) {
                    const storageKey = localStorage.key(i);
                    if (storageKey && storageKey.startsWith(this.prefix)) {
                        localStorage.removeItem(storageKey);
                    }
                }
            }
        } catch (error) {
            console.error('清除缓存失败:', error);
        }
    }
};

// 标题更新的回退方法
function fallbackTitleUpdate(newTitle) {
    // 创建一个临时元素来触发浏览器的重绘
    const tempEl = document.createElement('div');
    tempEl.style.position = 'absolute';
    tempEl.style.left = '-9999px';
    tempEl.textContent = newTitle;
    document.body.appendChild(tempEl);
    setTimeout(() => {
        document.body.removeChild(tempEl);
        // 再次设置标题，确保生效
        document.title = newTitle;
    }, 100);
}

function detectPaymentMethod() {
    const userAgent = navigator.userAgent.toLowerCase();
    if (userAgent.match(/micromessenger\/[\d\.]+/)) {
        selectedPayment = 'wechat';
    } else if (userAgent.match(/alipayclient\/[\d\.]+/)) {
        selectedPayment = 'alipay';
    } else {
        selectedPayment = 'wechat'; // 默认使用微信支付
    }
}

// 从URL获取参数
function getURLParams() {
	const params = new URLSearchParams(window.location.search);
	return {
		payment: params.get('payment'),
		categories: params.get('categories')
	};
}

// 获取支付配置信息
async function getPaymentConfig(paymentConfigId) {
    if (!paymentConfigId) {
        return null;
    }
    
    // 尝试从缓存获取
    const cacheKey = `payment_config_${paymentConfigId}`;
    const cachedData = FrontendCache.get(cacheKey);
    if (cachedData) {
        return cachedData;
    }
    
    try {
        const response = await fetch(`/api/payment-config/${paymentConfigId}`);
        if (!response.ok) {
            return null;
        }
        const data = await response.json();
        // 存入缓存
        FrontendCache.set(cacheKey, data);
        return data;
    } catch (error) {
        return null;
    }
}

// 获取类目信息
async function getCategory(categoryId) {
    if (!categoryId) {
        return null;
    }
    
    // 尝试从缓存获取
    const cacheKey = `category_${categoryId}`;
    const cachedData = FrontendCache.get(cacheKey);
    if (cachedData) {
        return cachedData;
    }
    
    try {
        const response = await fetch(`/api/category/${categoryId}`);
        if (!response.ok) {
            return null;
        }
        const data = await response.json();
        // 存入缓存
        FrontendCache.set(cacheKey, data);
        return data;
    } catch (error) {
        return null;
    }
}

// 初始化页面，处理URL参数
async function initPageWithParams() {
    const params = getURLParams();
    
    // 异步加载额外信息，不阻塞页面显示
    await loadExtraContent(params);
}



// 异步加载额外内容
async function loadExtraContent(params) {
    let merchantName = '';
    let categoryName = '';
    
    // 并行请求，提高加载速度
    const promises = [];
    
    if (params.payment) {
        promises.push(
            getPaymentConfig(params.payment).catch(error => {
                return null;
            })
        );
    }
    
    if (params.categories) {
        promises.push(
            getCategory(params.categories).catch(error => {
                return null;
            })
        );
    }
    
    // 等待所有请求完成
    if (promises.length > 0) {
        try {
            const results = await Promise.all(promises);
            
            // 处理结果
            for (const result of results) {
                if (result) {
                    // 检查是否是支付配置
                    if (result.store_name) {
                        merchantName = result.store_name;
                    }
                    // 检查是否是类目信息
                    if (result.name) {
                        categoryName = result.name;
                        // 更新金额标题
                        if (DOMCache.amountTitle) {
                            DOMCache.amountTitle.textContent = `${result.name} 金额`;
                        }
                    }
                }
            }
        } catch (error) {
            // 忽略错误，使用默认值
        }
    }
    
    // 构建完整标题
    let newTitle = '';
    if (merchantName && categoryName) {
        newTitle = `${merchantName} ${categoryName} 功德金支付`;
    } else if (merchantName) {
        newTitle = `${merchantName} 功德金支付`;
    } else if (categoryName) {
        newTitle = `${categoryName} 功德金支付`;
    } else {
        newTitle = '无量福报积善功德榜';
    }
    
    // 更新页面标题
    document.title = newTitle;
    
    // 为Alipay浏览器添加特殊处理，确保标题更新生效
    if (navigator.userAgent.toLowerCase().includes('alipayclient')) {
        // 尝试使用支付宝小程序原生API my.setNavigationBar() 实时调整标题
        if (typeof my !== 'undefined' && my.setNavigationBar) {
            try {
                my.setNavigationBar({
                    title: newTitle,
                    success: function() {
                        console.log('支付宝标题设置成功:', newTitle);
                    },
                    fail: function(error) {
                        console.error('支付宝标题设置失败:', error);
                        // 失败时回退到传统方法
                        fallbackTitleUpdate(newTitle);
                    }
                });
            } catch (error) {
                console.error('调用my.setNavigationBar失败:', error);
                // 出错时回退到传统方法
                fallbackTitleUpdate(newTitle);
            }
        } else {
            // 没有my.setNavigationBar API时，使用传统方法
            fallbackTitleUpdate(newTitle);
        }
    }

    
    // 更新h1元素
    if (DOMCache.h1Element) {
        DOMCache.h1Element.textContent = newTitle;
    }
}

// 提交捐款
function initSubmitButton(elements) {
    if (elements.submitBtn) {
        // 缓存额外的DOM元素
        const form = document.getElementById('pay-form');
        const formAmount = document.getElementById('form-amount');
        const formPayment = document.getElementById('form-payment');
        const formCategory = document.getElementById('form-category');
        const formBlessing = document.getElementById('form-blessing');
        const formPaymentConfigId = document.getElementById('form-payment-config-id');
        
        elements.submitBtn.addEventListener('click', async (e) => {
            e.preventDefault(); // 阻止默认行为
            
            const amount = parseFloat(elements.amountInput.value);
            if (isNaN(amount) || amount <= 0) {
                alert('请输入有效的捐款金额');
                return;
            }
            
            // 从URL获取参数
            const params = getURLParams();
            
            // 显示加载状态
            const originalText = elements.submitBtn.textContent;
            elements.submitBtn.textContent = '正在处理支付...';
            elements.submitBtn.disabled = true;
            
            try {
                // 使用表单提交实现302重定向
                formAmount.value = amount;
                formPayment.value = selectedPayment;
                formCategory.value = params.categories || '';
                formBlessing.value = elements.blessingTextarea.value || '';
                formPaymentConfigId.value = params.payment || '';
                
                // 提交表单，浏览器会处理302重定向
                form.submit();
            } catch (error) {
                // 恢复按钮状态
                elements.submitBtn.textContent = originalText;
                elements.submitBtn.disabled = false;
            }
        });
    }
}

// 更新返回链接，添加当前页面的参数
function updateBackLink(elements) {
    const params = getURLParams();
    if (elements.backLink) {
        let url = '/';
        
        // 添加参数
        if (params.payment) {
            url += `?payment=${params.payment}`;
            if (params.categories) {
                url += `&categories=${params.categories}`;
            }
        } else if (params.categories) {
            url += `?categories=${params.categories}`;
        }
        
        elements.backLink.href = url;
    }
}

// 读取cookie的函数
function getCookie(name) {
    const value = `; ${document.cookie}`;
    const parts = value.split(`; ${name}=`);
    if (parts.length === 2) {
        return parts.pop().split(';').shift();
    }
    return null;
}

// 设置cookie的函数
function setCookie(name, value, days) {
    let expires = '';
    if (days) {
        const date = new Date();
        date.setTime(date.getTime() + (days * 24 * 60 * 60 * 1000));
        expires = '; expires=' + date.toUTCString();
    }
    document.cookie = name + '=' + (value || '') + expires + '; path=/;';
}



// 更新用户时间信息
function updateUserTime(elements) {
    const userTimeElement = elements ? elements.userTime : DOMCache.userTime;
    if (!userTimeElement) return;
    
    // 获取当前时间
    const now = new Date();
    const year = now.getFullYear();
    const month = (now.getMonth() + 1).toString().padStart(2, '0');
    const day = now.getDate().toString().padStart(2, '0');
    const hours = now.getHours().toString().padStart(2, '0');
    const minutes = now.getMinutes().toString().padStart(2, '0');
    const seconds = now.getSeconds().toString().padStart(2, '0');
    
    // 构建时间字符串
    const timeString = `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
    
    // 只有当时间字符串变化时才更新DOM，减少不必要的重绘
    if (userTimeElement.textContent !== timeString) {
        userTimeElement.textContent = timeString;
    }
}

// 启动时间更新
function startTimeUpdate(elements) {
    // 立即更新一次
    updateUserTime(elements);
    
    // 每隔1秒更新一次
    setInterval(() => updateUserTime(elements), 1000);
}

// 缓存DOM元素
const DOMCache = {
    userAvatar: null,
    userNickname: null,
    unauthorizeLink: null,
    userTime: null,
    amountInput: null,
    submitBtn: null,
    blessingTextarea: null,
    blessingCounter: null,
    quickAmountsContainer: null,
    toggleBtn: null,
    blessingContainer: null,
    backLink: null,
    amountTitle: null,
    h1Element: null,
    
    // 初始化缓存
    init() {
        this.userAvatar = document.getElementById('user-avatar');
        this.userNickname = document.getElementById('user-nickname');
        this.unauthorizeLink = document.getElementById('unauthorize-link');
        this.userTime = document.getElementById('user-time');
        this.amountInput = document.getElementById('amount');
        this.submitBtn = document.getElementById('submit-btn');
        this.blessingTextarea = document.getElementById('blessing');
        this.blessingCounter = document.getElementById('blessing-counter');
        this.quickAmountsContainer = document.querySelector('.quick-amounts');
        this.toggleBtn = document.getElementById('toggle-blessing');
        this.blessingContainer = document.getElementById('blessing-container');
        this.backLink = document.getElementById('back-to-ranking');
        this.amountTitle = document.getElementById('amount-title');
        this.h1Element = document.querySelector('h1');
        
        // 输出DOM元素初始化状态
        console.log('DOMCache初始化状态:', {
            userAvatar: this.userAvatar ? '找到' : '未找到',
            userNickname: this.userNickname ? '找到' : '未找到'
        });
    }
};

// 显示用户信息
function displayUserInfo(paymentType, userID, nickname, avatar, authURL, userType) {
    if (!DOMCache.userAvatar || !DOMCache.userNickname) {
        return;
    }
    
    // 显示用户昵称
    if (nickname && nickname !== '匿名施主' && nickname.trim() !== '') {
        try {
            // 尝试解码昵称，因为cookie中的值可能是编码过的
            let decodedNickname = nickname;
            if (nickname.includes('%')) {
                decodedNickname = decodeURIComponent(nickname);
            }
            DOMCache.userNickname.textContent = decodedNickname;
        } catch (e) {
            DOMCache.userNickname.textContent = nickname;
        }
        // 显示取消授权链接
        if (DOMCache.unauthorizeLink) {
            DOMCache.unauthorizeLink.style.display = 'block';
        }
    } else {
        // 构建授权链接，包含当前页面的重定向参数
        const currentURL = window.location.href;
        const fullAuthURL = window.location.origin + authURL + '?redirect_url=' + encodeURIComponent(currentURL);
        const buttonColor = paymentType === 'wechat' ? '#8b0000' : '#1677ff';
        DOMCache.userNickname.innerHTML = '未授权："匿名施主" </br><a href="' + fullAuthURL + '" style="color: ' + buttonColor + '; text-decoration: none; font-size: 14px; font-weight: bold;">点击这里获取' + userType + '授权</a>';
        // 隐藏取消授权链接
        if (DOMCache.unauthorizeLink) {
            DOMCache.unauthorizeLink.style.display = 'none';
        }
    }
    
    // 显示用户头像
    if (nickname && nickname !== '匿名施主' && nickname.trim() !== '') {
        // 用户已授权，使用存储的头像
        console.log('用户已授权，处理头像:', {
            avatar: avatar,
            isDefault: avatar === './static/avatar.jpeg'
        });
        
        if (avatar && avatar !== './static/avatar.jpeg') {
            // 确保头像URL是完整的
            let avatarURL = avatar;
            console.log('原始头像URL:', avatarURL);
            
            try {
                // 解码URL，因为cookie中的值可能是编码过的
                if (avatarURL.includes('%')) {
                    avatarURL = decodeURIComponent(avatarURL);
                    console.log('解码后头像URL:', avatarURL);
                }
            } catch (e) {
                // 忽略错误，使用原始URL
                console.error('解码头像URL失败:', e);
            }
            
            // 移除可能的额外引号
            avatarURL = avatarURL.replace(/^["']|["']$/g, '');
            console.log('清理后头像URL:', avatarURL);
            
            // 确保头像URL是有效的
            if (avatarURL && (avatarURL.startsWith('http://') || avatarURL.startsWith('https://'))) {
                console.log('设置头像:', avatarURL);
                // 同时更新src和data-src，确保懒加载不会覆盖
                DOMCache.userAvatar.src = avatarURL;
                DOMCache.userAvatar.dataset.src = avatarURL;
            } else {
                console.log('头像URL无效，使用默认头像:', avatarURL);
                DOMCache.userAvatar.src = './static/avatar.jpeg';
                DOMCache.userAvatar.dataset.src = './static/avatar.jpeg';
            }
        } else {
            console.log('使用默认头像，因为avatar为空或为默认值:', avatar);
            DOMCache.userAvatar.src = './static/avatar.jpeg';
        }
    } else {
        console.log('用户未授权，使用默认头像');
        DOMCache.userAvatar.src = './static/avatar.jpeg';
    }
    
    // 更新用户时间
    updateUserTime();
}

// 显示当前获得的openid
function displayOpenID() {
    // 尝试从cookie中读取微信openid
    const wechatOpenid = getCookie('wechat_openid');
    const wechatNickname = getCookie('wechat_user_name');
    const wechatAvatar = getCookie('wechat_avatar_url');
    
    console.log('微信用户信息:', {
        openid: wechatOpenid,
        nickname: wechatNickname,
        avatar: wechatAvatar
    });
    
    if (wechatOpenid && wechatOpenid.trim() !== '') {
        // 显示微信用户信息
        console.log('使用微信用户信息');
        displayUserInfo('wechat', wechatOpenid, wechatNickname, wechatAvatar, '/api/wechat/auth', '微信用户');
        return;
    }
    
    // 尝试从cookie中读取支付宝user_id
    const alipayUserId = getCookie('alipay_user_id');
    const alipayNickname = getCookie('alipay_user_name');
    const alipayAvatar = getCookie('alipay_avatar_url');
    
    console.log('支付宝用户信息:', {
        user_id: alipayUserId,
        nickname: alipayNickname,
        avatar: alipayAvatar
    });
    
    if (alipayUserId && alipayUserId.trim() !== '') {
        // 显示支付宝用户信息
        console.log('使用支付宝用户信息');
        displayUserInfo('alipay', alipayUserId, alipayNickname, alipayAvatar, '/api/alipay/auth', '支付宝用户');
        return;
    }
    
    // 如果都没有，显示未获取到并显示授权链接
    
    // 构建授权链接，包含当前页面的重定向参数
    const currentURL = window.location.href;
    
    // 检测当前环境
    const userAgent = navigator.userAgent.toLowerCase();
    
    // 更新用户信息显示
    const userNickname = document.getElementById('user-nickname');
    const userAvatar = document.getElementById('user-avatar');
    const unauthorizeLink = document.getElementById('unauthorize-link');
    if (userNickname) {
        if (userAgent.includes('micromessenger')) {
            // 微信环境，显示微信授权链接
            const wechatAuthURL = window.location.origin + '/api/wechat/auth?redirect_url=' + encodeURIComponent(currentURL);
            userNickname.innerHTML = '未授权："匿名施主" </br><a href="' + wechatAuthURL + '" style="color: #8b0000; text-decoration: none; font-size: 14px; font-weight: bold;">点击这里获取微信授权</a>';
        } else if (userAgent.includes('alipayclient')) {
            // 支付宝环境，显示支付宝授权链接
            const alipayAuthURL = window.location.origin + '/api/alipay/auth?redirect_url=' + encodeURIComponent(currentURL);
            userNickname.innerHTML = '未授权："匿名施主" </br><a href="' + alipayAuthURL + '" style="color: #1677ff; text-decoration: none; font-size: 14px; font-weight: bold;">点击这里获取支付宝授权</a>';
        } else {
            // 其他环境，同时显示微信和支付宝授权链接
            const wechatAuthURL = window.location.origin + '/api/wechat/auth?redirect_url=' + encodeURIComponent(currentURL);
            const alipayAuthURL = window.location.origin + '/api/alipay/auth?redirect_url=' + encodeURIComponent(currentURL);
            userNickname.innerHTML = '未授权："匿名施主" </br>' +
                '<a href="' + wechatAuthURL + '" style="color: #8b0000; text-decoration: none; font-size: 14px; font-weight: bold; margin-right: 10px;">微信授权</a>' +
                '<a href="' + alipayAuthURL + '" style="color: #1677ff; text-decoration: none; font-size: 14px; font-weight: bold;">支付宝授权</a>';
        }
        // 隐藏取消授权链接
        if (unauthorizeLink) {
            unauthorizeLink.style.display = 'none';
        }
    }
    
    // 更新用户头像为默认头像
    if (userAvatar) {
        userAvatar.src = './static/avatar.jpeg';
    }
    
    // 更新用户时间
    updateUserTime();
}

// 取消授权功能
function cancelAuthorization() {
    // 清除微信用户信息cookie
    setCookie('wechat_openid', '', -1);
    setCookie('wechat_user_name', '', -1);
    setCookie('wechat_avatar_url', '', -1);
    
    // 清除支付宝用户信息cookie
    setCookie('alipay_user_id', '', -1);
    setCookie('alipay_user_name', '', -1);
    setCookie('alipay_avatar_url', '', -1);
    
    // 重新显示用户信息，调用 displayOpenID() 以显示授权链接
    displayOpenID();
    
    // 显示提示信息
    alert('已成功取消授权，变回匿名施主');
}



// 页面加载完成后初始化
async function initPage() {
    // 初始化DOM缓存
    DOMCache.init();
    
    // 自动检测支付方式
    detectPaymentMethod();
    
    // 初始化页面参数
    await initPageWithParams();
    
    // 批量初始化其他功能
    initPageComponents();
    
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

// 初始化页面组件
function initPageComponents() {
    // 使用DOMCache中的元素引用
    const elements = {
        quickAmountsContainer: DOMCache.quickAmountsContainer,
        toggleBtn: DOMCache.toggleBtn,
        blessingContainer: DOMCache.blessingContainer,
        blessingTextarea: DOMCache.blessingTextarea,
        blessingCounter: DOMCache.blessingCounter,
        amountInput: DOMCache.amountInput,
        backLink: DOMCache.backLink,
        submitBtn: DOMCache.submitBtn,
        userAvatar: DOMCache.userAvatar,
        userNickname: DOMCache.userNickname,
        unauthorizeLink: DOMCache.unauthorizeLink,
        userTime: DOMCache.userTime
    };
    
    // 初始化快速金额选择
    initQuickAmounts(elements);
    
    // 初始化祝福语切换
    initBlessingToggle(elements);
    
    // 初始化祝福语计数器
    initBlessingCounter(elements);
    
    // 更新返回链接
    updateBackLink(elements);
    
    // 执行displayOpenID
    displayOpenID();
    
    // 启动时间更新
    startTimeUpdate(elements);
    
    // 初始化提交按钮
    initSubmitButton(elements);
}

// 监听页面加载完成事件
document.addEventListener('DOMContentLoaded', initPage);

// 监听URL变化，确保在授权后重定向回来时也能显示openid和更新页面标题
window.addEventListener('popstate', async function() {
    console.log('popstate事件触发，准备显示用户信息');
    
    // 延迟执行，确保cookie已经设置完成
    setTimeout(() => {
        console.log('执行displayOpenID');
        displayOpenID();
    }, 1000);
    
    // 当URL变化时，重新初始化页面参数，确保页面标题和内容更新
    await initPageWithParams();
});