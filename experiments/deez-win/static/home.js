/* Home-page whimsy: sparkles around the wordmark, a magnetic play button,
 * a squish on press. All decorative — the page works with this file gone. */
(function () {
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (reduce) return;

  const mark = document.querySelector('.wordmark');
  const field = document.querySelector('.sparkles');
  const hero = document.querySelector('.hero-big');

  // Sparkles: a few at load, then one every so often, bursts on hover.
  function sparkle(x, y) {
    if (!field) return;
    const s = document.createElement('i');
    const size = 8 + Math.random() * 14;
    s.className = 'sparkle' + (Math.random() < 0.4 ? ' mint' : '');
    s.style.setProperty('--s', size + 'px');
    s.style.left = x + 'px';
    s.style.top = y + 'px';
    field.appendChild(s);
    s.addEventListener('animationend', () => s.remove(), { once: true });
  }
  function around() {
    if (!mark || !hero) return;
    const m = mark.getBoundingClientRect(), h = hero.getBoundingClientRect();
    const pad = 28;
    sparkle(m.left - h.left - pad + Math.random() * (m.width + pad * 2),
            m.top - h.top - pad + Math.random() * (m.height + pad * 2));
  }
  let burst = 0;
  setTimeout(() => { for (let i = 0; i < 4; i++) setTimeout(around, i * 160); }, 700);
  setInterval(() => { if (document.visibilityState === 'visible') around(); }, 1400);
  mark && mark.addEventListener('mouseenter', () => { if (burst++ % 2 === 0) for (let i = 0; i < 3; i++) setTimeout(around, i * 90); });

  // Squish the whole word on press, like a real button.
  if (mark) {
    mark.addEventListener('pointerdown', () => mark.classList.add('pressed'));
    ['pointerup', 'pointerleave'].forEach((e) => mark.addEventListener(e, () => mark.classList.remove('pressed')));
  }

  // Magnetic button: the label leans toward the cursor and springs back.
  document.querySelectorAll('.magnet').forEach((btn) => {
    const inner = btn.firstElementChild || btn;
    btn.addEventListener('pointermove', (e) => {
      const r = btn.getBoundingClientRect();
      const dx = (e.clientX - (r.left + r.width / 2)) / r.width;
      const dy = (e.clientY - (r.top + r.height / 2)) / r.height;
      btn.style.transform = `translate(${dx * 10}px, ${dy * 8}px)`;
      inner.style.transform = `translate(${dx * 6}px, ${dy * 4}px)`;
      btn.style.transition = inner.style.transition = 'transform .08s linear';
    });
    btn.addEventListener('pointerleave', () => {
      btn.style.transition = inner.style.transition = 'transform .6s cubic-bezier(.2,2,.4,1)';
      btn.style.transform = inner.style.transform = '';
    });
  });
})();
