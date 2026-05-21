// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Gate the Mermaid CDN fetch on whether this page actually has a
// diagram to render. Most docs pages don't, and there's no reason to
// download the ~26KB entry module (plus init-time chunks) on every
// docs page.

if (document.querySelector(".mermaid")) {
  const { default: mermaid } = await import("mermaid");

  mermaid.initialize({
    startOnLoad: false,
    theme: "base",
    themeVariables: {
      fontFamily: '"Source Serif 4", Georgia, serif',
      fontSize: "14px",
      background: "transparent"
    },
    sequence: {
      noteMargin: 14
    }
  });
  await mermaid.run();

  // Yield one frame so the browser commits rendered SVGs to the DOM
  // before we walk their text nodes.
  await new Promise((resolve) => { requestAnimationFrame(resolve); });

  // Post-process rendered SVGs once -- CSS custom properties in docs.css
  // handle all color changes on theme toggle without re-rendering.
  const mono = '"JetBrains Mono", "Fira Code", "Consolas", monospace';

  document.querySelectorAll(".mermaid svg").forEach((svg) => {
    const walker = document.createTreeWalker(svg, NodeFilter.SHOW_TEXT, null);
    const nodes = [];
    for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
      if (node.textContent.includes("`")) {
        nodes.push(node);
      }
    }
    nodes.forEach((textNode) => {
      const parts = textNode.textContent.split(/`([^`]+)`/g);
      if (parts.length <= 1) return;
      const parent = textNode.parentNode;
      const frag = document.createDocumentFragment();
      for (let i = 0; i < parts.length; i++) {
        if (parts[i] === "") continue;
        if (i % 2 === 0) {
          frag.appendChild(document.createTextNode(parts[i]));
        } else {
          const tspan = document.createElementNS("http://www.w3.org/2000/svg", "tspan");
          tspan.style.fontFamily = mono;
          tspan.textContent = parts[i];
          frag.appendChild(tspan);
        }
      }
      parent.replaceChild(frag, textNode);
    });

    // Widen note rects when monospace text overflows the original box.
    svg.querySelectorAll(".note").forEach((noteRect) => {
      const g = noteRect.closest("g");
      if (!g) return;
      const text = g.querySelector(".noteText");
      if (!text) return;
      const textWidth = text.getBBox().width;
      const rectWidth = parseFloat(noteRect.getAttribute("width") || 0);
      const pad = 16;
      if (textWidth + pad > rectWidth) {
        const diff = textWidth + pad - rectWidth;
        noteRect.setAttribute("width", textWidth + pad);
        const x = parseFloat(noteRect.getAttribute("x") || 0);
        noteRect.setAttribute("x", x - diff / 2);
      }
    });

    // Replace hardcoded rect group fills with a CSS variable so they
    // adapt to theme toggles without re-rendering.
    svg.querySelectorAll("rect").forEach((rect) => {
      const fill = (rect.getAttribute("fill") || "").replace(/\s/g, "");
      if (fill === "rgb(240,235,228)") {
        rect.removeAttribute("fill");
        rect.style.fill = "var(--rect-fill)";
      }
    });
  });
}
