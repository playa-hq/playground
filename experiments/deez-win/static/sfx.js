/* The only JavaScript in the app.
 *
 * htmx owns every state transition; this just watches what it swaps in and
 * makes a noise. Sounds are 8-bit blips synthesized in WebAudio so the game has
 * audio with no assets and no key; anything dropped into /sfx/<name>.wav by
 * scripts/gen-audio.sh takes over automatically.
 */
(function () {
  let ctx = null;
  let muted = localStorage.getItem('wg_muted') === '1';
  const files = {};

  ['roll', 'select', 'correct', 'wrong', 'win'].forEach((name) => {
    const a = new Audio(`/sfx/${name}.wav`);
    a.addEventListener('canplaythrough', () => { files[name] = a; }, { once: true });
    a.addEventListener('error', () => {}, { once: true });
  });

  function blip(freq, dur, type = 'square', slideTo = null) {
    if (!ctx) ctx = new (window.AudioContext || window.webkitAudioContext)();
    if (ctx.state === 'suspended') ctx.resume();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = type;
    osc.frequency.setValueAtTime(freq, ctx.currentTime);
    if (slideTo) osc.frequency.exponentialRampToValueAtTime(slideTo, ctx.currentTime + dur);
    gain.gain.setValueAtTime(0.16, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + dur);
    osc.connect(gain).connect(ctx.destination);
    osc.start();
    osc.stop(ctx.currentTime + dur);
  }

  function play(name) {
    if (muted || !name) return;
    if (files[name]) { files[name].currentTime = 0; files[name].play().catch(() => {}); return; }
    switch (name) {
      case 'roll':    blip(180, 0.08, 'square', 420); break;
      case 'select':  blip(660, 0.06); break;
      case 'correct': [523, 659, 784].forEach((f, i) => setTimeout(() => blip(f, 0.12), i * 70)); break;
      case 'wrong':   blip(200, 0.28, 'sawtooth', 90); break;
      case 'win':     [523, 659, 784, 1047].forEach((f, i) => setTimeout(() => blip(f, 0.18), i * 110)); break;
    }
  }

  // Fire on change, not on every poll: the panel re-renders once a second and
  // we only want a sound when the phase or the question actually moved.
  let lastPhase = null;
  let lastQuestion = null;

  function scan() {
    const el = document.querySelector('[data-sfx-phase]');
    if (!el) return;

    const phase = el.dataset.sfxPhase;
    const question = el.dataset.question ?? null;

    if (phase !== lastPhase) {
      if (phase === 'rolling') play('roll');
      if (phase === 'results') play('win');
      lastPhase = phase;
      lastQuestion = null;
    }
    if (question !== null && question !== lastQuestion) lastQuestion = question;

    // data-sfx is set by the server on the swap that reveals your answer.
    if (el.dataset.sfx) {
      play(el.dataset.sfx);
      delete el.dataset.sfx;
    }
  }

  // htmx 4 renamed events to htmx:phase:action.
  document.body.addEventListener('htmx:after:swap', scan);
  document.addEventListener('DOMContentLoaded', scan);
  scan();

  document.addEventListener('click', (e) => {
    const mute = e.target.closest('[data-mute]');
    if (mute) {
      muted = !muted;
      localStorage.setItem('wg_muted', muted ? '1' : '0');
      mute.textContent = muted ? '🔇' : '🔊';
      return;
    }
    if (e.target.closest('button:not([disabled])')) play('select');
  });

  const btn = document.querySelector('[data-mute]');
  if (btn) btn.textContent = muted ? '🔇' : '🔊';
})();
