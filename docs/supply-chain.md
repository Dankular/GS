# Supply-chain controls

Pull requests and `main` pushes run Go vulnerability analysis, secret
detection, filesystem vulnerability/misconfiguration scanning, and source
SBOM generation in `.github/workflows/ci.yml`.

Production images remain digest-pinned in the Helm chart. The tag-triggered
`.github/workflows/release.yml` builds the application images, publishes
BuildKit SBOM/provenance, signs each immutable digest with GitHub OIDC through
Cosign, and publishes a signed CycloneDX SBOM attestation. This includes the
`gameservice-server` dedicated-server image built from the simulator Dockerfile.
The workflow still
requires the repository's GHCR permissions and a real release tag; it does not
claim that an image was released merely because CI passed.

The optional `supplyChain.admissionPolicy` Helm setting renders a Kyverno
`ClusterPolicy` which requires digest-pinned GameService pods to carry a
matching keyless Cosign signature from that release workflow. Enable it only
after Kyverno is installed and the registry/repository identity has been
reviewed:

```sh
helm upgrade --install gameservice ./deploy/helm/platform \
  --set supplyChain.admissionPolicy.enabled=true \
  --set images.controlApi.digest=sha256:... \
  --set images.outboxWorker.digest=sha256:... \
  --set images.leaderboardWorker.digest=sha256:... \
  --set images.matchmakingWorker.digest=sha256:... \
  --set images.reconciliationWorker.digest=sha256:... \
  --set images.migrations.digest=sha256:...
```

The repository CI job intentionally scans source and dependencies; the release
workflow and admission policy are deployment controls and require external
GHCR, Sigstore, and Kyverno services to be available.
