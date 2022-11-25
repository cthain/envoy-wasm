# envoy-wasm
A foray into WASM filters for Envoy

## Requirements

You need the following tools to play around with this project:

- [Go](https://go.dev/) - Go programming tools.
- [TinyGo](https://tinygo.org/getting-started/install/) - Compiles Go to WASM (among other things).
- [Docker](https://docs.docker.com/engine/install/) - Container runtime (for running an Envoy image).
- `bash` - Needs no introduction.
- `curl` - Also, needs no introduction.

## Usage

You can run the automated tests. This will:
- Build the WASM plugin
- Start Envoy in a Docker container configured with the WASM plugin
- Poke the WASM filter with a bunch of different inputs to test:
  - Normal operation
  - SQL injection detection
  - Rate limit enforcement

```shell
# Run the automated tests
./run-tests.sh
```

You can also just play around with this manually.
Take a look through the [`run-tests.sh`](./run-tests.sh) file for inspiration.

```shell
# Build the plugin
tinygo build -o overwatch.wasm -scheduler=none -target=wasi
```

```shell
# Launch Envoy.
docker run --rm --name envoy \
  -p 19000:19000 \
  -p 10000:10000 \
  -v $(pwd)/envoy-config.yaml:/etc/envoy/config.yaml \
  -v $(pwd)/overwatch.wasm:/etc/envoy/plugins/overwatch.wasm \
  envoyproxy/envoy:v1.24.0 -c /etc/envoy/config.yaml --component-log-level wasm:debug
```

```shell
# Send requests to the app through Envoy in a separate terminal
# If you send more than 5 requests in a 10 second window you will get rate limited.
curl localhost:10000 ...
```
