/* World Game — client.
 *
 * No build step and no framework on purpose: iteration 0 should be editable by
 * anyone on the team at 2am without a toolchain in the way.
 *
 * State lives on the server. The client polls once a second, which at 2-4
 * players is indistinguishable from a socket and costs us no reconnect logic.
 */

const $ = (sel) => document.querySelector(sel);
const app = $('#app');

const state = {
  user: null,
  room: null,
  view: 'home',        // home | room
  publicRooms: [],
  maxPlayers: 3,
  joinCode: '',
  topicInput: '',
  suggestions: [],
  error: '',
  busy: false,
  rolledLocally: false,
  lastPhase: null,
  lastQuestion: -1,
};

/* ---------------------------------------------------------------- sound ---
 * 8-bit effects synthesized in WebAudio so the game has sound with no assets
 * and no network. scripts/gen-audio.sh generates richer fal.ai versions; when
 * those land in /static/sfx they take over here automatically.
 */
const Sound = (() => {
  let ctx = null;
  let muted = localStorage.getItem('wg_muted') === '1';
  const files = {};

  const ensure = () => {
    if (!ctx) ctx = new (window.AudioContext || window.webkitAudioContext)();
    if (ctx.state === 'suspended') ctx.resume();
    return ctx;
  };

  // Probe for fal-generated assets; fall back to synthesis when absent.
  ['roll', 'select', 'correct', 'wrong', 'win'].forEach((name) => {
    const a = new Audio(`/sfx/${name}.wav`);
    a.addEventListener('canplaythrough', () => { files[name] = a; }, { once: true });
    a.addEventListener('error', () => {}, { once: true });
  });

  function blip(freq, dur, type = 'square', slideTo = null) {
    if (muted) return;
    const c = ensure();
    const osc = c.createOscillator();
    const gain = c.createGain();
    osc.type = type;
    osc.frequency.setValueAtTime(freq, c.currentTime);
    if (slideTo) osc.frequency.exponentialRampToValueAtTime(slideTo, c.currentTime + dur);
    gain.gain.setValueAtTime(0.16, c.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, c.currentTime + dur);
    osc.connect(gain).connect(c.destination);
    osc.start();
    osc.stop(c.currentTime + dur);
  }

  function play(name) {
    if (muted) return;
    if (files[name]) { files[name].currentTime = 0; files[name].play().catch(() => {}); return; }
    switch (name) {
      case 'roll':    blip(180, 0.08, 'square', 420); break;
      case 'select':  blip(660, 0.06); break;
      case 'correct': [523, 659, 784].forEach((f, i) => setTimeout(() => blip(f, 0.12), i * 70)); break;
      case 'wrong':   blip(200, 0.28, 'sawtooth', 90); break;
      case 'win':     [523, 659, 784, 1047].forEach((f, i) => setTimeout(() => blip(f, 0.18), i * 110)); break;
    }
  }

  return {
    play,
    get muted() { return muted; },
    toggle() { muted = !muted; localStorage.setItem('wg_muted', muted ? '1' : '0'); return muted; },
  };
})();

/* ------------------------------------------------------------------ api --- */
async function api(path, options = {}) {
  const res = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  const text = await res.text();
  const body = text ? JSON.parse(text) : {};
  if (!res.ok) throw new Error(body?.error?.message || `Request failed (${res.status})`);
  return body;
}

const post = (path, body) => api(path, { method: 'POST', body: JSON.stringify(body ?? {}) });

/* ----------------------------------------------------------------- auth --- */
async function ensureSession() {
  try {
    const me = await api('/d3bit/auth/me');
    state.user = me.data ?? me;
    if (state.user?.id) return;
  } catch { /* fall through to anonymous */ }

  try {
    const anon = await post('/d3bit/auth/anon');
    state.user = anon.data?.user ?? anon.user ?? anon.data ?? anon;
  } catch (e) {
    state.error = 'Could not start a session with D3BIT. ' + e.message;
  }
}

async function loginWithEmail(email) {
  const redirect = window.location.origin + '/auth/callback';
  return post('/d3bit/auth/login', { email, redirect });
}

function loginWithGoogle(d3bitURL) {
  const cb = window.location.origin + '/auth/callback';
  window.location.href = `${d3bitURL}/auth/google?redirect=${encodeURIComponent(cb)}`;
}

/* -------------------------------------------------------------- polling --- */
let pollTimer = null;
function startPolling(code) {
  stopPolling();
  pollTimer = setInterval(async () => {
    try {
      const room = await api(`/api/rooms/${code}`);
      applyRoom(room);
    } catch { /* transient; next tick retries */ }
  }, 1000);
}
function stopPolling() { if (pollTimer) clearInterval(pollTimer); pollTimer = null; }

