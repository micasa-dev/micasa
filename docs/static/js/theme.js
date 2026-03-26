// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

(() => {
  var stored = localStorage.getItem('theme');
  var prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  var isDark = stored === 'dark' || (!stored && prefersDark);
  if (isDark) {
    document.documentElement.setAttribute('data-theme', 'dark');
  }

  // Skip sun animation on page navigation within the same mode.
  // Click toggles always replay by removing sun-played first.
  try {
    if (!isDark && sessionStorage.getItem('theme-seen') === 'light') {
      document.documentElement.classList.add('sun-played');
    }
  } catch (_) {}

  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
    if (localStorage.getItem('theme')) return;
    if (e.matches) {
      document.documentElement.setAttribute('data-theme', 'dark');
    } else {
      document.documentElement.removeAttribute('data-theme');
      document.documentElement.classList.remove('sun-played');
    }
    document.dispatchEvent(new CustomEvent('theme-changed'));
  });

  window.toggleTheme = () => {
    var wasDark = document.documentElement.getAttribute('data-theme') === 'dark';
    if (wasDark) {
      document.documentElement.removeAttribute('data-theme');
      document.documentElement.classList.remove('sun-played');
      localStorage.setItem('theme', 'light');
    } else {
      document.documentElement.setAttribute('data-theme', 'dark');
      localStorage.setItem('theme', 'dark');
    }
    document.dispatchEvent(new CustomEvent('theme-changed'));
  };

  document.addEventListener('animationend', (e) => {
    if (e.animationName === 'sun-drift') {
      document.documentElement.classList.add('sun-played');
      try { sessionStorage.setItem('theme-seen', 'light'); } catch (_) {}
    }
  });
})();
