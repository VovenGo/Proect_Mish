const BRUSH_SIZES = {
  small: 2,
  medium: 6,
  large: 12,
  xl: 20,
};

const MAX_STROKE_POINTS = 48;

const state = {
  me: null,
  room: null,
  eventSource: null,
  drawing: false,
  currentPoints: [],
  strokeFlushPromise: Promise.resolve(),
  lastRoundNumber: 0,
  clearPending: false,
};

const els = {
  lobby: document.getElementById('lobby'),
  createName: document.getElementById('create-name'),
  createRoom: document.getElementById('create-room'),
  joinCode: document.getElementById('join-code'),
  joinName: document.getElementById('join-name'),
  joinRoom: document.getElementById('join-room'),
  lobbyGrid: document.getElementById('lobby-grid'),
  game: document.getElementById('game'),
  roomCode: document.getElementById('room-code'),
  roundTitle: document.getElementById('round-title'),
  timer: document.getElementById('timer'),
  board: document.getElementById('board'),
  color: document.getElementById('color'),
  size: document.getElementById('size'),
  brushSizeButtons: Array.from(document.querySelectorAll('[data-brush-size]')),
  clearCanvas: document.getElementById('clear-canvas'),
  startRound: document.getElementById('start-round'),
  phraseMasked: document.getElementById('phrase-masked'),
  phraseSecretWrap: document.getElementById('phrase-secret-wrap'),
  phraseSecret: document.getElementById('phrase-secret'),
  players: document.getElementById('players'),
  winnerBox: document.getElementById('winner-box'),
  copyLink: document.getElementById('copy-link'),
  chat: document.getElementById('chat'),
  chatForm: document.getElementById('chat-form'),
  chatInput: document.getElementById('chat-input'),
  confirmBox: document.getElementById('confirm-box'),
  confirmPlayer: document.getElementById('confirm-player'),
  confirmGuess: document.getElementById('confirm-guess'),
};

const ctx = els.board.getContext('2d');
ctx.lineCap = 'round';
ctx.lineJoin = 'round';

function initBoard() {
  ctx.clearRect(0, 0, els.board.width, els.board.height);
}

function api(path, body) {
  return fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(async res => {
    if (!res.ok) throw new Error(await res.text());
    return res.json();
  });
}

function saveSession() {
  if (state.me && state.room) {
    localStorage.setItem(`mish-room-${state.room.code}`, JSON.stringify({ player: state.me }));
  }
}

function loadSession(code) {
  const raw = localStorage.getItem(`mish-room-${code}`);
  if (!raw) return null;
  try { return JSON.parse(raw); } catch { return null; }
}

async function createRoom() {
  const name = els.createName.value.trim();
  const out = await api('/api/rooms', { name });
  enterRoom(out.room, out.player, true);
}

async function joinRoom() {
  const code = els.joinCode.value.trim().toUpperCase();
  const existing = loadSession(code);
  if (!els.joinName.value.trim() && existing?.player) {
    state.me = existing.player;
    const room = await fetch(`/api/rooms/${code}?playerId=${state.me.id}`).then(r => r.json());
    enterRoom(room, state.me, false);
    return;
  }
  const out = await api('/api/rooms/join', { code, name: els.joinName.value.trim() });
  enterRoom(out.room, out.player, false);
}

function enterRoom(room, player, pushHistory) {
  state.room = room;
  state.me = player;
  saveSession();
  if (pushHistory || window.location.pathname !== `/room/${room.code}`) {
    history.pushState({}, '', `/room/${room.code}`);
  }
  document.body.dataset.viewMode = 'game';
  els.lobbyGrid?.classList.add('hidden');
  els.lobbyGrid?.setAttribute('hidden', 'hidden');
  els.lobby?.classList.add('hidden');
  els.game.classList.remove('hidden');
  connectEvents();
  render(room);
}

function connectEvents() {
  if (state.eventSource) state.eventSource.close();
  state.eventSource = new EventSource(`/api/rooms/${state.room.code}/events?playerId=${state.me.id}`);
  state.eventSource.addEventListener('room', (event) => {
    const room = JSON.parse(event.data);
    state.room = room;
    saveSession();
    render(room);
  });
}

function resetTransientDrawing() {
  state.drawing = false;
  state.currentPoints = [];
  state.strokeFlushPromise = Promise.resolve();
}

