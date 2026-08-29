/* Shared motion: every action leans toward the pointer and springs home.
 * Event delegation also covers buttons that htmx swaps in later. */
(function () {
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (reduce) return;

  const mark = document.querySelector('.wordmark');

  // Squish the whole word on press, like a real button.
  if (mark) {
    mark.addEventListener('pointerdown', () => mark.classList.add('pressed'));
    ['pointerup', 'pointerleave'].forEach((e) => mark.addEventListener(e, () => mark.classList.remove('pressed')));
  }

  function actionAt(target) {
    return target instanceof Element ? target.closest('button, .btn') : null;
  }

  document.addEventListener('pointermove', (e) => {
    if (e.pointerType === 'touch') return;
    const btn = actionAt(e.target);
    if (!btn || btn.matches(':disabled')) return;
    const r = btn.getBoundingClientRect();
    const dx = (e.clientX - (r.left + r.width / 2)) / Math.max(1, r.width);
    const dy = (e.clientY - (r.top + r.height / 2)) / Math.max(1, r.height);
    const pull = btn.classList.contains('magnet') ? 9 : 4;
    btn.style.transition = 'transform .08s linear';
    btn.style.transform = `translate3d(${dx * pull}px, ${dy * pull * .7}px, 0) rotate(${dx * .8}deg)`;

    const label = btn.classList.contains('magnet') ? btn.querySelector('span') : null;
    if (label) {
      label.style.transition = 'transform .08s linear';
      label.style.transform = `translate3d(${dx * 5}px, ${dy * 3}px, 0)`;
    }
  });

  document.addEventListener('pointerout', (e) => {
    const btn = actionAt(e.target);
    if (!btn || (e.relatedTarget instanceof Node && btn.contains(e.relatedTarget))) return;
    btn.style.transition = 'transform .55s cubic-bezier(.2,2,.4,1)';
    btn.style.transform = '';
    const label = btn.classList.contains('magnet') ? btn.querySelector('span') : null;
    if (label) {
      label.style.transition = 'transform .55s cubic-bezier(.2,2,.4,1)';
      label.style.transform = '';
    }
  });
})();
