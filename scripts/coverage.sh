#!/bin/sh
# Воспроизводимый отчёт о покрытии (требование кейса, ограничения 07–08).
# Считается покрытие строк прикладного кода команды: backend/internal и backend/cmd/buddy.
# Не входят: зависимости, тесты, cmd/server (точка входа без логики).
# Результат: docs/coverage/coverage.txt (по функциям + итог) и docs/coverage/coverage.html.
set -eu
cd "$(dirname "$0")/../backend"
export LLM_PROVIDER= LLM_MODE=stub_ok MOCK_FAIL=
go test -count=1 ./... -coverprofile=coverage.out -coverpkg=./internal/...,./cmd/buddy
mkdir -p ../docs/coverage
go tool cover -func=coverage.out | sed "s#github.com/worker4556/naparnik/backend/##" > ../docs/coverage/coverage.txt
go tool cover -html=coverage.out -o ../docs/coverage/coverage.html
rm -f coverage.out
echo "Итог: $(tail -1 ../docs/coverage/coverage.txt | awk '{print $NF}') — docs/coverage/coverage.txt, docs/coverage/coverage.html"
