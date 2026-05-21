// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

(() => {
  const overlay = document.getElementById('search-overlay');
  const modal = document.getElementById('search-modal');
  const hint = document.getElementById('search-hint');
  if (!overlay || !modal) return;

  let initialized = false;
  let lastFocused = null;

  const focusableIn = (root) => {
    const sel =
      'a[href], button:not([disabled]), input:not([disabled]), ' +
      'textarea:not([disabled]), select:not([disabled]), ' +
      '[tabindex]:not([tabindex="-1"])';
    return Array.prototype.slice.call(root.querySelectorAll(sel))
      .filter((el) => el.offsetParent !== null || el === document.activeElement);
  };

  const open = () => {
    lastFocused = document.activeElement;
    if (!initialized) {
      new PagefindUI({ element: '#search', showSubResults: true, showImages: false });
      initialized = true;
    }
    overlay.classList.remove('closing');
    overlay.classList.add('open');
    if (hint) hint.setAttribute('aria-expanded', 'true');
    requestAnimationFrame(() => {
      const input = document.querySelector('.pagefind-ui__search-input');
      if (input) input.focus();
    });
  };

  const close = () => {
    overlay.classList.add('closing');
    modal.addEventListener('animationend', function handler() {
      modal.removeEventListener('animationend', handler);
      overlay.classList.remove('open', 'closing');
    });
    if (hint) hint.setAttribute('aria-expanded', 'false');
    if (lastFocused && typeof lastFocused.focus === 'function') {
      lastFocused.focus();
    }
  };

  document.addEventListener('keydown', (e) => {
    if (e.key === '/' && !overlay.classList.contains('open')) {
      const tag = (e.target.tagName || '').toLowerCase();
      if (tag === 'input' || tag === 'textarea' || e.target.isContentEditable) return;
      e.preventDefault();
      open();
      return;
    }
    if (!overlay.classList.contains('open')) return;
    if (e.key === 'Escape') {
      close();
      return;
    }
    if (e.key === 'Tab') {
      const focusables = focusableIn(modal);
      if (focusables.length === 0) {
        e.preventDefault();
        return;
      }
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }
  });

  overlay.addEventListener('click', (e) => {
    if (!modal.contains(e.target)) close();
  });

  if (hint) {
    hint.addEventListener('click', () => {
      if (overlay.classList.contains('open')) close(); else open();
    });
  }
})();
