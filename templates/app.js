class WS {
    constructor() {
        this.handlers = {};
        this.reconnectTimer = null;
        this.connect();
    }

    connect() {
        if (!location.host) {
            this.ws = null;
            this.fire('disconnect');
            return;
        }
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        try {
            this.ws = new WebSocket(proto + '//' + location.host + '/ws');
        } catch (_) {
            this.ws = null;
            this.fire('disconnect');
            return;
        }
        this.ws.onopen = () => this.fire('connect');
        this.ws.onclose = () => {
            this.fire('disconnect');
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = setTimeout(() => this.connect(), 1200);
        };
        this.ws.onerror = () => {};
        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                this.fire(message.event, message.data);
            } catch (_) {}
        };
    }

    on(event, fn) {
        (this.handlers[event] = this.handlers[event] || []).push(fn);
    }

    fire(event, data) {
        (this.handlers[event] || []).forEach(fn => fn(data));
    }

    emit(event, data) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify({ event: event, data: data || {} }));
        }
    }
}

const socket = new WS();
const statusBadge = document.getElementById('statusBadge');
const statusText = document.getElementById('statusText');
const touchpad = document.getElementById('touchpad');
const textInput = document.getElementById('textInput');
const appUrlInput = document.getElementById('appUrlInput');

const FEATURE_PORT = 5050;
const featureBase = location.hostname ? `${location.protocol}//${location.hostname}:${FEATURE_PORT}/feature` : '';

let mouseSensitivity = 2.5;
let scrollSensitivity = 3;
let favorites = safeReadJSON('favorites', []);
let recentHistory = safeReadJSON('recentHistory', []);
let lastTab = localStorage.getItem('lastTab') || 'touch';
let serverStateLoaded = false;
let featureAvailable = false;
let stateSyncTimer = null;
let featurePower = { sleepDeadline: 0, keepAwake: false };

socket.on('connect', () => {
    statusBadge.classList.add('connected');
    statusText.textContent = 'Online';
});

socket.on('disconnect', () => {
    statusBadge.classList.remove('connected');
    statusText.textContent = 'Offline';
});

socket.on('system_status', data => {
    alert(data.message);
});

socket.on('current_tab', data => {
    socket.emit('key', { key: 'esc' });

    if (data.error) {
        alert('탭 정보를 가져오지 못했습니다: ' + data.error);
        return;
    }

    const container = document.getElementById('currentTabContainer');
    const title = data.title || 'Unknown Title';
    const url = data.url || '';
    document.getElementById('currentTabTitle').textContent = title;
    document.getElementById('currentTabUrl').textContent = url || '주소를 읽지 못했습니다.';
    container.hidden = false;

    const openBtn = document.getElementById('openTabBtn');
    const favBtn = document.getElementById('favTabBtn');
    openBtn.disabled = !url;
    favBtn.disabled = !url;
    openBtn.onclick = () => {
        if (url) openUrl(url);
    };
    favBtn.onclick = () => {
        if (url) addToFavorites(url);
    };
});

function safeReadJSON(key, fallback) {
    try {
        const parsed = JSON.parse(localStorage.getItem(key) || 'null');
        return Array.isArray(parsed) ? parsed : fallback;
    } catch (_) {
        return fallback;
    }
}

function writeList(key, value) {
    localStorage.setItem(key, JSON.stringify(value));
}

function setupSensitivity() {
    const mouseInput = document.getElementById('mouseSens');
    const scrollInput = document.getElementById('scrollSens');
    const mouseValue = document.getElementById('mouseVal');
    const scrollValue = document.getElementById('scrollVal');

    const savedMouse = parseFloat(localStorage.getItem('mouseSens'));
    if (Number.isFinite(savedMouse) && savedMouse >= 1 && savedMouse <= 5) {
        mouseSensitivity = savedMouse;
    }

    const savedScroll = parseFloat(localStorage.getItem('scrollSens'));
    if (Number.isFinite(savedScroll) && savedScroll >= 1 && savedScroll <= 5) {
        scrollSensitivity = savedScroll;
    }

    mouseInput.value = mouseSensitivity;
    mouseValue.textContent = mouseSensitivity;
    scrollInput.value = scrollSensitivity;
    scrollValue.textContent = scrollSensitivity;

    mouseInput.addEventListener('input', event => {
        mouseSensitivity = parseFloat(event.target.value);
        mouseValue.textContent = mouseSensitivity;
        localStorage.setItem('mouseSens', mouseSensitivity);
        scheduleStateSync();
    });

    scrollInput.addEventListener('input', event => {
        scrollSensitivity = parseFloat(event.target.value);
        scrollValue.textContent = scrollSensitivity;
        localStorage.setItem('scrollSens', scrollSensitivity);
        scheduleStateSync();
    });
}

