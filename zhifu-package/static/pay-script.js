// 快速金额选择
function initQuickAmounts() {
    document.querySelectorAll('.quick-amount').forEach(btn => {
        btn.addEventListener('click', () => {
            document.getElementById('amount').value = btn.dataset.amount;
        });
    });
}

// 祝福语输入框折叠功能
function initBlessingToggle() {
    const toggleBtn = document.getElementById('toggle-blessing');
    if (toggleBtn) {
        toggleBtn.addEventListener('click', function() {
            const container = document.getElementById('blessing-container');
            const button = this;
            
            if (container.style.display === 'none') {
                container.style.display = 'block';
                button.textContent = '- 收起祝福语';
            } else {
                container.style.display = 'none';
                button.textContent = '+ 祝福语（选填）';
            }
        });
    }
}

// 祝福语字数限制
function initBlessingCounter() {
    const MAX_BLESSING_LENGTH = 50;
    const blessingTextarea = document.getElementById('blessing');
    const blessingCounter = document.getElementById('blessing-counter');
    
    if (blessingTextarea && blessingCounter) {
        blessingTextarea.addEventListener('input', function() {
            const currentLength = this.value.length;
            
            if (currentLength > MAX_BLESSING_LENGTH) {
                this.value = this.value.substring(0, MAX_BLESSING_LENGTH);
                blessingCounter.textContent = `${MAX_BLESSING_LENGTH}/${MAX_BLESSING_LENGTH}`;
                blessingCounter.style.color = '#e74c3c';
            } else {
                blessingCounter.textContent = `${currentLength}/${MAX_BLESSING_LENGTH}`;
                blessingCounter.style.color = currentLength >= MAX_BLESSING_LENGTH ? '#e74c3c' : '#666';
            }
        });
    }
}

// 自动检测支付方式
let selectedPayment = 'wechat'; // 默认支付方式

