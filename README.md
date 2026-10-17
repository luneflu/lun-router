# lun-router

lun-router is a Go server with an embedded SvelteKit frontend. The frontend builds as a single-page application (SPA) without server-side rendering (SSR). The `compile.sh` script builds the frontend and compiles the Go server into a standalone binary.

## Requirements

- Go
- Node.js
- pnpm (recommended) or npm

## Development

Install frontend dependencies:

```bash
pnpm install
```

Start the frontend development server:

```bash
pnpm dev
```

Open the local URL printed in the terminal. The development server runs the frontend separately; use the compiled Go binary to run the embedded application.

## Build

Build the frontend and Go server for the current platform:

```bash
./compile.sh
```

The script performs these steps:

1. Builds the Svelte frontend using `pnpm build`, falling back to `npm run build` when pnpm is unavailable.
2. Compiles the Go server with the frontend embedded.
3. Produces a binary named `lun-router` in the project root.

The Go build disables CGO and strips debug information to reduce binary size.

### Cross-compilation

Pass the target operating system and architecture as the first two arguments:

```bash
./compile.sh linux amd64
```

Optionally, pass a custom output path as the third argument:

```bash
./compile.sh linux amd64 lun-router-linux-amd64
```

When the target OS differs from the build host's OS, the script automatically names the output `lun-router-<os>-<arch>`, overriding the default output name. When targeting the host OS, the default output name is `lun-router`, unless a custom name is supplied.

You can also set `GOOS` and `GOARCH` environment variables instead of passing the first two arguments:

```bash
GOOS=linux GOARCH=arm64 ./compile.sh
```

Explicit arguments take precedence over environment variables. Unset values default to the host platform reported by `go env`.

### Run

Run the compiled binary:

```bash
./lun-router
```

For a cross-compiled build, replace `lun-router` with the generated target binary name. Cross-compiled binaries must run on a compatible target system.

## Production notes

- The frontend is built as an SPA; it does not use SvelteKit SSR in the production binary.
- The Go server embeds the built frontend, so deployment does not require a separate frontend server or static asset directory, provided the embedding setup includes the generated assets.
- Build separately for each target OS and architecture.

## Adding Svelte features

Use the Svelte CLI to add supported features:

```bash
pnpm dlx sv add
```

For example, to add Tailwind CSS:

```bash
pnpm dlx sv add tailwindcss
```

Review available options before changing existing project configuration.

## Project structure

- `src/` — Svelte application source.
- `static/` — Static frontend assets.
- `compile.sh` — Frontend build and Go compilation script.

See the Go source and `go.mod` for server implementation and Go dependency details.
