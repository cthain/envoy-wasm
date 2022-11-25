#!/bin/bash

die() {
  echo "*** $*" >&2
  exit 1
}

expect_result() {
  local expStatus=$1 ; shift
  local expBody=$1 ; shift
  local result="$*"

  obsStatus=$(echo $result | sed -n 's/.* status_code = \(.*\)/\1/p')
  obsBody=$(echo $result | sed -n 's/\(.*\) status_code .*/\1/p')

  local status="PASS"
  if [[ "$expStatus" != "$obsStatus" ]]; then
    echo "unexpected status: want $expStatus, have $obsStatus"
    status="FAIL"
  fi
  if ! echo $obsBody | grep -q "$expBody"; then
    echo "unexpected body: want $expBody, have $obsBody"
    status="FAIL"
  fi
  echo $status
}

# Build the plugin
echo "building overwatch.wasm"
tinygo build -o overwatch.wasm -scheduler=none -target=wasi || die "failed to build plugin!"

# Launch Envoy.
echo "launching Envoy"
docker run -d --rm --name envoy \
  -p 19000:19000 \
  -p 10000:10000 \
  -v $(pwd)/envoy-config.yaml:/etc/envoy/config.yaml \
  -v $(pwd)/overwatch.wasm:/etc/envoy/plugins/overwatch.wasm \
  envoyproxy/envoy:v1.24.0 -c /etc/envoy/config.yaml --component-log-level wasm:debug &> /dev/null

sleep 3

echo "running tests"

echo -n "  > Happy path:             "
result=$(curl -sS -w " status_code = %{http_code}\n" "localhost:10000/v1/api/catalog")
expect_result "200" "hello, world!" "$result"

echo -n "  > SQL injection (query):  "
result=$(curl -sS -w " status_code = %{http_code}\n" "localhost:10000/v1/api/catalog?name=foo%20or%201=1")
expect_result "400" "SQL injection" "$result"

echo -n "  > SQL injection (body):   "
result=$(curl -sS -w " status_code = %{http_code}\n" "localhost:10000" -d'WHERE 1=1 --')
expect_result "400" "SQL injection" "$result"

echo -n "  > Rate limit:             "
result=$(curl -sS -w " status_code = %{http_code}\n" "localhost:10000/v1/api/catalog")
result=$(curl -sS -w " status_code = %{http_code}\n" "localhost:10000/v1/api/catalog")
result=$(curl -sS -w " status_code = %{http_code}\n" "localhost:10000/v1/api/catalog")
expect_result "429" "rate limit exceeded" "$result"

echo -n "  > Happy path (again):     "
sleep 10
result=$(curl -sS -w " status_code = %{http_code}\n" "localhost:10000/v1/api/catalog")
expect_result "200" "hello, world!" "$result"

docker stop envoy &> /dev/null
