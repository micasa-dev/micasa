// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

(() => {
  const all = [
    {
      text: 'My partner asked why I was typing in a terminal at midnight. I said \u201cthe water heater warranty expires in eleven days.\u201d She said \u201cwe rent.\u201d',
      cite: 'Anonymous'
    },
    {
      text: 'A client pulled out a laptop and showed me a sorted maintenance log going back three years. I have been a plumber for twenty years and I have never felt so unprepared for a house call.',
      cite: 'A Plumber, Allegedly'
    },
    {
      text: 'I was going to use a spreadsheet like a normal person. Then I was going to use Notion like a normal person. Then I installed a TUI from GitHub at 2am. I am no longer a normal person.',
      cite: 'Hacker News Commenter, Almost Certainly'
    },
    {
      text: 'I can tell you the exact date I last cleaned the gutters, which filter is in the furnace, and what I paid the electrician in 2024. Nobody has ever asked me for any of this. I am ready.',
      cite: 'Overqualified Homeowner'
    },
    {
      text: 'My wife asked if I\u2019d called the roofer. I pulled up a timestamped service log, a quote comparison, and a vendor contact sheet. She said \u201cso\u2026 no?\u201d',
      cite: 'Local Dad'
    },
    {
      text: 'Finally replaced the sticky note on the fridge. It said \u201ccall roofer.\u201d It had been there since we moved in. The roofer has retired.',
      cite: 'Suburban Archaeologist'
    },
    {
      text: 'It\u2019s like having a filing cabinet except it\u2019s one file and I understand what\u2019s in it.',
      cite: 'Reluctant Convert'
    },
    {
      text: 'Found out the dishwasher was still under warranty three days before it expired. The app paid for itself. It was free.',
      cite: 'Frugal Victor'
    },
    {
      text: 'I\u2019ve been using the demo data for three weeks. I don\u2019t own a house.',
      cite: 'Aspiring Homeowner'
    },
    {
      text: 'My friend said \u201cyou should try this app for your house\u201d and then sent me a GitHub link. We are no longer friends. I am, however, extremely organized.',
      cite: 'Reluctant Early Adopter'
    },
    {
      text: 'My realtor asked how I knew the exact date the previous owner replaced the sump pump. I said \u201cSQLite.\u201d She stopped returning my calls.',
      cite: 'Due Diligence Enthusiast'
    },
    {
      text: 'The homeowner handed me a printed quote comparison. With dates. And vendor ratings. In a monospaced font. I need to lie down.',
      cite: 'A Contractor Who Has Seen Things'
    },
    {
      text: 'My house is still falling apart. But now I have a database about it, and somehow that feels like progress.',
      cite: 'Optimistic Realist'
    },
    {
      text: 'I showed my neighbor my appliance warranty tracker. He showed me his junk drawer. We both think the other one is insane.',
      cite: 'Suburban Standoff'
    },
    {
      text: 'micasa backup backup.db. That\u2019s my entire backup strategy. I sleep like a baby.',
      cite: 'Minimalist Sysadmin'
    },
    {
      text: 'The HVAC guy asked when the filter was last changed. I gave him a date. He looked at me like I was a wizard. I am not a wizard. I am a man with a terminal.',
      cite: 'Filter Wizard'
    },
    {
      text: 'I added my house to micasa and immediately discovered I was six months late on gutter cleaning. Ignorance really was bliss.',
      cite: 'Recently Informed'
    },
    {
      text: 'I used to keep track of home repairs in my head. My head is not a database. My head was wrong about the roof.',
      cite: 'Former Optimist'
    },
    {
      text: 'Showed my partner the vendor history and she said \u201cwhy do we have four different plumbers.\u201d I did not have a good answer but at least now we have the data to discuss it.',
      cite: 'Data-Driven Argument Starter'
    }
  ];

  const SHOW = 3;
  for (let i = all.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    const tmp = all[i]; all[i] = all[j]; all[j] = tmp;
  }
  const pick = all.slice(0, SHOW);

  const grid = document.getElementById('testimonials-grid');
  if (!grid) return;
  for (let k = 0; k < pick.length; k++) {
    const bq = document.createElement('blockquote');
    bq.className = 'testimonial';
    const p = document.createElement('p');
    p.textContent = pick[k].text;
    const cite = document.createElement('cite');
    cite.textContent = `\u2014 ${pick[k].cite}`;
    bq.appendChild(p);
    bq.appendChild(cite);
    grid.appendChild(bq);
  }

  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
  const items = grid.querySelectorAll('.testimonial');
  const STAGGER = 150;
  let revealed = false;
  const observer = new IntersectionObserver((entries) => {
    if (revealed) return;
    for (let e = 0; e < entries.length; e++) {
      if (entries[e].isIntersecting) {
        revealed = true;
        for (let n = 0; n < items.length; n++) {
          ((el, delay) => {
            setTimeout(() => { el.classList.add('visible'); }, delay);
          })(items[n], n * STAGGER);
        }
        observer.disconnect();
        break;
      }
    }
  }, { threshold: 0.15 });
  observer.observe(grid);
})();
