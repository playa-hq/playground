/* Home-page motion: a magnetic play button and a wordmark squish on press.
 * All decorative — the page works with this file gone. */
(function () {
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (reduce) return;

  const mark = document.querySelector('.wordmark');

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
