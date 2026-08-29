/* Account menus are native details elements; this adds the expected outside-click dismissal. */
(function () {
  document.addEventListener('pointerdown', (event) => {
    document.querySelectorAll('details.menu[open]').forEach((menu) => {
      if (!menu.contains(event.target)) menu.removeAttribute('open');
    });
  });

  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape') return;
    document.querySelectorAll('details.menu[open]').forEach((menu) => menu.removeAttribute('open'));
  });
})();