function renderAppsLists() {
    renderFavorites();
    renderRecent();
}

function renderFavorites() {
    const list = document.getElementById('favoritesList');
    list.innerHTML = '';

    if (favorites.length === 0) {
        list.appendChild(emptyState('즐겨찾기가 없습니다. 현재 탭이나 최근 기록에서 추가하세요.'));
        return;
    }

    favorites.forEach((item, index) => {
        list.appendChild(siteItem({
            title: item.label || getHostname(item.url),
            url: item.url,
            actions: [
                { text: '↑', actionClass: 'favorite', title: '위로 이동', disabled: index === 0, onClick: () => moveFavorite(index, -1) },
                { text: '↓', actionClass: 'favorite', title: '아래로 이동', disabled: index === favorites.length - 1, onClick: () => moveFavorite(index, 1) },
                { className: 'i-trash', actionClass: 'delete', title: '삭제', onClick: () => removeFromFavorites(index) }
            ]
        }));
    });
}

function renderRecent() {
    const section = document.getElementById('recentSection');
    const list = document.getElementById('recentList');
    list.innerHTML = '';
    section.style.display = 'flex';

    if (recentHistory.length === 0) {
        list.appendChild(emptyState('이전 기록이 없습니다.'));
        return;
    }

    recentHistory.forEach((url, index) => {
        list.appendChild(siteItem({
            title: getHostname(url),
            url: url,
            actions: [
                { className: 'i-star', actionClass: 'favorite', title: '즐겨찾기 추가', onClick: () => addToFavorites(url) },
                { className: 'i-trash', actionClass: 'delete', title: '삭제', onClick: () => removeFromRecent(index) }
            ]
        }));
    });
}

function siteItem({ title, url, actions }) {
    const item = document.createElement('div');
    item.className = 'site-item';

    const main = document.createElement('button');
    main.type = 'button';
    main.className = 'site-main';
    main.addEventListener('click', () => openUrl(url));

    const titleEl = document.createElement('div');
    titleEl.className = 'site-title';
    titleEl.textContent = title;
    const urlEl = document.createElement('div');
    urlEl.className = 'site-url';
    urlEl.textContent = url;
    main.appendChild(titleEl);
    main.appendChild(urlEl);

    const actionsEl = document.createElement('div');
    actionsEl.className = 'site-actions';
    actions.forEach(action => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'site-action ' + (action.actionClass || '');
        btn.title = action.title;
        btn.disabled = !!action.disabled;
        btn.addEventListener('click', event => {
            event.stopPropagation();
            if (!btn.disabled) action.onClick();
        });
        if (action.className) {
            const icon = document.createElement('span');
            icon.className = 'icon ' + action.className;
            btn.appendChild(icon);
        } else if (action.text) {
            btn.textContent = action.text;
        }
        actionsEl.appendChild(btn);
    });

    item.appendChild(main);
    item.appendChild(actionsEl);
    return item;
}

function emptyState(text) {
    const el = document.createElement('div');
    el.className = 'empty';
    el.textContent = text;
    return el;
}

function getHostname(rawUrl) {
    try {
        return new URL(rawUrl).hostname.replace(/^www\./, '');
    } catch (_) {
        return rawUrl;
    }
}

function normalizeUrl(raw) {
    if (!raw) return '';
    const value = String(raw).trim();
    if (!value) return '';
    return /^https?:\/\//i.test(value) ? value : 'https://' + value;
}

function addToRecent(url) {
    const normalized = normalizeUrl(url);
    if (!normalized) return;
    recentHistory = recentHistory.filter(item => item !== normalized);
    recentHistory.unshift(normalized);
    recentHistory = recentHistory.slice(0, 12);
    writeList('recentHistory', recentHistory);
    renderAppsLists();
    scheduleStateSync();
}

function removeFromRecent(index) {
    recentHistory.splice(index, 1);
    writeList('recentHistory', recentHistory);
    renderAppsLists();
    scheduleStateSync();
    vibrate(10);
}

function addToFavorites(url) {
    const normalized = normalizeUrl(url);
    if (!normalized) return;
    const label = prompt('즐겨찾기 이름', getHostname(normalized));
    if (label === null) return;
    favorites = favorites.filter(item => item.url !== normalized);
    favorites.unshift({ url: normalized, label: label.trim() || getHostname(normalized) });
    favorites = favorites.slice(0, 24);
    writeList('favorites', favorites);
    renderAppsLists();
    scheduleStateSync();
    vibrate(20);
}

