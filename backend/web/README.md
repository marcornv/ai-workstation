The web interface is built here by `cd frontend && npm run build` (output: `web/dist/`).
The Go binary embeds this folder, so `go build` works even before the interface is built:
in that case the app shows a "not built yet" message instead of the UI.