// applyRoom fires the sounds that depend on a transition rather than a click.
function applyRoom(room) {
  const prev = state.room;
  state.room = room;

  if (room.phase !== state.lastPhase) {
    if (room.phase === 'rolling') Sound.play('roll');
    if (room.phase === 'results') Sound.play('win');
    state.lastPhase = room.phase;
    state.rolledLocally = false;
  }
  if (room.question && room.question.index !== state.lastQuestion) {
    state.lastQuestion = room.question.index;
  }
  // Auto-roll: the dice are theatre, not a decision.
  if (room.phase === 'rolling' && !state.rolledLocally) {
    const me = room.players.find((p) => p.id === room.me);
    if (me && me.roll === 0) {
      state.rolledLocally = true;
      setTimeout(() => post(`/api/rooms/${room.code}/roll`).then(applyRoom).catch(() => {}), 900);
    }
  }
  render();
}

/* ------------------------------------------------------------- rendering --- */
function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
  ));
}

const PIP_LAYOUT = {
  1: [4], 2: [0, 8], 3: [0, 4, 8], 4: [0, 2, 6, 8], 5: [0, 2, 4, 6, 8], 6: [0, 2, 3, 5, 6, 8],
};

function die(value, rolling) {
  const pips = PIP_LAYOUT[value] || [];
  let cells = '';
  for (let i = 0; i < 9; i++) cells += pips.includes(i) ? '<span class="pip"></span>' : '<span></span>';
  return `<div class="die ${rolling ? 'rolling' : 'settled'}">${cells}</div>`;
}

function playerRow(p, room) {
  const isNext = room && (room.next_picker === p.id || (room.phase === 'topic' && room.topic_picker === p.id));
  return `
    <div class="player">
      <span class="dot" style="background:${esc(p.color)}"></span>
      <span class="name">${esc(p.display_name)}${p.id === room?.me ? ' <span class="muted small">(you)</span>' : ''}</span>
      ${isNext ? '<span class="tag live">picking</span>' : ''}
      ${p.is_host ? '<span class="tag">host</span>' : ''}
      <span class="score">${p.score}</span>
    </div>`;
}

function render() {
  app.innerHTML = state.view === 'home' ? renderHome() : renderRoom();
  bind();
}

function renderHome() {
  const u = state.user;
  const lobbies = state.publicRooms.length
    ? state.publicRooms.map((r) => `
        <div class="lobby-item">
          <span class="pixel" style="font-size:12px">${esc(r.code)}</span>
          <span class="muted small">${r.players}/${r.max_players} players</span>
          <button data-join="${esc(r.code)}">Join</button>
        </div>`).join('')
    // An empty public list reads as broken, so say what is actually going on.
    : '<p class="muted small" style="margin:6px 0 0">No public lobbies right now. Create one and it shows up here.</p>';

  return `
    <div class="topbar">
      <div>
        <h1 class="title">WORLD GAME</h1>
        <p class="subtitle">v0.1 — a quiz built from a verified entity graph</p>
      </div>
      <div class="row">
        <button id="mute" class="ghost small">${Sound.muted ? '🔇' : '🔊'}</button>
        ${u ? `<span class="tag ${u.is_anon ? '' : 'live'}">${esc(u.display_name || u.name || 'player')}${u.is_anon ? ' · guest' : ''}</span>` : ''}
      </div>
    </div>

    ${state.error ? `<div class="banner" style="border-color:var(--danger);color:var(--danger);margin-bottom:18px">${esc(state.error)}</div>` : ''}

    <div class="grid">
      <div class="stack">
        <div class="card stack">
          <div>
            <label class="field">Players</label>
            <div class="stepper">
              <button id="dec">−</button>
              <span class="val">${state.maxPlayers}</span>
              <button id="inc">+</button>
              <span class="muted small">2–4 · dice decide who picks the topic</span>
            </div>
          </div>
          <div class="row wrap">
            <button class="primary" id="create-private">Create a lobby</button>
            <button id="create-public">Create public lobby</button>
          </div>
        </div>

        <div class="card stack">
          <label class="field">Join game</label>
          <div class="row">
            <input type="text" class="code" id="code" maxlength="4" placeholder="CODE" value="${esc(state.joinCode)}">
            <button id="join">Join</button>
          </div>
        </div>

        <div class="card">
          <label class="field">Or public lobbies</label>
          ${lobbies}
        </div>
      </div>

      <div class="stack">
        <div class="card">
          <label class="field">Account</label>
          ${u && !u.is_anon
            ? `<p class="small">Signed in as <strong>${esc(u.email || u.display_name)}</strong>.</p>
               <button id="logout" class="ghost">Log out</button>`
            : `<p class="small muted">Playing as a guest. Sign in to keep your score.</p>
               <div class="stack">
                 <input type="text" id="email" placeholder="you@example.com" style="text-transform:none">
                 <div class="row wrap">
                   <button id="email-login">Email link</button>
                   <button id="google-login">Google</button>
                 </div>
               </div>
               <p id="login-msg" class="small muted"></p>`}
        </div>
        <div class="card">
          <label class="field">How it works</label>
          <ol class="small muted" style="padding-left:18px;margin:0">
            <li>Everyone rolls. Highest picks the topic.</li>
            <li>The rest each claim one of the top 5 sub-topics.</li>
            <li>Questions come from those axes — with sources.</li>
          </ol>
        </div>
      </div>
    </div>

    <p class="footer">CALA DATA &amp; SHOWCASED BY FAL.AI</p>`;
}

