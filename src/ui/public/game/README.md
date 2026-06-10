# Doom engine assets (vendored)

WebAssembly build of Chocolate Doom from the
[cloudflare/doom-wasm](https://github.com/cloudflare/doom-wasm) project
(GPL-2, see `LICENSE`). The repository publishes no prebuilt artifacts, so
the build was taken from the project's official demo deployment at
<https://silentspacemarine.com> on 2026-06-10:

| File | Source | sha256 |
|------|--------|--------|
| `websockets-doom.js` | silentspacemarine.com/websockets-doom.js | `a2909044a9fbc5529f941c8dbf93cc2931927690e0341c737545cf0b9cff23fb` |
| `websockets-doom.wasm` | silentspacemarine.com/websockets-doom.wasm | `6366f83a58fe8596ce742a66dbf86871d315862c89c11e65b54935be03c7e6c4` |
| `default.cfg` | raw.githubusercontent.com/cloudflare/doom-wasm/main/src/default.cfg | `eacd68e8e254bd250bc559c1535ab88df437340d0460cae6085f5f29b49fb6e2` |

To rebuild from source instead (emscripten + SDL2 toolchain):

```bash
git clone https://github.com/cloudflare/doom-wasm && cd doom-wasm
./scripts/clean.sh && ./scripts/build.sh
# outputs src/websockets-doom.{js,wasm}
```

`index.html` is the Homer host page (loaded in an iframe by the Doom
dashboard widget, see `src/ui/src/dashboard/widgets/DoomPanel.tsx`). It is
not part of the upstream build.

The IWAD (`doom1.wad`) is intentionally **not** stored here: everything in
`public/` is embedded into the homer-core binary via `go:embed`. The WAD is
served at runtime from disk through the coordinator's `/gamedata/` route
(`gamedata_dir` config key). Use `scripts/fetch-doom-wad.sh` to download the
shareware WAD onto the server.