function moveFavorite(index, offset) {
    const target = index + offset;
    if (index < 0 || target < 0 || index >= favorites.length || target >= favorites.length) return;
    [favorites[index], favorites[target]] = [favorites[target], favorites[index]];
    writeList('favorites', favorites);
    renderFavorites();
    scheduleStateSync();
    vibrate(12);
}

function removeFromFavorites(index) {
    favorites.splice(index, 1);
    writeList('favorites', favorites);
    renderAppsLists();
    scheduleStateSync();
    vibrate(10);
}

function addInputFavorite() {
    if (!appUrlInput.value) return;
    addToFavorites(appUrlInput.value);
    appUrlInput.value = '';
}

function clearFavorites() {
    if (favorites.length === 0) return;
    if (!confirm('즐겨찾기를 모두 초기화할까요?')) return;
    favorites = [];
    writeList('favorites', favorites);
    renderAppsLists();
    scheduleStateSync();
    vibrate(20);
}

function clearRecentHistory() {
    if (recentHistory.length === 0) return;
    if (!confirm('최근 기록을 모두 초기화할까요?')) return;
    recentHistory = [];
    writeList('recentHistory', recentHistory);
    renderAppsLists();
    scheduleStateSync();
    vibrate(20);
}

function switchTab(tabName, el, persist = true) {
    if (!['touch', 'input', 'apps'].includes(tabName)) tabName = 'touch';
    document.querySelectorAll('.panel').forEach(panel => panel.classList.remove('active'));
    const panel = document.getElementById('panel-' + tabName);
    if (panel) panel.classList.add('active');
    document.querySelectorAll('.nav-item').forEach(item => item.classList.remove('active'));
    if (el) el.classList.add('active');
    lastTab = tabName;
    localStorage.setItem('lastTab', lastTab);
    if (persist) scheduleStateSync();
}

function restoreLastTab(tabName) {
    const safeTab = ['touch', 'input', 'apps'].includes(tabName) ? tabName : 'touch';
    const nav = Array.from(document.querySelectorAll('.nav-item')).find(item => {
        const handler = item.getAttribute('onclick') || '';
        return handler.includes(`'${safeTab}'`) || handler.includes(`\"${safeTab}\"`);
    });
    switchTab(safeTab, nav, false);
}

function emitClick(btn) {
    socket.emit('click', { btn: btn });
    vibrate(btn === 'right' ? 35 : 16);
}

const keyDebounce = {};
function emitKey(key) {
    const now = Date.now();
    if (keyDebounce[key] && now - keyDebounce[key] < 90) return;
    keyDebounce[key] = now;
    socket.emit('key', { key: key });
    vibrate(14);
}

function emitHotkey(keys) {
    socket.emit('hotkey', { keys: keys });
    vibrate(18);
}

function reserveSleep() {
    const mins = prompt('몇 분 후에 절전 모드로 전환할까요?', '30');
    if (mins === null) return;
    const delay = parseInt(mins, 10);
    if (Number.isNaN(delay) || delay < 0 || delay > 1440) {
        alert('0~1440분 사이의 시간을 입력해주세요.');
        return;
    }
    scheduleSleep(delay);
}

function requestCurrentTab() {
    socket.emit('get_current_tab', {});
    vibrate(18);
}

function hideCurrentTab() {
    document.getElementById('currentTabContainer').hidden = true;
}

function openUrl(url) {
    const normalized = normalizeUrl(url);
    if (!normalized) return;
    socket.emit('open', { url: normalized });
    addToRecent(normalized);
    vibrate(25);
}

function openAppUrl() {
    if (!appUrlInput.value) return;
    openUrl(appUrlInput.value);
    appUrlInput.value = '';
}

function sendText(pressEnter) {
    const text = textInput.value.trim();
    if (!text) return;
    socket.emit('type', { text: text, pressEnter: !!pressEnter });
    textInput.value = '';
    vibrate(30);
}

function vibrate(ms) {
    if (navigator.vibrate) navigator.vibrate(ms);
}

function scrollEdgeWidth(rect) {
    return Math.max(66, Math.min(92, rect.width * 0.17));
}

function isInScrollZone(clientX, rect) {
    return clientX - rect.left >= rect.width - scrollEdgeWidth(rect);
}

