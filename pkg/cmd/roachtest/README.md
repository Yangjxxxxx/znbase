# Overview

`roachtest` is a tool for performing large-scale (multi-machine)
automated tests. It relies on the concurrently-developed (but
separate) tool `roachprod`.

# Setup

1. [Set up `roachprod`](https://github.com/znbasedb/znbase/blob/master/pkg/cmd/roachprod/README.md), if you haven't already. This includes making sure `$PWD/bin` is on your `PATH` and `gcloud` is installed and properly configured.
1. Build a linux release binary of `znbase`: `build/builder.sh mkrelease amd64-linux-gnu`
1. Build a linux binary of the `workload` tool: `build/builder.sh make bin/workload`
1. Build a local binary of `roachtest`: `make bin/roachtest`

# Usage

You'll usually use `roachtest run` and specify a single test.

For ZNBase Labs employees, if your local username does not match
your gcloud/`@znbaselabs.com` username, you'll need to add
`--user=$CRL_USERNAME` to the `roachtest` command. For non-ZNBase
Labs employees, set `--gce-project` (and maybe `--user` too).

If `$PWD/bin` isn't on your `PATH`, you'll need to add `--roachprod=<path-to-roachprod>`
to the `roachtest` command.

```shell
roachtest run jepsen/1/bank/split
```

While iterating, don't forget to rebuild whichever of the above
binaries you've touched. For example, the command line that you might use
while iterating on jepsen tests is `make bin/roachtest &&
roachtest run jepsen/1/bank/split`

TODO: document process for creating and reusing a cluster.
