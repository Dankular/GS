# Supply-chain controls

Pull requests and `main` pushes run Go vulnerability analysis, secret
detection, filesystem vulnerability/misconfiguration scanning, and source
SBOM generation in `.github/workflows/ci.yml`.

Production images remain digest-pinned in the Helm chart. Before a production
release, the release pipeline must build each image from the reviewed commit,
publish an SBOM and provenance attestation, sign the image digest, and make
the admission policy require the signature and vulnerability threshold. The
repository CI job intentionally scans source and dependencies; it does not
pretend to sign or admit an image into an external registry it cannot access.