const pointerData = {};
let pendingMoveX = 0;
let pendingMoveY = 0;
let pendingScroll = 0;
let pointerFrame = null;

function schedulePointerFlush() {
    if (pointerFrame !== null) return;
    pointerFrame = requestAnimationFrame(flushPointerDeltas);
}

function flushPointerDeltas() {
    pointerFrame = null;
    if (Math.abs(pendingMoveX) > 0.05 || Math.abs(pendingMoveY) > 0.05) {
        socket.emit('move', { dx: pendingMoveX, dy: pendingMoveY });
        pendingMoveX = 0;
        pendingMoveY = 0;
    }
    if (Math.abs(pendingScroll) >= 1) {
        socket.emit('scroll', { dy: Math.max(-10, Math.min(10, pendingScroll)) });
        pendingScroll = 0;
    }
}

touchpad.addEventListener('pointerdown', event => {
    if (event.pointerType === 'mouse' && event.button !== 0) return;
    event.preventDefault();
    touchpad.setPointerCapture(event.pointerId);
    const rect = touchpad.getBoundingClientRect();
    pointerData[event.pointerId] = {
        x: event.clientX,
        y: event.clientY,
        startX: event.clientX,
        startY: event.clientY,
        time: Date.now(),
        scrollAccum: 0,
        isScroll: isInScrollZone(event.clientX, rect)
    };
}, { passive: false });

touchpad.addEventListener('pointermove', event => {
    const prev = pointerData[event.pointerId];
    if (!prev) return;
    event.preventDefault();

    const rawDx = event.clientX - prev.x;
    const rawDy = event.clientY - prev.y;

    if (prev.isScroll) {
        prev.scrollAccum += rawDy;
        const threshold = 28 / scrollSensitivity;
        if (Math.abs(prev.scrollAccum) >= threshold) {
            const steps = Math.min(6, Math.floor(Math.abs(prev.scrollAccum) / threshold));
            const dir = prev.scrollAccum > 0 ? 1 : -1;
            pendingScroll += dir * 2 * steps;
            prev.scrollAccum -= dir * steps * threshold;
            schedulePointerFlush();
        }
    } else {
        const dx = rawDx * mouseSensitivity;
        const dy = rawDy * mouseSensitivity;
        if (Math.abs(dx) > 0.12 || Math.abs(dy) > 0.12) {
            pendingMoveX += dx;
            pendingMoveY += dy;
            schedulePointerFlush();
        }
    }

    prev.x = event.clientX;
    prev.y = event.clientY;
}, { passive: false });

function finishPointer(event) {
    const prev = pointerData[event.pointerId];
    if (!prev) return;
    flushPointerDeltas();
    const elapsed = Date.now() - prev.time;
    const totalMove = Math.hypot(event.clientX - prev.startX, event.clientY - prev.startY);
    if (elapsed < 260 && totalMove < 10) {
        emitClick(prev.isScroll ? 'right' : 'left');
    }
    delete pointerData[event.pointerId];
}

touchpad.addEventListener('pointerup', finishPointer);
touchpad.addEventListener('pointercancel', finishPointer);

