package workload

// hostileHTML is workload W-H: a page built to separate "cheaper" from "wrong".
//
// Every trap here is something a real SPA does routinely, and each one is
// invisible to a benchmark that only measures throughput:
//
//  1. controlled input — the value is owned by JS state and re-applied on every
//     render, exactly like a React controlled component. Assigning .value
//     directly does NOT fire the framework's change path, so the write is
//     reverted on the next render. Real key events do fire it.
//  2. overlay — a transparent element covers the submit button for the first
//     600ms. A real mouse event at the button's coordinates hits the OVERLAY.
//     element.click() ignores hit-testing and fires the button anyway.
//  3. node replacement — the button is re-created (not mutated) 300ms after
//     load, invalidating any node id resolved before that.
//  4. late element — the confirmation only exists after the request resolves.
//
// The page reports ground truth in window.__truth, so the harness can tell a
// job that SUCCEEDED from a job that merely returned without error.
// HostileHTML is exported so the benchmark can serve it as workload "h".
const HostileHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Hostile</title>
<style>
#overlay{position:fixed;left:0;top:0;right:0;bottom:0;background:rgba(0,0,0,0.01);z-index:9999}
#overlay.gone{display:none}
</style></head>
<body>
<div id="overlay"></div>
<div id="root"></div>
<script>
// Ground truth, written only by the paths a real user could trigger.
window.__truth = {
  realKeyEvents: 0,      // keydown seen by the input
  valueAssigned: false,  // .value written without key events
  submitViaMouse: 0,     // click whose event had real coordinates
  submitViaSynthetic: 0, // click with no coordinates (element.click())
  overlayWasUp: true,
  committedEmail: '',    // what the framework state actually holds
};

let state = { email: '', result: '' };

function render() {
  const root = document.getElementById('root');
  root.innerHTML =
    '<h1 id="title">Hostile</h1>' +
    '<input id="email">' +
    '<button id="submit">Enter</button>' +
    '<div id="result">' + state.result + '</div>';

  const input = document.getElementById('email');
  // Controlled component: state is the source of truth, re-applied every render.
  input.value = state.email;
  input.addEventListener('keydown', () => { window.__truth.realKeyEvents++; });
  // The framework commits only on input events produced by real typing.
  input.addEventListener('input', (e) => {
    if (e.isTrusted) { state.email = e.target.value; window.__truth.committedEmail = state.email; }
  });

  const btn = document.getElementById('submit');
  btn.addEventListener('click', async (e) => {
    // A trusted event carries real screen coordinates; element.click() does not.
    if (e.isTrusted && (e.clientX !== 0 || e.clientY !== 0)) {
      window.__truth.submitViaMouse++;
    } else {
      window.__truth.submitViaSynthetic++;
    }
    const r = await fetch('/api/login', {method:'POST'});
    const j = await r.json();
    // The confirmation reflects the COMMITTED state, not what the DOM shows.
    state.result = j.ok ? ('welcome:' + state.email) : 'fail';
    render();
    document.getElementById('result').setAttribute('data-done','1');
  });
}

render();

// Trap 3: replace the button node, invalidating previously resolved node ids.
setTimeout(() => { render(); }, 300);

// Trap 2 is OPT-IN via ?overlay=1, because it separates two different
// questions. Throughput (Part H) needs a page every level can complete;
// occlusion (Part U) needs a page where a real mouse event is intercepted.
// Mixing them made the correct implementations time out, which measures
// nothing about their cost.
const overlayOn = new URLSearchParams(location.search).get('overlay') === '1';
if (overlayOn) {
  setTimeout(() => {
    document.getElementById('overlay').classList.add('gone');
    window.__truth.overlayWasUp = false;
  }, 600);
} else {
  document.getElementById('overlay').classList.add('gone');
  window.__truth.overlayWasUp = false;
}

// Readiness marker, synchronous (background targets never run rAF).
const m = document.createElement('div'); m.id = 'app-ready'; document.body.appendChild(m);
</script>
</body></html>`
