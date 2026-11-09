# Contributing to Bancada

*Em português: contribuições são muito bem-vindas! Pode abrir issues e pull requests em português ou em inglês. O guia abaixo está em inglês porque as notas de desenvolvimento também estão.*

Thanks for wanting to help. Bancada is a test bench for language models, built for people who don't code, so changes that keep it **simple to install and simple to use** are the most valuable ones.

You can write issues and pull requests in English or Portuguese.

## Ways to help

- **Report a bug or confusing behavior.** Use the bug report form in [Issues](../../issues/new/choose). Reports from people who don't code are just as welcome: describe what you clicked and what you saw.
- **Suggest a feature.** Open a feature request first, before writing code, so we can agree on the approach.
- **Improve the docs.** Typos, unclear steps in the beginner guide and missing screenshots all count.
- **Pick an issue.** Issues labeled `good first issue` are a good place to start.

## Setting up for development

You need Go (version in `backend/go.mod`), Node.js 22 and Python 3.12. Docker is only needed to test the Docker setup.

```bash
make evaluator   # once: creates evaluator/.venv with the Python service
make dev         # API on :8080, UI with hot reload on http://localhost:5173
make test        # Go vet + tests and Python tests
make build       # builds the UI and the ./bancada binary
```

The app uses the repository root as its workspace in dev mode (`bancada.yaml`, `experiments/`, `evaluators/`, `data/`). Your local `bancada.yaml`, `.env` and database are git-ignored, so you can't commit your keys by accident.

Read the [Developer notes](README.md#developer-notes-english) before your first change. They explain the project layout, what each backend file does and how the Python service fits in.

## Making a change

1. Fork the repository and create a branch from `main`.
2. Make your change. Keep it focused: one fix or feature per pull request.
3. Add or update tests when you change behavior (`backend/*_test.go`, `evaluator/test_*.py`).
4. Run `make test` and `make build`. CI runs the same checks, plus `gofmt`.
5. Open a pull request and fill in the template.

### Conventions

- **Two languages for everything the user sees.** UI text, error messages and evaluator reasons exist in Portuguese (pt-BR) and English, side by side: `t("Salvar", "Save")` in the frontend, `tr("...", "...")` in Go and Python. A new message needs both.
- **Both READMEs.** If you change something documented, update `README.md` (Portuguese) and `README.en.md` (English).
- **Code and comments in English.** Each file starts with a comment saying what it does; keep that up to date.
- **Formatting:** `gofmt` for Go. For TypeScript and Python, follow the style of the surrounding code.
- **No new infrastructure without discussion.** Bancada runs locally with one command. Changes that add services, accounts or cloud dependencies need an issue first.
- **Never commit real API keys or personal data**, including in example files, screenshots or tests.

## Example files

`experiments/`, `evaluators/` and `data/exemplo-*` / `data/example-*` are copied into new workspaces on first start. If you change them, keep the Portuguese and English versions in sync.

## Code of conduct

Be kind and patient, especially with people who are new to code or to AI. Disagree with ideas, not with people.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
