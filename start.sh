#!/usr/bin/env bash
# Starts Bancada. The first time, it installs what is needed (this can take a few minutes).
# Requirements: Go 1.25+, Node.js 20+ and Python 3.10+.
set -e
cd "$(dirname "$0")"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "❌ Não encontrei o programa '$1'. Instale: $2"
    exit 1
  fi
}

if [ ! -x ./bancada ] || [ ! -f backend/web/dist/assets/.built ]; then
  need go "https://go.dev/dl/"
  need npm "https://nodejs.org/"
  echo "→ Preparando a interface (só na primeira vez)…"
  (cd frontend && npm install --no-fund --no-audit --silent && npm run build --silent)
  touch backend/web/dist/assets/.built
  echo "→ Compilando o Bancada…"
  (cd backend && go build -o ../bancada .)
fi

if [ ! -x evaluator/.venv/bin/python ]; then
  if command -v python3 >/dev/null 2>&1; then
    echo "→ Instalando o serviço de avaliação (Python)…"
    python3 -m venv evaluator/.venv && evaluator/.venv/bin/pip install -q -r evaluator/requirements.txt \
      || echo "⚠ Não foi possível instalar a avaliação. O resto funciona normalmente."
  else
    echo "⚠ Python 3 não encontrado: a avaliação ficará desligada. O resto funciona normalmente."
  fi
fi

echo "→ Abrindo o Bancada em http://127.0.0.1:8080 (feche esta janela ou aperte Ctrl+C para sair)"
exec ./bancada
