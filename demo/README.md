# Kumquat local Kind demo

The demo owns `kumquat-hub` plus the Docker network `kumquat-net`. It
deploys Engine (including a Hub agent that reports scheduler capacity), API,
and MySQL 8; SQLite is not used by this deployment. Remote Edge credential
exchange and rotation need a separate security design and are not offered by
this local demo.

Prerequisites: Docker, Kind, kubectl, Helm, Go, curl, Python 3, and OpenSSL.

```bash
./demo/demo.sh up
./demo/demo.sh test
./demo/demo.sh status
./demo/demo.sh down
```

`up` is idempotent. API and database credentials are generated locally and
kept in `kumquat-system/kumquat-api`; they are never printed. Engine manager
and Hub reporter use their in-cluster ServiceAccount identities, so the demo
has no expiring bootstrap-token chain. MySQL has its own persistent volume.

Clusters and the network carry demo ownership markers. `up` refuses an
unowned name collision and `down` deletes only marked resources; existing
clusters such as `kumquat-dev` and locally built images are retained.

Database-backed API tests use a separate, temporary Docker MySQL instance:

```bash
make test-api-mysql
```

That target waits on MySQL health, creates one database per test, rejects test
skips, and removes only its own Compose resources and volumes on exit.