function render(room) {
  room = room || {};
  room.players = Array.isArray(room.players) ? room.players : [];
  room.chat = Array.isArray(room.chat) ? room.chat : [];
  room.strokes = Array.isArray(room.strokes) ? room.strokes : [];
  room.round = room.round || {};

  if ((room.round.number || 0) !== state.lastRoundNumber) {
    resetTransientDrawing();
    state.lastRoundNumber = room.round.number || 0;
    state.clearPending = false;
  }
  if (room.strokes.length === 0) {
    resetTransientDrawing();
    state.clearPending = false;
  }

  els.roomCode.textContent = room.code || '—';
  els.roundTitle.textContent = room.round.status === 'active'
    ? `Раунд ${room.round.number}: рисует ${room.round.drawerName}`
    : room.round.status === 'guessed'
      ? `Раунд ${room.round.number} окончен — угадал ${room.round.winnerName}`
      : room.round.status === 'timeout'
        ? `Раунд ${room.round.number} сгорел по таймеру`
        : 'Ждём старт нового шаманства';

  els.phraseMasked.textContent = room.round.phraseMasked || '—';
  const myId = state.me?.id;
  const amCurrentDrawer = room.round.drawerId === myId;
  const canSeeSecretPhrase = amCurrentDrawer && !!room.round.phraseForDrawer;
  els.phraseSecret.textContent = canSeeSecretPhrase ? room.round.phraseForDrawer : '';
  els.phraseSecretWrap.classList.toggle('hidden', !canSeeSecretPhrase);
  const amDrawer = amCurrentDrawer || room.players.find(p => p.id === myId)?.role === 'drawer';
  els.startRound.disabled = !amDrawer || room.players.length < 2 || room.round.status === 'active';
  els.clearCanvas.disabled = state.clearPending || !amDrawer || room.round.status !== 'active';
  els.clearCanvas.textContent = state.clearPending ? 'Очищаем...' : 'Очистить холст';

  renderPlayers(room, amDrawer);
  renderChat(room);
  renderBoard(room);
  renderTimer(room);
  renderWinner(room);
  renderConfirm(room, amDrawer);
  syncBrushButtons();
}

function renderPlayers(room, amDrawer) {
  els.players.innerHTML = '';
  (room.players || []).forEach(player => {
    const li = document.createElement('li');
    li.className = player.id === state.me.id ? 'active' : '';
    li.innerHTML = `<strong>${player.name}</strong><div class="muted">${player.role === 'drawer' ? 'рисует' : 'угадывает'}</div>`;
    els.players.appendChild(li);
  });
  els.board.style.pointerEvents = amDrawer && room.round.status === 'active' ? 'auto' : 'none';
}

function renderChat(room) {
  els.chat.innerHTML = '';
  (room.chat || []).forEach(msg => {
    const div = document.createElement('div');
    div.className = `chat-message ${msg.kind}`;
    div.innerHTML = `<div class="meta">${msg.player || 'Система'} · ${new Date(msg.createdAt).toLocaleTimeString('ru-RU', {hour:'2-digit', minute:'2-digit'})}</div><div>${escapeHTML(msg.text)}</div>`;
    els.chat.appendChild(div);
  });
  els.chat.scrollTop = els.chat.scrollHeight;
}

function renderBoard(room) {
  initBoard();
  (room.strokes || []).forEach(drawStroke);
}

function renderTimer(room) {
  const update = () => {
    if (room.round.status !== 'active' || !room.round.endsAt) { els.timer.textContent = '--:--'; return; }
    const left = Math.max(0, Math.floor((new Date(room.round.endsAt) - new Date()) / 1000));
    const m = String(Math.floor(left / 60)).padStart(2, '0');
    const s = String(left % 60).padStart(2, '0');
    els.timer.textContent = `${m}:${s}`;
  };
  update();
  clearInterval(renderTimer.timer);
  renderTimer.timer = setInterval(update, 1000);
}

function renderWinner(room) {
  const text = room.lastWinner
    ? `Последний красавчик: ${room.lastWinner}${room.lastWinningGuess ? ` — «${room.lastWinningGuess}»` : ''}`
    : '';
  els.winnerBox.classList.toggle('hidden', !text);
  els.winnerBox.textContent = text;
}

function renderConfirm(room, amDrawer) {
  const options = (room.players || []).filter(p => p.id !== state.me?.id).map(p => `<option value="${p.id}">${p.name}</option>`).join('');
  els.confirmPlayer.innerHTML = options;
  els.confirmBox.classList.toggle('hidden', !(amDrawer && room.round.status === 'active' && room.players.length > 1));
}

function drawStroke(stroke) {
  if (!stroke.points?.length) return;
  ctx.beginPath();
  ctx.strokeStyle = stroke.color;
  ctx.lineWidth = stroke.width * 2.5;
  const first = denorm(stroke.points[0]);
  ctx.moveTo(first.x, first.y);
  stroke.points.slice(1).forEach(p => {
    const q = denorm(p);
    ctx.lineTo(q.x, q.y);
  });
  if (stroke.points.length === 1) ctx.lineTo(first.x + 0.1, first.y + 0.1);
  ctx.stroke();
}

function denorm(point) {
  return { x: point.x * els.board.width, y: point.y * els.board.height };
}

