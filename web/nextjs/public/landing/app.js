/* ============================================================================
   JobShout landing · bespoke page logic
   ----------------------------------------------------------------------------
   Scroll state is read from --sc-p on each act. The computer demo is a
   pure function of progress so scrubbing backwards rewinds the scene.
   ========================================================================== */
(function (global) {
  'use strict';

  var reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;
  var clamp01 = function (x) { return x < 0 ? 0 : x > 1 ? 1 : x; };

  var JOBS = {
    mail: {
      input: 'the senders to watch and how you answer them',
      output: 'reply drafts parked in the mailbox for approval',
      title: 'Mail Agent',
      body: 'Watch the senders you care about, research the thread, and leave a reply draft. Nothing is sent until you approve.',
      app: 'mail / inbox',
      chip: '2 drafts',
      wait: true,
      preview: '<p><strong>Acme Freight</strong> · Overdue INV-2041</p><p><strong>Nordwind</strong> · Overdue INV-2033</p><p class="p-soft">Drafts parked. Approve in Mail Agent to send.</p>'
    },
    career: {
      input: 'a job URL or a pasted job description',
      output: 'a fit score, the gaps, and a tailored CV draft',
      title: 'Career Agent',
      body: 'Evaluate a job URL or pasted JD against your career profile. Tailor a CV if you ask. Nothing is submitted for you.',
      app: 'career / evaluate',
      chip: 'draft only',
      wait: false,
      preview: '<p><strong>Staff engineer · Lattice</strong></p><p>Fit 7/10 · gaps: public speaking, EU travel</p><p class="p-soft">CV draft ready. You send it.</p>'
    },
    research: {
      input: 'a subject and how deep to go',
      output: 'a brief whose claims are checked against their sources',
      title: 'Research Agent',
      body: 'Plan searches, read the sources, and return cited findings checked against the pages they came from.',
      app: 'research / brief',
      chip: '12 sources',
      wait: false,
      preview: '<p><strong>Kubernetes cost patterns, 2026</strong></p><p>Bin-packing + spot for batch. Citation checked.</p><p class="p-soft">Brief filed on the board.</p>'
    },
    article: {
      input: 'a topic — the writer picks its own title',
      output: 'an SEO-ready draft filed in the CMS for review',
      title: 'Article Writer',
      body: 'Give a topic. The writer researches, picks its own title, and files an SEO-ready draft in the CMS for review.',
      app: 'articles / draft',
      chip: 'in review',
      wait: false,
      preview: '<p><strong>Edge inference without the tax</strong></p><p>1,140 words · 3 code samples · Further reading</p><p class="p-soft">Draft in the CMS. You publish.</p>'
    },
    security: {
      input: 'an authorised target and a budget: quick, standard or deep',
      output: 'findings ranked by severity, with reproduction steps',
      title: 'Security Tester',
      body: 'Start an authorised scan against a live target. Quick, standard, or deep — you set the budget.',
      app: 'pentest / run',
      chip: 'authorised',
      wait: false,
      preview: '<p><strong>int.example.com</strong> · quick · 11 min</p><p>2 high, 4 medium. Repro steps attached.</p><p class="p-soft">Report on the board.</p>'
    },
    review: {
      input: 'a GitHub pull request',
      output: 'a verdict and line-level comments, previewed before posting',
      title: 'PR Reviewer',
      body: 'Queue an AI review of a GitHub pull request. Preview first. Nothing posts to the PR unless you say so.',
      app: 'review / acme#184',
      chip: 'preview',
      wait: true,
      preview: '<p><strong>MERGE</strong> · one nit on the timeout path</p><p>Explored the diff and the surrounding tests.</p><p class="p-soft">Preview only. You post it.</p>'
    },
    image: {
      input: 'a prompt describing the picture',
      output: 'one image stored on the task, ready to attach',
      title: 'Image Generator',
      body: 'Generate one image from a prompt. The board task stores the result.',
      app: 'images / generate',
      chip: '1 image',
      wait: false,
      preview: '<p><strong>Harbour at night, editorial cover</strong></p><p>Stored on the task. Ready to attach.</p>'
    }
  };

  // The landing page is "/" for everyone, signed in or not. Rather than bounce
  // an authenticated visitor away from the marketing page, point the header at
  // the workspace. Presence of the token is enough — it is only a link target,
  // and the app re-validates on arrival.
  function reflectSession() {
    var token;
    try { token = localStorage.getItem('access_token'); } catch (e) { return; }
    if (!token) return;
    var cta = document.querySelector('[data-js-cta]');
    var signin = document.querySelector('[data-js-signin]');
    if (cta) { cta.setAttribute('href', '/chat'); cta.textContent = 'Open workspace'; }
    if (signin) { signin.remove(); }
  }

  function init() {
    var $ = function (s) { return document.querySelector(s); };
    reflectSession();
    var actDemo = $('#demo');
    var desk = $('#desk');
    var chatText = $('#chatcard-text');
    var teachTime = $('#teach-time');
    var cursor = $('#cursor');
    var modeControl = $('#mode-control');
    var modeWork = $('#mode-work');

    var scenes = {
      work: $('#scene-work'),
      teach: $('#scene-teach'),
      memory: $('#scene-memory'),
      connect: $('#scene-connect')
    };

    var CARDS = {
      signin: 'Sign in to Gmail so I can watch the support queue.',
      inbox: 'Watch Acme and Nordwind overdue invoices. Draft reminders — don’t send.',
      teach: 'Follow along this once. Save it as the weekly chase.',
      memory: '',
      connect: ''
    };

    function actP(el) {
      var v = parseFloat(el.style.getPropertyValue('--sc-p'));
      return isNaN(v) ? 0 : v;
    }

    function setOn(el, on) { el.classList.toggle('is-on', on); }

    function placeCursor(workP, signingIn) {
      if (!cursor || !signingIn) return;
      var next = $('#signin-next');
      var screen = desk.querySelector('.desk__screen');
      if (!next || !screen) return;
      var sr = screen.getBoundingClientRect();
      var nr = next.getBoundingClientRect();
      if (!sr.width || !sr.height) return;
      var targetX = ((nr.left + nr.width * 0.72 - sr.left) / sr.width) * 100;
      var targetY = ((nr.top + nr.height * 0.55 - sr.top) / sr.height) * 100;
      var t = clamp01((workP - 0.10) / 0.30);
      cursor.style.left = (18 + (targetX - 18) * t).toFixed(1) + '%';
      cursor.style.top = (16 + (targetY - 16) * t).toFixed(1) + '%';
    }

    function sceneOf(p) {
      if (p < 0.26) return 'work';
      if (p < 0.50) return 'teach';
      if (p < 0.74) return 'memory';
      return 'connect';
    }

    var lastScene = '';
    var lastLabel = -1;

    function demo(p) {
      var scene = sceneOf(p);
      if (scene !== lastScene) {
        Object.keys(scenes).forEach(function (k) { setOn(scenes[k], k === scene); });
        lastScene = scene;
      }

      var workP = clamp01(p / 0.26);
      var teachP = clamp01((p - 0.26) / 0.24);
      var memP = clamp01((p - 0.50) / 0.24);
      var conP = clamp01((p - 0.74) / 0.24);

      var signingIn = scene === 'work' && workP < 0.60;
      var inControl = signingIn;
      var working = !inControl;
      setOn(modeControl, inControl);
      setOn(modeWork, working);
      desk.classList.toggle('is-working', working);
      desk.classList.toggle('is-signin', signingIn);
      desk.classList.toggle('is-inbox', scene === 'work' && !signingIn);

      desk.classList.toggle('is-card', scene === 'work' || (scene === 'teach' && teachP < 0.88));
      desk.classList.toggle('is-draft', scene === 'work' && workP > 0.72);
      desk.classList.toggle('is-mem', scene === 'memory' && memP > 0.16);
      desk.classList.toggle('is-ask', scene === 'connect' && conP > 0.08);
      desk.classList.toggle('is-research', scene === 'connect' && conP > 0.24);
      desk.classList.toggle('is-close', scene === 'connect' && conP > 0.46);
      desk.classList.toggle('is-click', signingIn && workP > 0.38 && workP < 0.58);
      desk.classList.toggle('is-cursor', signingIn && workP > 0.10);

      var card = '';
      if (scene === 'work') card = signingIn ? CARDS.signin : CARDS.inbox;
      else if (scene === 'teach') card = CARDS.teach;
      if (chatText.textContent !== card) chatText.textContent = card;

      if (scene === 'teach') {
        var sec = reduce ? 4 : Math.min(9, Math.floor(teachP * 10));
        var label = '0:0' + sec;
        if (sec !== lastLabel) { teachTime.textContent = label; lastLabel = sec; }
      }

      if (cursor && !reduce) {
        placeCursor(workP, signingIn);
      }

      var phase = scene === 'work' ? 1 : scene === 'teach' ? 2 : scene === 'memory' ? 3 : 4;
      desk.setAttribute('data-sc-verify-state', 'scene:' + phase);
      if (p > 0.9 && p <= 1) desk.setAttribute('data-sc-verify-hold', 'true');
      else desk.removeAttribute('data-sc-verify-hold');
    }

    function showJob(key) {
      var job = JOBS[key];
      if (!job) return;
      $('#job-title').textContent = job.title;
      $('#job-body').textContent = job.body;
      $('#job-input').textContent = job.input;
      $('#job-output').textContent = job.output;
      $('#job-preview-app').textContent = job.app;
      var chip = $('#job-preview .chip');
      chip.textContent = job.chip;
      chip.classList.toggle('chip--wait', job.wait);
      $('#job-preview-body').innerHTML = job.preview;
      $('#job-panel').setAttribute('aria-labelledby', 'tab-' + key);
      document.querySelectorAll('.jobs__picks [role="tab"]').forEach(function (btn) {
        var on = btn.getAttribute('data-job') === key;
        btn.setAttribute('aria-selected', on ? 'true' : 'false');
      });
    }

    document.querySelectorAll('.jobs__picks [role="tab"]').forEach(function (btn) {
      btn.addEventListener('click', function () { showJob(btn.getAttribute('data-job')); });
      btn.addEventListener('keydown', function (e) {
        if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
        var tabs = Array.prototype.slice.call(document.querySelectorAll('.jobs__picks [role="tab"]'));
        var i = tabs.indexOf(btn);
        var next = e.key === 'ArrowRight' ? tabs[(i + 1) % tabs.length] : tabs[(i - 1 + tabs.length) % tabs.length];
        next.focus();
        showJob(next.getAttribute('data-job'));
        e.preventDefault();
      });
    });

    function update() { demo(actP(actDemo)); }

    var ticking = false;
    addEventListener('scroll', function () {
      if (ticking) return;
      ticking = true;
      requestAnimationFrame(function () { update(); ticking = false; });
    }, { passive: true });
    addEventListener('resize', function () { requestAnimationFrame(update); }, { passive: true });

    $('#shout-form').addEventListener('submit', function () {
      var field = $('#brief');
      if (!field.value.trim()) field.disabled = true;
    });

    update();
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(update);
  }

  global.JobShout = { init: init };
})(window);