function renderRoom() {
  const r = state.room;
  if (!r) return '<p class="muted">Loading…</p>';

  const me = r.players.find((p) => p.id === r.me);
  const header = `
    <div class="topbar">
      <div>
        <h1 class="title">${esc(r.code)}</h1>
        <p class="subtitle">${esc(r.topic || 'no topic yet')} · ${r.players.length}/${r.max_players} players
          ${r.cala_enabled ? '' : '<span class="tag warn">offline data</span>'}</p>
      </div>
      <div class="row">
        <button id="mute" class="ghost small">${Sound.muted ? '🔇' : '🔊'}</button>
        <button id="leave" class="ghost small">Leave</button>
      </div>
    </div>`;

  const roster = `
    <div class="card">
      <label class="field">Players</label>
      ${r.players.map((p) => playerRow(p, r)).join('')}
    </div>`;

  return `${header}
    ${r.error ? `<div class="banner" style="border-color:var(--danger);color:var(--danger);margin-bottom:18px">${esc(r.error)}</div>` : ''}
    <div class="grid">
      <div class="stack fade-in">${phaseBody(r, me)}</div>
      <div class="stack">${roster}</div>
    </div>
    <p class="footer">CALA DATA &amp; SHOWCASED BY FAL.AI</p>`;
}

