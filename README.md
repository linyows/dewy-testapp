Dewy TestApp
==

https://github.com/linyows/dewy

The test application that dewy's end-to-end tests deploy. It serves HTTP on:

1. The ports given with `-ports` (for example, `-ports 8080,8081`).
2. The TCP and Unix domain sockets that `dewy server` passes in `DEWY_PORT` (or server-starter in `SERVER_STARTER_PORT`).
3. Port 3333, when neither is given, as in a container.

The sockets are opened with dewy's [sock](https://pkg.go.dev/github.com/linyows/dewy/sock) module. Under `dewy server --health-before-switch`, the app also serves on the health check socket in `DEWY_HEALTH_PORT`.

Endpoints:

- `/health`: version, listeners and mode as JSON
- `/version`: the version
- `/listener`: the listener that served the request
- `/mode`: `dewy` or `standalone`
