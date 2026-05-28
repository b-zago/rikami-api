# Rikami API

API used alongside [rika](https://github.com/b-zago/rikami) to fully automate the deployment pipeline. When `rika` operates without the `-local` flag, it delegates chart generation and deployment to this service.

Generated charts use [rikami-charts](https://github.com/b-zago/rikami-charts) as the underlying Helm library.

## What it does

- Automatically syncs vessel and shard resources with its own repo
- Automatically pushes generated charts to the [ k3s-cluster ](https://github.com/b-zago/k3s-cluster) repo
- Automatically seals secrets using the latest Sealed Secrets public key fetched from inside the cluster

## Endpoints

Both endpoints are secured with HMAC.

### `POST /summon`

Generates a Helm chart from a vessel. Accepts optional `.env` files. Equivalent to running `rika summon` locally.

### `POST /app`

Performs app-level actions (kill, sleep, awake, update) on existing charts in the cluster repo. Equivalent to running `rika app` locally.