function setupLongPressMediaButtons() {
    const keys = new Set(['left', 'right', 'volumedown', 'volumeup']);
    document.querySelectorAll('.media-grid .card-btn').forEach(button => {
        const inline = button.getAttribute('onclick') || '';
        const match = inline.match(/emitKey\(['\"]([^'\"]+)['\"]\)/);
        if (!match || !keys.has(match[1])) return;

        const key = match[1];
        button.removeAttribute('onclick');
        let delayTimer = null;
        let repeatTimer = null;
        let repeating = false;

        const clearTimers = () => {
            clearTimeout(delayTimer);
            clearInterval(repeatTimer);
            delayTimer = null;
            repeatTimer = null;
        };

        button.addEventListener('pointerdown', event => {
            if (event.pointerType === 'mouse' && event.button !== 0) return;
            event.preventDefault();
            repeating = false;
            try { button.setPointerCapture(event.pointerId); } catch (_) {}
            delayTimer = setTimeout(() => {
                repeating = true;
                emitKey(key);
                repeatTimer = setInterval(() => emitKey(key), 110);
            }, 350);
        });

        button.addEventListener('pointerup', event => {
            event.preventDefault();
            const wasRepeating = repeating;
            clearTimers();
            repeating = false;
            if (!wasRepeating) emitKey(key);
        });
        button.addEventListener('pointercancel', () => {
            clearTimers();
            repeating = false;
        });
    });
}

textInput.addEventListener('keydown', event => {
    if (event.key === 'Enter') {
        sendText(true);
        event.preventDefault();
    }
});

appUrlInput.addEventListener('keydown', event => {
    if (event.key === 'Enter') {
        openAppUrl();
        event.preventDefault();
    }
});

async function featureRequest(path, options = {}) {
    if (!featureBase) throw new Error('feature server unavailable');
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 1600);
    try {
        const response = await fetch(featureBase + path, {
            ...options,
            headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
            signal: controller.signal
        });
        featureAvailable = true;
        const data = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
        return data;
    } catch (error) {
        if (!(error && /^HTTP /.test(error.message || ''))) featureAvailable = false;
        throw error;
    } finally {
        clearTimeout(timeout);
    }
}

function localConfigSnapshot() {
    return {
        version: 1,
        favorites: favorites,
        recentHistory: recentHistory,
        mouseSensitivity: mouseSensitivity,
        scrollSensitivity: scrollSensitivity,
        lastTab: lastTab
    };
}

function scheduleStateSync(immediate = false) {
    if (!serverStateLoaded && !immediate) return;
    clearTimeout(stateSyncTimer);
    if (immediate) {
        syncStateNow();
        return;
    }
    stateSyncTimer = setTimeout(syncStateNow, 220);
}

async function syncStateNow() {
    if (!featureBase) return;
    try {
        await featureRequest('/state', {
            method: 'POST',
            body: JSON.stringify(localConfigSnapshot())
        });
    } catch (_) {
        featureAvailable = false;
    }
}

function applyServerConfig(cfg) {
    if (!cfg) return;
    favorites = Array.isArray(cfg.favorites) ? cfg.favorites : [];
    recentHistory = Array.isArray(cfg.recentHistory) ? cfg.recentHistory : [];
    if (Number.isFinite(cfg.mouseSensitivity)) mouseSensitivity = cfg.mouseSensitivity;
    if (Number.isFinite(cfg.scrollSensitivity)) scrollSensitivity = cfg.scrollSensitivity;
    lastTab = ['touch', 'input', 'apps'].includes(cfg.lastTab) ? cfg.lastTab : 'touch';

    writeList('favorites', favorites);
    writeList('recentHistory', recentHistory);
    localStorage.setItem('mouseSens', mouseSensitivity);
    localStorage.setItem('scrollSens', scrollSensitivity);
    localStorage.setItem('lastTab', lastTab);

    const mouseInput = document.getElementById('mouseSens');
    const scrollInput = document.getElementById('scrollSens');
    if (mouseInput) mouseInput.value = mouseSensitivity;
    if (scrollInput) scrollInput.value = scrollSensitivity;
    const mouseValue = document.getElementById('mouseVal');
    const scrollValue = document.getElementById('scrollVal');
    if (mouseValue) mouseValue.textContent = mouseSensitivity;
    if (scrollValue) scrollValue.textContent = scrollSensitivity;

    renderAppsLists();
    restoreLastTab(lastTab);
}

async function initFeatureIntegration() {
    enhanceTools();
    try {
        const state = await featureRequest('/state');
        featurePower = state.power || featurePower;
        if (state.initialized) {
            applyServerConfig(state.config);
            serverStateLoaded = true;
        } else {
            serverStateLoaded = true;
            await syncStateNow();
        }
        renderPowerStatus();
    } catch (_) {
        featureAvailable = false;
        serverStateLoaded = false;
        renderPowerStatus();
    }
}

function enhanceTools() {
    const toolsSection = Array.from(document.querySelectorAll('#panel-apps .section')).find(section => {
        const heading = section.querySelector('h2');
        return heading && heading.textContent.trim() === 'Tools';
    });
    if (!toolsSection) return;

    const grid = toolsSection.querySelector('.grid-shortcuts');
    if (!grid) return;

    const oldSleep = Array.from(grid.querySelectorAll('button')).find(btn => btn.textContent.trim() === '절전');
    if (oldSleep) oldSleep.onclick = reserveSleep;

    const keepBtn = document.createElement('button');
    keepBtn.type = 'button';
    keepBtn.className = 'key-btn';
    keepBtn.id = 'keepAwakeBtn';
    keepBtn.textContent = '절전 방지';
    keepBtn.onclick = toggleKeepAwake;
    grid.appendChild(keepBtn);

    [30, 60, 90].forEach(minutes => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'key-btn';
        btn.textContent = `${minutes}분 후 절전`;
        btn.onclick = () => scheduleSleep(minutes);
        grid.appendChild(btn);
    });

    const cancelBtn = document.createElement('button');
    cancelBtn.type = 'button';
    cancelBtn.className = 'key-btn';
    cancelBtn.textContent = '절전 예약 취소';
    cancelBtn.onclick = cancelSleep;
    grid.appendChild(cancelBtn);

    const status = document.createElement('div');
    status.className = 'empty';
    status.id = 'powerStatus';
    toolsSection.appendChild(status);

    const displaySection = document.createElement('section');
    displaySection.className = 'section';
    displaySection.innerHTML = `
        <div class="section-head"><h2>Display</h2></div>
        <div class="grid-shortcuts" id="displayModeGrid"></div>
    `;
    toolsSection.insertAdjacentElement('afterend', displaySection);

    const displayGrid = displaySection.querySelector('#displayModeGrid');
    [
        ['internal', '노트북만'],
        ['clone', '복제'],
        ['extend', '확장'],
        ['external', 'TV만']
    ].forEach(([mode, label]) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'key-btn';
        btn.textContent = label;
        btn.onclick = () => setDisplayMode(mode);
        displayGrid.appendChild(btn);
    });

    renderPowerStatus();
}

