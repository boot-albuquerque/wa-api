// Package workload serves the deterministic SPAs the benchmark drives.
//
// The pages are served locally and embedded in the binary so a run depends on
// no network and no third party: the same bytes are served on every host, which
// is what makes results comparable across machines. Hitting a real site would
// put someone else's CDN, rate limiting and A/B tests inside the measurement.
package workload

import (
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// Server serves workloads A (light SPA) and B (heavy SPA) plus the endpoints
// they call.
type Server struct {
	ln       net.Listener
	srv      *http.Server
	requests atomic.Int64
}

// Start binds an ephemeral port on loopback and serves until Close.
func Start() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln}
	mux := http.NewServeMux()
	mux.HandleFunc("/a", s.count(pageA))
	mux.HandleFunc("/b", s.count(pageB))
	mux.HandleFunc("/h", s.count(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, HostileHTML)
	}))
	// api/rows feeds workload B's table. The delay is fixed, not random: a
	// random delay would show up as latency variance and be misread as
	// controller jitter.
	mux.HandleFunc("/api/rows", s.count(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, rowsJSON)
	}))
	mux.HandleFunc("/api/login", s.count(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"token":"t0ken"}`)
	}))
	s.srv = &http.Server{Handler: mux}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func (s *Server) count(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		h(w, r)
	}
}

// URL returns the base URL, e.g. http://127.0.0.1:54321.
func (s *Server) URL() string { return "http://" + s.ln.Addr().String() }

// Requests returns how many requests the server answered, used to assert that
// every controller actually exercised the same flow.
func (s *Server) Requests() int64 { return s.requests.Load() }

// Close stops the server.
func (s *Server) Close() error { return s.srv.Close() }

func pageA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, htmlA)
}

func pageB(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, htmlB)
}

// htmlA is workload A: a light SPA. Client-side rendering, a form, a fetch, and
// a deterministic ready signal (#app-ready) so every controller waits on the
// same event instead of on a sleep.
const htmlA = `<!doctype html><html><head><meta charset="utf-8"><title>Workload A</title></head>
<body>
<div id="root"></div>
<script>
// Minimal hand-rolled SPA: no framework, so page A isolates controller cost
// from framework cost. Page B adds the framework-like load.
const root = document.getElementById('root');
function render(state) {
  root.innerHTML = ` + "`" + `
    <h1 id="title">Workload A</h1>
    <form id="login">
      <input id="email" name="email" value="">
      <input id="password" type="password" name="password" value="">
      <button id="submit" type="button">Enter</button>
    </form>
    <div id="result">${state.result || ''}</div>
    <ul id="items">${state.items.map(i => '<li class="item">'+i+'</li>').join('')}</ul>
  ` + "`" + `;
  document.getElementById('submit').addEventListener('click', async () => {
    const email = document.getElementById('email').value;
    const r = await fetch('/api/login', {method:'POST'});
    const j = await r.json();
    state.result = j.ok ? ('welcome:' + email) : 'fail';
    render(state);
    document.getElementById('result').setAttribute('data-done', '1');
  });
}
const state = {items: Array.from({length: 20}, (_, i) => 'item-' + i), result: ''};
render(state);
// Ready marker, appended synchronously after the first render.
//
// It deliberately does NOT use requestAnimationFrame. A CDP target created in
// the background never becomes the active tab, Chromium throttles frame
// production for it, and rAF callbacks can therefore never run — the readiness
// signal would wait forever on a page that is fully rendered. Measured in this
// study: every controller hung on a page whose DOM was already complete.
const m = document.createElement('div');
m.id = 'app-ready';
document.body.appendChild(m);
</script>
</body></html>`

// htmlB is workload B: a heavy SPA. It carries a framework-shaped render loop,
// concurrent fetches, a WebSocket-like polling loop, lazy images, a modal and a
// table that re-renders — the load profile the study is actually about.
const htmlB = `<!doctype html><html><head><meta charset="utf-8"><title>Workload B</title>
<style>
.row{padding:2px;border-bottom:1px solid #eee}
.modal{display:none;position:fixed;top:10px;left:10px;background:#fff;border:1px solid #333}
.modal.open{display:block}
</style></head>
<body>
<div id="root"></div>
<script>
// A deliberately expensive render path: a virtual-DOM-ish diff over a few
// hundred rows, re-run on every state change. This is what makes page B cost
// renderer CPU rather than just controller round-trips.
let state = {rows: [], filter: '', modal: false, ticks: 0, result: '', email: ''};
const root = document.getElementById('root');

function vnode(tag, attrs, children) { return {tag, attrs: attrs||{}, children: children||[]}; }
function build(s) {
  const visible = s.rows.filter(r => !s.filter || r.name.includes(s.filter));
  return vnode('div', {id:'app'}, [
    vnode('h1', {id:'title'}, ['Workload B']),
    vnode('input', {id:'filter', value: s.filter}, []),
    // Same element contract as workload A. Without it the shared job flow
    // (fill #email, click #submit, await #result[data-done]) has nothing to act
    // on, and every controller fails on a page that rendered perfectly — which
    // is exactly what the first run of this matrix produced.
    vnode('input', {id:'email', value: s.email || ''}, []),
    vnode('input', {id:'password', type:'password'}, []),
    vnode('button', {id:'submit'}, ['Enter']),
    vnode('button', {id:'open-modal'}, ['open']),
    vnode('div', {id:'ticks'}, [String(s.ticks)]),
    vnode('div', {id:'count'}, [String(visible.length)]),
    vnode('div', {id:'result'}, [s.result]),
    vnode('div', {class:'modal' + (s.modal ? ' open' : ''), id:'modal'}, [
      vnode('button', {id:'close-modal'}, ['close']),
    ]),
    vnode('div', {id:'table'}, visible.map(r =>
      vnode('div', {class:'item row', 'data-id': String(r.id)}, [r.name + ':' + r.value]))),
  ]);
}
function toDom(v) {
  if (typeof v === 'string') return document.createTextNode(v);
  const el = document.createElement(v.tag);
  for (const k in v.attrs) {
    if (k === 'value') { el.value = v.attrs[k]; } else { el.setAttribute(k, v.attrs[k]); }
  }
  v.children.forEach(c => el.appendChild(toDom(c)));
  return el;
}
function render() {
  const next = toDom(build(state));
  root.replaceChildren(next);
  document.getElementById('submit').addEventListener('click', async () => {
    state.email = document.getElementById('email').value;
    const r = await fetch('/api/login', {method:'POST'});
    const j = await r.json();
    state.result = j.ok ? ('welcome:' + state.email) : 'fail';
    render();
    document.getElementById('result').setAttribute('data-done', '1');
  });
  document.getElementById('open-modal').addEventListener('click', () => { state.modal = true; render(); });
  document.getElementById('close-modal').addEventListener('click', () => { state.modal = false; render(); });
  document.getElementById('filter').addEventListener('input', (e) => { state.filter = e.target.value; render(); });
}

// Concurrent fetches on load, like a real dashboard fanning out to its APIs.
Promise.all([fetch('/api/rows'), fetch('/api/rows'), fetch('/api/rows')])
  .then(rs => Promise.all(rs.map(r => r.json())))
  .then(all => {
    state.rows = all[0];
    render();
    // Polling loop stands in for a WebSocket feed: steady background work that
    // keeps the renderer from ever going fully idle.
    setInterval(() => { state.ticks++; const t = document.getElementById('ticks'); if (t) t.textContent = String(state.ticks); }, 100);
    const m = document.createElement('div'); m.id = 'app-ready'; document.body.appendChild(m);
  });

window.__submit = async function() {
  const r = await fetch('/api/login', {method:'POST'});
  const j = await r.json();
  state.result = j.ok ? 'ok' : 'fail';
  render();
  document.getElementById('result').setAttribute('data-done','1');
};
</script>
</body></html>`

// rowsJSON is fixed data: same bytes every run, so render cost does not vary.
var rowsJSON = func() string {
	s := "["
	for i := 0; i < 300; i++ {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf(`{"id":%d,"name":"row-%d","value":%d}`, i, i, i*7%1000)
	}
	return s + "]"
}()
