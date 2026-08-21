'use strict';

var WAD_URL = '/gamedata/doom1.wad';
var statusEl = document.getElementById('status');
var statusText = document.getElementById('statustext');
var barFill = document.querySelector('#bar > div');
var canvas = document.getElementById('canvas');

canvas.addEventListener('contextmenu', function (event) {
  event.preventDefault();
});

function post(state, extra) {
  var msg = Object.assign({ type: 'doom', state: state }, extra || {});
  try { window.parent.postMessage(msg, window.location.origin); } catch (e) {}
}

function fail(code, detail) {
  statusText.textContent = code === 'wad-missing'
    ? 'WAD NOT FOUND \u2014 see widget hint'
    : 'ERROR: ' + (detail || code);
  document.getElementById('bar').style.display = 'none';
  post('error', { code: code, detail: detail || '' });
}

function setProgress(pct, label) {
  statusText.textContent = label;
  barFill.style.width = pct + '%';
}

// Fetch with download progress. Content-Length is present since the
// coordinator serves the WAD as a plain static file.
function fetchWithProgress(url, label) {
  return fetch(url).then(function (resp) {
    if (resp.status === 404) { throw { code: 'wad-missing' }; }
    if (!resp.ok) { throw { code: 'http-' + resp.status }; }
    var total = parseInt(resp.headers.get('Content-Length') || '0', 10);
    if (!resp.body || !total) { return resp.arrayBuffer().then(function (b) { return new Uint8Array(b); }); }
    var reader = resp.body.getReader();
    var chunks = [];
    var received = 0;
    function pump() {
      return reader.read().then(function (r) {
        if (r.done) {
          var out = new Uint8Array(received);
          var off = 0;
          chunks.forEach(function (c) { out.set(c, off); off += c.length; });
          return out;
        }
        chunks.push(r.value);
        received += r.value.length;
        setProgress(Math.round((received / total) * 100), label);
        return pump();
      });
    }
    return pump();
  });
}

post('loading');

Promise.all([
  fetchWithProgress(WAD_URL, 'DOWNLOADING IWAD\u2026'),
  fetch('default.cfg').then(function (r) {
    if (!r.ok) { throw { code: 'cfg-missing' }; }
    return r.arrayBuffer().then(function (b) { return new Uint8Array(b); });
  }),
]).then(function (results) {
  var wad = results[0];
  var cfg = results[1];

  var magic = String.fromCharCode.apply(null, wad.subarray(0, 4));
  if (magic !== 'IWAD' && magic !== 'PWAD') {
    throw { code: 'bad-wad', detail: 'magic: ' + magic };
  }

  setProgress(100, 'STARTING ENGINE\u2026');

  window.Module = {
    noInitialRun: true,
    onRuntimeInitialized: function () {
      callMain(['-iwad', 'doom1.wad', '-window', '-nogui', '-nomusic', '-config', 'default.cfg']);
      statusEl.classList.add('hidden');
      post('running');
      canvas.focus();
    },
    preRun: function () {
      Module.FS.createDataFile('/', 'doom1.wad', wad, true, true, true);
      Module.FS.createDataFile('/', 'default.cfg', cfg, true, true, true);
    },
    canvas: (function () {
      canvas.addEventListener('webglcontextlost', function (e) {
        e.preventDefault();
        fail('context-lost', 'WebGL context lost, remove and re-add the widget');
      }, false);
      return canvas;
    })(),
    print: function (text) {
      // Engine status protocol: "doom: <n>, <message>"
      if (typeof text === 'string' && text.indexOf('doom:') === 0) {
        post('engine', { message: text });
      }
    },
    printErr: function (text) { console.error(text); },
    setStatus: function () {},
  };

  window.onerror = function (_msg, _src, _line, _col, err) {
    fail('engine-crash', err && err.message ? err.message : String(_msg));
  };

  // Inject the engine only after the WAD is in memory.
  var s = document.createElement('script');
  s.src = 'websockets-doom.js';
  s.onerror = function () { fail('engine-missing', 'websockets-doom.js failed to load'); };
  document.body.appendChild(s);
}).catch(function (err) {
  fail(err && err.code ? err.code : 'fetch-failed', err && err.detail);
});

// Commands from the parent widget.
window.addEventListener('message', function (ev) {
  if (ev.origin !== window.location.origin) { return; }
  var data = ev.data || {};
  if (data.type !== 'doom-cmd') { return; }
  if (data.cmd === 'fullscreen' && window.Module && Module.requestFullscreen) {
    Module.requestFullscreen(false, false);
  }
  if (data.cmd === 'focus') {
    canvas.focus();
  }
});

// Make sure clicks restore keyboard focus to the engine canvas.
window.addEventListener('mousedown', function () {
  canvas.focus();
});