function pos(event) {
  const rect = els.board.getBoundingClientRect();
  const p = event.touches ? event.touches[0] : event;
  return { x: (p.clientX - rect.left) / rect.width, y: (p.clientY - rect.top) / rect.height };
}

function queueStrokeChunk(points) {
  if (state.clearPending || !points?.length) return Promise.resolve();
  const payload = {
    playerId: state.me.id,
    color: els.color.value,
    width: Number(els.size.value),
    points,
  };
  state.strokeFlushPromise = state.strokeFlushPromise
    .then(() => api(`/api/rooms/${state.room.code}/stroke`, payload))
    .catch((err) => {
      alert(err.message || String(err));
    });
  return state.strokeFlushPromise;
}

function startDraw(e) {
  if (els.board.style.pointerEvents === 'none') return;
  state.drawing = true;
  state.currentPoints = [pos(e)];
  e.preventDefault();
}
function moveDraw(e) {
  if (!state.drawing) return;
  const point = pos(e);
  state.currentPoints.push(point);
  drawStroke({ color: els.color.value, width: Number(els.size.value), points: state.currentPoints.slice(-2) });
  if (state.currentPoints.length >= MAX_STROKE_POINTS) {
    const chunk = state.currentPoints.slice();
    state.currentPoints = state.currentPoints.slice(-1);
    queueStrokeChunk(chunk);
  }
  e.preventDefault();
}
async function stopDraw() {
  if (!state.drawing) return;
  state.drawing = false;
  const points = state.currentPoints.slice();
  state.currentPoints = [];
  if (state.clearPending || !points.length) return;
  await queueStrokeChunk(points);
}

function setBrushSize(value) {
  els.size.value = String(value);
  syncBrushButtons();
}

function syncBrushButtons() {
  const current = Number(els.size.value);
  els.brushSizeButtons.forEach(button => {
    button.classList.toggle('active', Number(button.dataset.brushSizeValue) === current);
  });
}

function escapeHTML(str) {
  return str.replace(/[&<>"']/g, ch => ({ '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;' }[ch]));
}

els.createRoom.addEventListener('click', () => createRoom().catch(showErr));
els.joinRoom.addEventListener('click', () => joinRoom().catch(showErr));
els.startRound.addEventListener('click', async () => {
  try { await api(`/api/rooms/${state.room.code}/start`, { playerId: state.me.id }); } catch (err) { showErr(err); }
});
els.clearCanvas.addEventListener('click', async () => {
  if (state.clearPending) return;
  state.clearPending = true;
  resetTransientDrawing();
  render({ ...state.room, strokes: [] });
  try {
    await api(`/api/rooms/${state.room.code}/clear`, { playerId: state.me.id });
  } catch (err) {
    state.clearPending = false;
    render(state.room);
    showErr(err);
  }
});
els.chatForm.addEventListener('submit', async (e) => {
  e.preventDefault();
  try {
    await api(`/api/rooms/${state.room.code}/guess`, { playerId: state.me.id, text: els.chatInput.value.trim() });
    els.chatInput.value = '';
  } catch (err) { showErr(err); }
});
els.confirmGuess.addEventListener('click', async () => {
  try {
    await api(`/api/rooms/${state.room.code}/confirm`, { playerId: state.me.id, winnerId: els.confirmPlayer.value });
  } catch (err) { showErr(err); }
});
els.copyLink.addEventListener('click', async () => {
  try { await navigator.clipboard.writeText(window.location.origin + `/room/${state.room.code}`); } catch {}
});
els.brushSizeButtons.forEach(button => {
  button.addEventListener('click', () => setBrushSize(button.dataset.brushSizeValue));
});
['mousedown', 'touchstart'].forEach(ev => els.board.addEventListener(ev, startDraw, { passive: false }));
['mousemove', 'touchmove'].forEach(ev => els.board.addEventListener(ev, moveDraw, { passive: false }));
['mouseup', 'mouseleave', 'touchend'].forEach(ev => els.board.addEventListener(ev, stopDraw));

function showErr(err) { alert(err.message || String(err)); }

(function boot() {
  initBoard();
  setBrushSize(BRUSH_SIZES.medium);
  const code = (document.body.dataset.roomCode || '').trim().toUpperCase();
  if (code) {
    document.body.dataset.viewMode = 'room';
    els.lobbyGrid?.classList.add('hidden');
    els.lobbyGrid?.setAttribute('hidden', 'hidden');
    els.joinCode.value = code;
    const existing = loadSession(code);
    if (existing?.player) {
      state.me = existing.player;
      fetch(`/api/rooms/${code}?playerId=${state.me.id}`)
        .then(async res => { if (!res.ok) throw new Error(await res.text()); return res.json(); })
        .then(room => enterRoom(room, state.me, false))
        .catch(() => {});
    }
  }
})();
