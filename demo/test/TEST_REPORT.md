# E2E execution evidence

This file intentionally contains no historical pass claim. Reproduce current
evidence with:

```bash
./demo/demo.sh up
./demo/demo.sh test
./demo/demo.sh down
```

The test fails on any missing authentication step, non-success operation,
missing Engine projection, incorrect immutable workload label, non-MySQL API
configuration, failure to resume Hub heartbeats after Engine manager/agent
restarts, or loss of business data after an API restart.
