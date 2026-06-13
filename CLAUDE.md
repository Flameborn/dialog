# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

This repository contains the Å-machine — a virtual machine for delivering interactive fiction stories written in the Dialog programming language. The Å-machine is to Dialog what Glulx is to Inform 7. It targets a wide range of platforms, from modern web browsers down to 8-bit hardware (Commodore 64).

Stories compile to `.aastory` files. This repo does not contain the Dialog compiler — only the runtime engines, frontends, packaging tools, and specification.

## Build and test commands

The build system is plain `make`. The top-level `Makefile` delegates to `src/` and `test/`.

```sh
make                # builds aamshow, aambundle, 6502 blobs, then runs tests
make no6502         # builds C tools and tests them without rebuilding 6502 assets
make 6502           # builds only the 6502 engine, frontends, and the aambox6502 emulator
make windows        # cross-compiles .exe versions (needs i686-w64-mingw32-gcc)
make test           # runs the test suite (requires the C tools to be built)
make clean          # removes build outputs
make install        # copies aamshow and aambundle to /usr/local/bin
```

Building requires `gcc`, `make`, `node` (for tests and `aamrun.*` packaging), `luajit` (for the Lua engine tests), and `xa65` (the xa assembler for 6502 code). The `aamrun.*` targets in `src/Makefile` additionally need `@yao-pkg/pkg` installed globally via npm. Building the Go engine requires `go`.

### Running a single test

Each subdirectory under `test/` is an independent test case with its own `Makefile`. Each one runs the same story through the JS engine (via `nodefrontend.js`), the 6502 engine (via the `aambox6502` emulator), the Lua engine (via `frontend.lua`), and the Go engine (via `aamrun`), then diffs against gold files. To run just one:

```sh
make -C test/gosling test          # all engines
make -C test/gosling test.js       # only the JS engine
make -C test/gosling test.6502     # only the 6502 engine
make -C test/gosling test.lua      # only the Lua engine
make -C test/gosling test.go       # only the Go engine
make -C test/gosling DIFF=meld     # use meld for a visual diff on failure
```

`test/familiar/` contains Dialog source for a larger story used for manual testing; it has no automated test target — building its `.aastory` requires the external Dialog compiler.

### Running the example story

```sh
node src/js/nodefrontend.js example/cloak-rel2.aastory     # text-mode JS
open example/web/play.html                                  # web frontend
x64sc -truedrive -drivesound -reu -reusize 256 example/cloak-rel2.d64    # C64 via VICE
```

## Architecture

The codebase implements **four independent VM engines** (JavaScript, 6502 assembler, LuaJIT, and Go), each combined with one or more **frontends** that handle platform-specific I/O. Plus a set of **C tools** for inspecting and packaging story files.

### Story file format (`.aastory`)

IFF-style file with a `FORM` header containing typed chunks (`META`, `WRIT`, `FILE`, etc.). The file format major/minor version is checked against `AAVM_FORMAT_MAJOR`/`MINOR` in `src/aavm.h` — older 0.x files must still load on 1.x engines (see `test/gosling` for the regression). Opcode constants (`AA_*`) and metadata tag IDs (`AAMETA_*`) live in `src/aavm.h` and are mirrored in `src/js/engine.js`, `src/6502/engine.s`, `src/lua/engine.lua`, and `src/go/engine.go`. Any opcode change touches all four.

### The four engines

- **`src/js/engine.js`** — the JavaScript VM. Pure ES5-ish, self-contained, exported via `module.exports` for Node and consumed directly by the browser. Shared by both the web and Node frontends.
- **`src/6502/engine.s`** — the 6502 VM, written in xa65 assembler. Designed to be included from a platform frontend (`c64_frontend.s` or `aambox_frontend.s`). Generic 6502; no undocumented opcodes; can run from ROM. Zero-page register conventions are defined at the top of the file.
- **`src/lua/engine.lua`** — the LuaJIT VM. Port of the JS engine, using LuaJIT FFI for performance where beneficial. Shared by the Lua terminal frontend.
- **`src/go/engine.go`** (plus `vm.go`, `init.go`, `run.go`, `parse.go`) — the Go VM. Struct-based, with `int` for index/pointer variables and `uint16` for data values. Uses panic/recover for VM exceptions. Shared by the Go terminal frontend.

