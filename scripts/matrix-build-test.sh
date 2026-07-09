#!/usr/bin/env bash
# Scaffolds justgo projects across representative config combinations and
# confirms `go build ./...` succeeds. Catches template/import regressions
# (e.g. unused fmt/log imports, import cycles) that only show up for
# specific .justgo.json combinations.
#
# Usage:
#   ./scripts/matrix-build-test.sh         Standard architecture: router x database x observability (12 cases)
#   ./scripts/matrix-build-test.sh --hex   Hexagonal architecture: router x database x messaging (6 representative cases)
# Run from the justgo/ directory after `go build -o justgo main.go`.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
JUSTGO_BIN="$SCRIPT_DIR/../justgo"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

if [ ! -x "$JUSTGO_BIN" ]; then
	echo "error: $JUSTGO_BIN not found or not executable; run 'go build -o justgo main.go' first" >&2
	exit 1
fi

pass=0
fail=0

run_case() {
	name="$1"
	newInput="$2"

	cd "$WORKDIR" || exit 1
	printf "%s" "$newInput" | "$JUSTGO_BIN" new >"$WORKDIR/$name.log" 2>&1

	cd "$WORKDIR/$name" || { echo "FAIL: $name (project not generated)"; fail=$((fail + 1)); return; }

	if [ -f ".justgo.json" ] && grep -q '"architecture": "hexagonal"' .justgo.json; then
		"$JUSTGO_BIN" gen module billing >"$WORKDIR/$name.gen.log" 2>&1
	fi
	go mod tidy >"$WORKDIR/$name.tidy.log" 2>&1

	if go build ./... >"$WORKDIR/$name.build.log" 2>&1; then
		echo "PASS: $name"
		pass=$((pass + 1))
	else
		echo "FAIL: $name"
		cat "$WORKDIR/$name.build.log"
		fail=$((fail + 1))
	fi
}

if [ "${1:-}" = "--hex" ]; then
	# router choice x db x messaging choice (+ broker sub-choice for watermill)
	run_case "gin_db_direct"              "gin_db_direct\n\n1\n2\ny\n1\ny\n1\n\ny\n"
	run_case "gin_nodb_inmemory"           "gin_nodb_inmemory\n\n1\n2\nn\ny\n2\n\ny\n"
	run_case "fiber_db_inmemory"           "fiber_db_inmemory\n\n2\n2\ny\n1\ny\n2\n\ny\n"
	run_case "fiber_nodb_watermill_rabbit" "fiber_nodb_watermill_rabbit\n\n2\n2\nn\ny\n3\n1\n\ny\n"
	run_case "std_db_watermill_kafka"      "std_db_watermill_kafka\n\n3\n2\ny\n1\ny\n3\n2\n\ny\n"
	run_case "std_nodb_direct"             "std_nodb_direct\n\n3\n2\nn\ny\n1\n\ny\n"
else
	combos="gin:1 fiber:2 std:3"
	for routerpair in $combos; do
		router="${routerpair%%:*}"
		choice="${routerpair##*:}"
		for db in y n; do
			for obs in y n; do
				name="t_${router}_db${db}_obs${obs}"
				dbinput="n\n"
				if [ "$db" = "y" ]; then
					dbinput="y\n1\n"
				fi
				run_case "$name" "${name}\n\n${choice}\n1\n${dbinput}${obs}\n\ny\n"
			done
		done
	done
fi

echo "---"
echo "pass=$pass fail=$fail"
[ "$fail" -eq 0 ]
