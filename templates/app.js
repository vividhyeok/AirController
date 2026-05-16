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

let mouseSensitivity = 2.5;
let scrollSensitivity = 3;
let favorites = safeReadJSON('favorites', []);
let recentHistory = safeReadJSON('recentHistory', []);

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
    });

    scrollInput.addEventListener('input', event => {
        scrollSensitivity = parseFloat(event.target.value);
        scrollValue.textContent = scrollSensitivity;
        localStorage.setItem('scrollSens', scrollSensitivity);
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
        btn.addEventListener('click', event => {
            event.stopPropagation();
            action.onClick();
        });
        const icon = document.createElement('span');
        icon.className = 'icon ' + action.className;
        btn.appendChild(icon);
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
}

function removeFromRecent(index) {
    recentHistory.splice(index, 1);
    writeList('recentHistory', recentHistory);
    renderAppsLists();
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
    vibrate(20);
}

function removeFromFavorites(index) {
    favorites.splice(index, 1);
    writeList('favorites', favorites);
    renderAppsLists();
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
    vibrate(20);
}

function clearRecentHistory() {
    if (recentHistory.length === 0) return;
    if (!confirm('최근 기록을 모두 초기화할까요?')) return;
    recentHistory = [];
    writeList('recentHistory', recentHistory);
    renderAppsLists();
    vibrate(20);
}

function switchTab(tabName, el) {
    document.querySelectorAll('.panel').forEach(panel => panel.classList.remove('active'));
    document.getElementById('panel-' + tabName).classList.add('active');
    document.querySelectorAll('.nav-item').forEach(item => item.classList.remove('active'));
    if (el) el.classList.add('active');
}

function emitClick(btn) {
    socket.emit('click', { btn: btn });
    vibrate(btn === 'right' ? 35 : 16);
}

const keyDebounce = {};
function emitKey(key) {
    const now = Date.now();
    if (keyDebounce[key] && now - keyDebounce[key] < 110) return;
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
    if (Number.isNaN(delay) || delay < 0) {
        alert('올바른 시간을 입력해주세요.');
        return;
    }
    if (!confirm(delay + '분 후에 PC를 절전 모드로 전환할까요?')) return;
    socket.emit('system', { action: 'sleep', delay: delay });
    vibrate(30);
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
            for (let i = 0; i < steps; i++) {
                socket.emit('scroll', { dy: dir * 2 });
            }
            prev.scrollAccum -= dir * steps * threshold;
        }
    } else {
        const dx = rawDx * mouseSensitivity;
        const dy = rawDy * mouseSensitivity;
        if (Math.abs(dx) > 0.12 || Math.abs(dy) > 0.12) {
            socket.emit('move', { dx: dx, dy: dy });
        }
    }

    prev.x = event.clientX;
    prev.y = event.clientY;
}, { passive: false });

function finishPointer(event) {
    const prev = pointerData[event.pointerId];
    if (!prev) return;
    const elapsed = Date.now() - prev.time;
    const totalMove = Math.hypot(event.clientX - prev.startX, event.clientY - prev.startY);
    if (elapsed < 260 && totalMove < 10) {
        emitClick(prev.isScroll ? 'right' : 'left');
    }
    delete pointerData[event.pointerId];
}

touchpad.addEventListener('pointerup', finishPointer);
touchpad.addEventListener('pointercancel', finishPointer);

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

setupSensitivity();
renderAppsLists();
