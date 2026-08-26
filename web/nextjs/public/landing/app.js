/* ============================================================================
   JobShout landing · bespoke page logic
   ----------------------------------------------------------------------------
   Everything here reads scroll state from the --sc-p custom property the
   engine publishes on each act (uniqueness.md: bespoke JS in the page, engine
   untouched). Three systems:

     1. The hero console: scroll types the brief, sends it, drafts the plan.
     2. The run cascade: tool rows stream in, the deliverable lands, and the
        approval decision from act 4 changes what the run actually does.
     3. The signature move: the fixed job ticket that executes the visit.
        Monotonic on purpose: a log that un-writes itself is not a log.

   Console panel state is a pure function of progress, so scrolling back
   scrubs the demo backwards; that is the medium behaving honestly.
   ========================================================================== */
(function (global) {
  'use strict';

  var reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;
  var clamp01 = function (x) { return x < 0 ? 0 : x > 1 ? 1 : x; };

  function init() {
    var $ = function (s) { return document.querySelector(s); };

    var actWitness = $('#witness');
    var actRoster  = $('#roster');
    var actGate    = $('#gate');
    var actRun     = $('#run');
    var actShout   = $('#shout');

    var console1   = $('#console1');
    var runStatus  = $('#run-status');
    var typer      = $('#typer');
    var typedEl    = typer.querySelector('.composer__typed');
    var briefText  = typer.getAttribute('data-text');
    var msgBrief   = $('#msg-brief');
    var msgPickup  = $('#msg-pickup');
    var planItems  = document.querySelectorAll('#plan .plan__item');

    var gateChip   = $('#gate-chip');
    var btnApprove = $('#btn-approve');
    var btnDecline = $('#btn-decline');
    var stamp      = $('#stamp');

    var console2   = $('#console2');
    var run2Status = $('#run2-status');
    var meter      = $('#meter');
    var rows       = document.querySelectorAll('#log .row');
    var rowSendD   = $('#row-send .row__d');
    var rowSendDur = $('#row-send .row__dur');
    var outcome    = $('#artifact-outcome');

    var ticket     = $('#ticket');
    var ticketTime = $('#ticket-time');
    var doneTime   = $('#done-time');
    var stepLinks  = {};
    document.querySelectorAll('.ticket__steps a').forEach(function (a) {
      stepLinks[a.getAttribute('data-step')] = a;
    });
    var compact = document.createElement('span');
    compact.className = 'ticket__compact';
    ticket.querySelector('.ticket__id').appendChild(compact);

    var started = Date.now();
    var approval = null;          // null | 'approved' | 'declined'
    var stepsDone = {};
    var lastChars = -1;

    function fmt(ms) {
      var s = Math.floor(ms / 1000);
      return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
    }

    function actP(el) {
      var v = parseFloat(el.style.getPropertyValue('--sc-p'));
      return isNaN(v) ? 0 : v;
    }

    // ---- act 4: the approval decision --------------------------------------
    function resolveGate(kind, auto) {
      if (approval) return;
      approval = kind;
      stamp.textContent = kind === 'approved'
        ? (auto ? 'approved · sample default' : 'approved · by you')
        : 'declined · by you';
      stamp.classList.toggle('is-declined', kind === 'declined');
      btnApprove.disabled = true;
      btnDecline.disabled = true;
      gateChip.textContent = kind === 'approved' ? 'resolved · send allowed' : 'resolved · send skipped';
      gateChip.classList.remove('chip--wait');
      gateChip.dataset.state = 'live';
      rowSendD.textContent = rowSendD.getAttribute('data-' + kind);
      outcome.textContent = outcome.getAttribute('data-' + kind);
      if (kind === 'declined') rowSendDur.textContent = '0.0s';
    }
    btnApprove.addEventListener('click', function () { resolveGate('approved', false); update(); });
    btnDecline.addEventListener('click', function () { resolveGate('declined', false); update(); });

    // ---- act 1: the hero console -------------------------------------------
    function setStatus(el, text, state) {
      if (el.textContent !== text) el.textContent = text;
      if (state !== undefined && el.dataset.state !== state) el.dataset.state = state;
    }

    function hero(p) {
      var typing = clamp01((p - 0.06) / 0.26);
      var chars = reduce
        ? (p > 0.06 ? briefText.length : 0)
        : Math.round(briefText.length * typing);
      if (p >= 0.36) chars = briefText.length;
      if (chars !== lastChars) {
        typedEl.textContent = briefText.slice(0, chars);
        lastChars = chars;
        // Keep the newest characters in view when the line outruns the field,
        // exactly as a real input would.
        typer.scrollLeft = typer.scrollWidth;
      }
      console1.classList.toggle('has-text', chars > 0);
      console1.classList.toggle('is-typing', p >= 0.06 && p < 0.36);
      console1.classList.toggle('is-sent', p >= 0.36);
      console1.classList.toggle('is-planning', p >= 0.46);
      msgBrief.classList.toggle('is-on', p >= 0.36);
      msgPickup.classList.toggle('is-on', p >= 0.46);

      var planDone = 0;
      planItems.forEach(function (li, i) {
        var on = p >= 0.55 + i * 0.08;
        li.classList.toggle('is-done', on);
        if (on) planDone++;
      });

      if (p < 0.06)      setStatus(runStatus, 'new job', 'idle');
      else if (p < 0.36) setStatus(runStatus, 'briefing', 'idle');
      else if (p < 0.46) setStatus(runStatus, 'assigning', 'live');
      else if (p < 0.82) setStatus(runStatus, 'planning', 'live');
      else               setStatus(runStatus, 'queued to run', 'wait');

      var phase = p < 0.06 ? 0 : p < 0.36 ? 1 : p < 0.46 ? 2 : p < 0.82 ? 3 : 4;
      console1.setAttribute('data-sc-verify-state',
        'phase:' + phase + ' chars:' + chars + ' plan:' + planDone);
    }

    // ---- act 5: the run cascade --------------------------------------------
    function run(p) {
      var on = 0, done = 0;
      rows.forEach(function (row) {
        var at = parseFloat(row.getAttribute('data-at'));
        var isOn = p >= at;
        var isDone = p >= at + 0.045;
        row.classList.toggle('is-on', isOn);
        row.classList.toggle('is-done', isDone);
        if (isOn) on++;
        if (isDone) done++;
      });

      var progress = clamp01(p / 0.58);
      meter.style.transform = 'scaleX(' + progress.toFixed(3) + ')';

      var landed = p >= 0.62;
      console2.classList.toggle('is-artifact', landed);

      if (landed) setStatus(run2Status, 'delivered', 'done');
      else if (p >= 0.54) setStatus(run2Status, 'settling', 'live');
      else if (p > 0.02) setStatus(run2Status, 'executing', 'live');
      else setStatus(run2Status, 'queued', 'wait');

      console2.setAttribute('data-sc-verify-state',
        'rows:' + on + ' done:' + done + ' meter:' + Math.round(progress * 20) + ' art:' + (landed ? 1 : 0));
      // The finished frame rests before the act unpins; declare the hold so
      // the verification pass reads stillness there as intent.
      if (p > 0.86 && p <= 1) console2.setAttribute('data-sc-verify-hold', 'true');
      else console2.removeAttribute('data-sc-verify-hold');
    }

    // ---- the ticket ---------------------------------------------------------
    var STEPS = ['briefed', 'planned', 'crew', 'gate', 'run', 'done'];
    function stepHit(key, p1, p3, p4, p5, p6) {
      switch (key) {
        case 'briefed': return p1 >= 0.38;
        case 'planned': return p1 >= 0.8;
        case 'crew':    return p3 >= 0.5;
        case 'gate':    return approval !== null || p4 >= 0.6;
        case 'run':     return p5 >= 0.56;
        case 'done':    return p6 >= 0.3;
      }
      return false;
    }

    function ticketUpdate(p1, p3, p4, p5, p6) {
      var doneCount = 0, currentKey = null, currentLabel = '';
      STEPS.forEach(function (key) {
        var a = stepLinks[key];
        if (!stepsDone[key] && stepHit(key, p1, p3, p4, p5, p6)) {
          stepsDone[key] = fmt(Date.now() - started);
          a.title = 'done at ' + stepsDone[key] + ' into your visit';
          if (key === 'done') doneTime.textContent = stepsDone[key];
        }
        var isDone = !!stepsDone[key];
        a.classList.toggle('is-done', isDone);
        if (isDone) doneCount++;
        else if (!currentKey) {
          currentKey = key;
          currentLabel = a.textContent.trim();
        }
        a.classList.remove('is-current');
      });
      if (currentKey) stepLinks[currentKey].classList.add('is-current');
      compact.textContent = doneCount + '/6' + (currentLabel ? ' · next: ' + currentLabel.toLowerCase() : ' · done');
      ticket.setAttribute('data-sc-verify-state', 'steps:' + doneCount);
    }

    // ---- main loop ----------------------------------------------------------
    function update() {
      var p1 = actP(actWitness);
      var p3 = actP(actRoster);
      var p4 = actP(actGate);
      var p5 = actP(actRun);
      var p6 = actP(actShout);

      // Reaching the run with the gate untouched resolves it the honest way:
      // the sample defaults to approved, and the stamp says so.
      if (!approval && (p4 >= 0.92 || p5 > 0.03)) resolveGate('approved', true);

      hero(p1);
      run(p5);
      ticketUpdate(p1, p3, p4, p5, p6);
    }

    var ticking = false;
    addEventListener('scroll', function () {
      if (ticking) return;
      ticking = true;
      requestAnimationFrame(function () { update(); ticking = false; });
    }, { passive: true });
    addEventListener('resize', function () { requestAnimationFrame(update); }, { passive: true });

    setInterval(function () {
      ticketTime.textContent = fmt(Date.now() - started);
    }, 1000);

    // A keyboard user tabbing to the hero CTA after scrolling away lands on a
    // faded empty state; park the act back at its opening progress instead.
    // (verify.md: on a pinned act this is page-local work, the engine cannot
    // know which progress lights which control.)
    document.querySelectorAll('#hello a').forEach(function (a) {
      a.addEventListener('focus', function () {
        if (actP(actWitness) > 0.2) {
          var top = actWitness.getBoundingClientRect().top + scrollY;
          scrollTo({ top: top, left: 0, behavior: 'instant' });
        }
      });
    });

    // Keyboard focus into the gate or the close lands the control at the
    // viewport edge, where the card's reveal has not opened yet. Centre it;
    // centred, the flow act's progress puts every wipe past its window.
    document.querySelectorAll('#gate button, #shout input, #shout button').forEach(function (el) {
      el.addEventListener('focus', function () {
        var r = el.getBoundingClientRect();
        if (r.top < innerHeight * 0.25 || r.bottom > innerHeight * 0.75) {
          el.scrollIntoView({ block: 'center', behavior: 'instant' });
        }
      });
    });

    // Don't submit an empty brief as ?brief= noise.
    $('#shout-form').addEventListener('submit', function () {
      var field = $('#brief');
      if (!field.value.trim()) field.disabled = true;
    });

    update();
    // The engine measures after fonts load; settle our state once more then.
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(update);
  }

  global.JobShout = { init: init };
})(window);
