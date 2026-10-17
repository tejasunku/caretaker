// Auto-generated from cluster state. Do not edit manually.
// Run scripts/generate_cluster_data.py to regenerate.

export const CLUSTER_DATA = {
  "generatedAt": "2026-10-17T04:26:49Z",
  "clusters": [
    {
      "id": "test-1",
      "stage": "testing",
      "provider": "simulated"
    },
    {
      "id": "staging-1",
      "stage": "staging",
      "provider": "simulated"
    },
    {
      "id": "prod-a",
      "stage": "production",
      "provider": "simulated"
    },
    {
      "id": "prod-b",
      "stage": "production",
      "provider": "simulated"
    },
    {
      "id": "kind-caretaker",
      "stage": "production",
      "provider": "kind"
    }
  ],
  "deployments": [
    {
      "cluster": "test-1",
      "stage": "testing",
      "service": "api",
      "repository": "localhost:5000/test/api",
      "tag": "1.0-patched-20261017",
      "digest": "sha256:ce6d87487eab115fa2d74d8c3e9dab6ea69fe57c6f8a0e3a2742461828344e5c",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": false
    },
    {
      "cluster": "test-1",
      "stage": "testing",
      "service": "auth",
      "repository": "localhost:5000/test/auth",
      "tag": "1.0",
      "digest": "sha256:6cca1a15b21f3dd31ee5e995fb96da1acd21af134d1ac31ea2b352e8bfe3d36e",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": true
    },
    {
      "cluster": "test-1",
      "stage": "testing",
      "service": "cache",
      "repository": "localhost:5000/test/cache",
      "tag": "1.0",
      "digest": "sha256:f94f968084944e093da5ea007621261cee2b6e556029d00023e53f00f790e518",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": true
    },
    {
      "cluster": "test-1",
      "stage": "testing",
      "service": "payments",
      "repository": "localhost:5000/test/payments",
      "tag": "1.0",
      "digest": "sha256:95ce493c0d0c92a28e86a26f63bf94066e1405bfb55f79508087977783d29294",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": true
    },
    {
      "cluster": "test-1",
      "stage": "testing",
      "service": "web",
      "repository": "localhost:5000/test/web",
      "tag": "1.0",
      "digest": "sha256:0a13efaf9d9277c79103a606ab841fe48e4f08c7ce37e2e4280cc010f2309608",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": true
    },
    {
      "cluster": "test-1",
      "stage": "testing",
      "service": "worker",
      "repository": "localhost:5000/test/worker",
      "tag": "1.0",
      "digest": "sha256:206e12571bf87c82a910011ee85d58e7421c6d10cd4a10ad0d48256f56888082",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": true
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "service": "api",
      "repository": "localhost:5000/test/api",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:ff64434fd4c4969f659a27203d234349a2e52f806f494e9abdf74df227f62b75",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": true
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "service": "auth",
      "repository": "localhost:5000/test/auth",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:c6981ad8db69ed75849fd9dbaabd2cf3d3289b991176e2897d0e7d6e2fa7751d",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "service": "cache",
      "repository": "localhost:5000/test/cache",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:a6527b652ae1eed1e81b77d065a9ee66ec343fa09cde6c7094d548bb67c8db3b",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "service": "payments",
      "repository": "localhost:5000/test/payments",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:d89dedea98fd9cc9680d0888d80d68a56b16bbf74588e85df7e4c07dc3e4629a",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "service": "web",
      "repository": "localhost:5000/test/web",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:89b0f09fcb9890538b65a868a9031818855252b75d7d451433c4fa318dee02f5",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "service": "worker",
      "repository": "localhost:5000/test/worker",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:116fc6419bb85ffff54e5fcc2a666240efbd476bed16a6f9e5e08ca0ab508ae2",
      "replicas": 1,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": true
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "service": "api",
      "repository": "localhost:5000/test/api",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:ff64434fd4c4969f659a27203d234349a2e52f806f494e9abdf74df227f62b75",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": true
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "service": "auth",
      "repository": "localhost:5000/test/auth",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:c6981ad8db69ed75849fd9dbaabd2cf3d3289b991176e2897d0e7d6e2fa7751d",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "service": "cache",
      "repository": "localhost:5000/test/cache",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:a6527b652ae1eed1e81b77d065a9ee66ec343fa09cde6c7094d548bb67c8db3b",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "service": "payments",
      "repository": "localhost:5000/test/payments",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:d89dedea98fd9cc9680d0888d80d68a56b16bbf74588e85df7e4c07dc3e4629a",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "service": "web",
      "repository": "localhost:5000/test/web",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:89b0f09fcb9890538b65a868a9031818855252b75d7d451433c4fa318dee02f5",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "service": "worker",
      "repository": "localhost:5000/test/worker",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:116fc6419bb85ffff54e5fcc2a666240efbd476bed16a6f9e5e08ca0ab508ae2",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": true
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "service": "api",
      "repository": "localhost:5000/test/api",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:ff64434fd4c4969f659a27203d234349a2e52f806f494e9abdf74df227f62b75",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": true
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "service": "auth",
      "repository": "localhost:5000/test/auth",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:c6981ad8db69ed75849fd9dbaabd2cf3d3289b991176e2897d0e7d6e2fa7751d",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "service": "cache",
      "repository": "localhost:5000/test/cache",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:a6527b652ae1eed1e81b77d065a9ee66ec343fa09cde6c7094d548bb67c8db3b",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "service": "payments",
      "repository": "localhost:5000/test/payments",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:d89dedea98fd9cc9680d0888d80d68a56b16bbf74588e85df7e4c07dc3e4629a",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "service": "web",
      "repository": "localhost:5000/test/web",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:89b0f09fcb9890538b65a868a9031818855252b75d7d451433c4fa318dee02f5",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "service": "worker",
      "repository": "localhost:5000/test/worker",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:116fc6419bb85ffff54e5fcc2a666240efbd476bed16a6f9e5e08ca0ab508ae2",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": true
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "service": "api",
      "repository": "localhost:5000/test/api",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:ff64434fd4c4969f659a27203d234349a2e52f806f494e9abdf74df227f62b75",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": true
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "service": "auth",
      "repository": "localhost:5000/test/auth",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:c6981ad8db69ed75849fd9dbaabd2cf3d3289b991176e2897d0e7d6e2fa7751d",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "service": "cache",
      "repository": "localhost:5000/test/cache",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:a6527b652ae1eed1e81b77d065a9ee66ec343fa09cde6c7094d548bb67c8db3b",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "service": "payments",
      "repository": "localhost:5000/test/payments",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:d89dedea98fd9cc9680d0888d80d68a56b16bbf74588e85df7e4c07dc3e4629a",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "service": "web",
      "repository": "localhost:5000/test/web",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:89b0f09fcb9890538b65a868a9031818855252b75d7d451433c4fa318dee02f5",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261016",
      "stale": false
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "service": "worker",
      "repository": "localhost:5000/test/worker",
      "tag": "1.0-patched-20261016",
      "digest": "sha256:116fc6419bb85ffff54e5fcc2a666240efbd476bed16a6f9e5e08ca0ab508ae2",
      "replicas": 3,
      "latestPatchedTag": "1.0-patched-20261017",
      "stale": true
    }
  ],
  "events": [
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "provider": "kind",
      "at": "2026-10-17T04:26:43Z",
      "action": "swap",
      "service": "worker",
      "fromTag": "1.0",
      "fromDigest": "",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:116fc6419bb85ffff54e5fcc2a666240efbd476bed16a6f9e5e08ca0ab508ae2"
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "provider": "kind",
      "at": "2026-10-17T04:26:41Z",
      "action": "swap",
      "service": "web",
      "fromTag": "1.0",
      "fromDigest": "",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:89b0f09fcb9890538b65a868a9031818855252b75d7d451433c4fa318dee02f5"
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "provider": "kind",
      "at": "2026-10-17T04:26:40Z",
      "action": "swap",
      "service": "payments",
      "fromTag": "1.0",
      "fromDigest": "",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:d89dedea98fd9cc9680d0888d80d68a56b16bbf74588e85df7e4c07dc3e4629a"
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "provider": "kind",
      "at": "2026-10-17T04:26:39Z",
      "action": "swap",
      "service": "cache",
      "fromTag": "1.0",
      "fromDigest": "",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:a6527b652ae1eed1e81b77d065a9ee66ec343fa09cde6c7094d548bb67c8db3b"
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "provider": "kind",
      "at": "2026-10-17T04:26:38Z",
      "action": "swap",
      "service": "api",
      "fromTag": "1.0",
      "fromDigest": "",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:ff64434fd4c4969f659a27203d234349a2e52f806f494e9abdf74df227f62b75"
    },
    {
      "cluster": "kind-caretaker",
      "stage": "production",
      "provider": "kind",
      "at": "2026-10-17T04:26:38Z",
      "action": "swap",
      "service": "auth",
      "fromTag": "1.0",
      "fromDigest": "",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:c6981ad8db69ed75849fd9dbaabd2cf3d3289b991176e2897d0e7d6e2fa7751d"
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:32Z",
      "action": "swap",
      "service": "payments",
      "fromTag": "1.0",
      "fromDigest": "sha256:95ce493c0d0c92a28e86a26f63bf94066e1405bfb55f79508087977783d29294",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:d89dedea98fd9cc9680d0888d80d68a56b16bbf74588e85df7e4c07dc3e4629a"
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:32Z",
      "action": "swap",
      "service": "web",
      "fromTag": "1.0",
      "fromDigest": "sha256:0a13efaf9d9277c79103a606ab841fe48e4f08c7ce37e2e4280cc010f2309608",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:89b0f09fcb9890538b65a868a9031818855252b75d7d451433c4fa318dee02f5"
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:32Z",
      "action": "swap",
      "service": "worker",
      "fromTag": "1.0",
      "fromDigest": "sha256:206e12571bf87c82a910011ee85d58e7421c6d10cd4a10ad0d48256f56888082",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:116fc6419bb85ffff54e5fcc2a666240efbd476bed16a6f9e5e08ca0ab508ae2"
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:31Z",
      "action": "swap",
      "service": "api",
      "fromTag": "1.0",
      "fromDigest": "sha256:04e9733f401325001e53f4037b193a104f5857520398653bdcec0616f67bb1f5",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:ff64434fd4c4969f659a27203d234349a2e52f806f494e9abdf74df227f62b75"
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:31Z",
      "action": "swap",
      "service": "auth",
      "fromTag": "1.0",
      "fromDigest": "sha256:6cca1a15b21f3dd31ee5e995fb96da1acd21af134d1ac31ea2b352e8bfe3d36e",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:c6981ad8db69ed75849fd9dbaabd2cf3d3289b991176e2897d0e7d6e2fa7751d"
    },
    {
      "cluster": "prod-b",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:31Z",
      "action": "swap",
      "service": "cache",
      "fromTag": "1.0",
      "fromDigest": "sha256:f94f968084944e093da5ea007621261cee2b6e556029d00023e53f00f790e518",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:a6527b652ae1eed1e81b77d065a9ee66ec343fa09cde6c7094d548bb67c8db3b"
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:30Z",
      "action": "swap",
      "service": "payments",
      "fromTag": "1.0",
      "fromDigest": "sha256:95ce493c0d0c92a28e86a26f63bf94066e1405bfb55f79508087977783d29294",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:d89dedea98fd9cc9680d0888d80d68a56b16bbf74588e85df7e4c07dc3e4629a"
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:30Z",
      "action": "swap",
      "service": "web",
      "fromTag": "1.0",
      "fromDigest": "sha256:0a13efaf9d9277c79103a606ab841fe48e4f08c7ce37e2e4280cc010f2309608",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:89b0f09fcb9890538b65a868a9031818855252b75d7d451433c4fa318dee02f5"
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:30Z",
      "action": "swap",
      "service": "worker",
      "fromTag": "1.0",
      "fromDigest": "sha256:206e12571bf87c82a910011ee85d58e7421c6d10cd4a10ad0d48256f56888082",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:116fc6419bb85ffff54e5fcc2a666240efbd476bed16a6f9e5e08ca0ab508ae2"
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:29Z",
      "action": "swap",
      "service": "auth",
      "fromTag": "1.0",
      "fromDigest": "sha256:6cca1a15b21f3dd31ee5e995fb96da1acd21af134d1ac31ea2b352e8bfe3d36e",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:c6981ad8db69ed75849fd9dbaabd2cf3d3289b991176e2897d0e7d6e2fa7751d"
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:29Z",
      "action": "swap",
      "service": "cache",
      "fromTag": "1.0",
      "fromDigest": "sha256:f94f968084944e093da5ea007621261cee2b6e556029d00023e53f00f790e518",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:a6527b652ae1eed1e81b77d065a9ee66ec343fa09cde6c7094d548bb67c8db3b"
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "provider": "simulated",
      "at": "2026-10-17T04:26:28Z",
      "action": "swap",
      "service": "payments",
      "fromTag": "1.0",
      "fromDigest": "sha256:95ce493c0d0c92a28e86a26f63bf94066e1405bfb55f79508087977783d29294",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:d89dedea98fd9cc9680d0888d80d68a56b16bbf74588e85df7e4c07dc3e4629a"
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "provider": "simulated",
      "at": "2026-10-17T04:26:28Z",
      "action": "swap",
      "service": "web",
      "fromTag": "1.0",
      "fromDigest": "sha256:0a13efaf9d9277c79103a606ab841fe48e4f08c7ce37e2e4280cc010f2309608",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:89b0f09fcb9890538b65a868a9031818855252b75d7d451433c4fa318dee02f5"
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "provider": "simulated",
      "at": "2026-10-17T04:26:28Z",
      "action": "swap",
      "service": "worker",
      "fromTag": "1.0",
      "fromDigest": "sha256:206e12571bf87c82a910011ee85d58e7421c6d10cd4a10ad0d48256f56888082",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:116fc6419bb85ffff54e5fcc2a666240efbd476bed16a6f9e5e08ca0ab508ae2"
    },
    {
      "cluster": "prod-a",
      "stage": "production",
      "provider": "simulated",
      "at": "2026-10-17T04:26:28Z",
      "action": "swap",
      "service": "api",
      "fromTag": "1.0",
      "fromDigest": "sha256:04e9733f401325001e53f4037b193a104f5857520398653bdcec0616f67bb1f5",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:ff64434fd4c4969f659a27203d234349a2e52f806f494e9abdf74df227f62b75"
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "provider": "simulated",
      "at": "2026-10-17T04:26:27Z",
      "action": "swap",
      "service": "api",
      "fromTag": "1.0",
      "fromDigest": "sha256:04e9733f401325001e53f4037b193a104f5857520398653bdcec0616f67bb1f5",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:ff64434fd4c4969f659a27203d234349a2e52f806f494e9abdf74df227f62b75"
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "provider": "simulated",
      "at": "2026-10-17T04:26:27Z",
      "action": "swap",
      "service": "auth",
      "fromTag": "1.0",
      "fromDigest": "sha256:6cca1a15b21f3dd31ee5e995fb96da1acd21af134d1ac31ea2b352e8bfe3d36e",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:c6981ad8db69ed75849fd9dbaabd2cf3d3289b991176e2897d0e7d6e2fa7751d"
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "provider": "simulated",
      "at": "2026-10-17T04:26:27Z",
      "action": "swap",
      "service": "cache",
      "fromTag": "1.0",
      "fromDigest": "sha256:f94f968084944e093da5ea007621261cee2b6e556029d00023e53f00f790e518",
      "toTag": "1.0-patched-20261016",
      "toDigest": "sha256:a6527b652ae1eed1e81b77d065a9ee66ec343fa09cde6c7094d548bb67c8db3b"
    },
    {
      "cluster": "test-1",
      "stage": "testing",
      "provider": "simulated",
      "at": "2026-10-17T04:26:21Z",
      "action": "patch",
      "service": "api",
      "fromTag": "1.0",
      "fromDigest": "sha256:04e9733f401325001e53f4037b193a104f5857520398653bdcec0616f67bb1f5",
      "toTag": "1.0-patched-20261017",
      "toDigest": "sha256:ce6d87487eab115fa2d74d8c3e9dab6ea69fe57c6f8a0e3a2742461828344e5c"
    },
    {
      "cluster": "staging-1",
      "stage": "staging",
      "provider": "simulated",
      "at": "2026-10-17T04:21:12Z",
      "action": "swap",
      "service": "worker",
      "fromTag": "1.0",
      "fromDigest": "sha256:206e12571bf87c82a910011ee85d58e7421c6d10cd4a10ad0d48256f56888082",
      "toTag": "1.0-patched-20261017",
      "toDigest": "sha256:e104716aaeb18b937d0845d1df0b0c96eef0b183b1d417075c299cec7b3bd857"
    }
  ],
  "patches": [
    {
      "id": "CVE-2024-6119",
      "package": "openssl",
      "installedVersion": "3.0.13",
      "fixedVersion": "3.0.14",
      "severity": "critical",
      "releasedAt": "2026-09-18T10:00:00Z"
    },
    {
      "id": "INTERNAL-2024-001",
      "package": "libssl3",
      "installedVersion": "3.0.13",
      "fixedVersion": "3.0.14-internal1",
      "severity": "high",
      "releasedAt": "2026-09-20T10:00:00Z"
    },
    {
      "id": "VENDOR-2024-042",
      "package": "curl",
      "installedVersion": "7.88.1",
      "fixedVersion": "7.88.1-10+deb12u15",
      "severity": "medium",
      "releasedAt": "2026-09-21T10:00:00Z"
    },
    {
      "id": "POLICY-2024-003",
      "package": "zlib1g",
      "installedVersion": "1.2.13.dfsg-1",
      "fixedVersion": "1.2.13.dfsg-1+deb12u1",
      "severity": "high",
      "releasedAt": "2026-09-22T10:00:00Z"
    },
    {
      "id": "CVE-2024-25062",
      "package": "libxml2",
      "installedVersion": "2.9.14+dfsg-1.3",
      "fixedVersion": "2.9.14+dfsg-1.3+deb12u1",
      "severity": "low",
      "releasedAt": "2026-09-24T10:00:00Z"
    },
    {
      "id": "CVE-2024-20696",
      "package": "libarchive13",
      "installedVersion": "3.6.2-1+deb12u2",
      "fixedVersion": "3.6.2-1+deb12u3",
      "severity": "medium",
      "releasedAt": "2026-09-15T10:00:00Z"
    },
    {
      "id": "CVE-2024-32002",
      "package": "git",
      "installedVersion": "2.39.2-1.1",
      "fixedVersion": "2.39.5-0+deb12u1",
      "severity": "high",
      "releasedAt": "2026-09-11T10:00:00Z"
    }
  ]
} as const;
