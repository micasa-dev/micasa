// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

(() => {
  const scene = document.getElementById('house-scene');
  const house = document.getElementById('hero-house');
  const caption = document.getElementById('crumble-caption');
  if (!scene || !house) return;
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  const GRAVITY       = 1400;
  const RESTITUTION   = 0.3;
  const BOUNCE_DRAG   = 0.5;
  const MAX_BOUNCES   = 3;
  const BLAST_SPEED   = 350;
  const STAGGER_SEC   = 0.25;
  const SETTLE_FADE   = 0.3;

  const ROW_DELAY_MS      = 100;
  const FLIGHT_SEC        = 0.6;
  const DISSOLVE_MS       = 200;
  const REBUILD_EASING    = 'cubic-bezier(0.0, 0.0, 0.2, 1.0)';

  const EMBER_BASE     = '#a04e30';
  const EMBER_COLORS   = ['#e07040', '#d4783c', '#c05e3c'];
  const EMBER_CHANCE   = 0.15;
  const SMOKE_GLYPHS   = ['\u2591', '\u2591', '\u2592', '\u2592', '\u2593'];
  const MAX_RUBBLE_SMOKE = 10;

  let animating = false;
  let destroyed = false;
  let rubbleEls = [];
  const originalHTML = house.innerHTML;

  let emberTimer = 0;
  let rubbleSmokeAnimId = 0;
  let rubbleSmokeSpawnTimer = 0;
  let rubbleSmokeParticles = [];

  function rand(lo, hi) { return lo + Math.random() * (hi - lo); }

  function dist(ax, ay, bx, by) {
    const dx = ax - bx, dy = ay - by;
    return Math.sqrt(dx * dx + dy * dy);
  }

  const BLOCK_RE = /[\u2580-\u259F]/;
  function wrapBlockChars(root) {
    const spans = [];
    let row = 0;

    function walk(node) {
      if (node.nodeType === 3) {
        const text = node.textContent;
        if (!text) return;
        const frag = document.createDocumentFragment();
        for (let i = 0; i < text.length; i++) {
          if (BLOCK_RE.test(text[i])) {
            const s = document.createElement('span');
            s.textContent = text[i];
            s._row = row;
            frag.appendChild(s);
            spans.push(s);
          } else {
            frag.appendChild(document.createTextNode(text[i]));
          }
        }
        node.parentNode.replaceChild(frag, node);
      } else if (node.nodeType === 1) {
        if (node.tagName === 'BR') { row++; return; }
        if (node.style && node.style.display === 'none') return;
        const kids = [].slice.call(node.childNodes);
        for (let j = 0; j < kids.length; j++) walk(kids[j]);
      }
    }

    const kids = [].slice.call(root.childNodes);
    for (let i = 0; i < kids.length; i++) walk(kids[i]);
    return spans;
  }

  function measureAndHide(sceneRect) {
    const spans = wrapBlockChars(house);
    const blocks = [];
    let groundY = 0;
    for (let i = 0; i < spans.length; i++) {
      const r = spans[i].getBoundingClientRect();
      const b = {
        ch: spans[i].textContent,
        x: r.left - sceneRect.left,
        y: r.top - sceneRect.top,
        row: spans[i]._row
      };
      if (b.y > groundY) groundY = b.y;
      blocks.push(b);
    }
    house.innerHTML = originalHTML;
    house.style.visibility = 'hidden';
    return { blocks: blocks, groundY: groundY };
  }

  function scatterSmoke(smokeBed, sceneRect, clickX, clickY) {
    const scattered = [];
    if (!smokeBed) return scattered;

    const els = smokeBed.querySelectorAll('.smoke-particle');
    for (let i = 0; i < els.length; i++) {
      const r = els[i].getBoundingClientRect();
      const sx = r.left - sceneRect.left;
      const sy = r.top - sceneRect.top;
      const alpha = parseFloat(els[i].style.opacity) || 0;
      if (alpha < 0.05) continue;

      const clone = document.createElement('span');
      clone.className = 'smoke-particle';
      clone.textContent = els[i].textContent;
      clone.style.position = 'absolute';
      clone.style.left = `${sx}px`;
      clone.style.top = `${sy}px`;
      clone.style.opacity = String(alpha);
      clone.style.fontSize = '0.9rem';
      clone.style.transform = 'none';
      scene.appendChild(clone);

      const d = dist(sx, sy, clickX, clickY) || 1;
      const speed = rand(200, 350);
      scattered.push({
        el: clone, x: 0, y: 0,
        vx: ((sx - clickX) / d) * speed + rand(-40, 40),
        vy: ((sy - clickY) / d) * speed - rand(0, 100),
        startAlpha: alpha,
        maxLife: rand(0.4, 0.7),
        age: 0
      });
    }
    smokeBed.style.display = 'none';
    return scattered;
  }

  function createBlockEls(blocks) {
    const items = [];
    for (let i = 0; i < blocks.length; i++) {
      const el = document.createElement('span');
      el.className = 'crumble-block';
      el.textContent = blocks[i].ch;
      el.style.left = `${blocks[i].x}px`;
      el.style.top = `${blocks[i].y}px`;
      scene.appendChild(el);
      items.push({
        el: el, b: blocks[i],
        x: 0, y: 0, angle: 0,
        vx: 0, vy: 0, va: 0,
        active: false, settled: false, bounces: 0
      });
    }
    return items;
  }

  function blockDistances(items, clickX, clickY) {
    const dists = [];
    let max = 0;
    for (let i = 0; i < items.length; i++) {
      const d = dist(items[i].b.x, items[i].b.y, clickX, clickY);
      dists.push(d);
      if (d > max) max = d;
    }
    return { dists: dists, max: max };
  }

  function initBlastVelocity(item, clickX, clickY, d, blastRadius) {
    const proximity = 1 - Math.min(d / blastRadius, 1);
    const force = 0.3 + proximity * 0.7;
    const len = Math.max(d, 1);
    const nx = (item.b.x - clickX) / len;
    const ny = (item.b.y - clickY) / len;
    const speed = BLAST_SPEED * force;
    item.vx = nx * speed + rand(-50, 50);
    item.vy = ny * speed - rand(0, 300) * force;
    item.va = rand(-450, 450) * force;
    item.active = true;
  }

  function stepBlock(item, dt, groundY) {
    if (item.settled) return false;
    item.vy += GRAVITY * dt;
    item.x += item.vx * dt;
    item.y += item.vy * dt;
    item.angle += item.va * dt;

    if (item.b.y + item.y >= groundY) {
      item.y = groundY - item.b.y;
      if (Math.abs(item.vy) < 50 || item.bounces >= MAX_BOUNCES) {
        item.settled = true;
      } else {
        item.vy = -Math.abs(item.vy) * RESTITUTION;
        item.vx *= BOUNCE_DRAG;
        item.va *= 0.3;
        item.bounces++;
      }
    }
    return !item.settled;
  }

  function stepScatteredSmoke(sp, dt) {
    sp.age += dt;
    if (sp.age >= sp.maxLife) { sp.el.remove(); return false; }
    sp.x += sp.vx * dt;
    sp.y += sp.vy * dt;
    sp.vx *= (1 - 1.5 * dt);
    sp.vy *= (1 - 1.5 * dt);
    const fade = 1 - sp.age / sp.maxLife;
    sp.el.style.transform = `translate(${sp.x.toFixed(1)}px,${sp.y.toFixed(1)}px)`;
    sp.el.style.opacity = (sp.startAlpha * fade).toFixed(2);
    return true;
  }

  function startSmolder(houseRect, groundY) {
    for (let i = 0; i < rubbleEls.length; i++) {
      rubbleEls[i].el.style.transition = 'opacity 0.8s, color 0.8s';
      rubbleEls[i].el.style.opacity = '0.5';
      rubbleEls[i].el.style.color = EMBER_BASE;
    }

    emberTimer = setInterval(() => {
      for (let i = 0; i < rubbleEls.length; i++) {
        if (Math.random() < EMBER_CHANCE) {
          const el = rubbleEls[i].el;
          el.style.color = EMBER_COLORS[Math.floor(Math.random() * EMBER_COLORS.length)];
          el.style.opacity = rand(0.5, 0.9).toFixed(2);
          setTimeout(((ref) => () => { ref.style.color = EMBER_BASE; ref.style.opacity = '0.35'; })(el), rand(200, 700));
        }
      }
    }, 100);

    const centerX = houseRect.width / 2;
    rubbleSmokeSpawnTimer = setInterval(() => {
      if (rubbleSmokeParticles.length >= MAX_RUBBLE_SMOKE) return;
      const el = document.createElement('span');
      el.className = 'smoke-particle';
      el.textContent = SMOKE_GLYPHS[Math.floor(Math.random() * SMOKE_GLYPHS.length)];
      el.style.position = 'absolute';
      el.style.left = `${centerX + rand(-0.6, 0.6) * houseRect.width}px`;
      el.style.top = `${groundY}px`;
      el.style.color = getComputedStyle(document.documentElement).getPropertyValue('--warm-gray').trim();
      el.style.fontSize = `${rand(0.6, 1.0)}em`;
      el.style.opacity = '0';
      scene.appendChild(el);
      rubbleSmokeParticles.push({
        el: el, age: 0,
        lifetime: rand(2000, 5000),
        drift: rand(-25, 25),
        rise: rand(40, 80)
      });
    }, 250);

    let lastT = 0;
    function tickSmoke(now) {
      if (!lastT) lastT = now;
      const dt = Math.min((now - lastT) / 1000, 0.1);
      lastT = now;
      for (let i = rubbleSmokeParticles.length - 1; i >= 0; i--) {
        const p = rubbleSmokeParticles[i];
        p.age += dt * 1000;
        const t = p.age / p.lifetime;
        if (t >= 1) { p.el.remove(); rubbleSmokeParticles.splice(i, 1); continue; }
        const alpha = t < 0.15 ? (t / 0.15) * 0.4 : 0.4 * (1 - (t - 0.15) / 0.85);
        p.el.style.transform = `translate(${(p.drift * t).toFixed(1)}px, -${(p.rise * t).toFixed(1)}px)`;
        p.el.style.opacity = alpha.toFixed(3);
      }
      rubbleSmokeAnimId = requestAnimationFrame(tickSmoke);
    }
    rubbleSmokeAnimId = requestAnimationFrame(tickSmoke);
  }

  function stopSmolder() {
    clearInterval(emberTimer);    emberTimer = 0;
    clearInterval(rubbleSmokeSpawnTimer); rubbleSmokeSpawnTimer = 0;
    cancelAnimationFrame(rubbleSmokeAnimId); rubbleSmokeAnimId = 0;
    for (let i = 0; i < rubbleSmokeParticles.length; i++) rubbleSmokeParticles[i].el.remove();
    rubbleSmokeParticles = [];
  }

  function destroyHouse(evt) {
    animating = true;

    const sceneRect = scene.getBoundingClientRect();
    const houseRect = house.getBoundingClientRect();
    const clickX = evt.clientX - sceneRect.left;
    const clickY = evt.clientY - sceneRect.top;

    const scattered = scatterSmoke(document.getElementById('smoke-bed'), sceneRect, clickX, clickY);
    const measured = measureAndHide(sceneRect);
    const items = createBlockEls(measured.blocks);
    const bd = blockDistances(items, clickX, clickY);
    const blastRadius = Math.sqrt(houseRect.width * houseRect.width + houseRect.height * houseRect.height);

    const started = performance.now();
    let lastTime = started;

    function tick(now) {
      const dt = Math.min((now - lastTime) / 1000, 0.05);
      lastTime = now;
      const elapsed = (now - started) / 1000;
      let allDone = true;

      for (let i = 0; i < items.length; i++) {
        const item = items[i];
        if (item.settled) continue;

        const delay = (bd.max > 0 ? bd.dists[i] / bd.max : 0) * STAGGER_SEC;
        if (elapsed < delay) { allDone = false; continue; }

        if (!item.active) initBlastVelocity(item, clickX, clickY, bd.dists[i], blastRadius);
        if (stepBlock(item, dt, measured.groundY)) allDone = false;

        const alpha = Math.max(0.4, 1 - (elapsed - delay) * SETTLE_FADE);
        item.el.style.transform =
          `translate(${item.x.toFixed(1)}px,${item.y.toFixed(1)}px) rotate(${item.angle.toFixed(0)}deg)`;
        item.el.style.opacity = alpha.toFixed(2);
      }

      for (let j = scattered.length - 1; j >= 0; j--) {
        if (!stepScatteredSmoke(scattered[j], dt)) scattered.splice(j, 1);
      }

      if (!allDone || scattered.length > 0) {
        requestAnimationFrame(tick);
        return;
      }

      setTimeout(() => {
        rubbleEls = items;
        startSmolder(houseRect, measured.groundY);

        const onClick = (e) => { e.stopPropagation(); if (!animating && destroyed) rebuildHouse(); };
        for (let i = 0; i < items.length; i++) items[i].el.addEventListener('click', onClick);

        if (caption) caption.classList.add('visible');
        destroyed = true;
        animating = false;
      }, 200);
    }

    requestAnimationFrame(tick);
  }

  function rebuildHouse() {
    animating = true;
    stopSmolder();
    if (caption) caption.classList.remove('visible');

    let maxRow = 0;
    for (let i = 0; i < rubbleEls.length; i++) {
      if (rubbleEls[i].b.row > maxRow) maxRow = rubbleEls[i].b.row;
    }

    for (let i = 0; i < rubbleEls.length; i++) {
      const delay = (maxRow - rubbleEls[i].b.row) * ROW_DELAY_MS;
      rubbleEls[i].el.style.transition =
        'transform ' + FLIGHT_SEC + 's ' + REBUILD_EASING + ' ' + delay + 'ms, ' +
        'opacity 0.3s ease ' + delay + 'ms, color 0.3s ease ' + delay + 'ms';
      rubbleEls[i].el.style.opacity = '1';
      rubbleEls[i].el.style.color = '';
      rubbleEls[i].el.style.transform = 'translate(0,0) rotate(0deg)';
    }

    const flightEnd = maxRow * ROW_DELAY_MS + FLIGHT_SEC * 1000 + 50;

    setTimeout(() => {
      house.innerHTML = originalHTML;
      house.style.visibility = 'visible';
      house.style.opacity = '1';
      house.style.transition = 'none';
      const bed = document.getElementById('smoke-bed');
      if (bed) bed.style.display = 'none';
      void house.offsetHeight;

      for (let i = 0; i < rubbleEls.length; i++) {
        rubbleEls[i].el.style.transition = `opacity ${DISSOLVE_MS}ms ease`;
        rubbleEls[i].el.style.opacity = '0';
      }

      setTimeout(() => {
        for (let i = 0; i < rubbleEls.length; i++) rubbleEls[i].el.remove();
        rubbleEls = [];
        const bed = document.getElementById('smoke-bed');
        if (bed) bed.style.display = '';
        destroyed = false;
        animating = false;
      }, DISSOLVE_MS + 50);
    }, flightEnd);
  }

  scene.addEventListener('click', (evt) => {
    if (animating) return;
    if (destroyed) rebuildHouse(); else destroyHouse(evt);
  });
})();
