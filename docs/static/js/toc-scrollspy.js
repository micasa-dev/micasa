// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

(() => {
  const toc = document.querySelector('.toc');
  if (!toc) return;
  const links = toc.querySelectorAll('a[href^="#"]');
  if (!links.length) return;

  const ids = [];
  links.forEach((a) => { ids.push(a.getAttribute('href').slice(1)); });

  const visible = new Set();

  const highlight = () => {
    let active = null;
    for (let i = 0; i < ids.length; i++) {
      if (visible.has(ids[i])) { active = ids[i]; break; }
    }
    links.forEach((a) => {
      a.classList.toggle('active', a.getAttribute('href') === `#${active}`);
    });
  };

  const observer = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (entry.isIntersecting) {
        visible.add(entry.target.id);
      } else {
        visible.delete(entry.target.id);
      }
    });
    highlight();
  }, { rootMargin: '0px 0px -70% 0px', threshold: 0 });

  ids.forEach((id) => {
    const el = document.getElementById(id);
    if (el) observer.observe(el);
  });

  links.forEach((a) => {
    a.addEventListener('click', () => {
      const id = a.getAttribute('href').slice(1);
      visible.clear();
      visible.add(id);
      highlight();
    });
  });
})();