async function scheduleSleep(minutes) {
    if (!Number.isFinite(minutes) || minutes < 0 || minutes > 1440) return;
    const label = minutes === 0 ? '지금' : `${minutes}분 후`;
    if (!confirm(`${label} PC를 절전 모드로 전환할까요?`)) return;
    try {
        featurePower = await featureRequest('/sleep', {
            method: 'POST',
            body: JSON.stringify({ action: 'set', minutes })
        });
        renderPowerStatus();
        vibrate(30);
    } catch (error) {
        alert('절전 예약을 설정하지 못했습니다: ' + friendlyFeatureError(error));
    }
}

async function cancelSleep() {
    try {
        featurePower = await featureRequest('/sleep', {
            method: 'POST',
            body: JSON.stringify({ action: 'cancel', minutes: 0 })
        });
        renderPowerStatus();
        vibrate(20);
    } catch (error) {
        alert('절전 예약을 취소하지 못했습니다: ' + friendlyFeatureError(error));
    }
}

async function toggleKeepAwake() {
    const next = !featurePower.keepAwake;
    try {
        featurePower = await featureRequest('/keep-awake', {
            method: 'POST',
            body: JSON.stringify({ enabled: next })
        });
        renderPowerStatus();
        vibrate(20);
    } catch (error) {
        alert('절전 방지 설정을 바꾸지 못했습니다: ' + friendlyFeatureError(error));
    }
}

async function setDisplayMode(mode) {
    try {
        await featureRequest('/display', {
            method: 'POST',
            body: JSON.stringify({ mode })
        });
        vibrate(25);
    } catch (error) {
        alert('화면 모드를 바꾸지 못했습니다: ' + friendlyFeatureError(error));
    }
}

function friendlyFeatureError(error) {
    if (!featureAvailable) return '확장 기능 서버에 연결할 수 없습니다. AirController를 다시 실행해보세요.';
    return error && error.message ? error.message : '알 수 없는 오류';
}

function renderPowerStatus() {
    const el = document.getElementById('powerStatus');
    const keepBtn = document.getElementById('keepAwakeBtn');
    if (!el) return;

    if (!featureAvailable) {
        el.textContent = '확장 기능 연결 확인 중… 기본 리모컨 기능은 그대로 사용할 수 있습니다.';
        if (keepBtn) keepBtn.textContent = '절전 방지';
        return;
    }

    const parts = [];
    if (featurePower.keepAwake) parts.push('자동 절전 방지 켜짐');
    else parts.push('자동 절전 방지 꺼짐');

    const deadline = Number(featurePower.sleepDeadline || 0);
    if (deadline > Date.now()) {
        const seconds = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
        const minutes = Math.floor(seconds / 60);
        const remain = seconds % 60;
        parts.push(`절전까지 ${minutes}:${String(remain).padStart(2, '0')}`);
    } else {
        if (deadline) featurePower.sleepDeadline = 0;
        parts.push('절전 예약 없음');
    }

    el.textContent = parts.join(' · ');
    if (keepBtn) keepBtn.textContent = featurePower.keepAwake ? '절전 방지 끄기' : '절전 방지 켜기';
}

setupSensitivity();
renderAppsLists();
restoreLastTab(lastTab);
setupLongPressMediaButtons();
initFeatureIntegration();
setInterval(renderPowerStatus, 1000);
