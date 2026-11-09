# Security policy

*Em português: se você encontrou uma falha de segurança, **não abra uma issue pública**. Use o link de relato privado abaixo. Pode escrever em português.*

## Reporting a vulnerability

Please **don't open a public issue** for security problems. Report them privately through GitHub:

**[Report a vulnerability](../../security/advisories/new)**

Include what you found, how to reproduce it and what an attacker could do with it. You can write in English or Portuguese.

This is a personal open source project maintained in spare time. I'll reply as soon as I can, usually within a week, and keep you updated until it's fixed. With your permission, you'll be credited in the release notes.

## Supported versions

Only the latest version on `main` (and the latest release, once there are releases) gets security fixes.

## What's in scope

Bancada runs on your own computer and stores API keys, so these matter most:

- API keys leaking (to the browser, logs, exported files, results or error messages)
- Reading or writing files outside the workspace (for example through dataset or image paths)
- Other sites or devices on the network being able to use the API, which would spend your API credits
- Anything in the Docker setup that exposes the app beyond `127.0.0.1` by default

Out of scope: problems that need someone who already has access to your computer or your `bancada-data/` folder, and issues in the model providers themselves (OpenAI, Anthropic, Google, Ollama).

## Keeping your setup safe

- Keep the default port binding (`127.0.0.1:8080`). Anyone who can open the page can use your API keys.
- Your keys live in `.env` (or `bancada-data/.env` with Docker). Don't commit or share that file.