When changing VM semantics, all four engines must be updated in lock-step. The test suite catches divergence by diffing transcripts from all engines against gold files (for `body_not_status` and `impossible`; `gosling` has separate `js.gold`, `6502.gold`, `lua.gold`, and `go.gold` files).

### Frontends

- **`src/js/webfrontend.{js,html,css}`** — browser frontend. jQuery-based. Reads a story file and provides full UI including styling, hyperlinks, transcripts, embedded fonts/audio, and localStorage save state.
- **`src/js/nodefrontend.js`** — Node text frontend. Used for automated tests and command-line play. Wraps `engine.js` and handles word-wrapped terminal I/O via `readline`.
- **`src/lua/frontend.lua`** — LuaJIT text frontend. Port of `nodefrontend.js`. Wraps `engine.lua` and handles word-wrapped terminal I/O. Output is byte-for-byte compatible with the Node frontend.
- **`src/go/main.go`** — Go text frontend. Port of `nodefrontend.js`. Wraps the Go engine and handles word-wrapped terminal I/O via `bufio.Scanner`.
- **`src/6502/c64_frontend.s`** — Commodore 64 frontend with custom font and a 1541 floppy driver. The story is delivered on a `.d64` disk image with a loader (`c64_loader.s`) and drive code (`c64_drivecode.s`). The whole thing is run-length crunched by the `cruncher` tool into `c64_crunched.bin`.
- **`src/6502/aambox_frontend.s`** + **`aambox6502.c`** — a synthetic 6502 platform for automated testing of the 6502 engine. `aambox6502.c` is the emulator (built on Mike Chambers's `fake6502.c`); the frontend assembles to `aambox_frontend.bin` and the emulator loads it plus a story file.

### C tools (`src/`)

- **`aamshow`** — disassembler and inspector for `.aastory` files (and savefiles). Built from `aamshow.c`, `aavm.c`, `crc32.c`.
- **`aambundle`** — packages an `.aastory` for distribution. Targets:
  - `web` (default) — directory with the web interpreter and the story
  - `c64` — directory with a `.d64` disk image
  - `web:story` — just `story.js`, for embedding in a larger web build
- **`aavm.c` / `aavm.h`** — the opcode dictionary (`aaopinfo`) and metadata constants shared by `aamshow` and `aambundle`.
- **`mkheader`** — a tiny C tool that wraps binary blobs into C headers. `src/Makefile` uses it to embed the web interpreter (`engine.js`, `webfrontend.{js,css,html}`, jQuery, license), the C64 interpreter (`c64_crunched.bin`, `c64_loader.prg`, `c64_drivecode.bin`, license), and CSS into `aambundle` so the binary is self-contained.

### Bundling flow

`aambundle` parses the IFF chunks (see `visit_chunks` and `trim_chunks` in `aambundle.c`), then either copies a fixed set of embedded blobs (web target — see `bundle_web.c`) or assembles a C64 disk image with the crunched interpreter prepended to the story (`bundle_c64.c`). The embedded blobs are produced by `mkheader` at build time from the `js/` and `6502/` sources — so changes to the web or C64 interpreter only flow into the shipping `aambundle` binary after `src/Makefile` regenerates the `table_*.h` headers.

## Specification

`docs/aam-specification-1.0.txt` is the authoritative specification (older versions kept for history). When changing opcode behavior, encoding, or output semantics, the spec must be updated alongside the engines. The spec is paired with concrete release notes in the top-level `readme.txt`.

## Versioning

When bumping the version, update **all five** locations listed in `version_numbers.txt`:

- `VERSION` / `VER_MAJOR` / `VER_MINOR` defines at the top of `src/Makefile`
- `VERSION` define at the top of `src/6502/Makefile`
- `VER_MAJOR` / `VER_MINOR` constants in `src/js/engine.js` (these track the
  supported aastory file format, so they only change on a major or minor bump,
  not on a patch release)
- The "about" blurb in `src/js/webfrontend.html`
- The `VERSION` constant in `src/js/nodefrontend.js`

The three-part version has documented semantics: major = spec-breaking, minor = backwards-compatible spec change or spec doc fix, patch = tool-only improvement.
