@echo off
REM Starts Bancada on Windows. The first time, it installs what is needed.
REM Requirements: Go 1.25+, Node.js 20+ and Python 3.10+.
cd /d "%~dp0"

if not exist bancada.exe (
  echo Preparando a interface, so na primeira vez...
  pushd frontend
  call npm install --no-fund --no-audit || goto :fail
  call npm run build || goto :fail
  popd
  echo Compilando o Bancada...
  pushd backend
  go build -o ..\bancada.exe . || goto :fail
  popd
)

if not exist evaluator\.venv\Scripts\python.exe (
  echo Instalando o servico de avaliacao, Python...
  python -m venv evaluator\.venv && evaluator\.venv\Scripts\pip install -q -r evaluator\requirements.txt
)

echo Abrindo o Bancada em http://127.0.0.1:8080
bancada.exe
goto :eof

:fail
echo Algo deu errado. Verifique se Go e Node.js estao instalados.
pause
