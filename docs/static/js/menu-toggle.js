// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

(() => {
  const sidebar = document.getElementById('docs-sidebar');
  const toggle = document.getElementById('menu-toggle');
  if (!sidebar || !toggle) return;

  const setOpen = (open) => {
    sidebar.classList.toggle('open', open);
    toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
  };

  toggle.addEventListener('click', (e) => {
    e.stopPropagation();
    setOpen(!sidebar.classList.contains('open'));
  });

  document.addEventListener('click', (e) => {
    if (!sidebar.classList.contains('open')) return;
    if (sidebar.contains(e.target) || toggle.contains(e.target)) return;
    setOpen(false);
  });
})();