function phaseBody(r, me) {
  switch (r.phase) {
    case 'lobby': return `
      <div class="card stack">
        <h2 class="pixel" style="font-size:14px;margin:0">Waiting for players</h2>
        <p class="muted small">Share the code <strong class="pixel">${esc(r.code)}</strong> — the game starts on its own when the room fills.</p>
        <div class="progress"><i style="width:${(r.players.length / r.max_players) * 100}%"></i></div>
      </div>`;

    case 'rolling': return `
      <div class="card">
        <h2 class="pixel" style="font-size:14px;margin:0 0 6px">Rolling for order</h2>
        <p class="muted small">Highest roll picks the overall topic.</p>
        <div class="dice-wrap">
          ${r.players.map((p) => `
            <div>
              ${die(p.roll || 1 + Math.floor(Math.random() * 6), p.roll === 0)}
              <div class="die-label">${esc(p.display_name)}</div>
            </div>`).join('')}
        </div>
      </div>`;

    case 'topic': {
      const mine = r.topic_picker === r.me;
      const picker = r.players.find((p) => p.id === r.topic_picker);
      if (!mine) return `
        <div class="card">
          <h2 class="pixel" style="font-size:14px;margin:0 0 6px">Topic</h2>
          <p class="muted">${esc(picker?.display_name || 'The winner')} won the roll and is choosing the topic…</p>
        </div>`;
      return `
        <div class="card stack">
          <h2 class="pixel" style="font-size:14px;margin:0">You won the roll</h2>
          <p class="muted small">Pick the overall topic. Everyone else claims a sub-topic from it.</p>
          <input type="text" id="topic" placeholder="cities, fintechs, space…" value="${esc(state.topicInput)}" style="text-transform:none">
          <div class="row wrap">
            ${state.suggestions.map((t) => `<button class="ghost small" data-topic="${esc(t)}">${esc(t)}</button>`).join('')}
          </div>
          <button class="primary" id="set-topic">Lock it in</button>
        </div>`;
    }

    case 'subtopics': {
      const mine = r.next_picker === r.me;
      const next = r.players.find((p) => p.id === r.next_picker);
      return `
        <div class="card stack">
          <h2 class="pixel" style="font-size:14px;margin:0">Sub-topics</h2>
          <p class="muted small">
            ${mine ? 'Your pick — claim one axis.' : `Waiting for ${esc(next?.display_name || 'the next player')}…`}
            These are the top 5 things the graph actually holds for <strong>${esc(r.topic)}</strong>.
          </p>
          <div class="stack">
            ${r.sub_topics.map((st) => {
              const owner = r.players.find((p) => p.id === st.claimed_by);
              return `
                <button class="subtopic ${st.claimed_by ? 'claimed' : ''}"
                        data-subtopic="${esc(st.key)}" ${!mine || st.claimed_by ? 'disabled' : ''}>
                  <strong>${esc(st.label)}</strong>
                  <span class="small muted">${esc(st.kind)}${owner ? ` · taken by ${esc(owner.display_name)}` : ''}</span>
                </button>`;
            }).join('')}
          </div>
        </div>`;
    }

    case 'building': return `
      <div class="card">
        <h2 class="pixel" style="font-size:14px;margin:0 0 8px">Building the round…</h2>
        <p class="muted small">Pulling verified facts for <strong>${esc(r.topic)}</strong>.</p>
        <div class="progress"><i style="width:60%"></i></div>
      </div>`;

    case 'quiz': {
      const q = r.question;
      if (!q) return '<div class="card"><p class="muted">Loading question…</p></div>';
      const pct = ((q.index) / Math.max(1, r.question_count)) * 100;
      const seeded = q.seeded_by === r.me;

      return `
        <div class="card">
          <div class="row spread small muted">
            <span>Question ${q.index + 1} / ${r.question_count}</span>
            ${seeded ? '<span class="tag warn">your axis · half points</span>' : ''}
          </div>
          <div class="progress" style="margin-top:10px"><i style="width:${pct}%"></i></div>
          <p class="prompt">${esc(q.prompt)}</p>
          <div class="choices">
            ${q.options.map((opt, i) => {
              let cls = '';
              if (q.answered) {
                if (i === q.answer) cls = 'correct';
                else if (i === state.myChoice) cls = 'wrong';
              }
              return `<button class="choice ${cls}" data-choice="${i}" ${q.answered ? 'disabled' : ''}>
                        <span class="key">${String.fromCharCode(65 + i)}</span> ${esc(opt)}
                      </button>`;
            }).join('')}
          </div>
          ${q.answered ? `
            <div class="fact">
              <div class="small">${esc(q.fact || '')}</div>
              ${q.source ? `<div class="small muted" style="margin-top:6px">Source:
                ${q.source_url ? `<a href="${esc(q.source_url)}" target="_blank" rel="noopener">${esc(q.source)}</a>` : esc(q.source)}</div>` : ''}
            </div>
            <p class="small muted" style="margin-top:12px">
              ${q.waiting > 0 ? `Waiting for ${q.waiting} more player${q.waiting === 1 ? '' : 's'}…` : 'Next question…'}
            </p>` : ''}
        </div>`;
    }

    case 'results': {
      const ranked = [...r.players].sort((a, b) => b.score - a.score);
      return `
        <div class="card stack">
          <h2 class="pixel" style="font-size:14px;margin:0">Final</h2>
          ${ranked.map((p, i) => `
            <div class="player">
              <span class="pixel" style="font-size:12px;width:22px">${i + 1}</span>
              <span class="dot" style="background:${esc(p.color)}"></span>
              <span class="name">${esc(p.display_name)}</span>
              <span class="score">${p.score}</span>
            </div>`).join('')}
          <button class="primary" id="again">Play again</button>
        </div>
        ${r.review?.length ? `
          <div class="card">
            <label class="field">What you learned</label>
            ${r.review.map((q) => `
              <div style="padding:10px 0;border-bottom:1px solid var(--line)">
                <div class="small">${esc(q.prompt)} → <strong>${esc(q.options[q.answer])}</strong></div>
                ${q.fact ? `<div class="small muted">${esc(q.fact)}</div>` : ''}
                ${q.source ? `<div class="small muted">Source:
                  ${q.source_url ? `<a href="${esc(q.source_url)}" target="_blank" rel="noopener">${esc(q.source)}</a>` : esc(q.source)}</div>` : ''}
              </div>`).join('')}
          </div>` : ''}`;
    }

    default: return '<div class="card"><p class="muted">…</p></div>';
  }
}

