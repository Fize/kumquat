# Kumquat local Kind demo

The demo owns `kumquat-hub` plus the Docker network `kumquat-net`. It
deploys Engine, API, and a local persistence service for the API. Remote Edge
credential exchange and rotation need a separate security design and are not
offered by this local demo.

Prerequisites: Docker, Kind, kubectl, Helm, Go, curl, Python 3, and OpenSSL.

```bash
./demo/demo.sh up
./demo/demo.sh test
./demo/demo.sh status
./demo/demo.sh down
```

`up` is idempotent. API credentials are generated locally and kept in
`kumquat-system/kumquat-api`; they are never printed. Engine manager and Hub
reporter use their in-cluster ServiceAccount identities, so the demo has no
expiring bootstrap-token chain. Demo data is kept in a dedicated persistent
volume.

Clusters and the network carry demo ownership markers. `up` refuses an
unowned name collision and `down` deletes only marked resources; existing
clusters such as `kumquat-dev` and locally built images are retained.