function detectPaymentMethod() {
    const userAgent = navigator.userAgent.toLowerCase();
    if (userAgent.match(/micromessenger\/[\d\.]+/)) {
        selectedPayment = 'wechat';
        console.log('自动检测到微信环境，使用微信支付');
    } else if (userAgent.match(/alipayclient\/[\d\.]+/)) {
        selectedPayment = 'alipay';
        console.log('自动检测到支付宝环境，使用支付宝');
    } else {
        selectedPayment = 'wechat'; // 默认使用微信支付
        console.log('未检测到特定支付环境，默认使用微信支付');
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
    
    try {
        const response = await fetch(`/api/payment-config/${paymentConfigId}`);
        if (!response.ok) {
            throw new Error(`网络请求失败: ${response.status}`);
        }
        return await response.json();
    } catch (error) {
        console.error('获取支付配置失败:', error);
        return null;
    }
}

// 获取类目信息
async function getCategory(categoryId) {
    if (!categoryId) {
        return null;
    }
    
    try {
        const response = await fetch(`/api/category/${categoryId}`);
        if (!response.ok) {
            throw new Error(`网络请求失败: ${response.status}`);
        }
        return await response.json();
    } catch (error) {
        console.error('获取类目信息失败:', error);
        return null;
    }
}

// 初始化页面，处理URL参数
function initPageWithParams() {
    const params = getURLParams();
    
    // 立即更新基本内容，确保页面快速显示
    updateBasicContent(params);
    
    // 异步加载额外信息，不阻塞页面显示
    loadExtraContent(params);
}

// 更新基本内容，确保页面快速显示
function updateBasicContent(params) {
    // 构建默认标题
    let defaultTitle = '无量福报积善功德榜';
    
    // 更新页面标题和h1
    document.title = defaultTitle;
    const h1Element = document.querySelector('h1');
    if (h1Element) {
        h1Element.textContent = defaultTitle;
    }
    
    // 更新金额标题
    const amountTitle = document.getElementById('amount-title');
    if (amountTitle) {
        amountTitle.textContent = '金额';
    }
}

// 异步加载额外内容
async function loadExtraContent(params) {
    let merchantName = '';
    let categoryName = '';
    
    // 并行请求，提高加载速度
    const promises = [];
    
    if (params.payment) {
        promises.push(getPaymentConfig(params.payment));
    }
    
    if (params.categories) {
        promises.push(getCategory(params.categories));
    }
    
    // 等待所有请求完成
    if (promises.length > 0) {
        const results = await Promise.all(promises);
        
        // 处理结果
        for (const result of results) {
            if (result && result.store_name) {
                merchantName = result.store_name;
            } else if (result && result.name) {
                categoryName = result.name;
                // 更新金额标题
                const amountTitle = document.getElementById('amount-title');
                if (amountTitle) {
                    amountTitle.textContent = `${result.name} 金额`;
                }
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
        const h1Element = document.querySelector('h1');
        if (h1Element) {
            h1Element.textContent = newTitle;
        }
    }
}

// 提交捐款
function initSubmitButton() {
    const submitBtn = document.getElementById('submit-btn');
    if (submitBtn) {
        submitBtn.addEventListener('click', async (e) => {
            e.preventDefault(); // 阻止默认行为
            
            const amount = parseFloat(document.getElementById('amount').value);
            if (isNaN(amount) || amount <= 0) {
                alert('请输入有效的捐款金额');
                return;
            }
            
            // 从URL获取参数
            const params = getURLParams();
            
            // 显示加载状态
            const originalText = submitBtn.textContent;
            submitBtn.textContent = '正在处理支付...';
            submitBtn.disabled = true;
            
            try {
                console.log('开始创建捐款订单，金额：', amount, '支付方式：', selectedPayment);
                
                // 使用表单提交实现302重定向
                const form = document.getElementById('pay-form');
                document.getElementById('form-amount').value = amount;
                document.getElementById('form-payment').value = selectedPayment;
                document.getElementById('form-category').value = params.categories || '';
                document.getElementById('form-blessing').value = document.getElementById('blessing').value || '';
                document.getElementById('form-payment-config-id').value = params.payment || '';
                
                // 提交表单，浏览器会处理302重定向
                form.submit();
            } catch (error) {
                console.error('提交捐款失败:', error);
                // 恢复按钮状态
                submitBtn.textContent = originalText;
                submitBtn.disabled = false;
            }
        });
    }
}

// 更新返回链接，添加当前页面的参数
function updateBackLink() {
    const params = getURLParams();
    const backLink = document.getElementById('back-to-ranking');
    if (backLink) {
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
        
        backLink.href = url;
    }
}

// 读取cookie的函数
function getCookie(name) {
    const value = `; ${document.cookie}`;
    const parts = value.split(`; ${name}=`);
    if (parts.length === 2) {
        const cookieValue = parts.pop().split(';').shift();
        return cookieValue;
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
function updateUserTime() {
    const userTimeElement = document.getElementById('user-time');
    if (!userTimeElement) return;
    
    // 获取当前时间
    const now = new Date();
    const year = now.getFullYear();
    const month = (now.getMonth() + 1).toString().padStart(2, '0');
    const day = now.getDate().toString().padStart(2, '0');
    const hours = now.getHours().toString().padStart(2, '0');
    const minutes = now.getMinutes().toString().padStart(2, '0');
    const seconds = now.getSeconds().toString().padStart(2, '0');
    
    // 显示当前时间（包含年月日时分秒）
    userTimeElement.textContent = `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
}

// 启动时间更新
function startTimeUpdate() {
    // 立即更新一次
    updateUserTime();
    
    // 每隔1秒更新一次
    setInterval(updateUserTime, 1000);
}

// 显示用户信息
function displayUserInfo(paymentType, userID, nickname, avatar, authURL, userType) {
    const userAvatar = document.getElementById('user-avatar');
    const userNickname = document.getElementById('user-nickname');
    const unauthorizeLink = document.getElementById('unauthorize-link');
    
    if (!userAvatar || !userNickname) {
        return;
    }
    
    // 显示用户昵称
    if (nickname && nickname !== '匿名施主') {
        try {
            // 尝试解码昵称
            const decodedNickname = decodeURIComponent(nickname);
            userNickname.textContent = decodedNickname;
        } catch (e) {
            console.error('URL解码失败:', e);
            userNickname.textContent = nickname;
        }
        // 显示取消授权链接
        if (unauthorizeLink) {
            unauthorizeLink.style.display = 'block';
        }
    } else if (userID && userID.trim() !== '') {
        userNickname.textContent = userType + ' ' + userID.substring(0, 8) + '...';
        // 显示取消授权链接
        if (unauthorizeLink) {
            unauthorizeLink.style.display = 'block';
        }
    } else {
        // 构建授权链接，包含当前页面的重定向参数
        const currentURL = window.location.href;
        const fullAuthURL = window.location.origin + authURL + '?redirect_url=' + encodeURIComponent(currentURL);
        const buttonColor = paymentType === 'wechat' ? '#8b0000' : '#1677ff';
        userNickname.innerHTML = '未授权："匿名施主" </br><a href="' + fullAuthURL + '" style="color: ' + buttonColor + '; text-decoration: none; font-size: 14px; font-weight: bold;">点击这里获取' + userType + '授权</a>';
        // 隐藏取消授权链接
        if (unauthorizeLink) {
            unauthorizeLink.style.display = 'none';
        }
    }
    
    // 显示用户头像
    if ((nickname && nickname !== '匿名施主') || (userID && userID.trim() !== '')) {
        // 用户已授权，使用存储的头像
        if (avatar && avatar !== './static/avatar.jpeg') {
            // 确保头像URL是完整的
            let avatarURL = avatar;
            try {
                // 解码URL
                avatarURL = decodeURIComponent(avatarURL);
            } catch (e) {
                console.error('URL解码失败:', e);
            }
            // 移除可能的额外引号
            avatarURL = avatarURL.replace(/^["']|["']$/g, '');
            userAvatar.src = avatarURL;
        } else {
            userAvatar.src = './static/avatar.jpeg';
        }
    } else {
        // 用户未授权，强制使用默认头像
        userAvatar.src = './static/avatar.jpeg';
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
    
    if (wechatOpenid && wechatOpenid.trim() !== '') {
        // 显示微信用户信息
        displayUserInfo('wechat', wechatOpenid, wechatNickname, wechatAvatar, '/api/wechat/auth', '微信用户');
        return;
    }
    
    // 尝试从cookie中读取支付宝user_id
    const alipayUserId = getCookie('alipay_user_id');
    const alipayNickname = getCookie('alipay_user_name');
    const alipayAvatar = getCookie('alipay_avatar_url');
    
    if (alipayUserId && alipayUserId.trim() !== '') {
        // 显示支付宝用户信息
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
    setCookie('wechat_user_name', '匿名施主', 30); // 保留30天
    setCookie('wechat_avatar_url', './static/avatar.jpeg', 30); // 保留30天
    
    // 清除支付宝用户信息cookie
    setCookie('alipay_user_id', '', -1);
    setCookie('alipay_user_name', '匿名施主', 30); // 保留30天
    setCookie('alipay_avatar_url', './static/avatar.jpeg', 30); // 保留30天
    
    // 重新显示用户信息，调用 displayOpenID() 以显示授权链接
    displayOpenID();
    
    // 显示提示信息
    alert('已成功取消授权，变回匿名施主');
}

// 页面加载完成后初始化
function initPage() {
    // 初始化快速金额选择
    initQuickAmounts();
    
    // 初始化祝福语切换
    initBlessingToggle();
    
    // 初始化祝福语计数器
    initBlessingCounter();
    
    // 自动检测支付方式
    detectPaymentMethod();
    
    // 初始化页面参数
    initPageWithParams();
    
    // 更新返回链接
    updateBackLink();
    
    // 延迟执行displayOpenID，确保cookie已经设置完成
    setTimeout(displayOpenID, 500);
    
    // 启动时间更新
    startTimeUpdate();
    
    // 初始化提交按钮
    initSubmitButton();
}

// 监听页面加载完成事件
document.addEventListener('DOMContentLoaded', initPage);

// 监听URL变化，确保在授权后重定向回来时也能显示openid
window.addEventListener('popstate', function() {
    setTimeout(displayOpenID, 500);
});