/* ------------------------------------------------------------- handlers --- */
function bind() {
  const on = (sel, ev, fn) => { const el = $(sel); if (el) el.addEventListener(ev, fn); };

  on('#mute', 'click', () => { Sound.toggle(); render(); });
  on('#inc', 'click', () => { state.maxPlayers = Math.min(4, state.maxPlayers + 1); Sound.play('select'); render(); });
  on('#dec', 'click', () => { state.maxPlayers = Math.max(2, state.maxPlayers - 1); Sound.play('select'); render(); });

  on('#create-private', 'click', () => createRoom(false));
  on('#create-public', 'click', () => createRoom(true));

  on('#code', 'input', (e) => { state.joinCode = e.target.value.toUpperCase(); });
  on('#join', 'click', () => joinRoom(state.joinCode));
  document.querySelectorAll('[data-join]').forEach((b) =>
    b.addEventListener('click', () => joinRoom(b.dataset.join)));

  on('#leave', 'click', () => { stopPolling(); state.room = null; state.view = 'home'; loadPublicRooms(); render(); });
  on('#again', 'click', () => { stopPolling(); state.room = null; state.view = 'home'; loadPublicRooms(); render(); });

  on('#topic', 'input', (e) => { state.topicInput = e.target.value; });
  on('#set-topic', 'click', () => setTopic(state.topicInput));
  document.querySelectorAll('[data-topic]').forEach((b) =>
    b.addEventListener('click', () => { state.topicInput = b.dataset.topic; setTopic(b.dataset.topic); }));

  document.querySelectorAll('[data-subtopic]').forEach((b) =>
    b.addEventListener('click', () => pickSubTopic(b.dataset.subtopic)));

  document.querySelectorAll('[data-choice]').forEach((b) =>
    b.addEventListener('click', () => answer(parseInt(b.dataset.choice, 10))));

  on('#email-login', 'click', async () => {
    const email = $('#email')?.value?.trim();
    const msg = $('#login-msg');
    if (!email) { if (msg) msg.textContent = 'Enter an email first.'; return; }
    try {
      await loginWithEmail(email);
      if (msg) msg.textContent = 'Check your inbox for the sign-in link.';
    } catch (e) {
      if (msg) msg.textContent = e.message;
    }
  });
  on('#google-login', 'click', async () => {
    const cfg = await api('/api/config');
    loginWithGoogle(cfg.d3bit_url);
  });
  on('#logout', 'click', async () => {
    await post('/d3bit/auth/logout').catch(() => {});
    state.user = null;
    await ensureSession();
    render();
  });
}

async function createRoom(isPublic) {
  if (state.busy) return;
  state.busy = true;
  try {
    const room = await post('/api/rooms', { public: isPublic, max_players: state.maxPlayers });
    enterRoom(room);
  } catch (e) {
    state.error = e.message; render();
  } finally { state.busy = false; }
}

async function joinRoom(code) {
  if (!code || code.length < 4) { state.error = 'That code looks incomplete.'; render(); return; }
  try {
    const room = await post(`/api/rooms/${code}/join`);
    enterRoom(room);
  } catch (e) {
    state.error = e.message; render();
  }
}

function enterRoom(room) {
  state.error = '';
  state.view = 'room';
  state.lastPhase = null;
  loadSuggestions();
  applyRoom(room);
  startPolling(room.code);
}

async function setTopic(topic) {
  if (!topic?.trim()) return;
  try {
    Sound.play('select');
    applyRoom(await post(`/api/rooms/${state.room.code}/topic`, { topic: topic.trim() }));
  } catch (e) { state.error = e.message; render(); }
}

async function pickSubTopic(key) {
  try {
    Sound.play('select');
    applyRoom(await post(`/api/rooms/${state.room.code}/subtopic`, { key }));
  } catch (e) { state.error = e.message; render(); }
}

async function answer(choice) {
  const q = state.room?.question;
  if (!q || q.answered) return;
  state.myChoice = choice;
  try {
    const room = await post(`/api/rooms/${state.room.code}/answer`, { index: q.index, choice });
    Sound.play(room.question?.answer === choice ? 'correct' : 'wrong');
    applyRoom(room);
  } catch (e) { state.error = e.message; render(); }
}

async function loadSuggestions() {
  try {
    const res = await api(`/api/rooms/${state.room?.code || 'x'}/topics`);
    state.suggestions = res.topics || [];
  } catch { state.suggestions = []; }
}

async function loadPublicRooms() {
  try {
    const res = await api('/api/rooms');
    state.publicRooms = res.rooms || [];
  } catch { state.publicRooms = []; }
  render();
}

/* ------------------------------------------------------------------ boot --- */
(async function boot() {
  render();
  await ensureSession();
  await loadPublicRooms();
  render();

  // Deep link: /?code=ABCD drops you straight into a room.
  const code = new URLSearchParams(location.search).get('code');
  if (code) joinRoom(code.toUpperCase());

  setInterval(() => { if (state.view === 'home') loadPublicRooms(); }, 5000);
})();
