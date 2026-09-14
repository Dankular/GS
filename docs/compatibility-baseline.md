# Compatibility baseline

This is the evidence record for the pinned validation stack on the configured
VPS Kind cluster. It is a single-node compatibility baseline, not a production
HA qualification. Image IDs were read from the running containers with
`docker image inspect` for Compose images and Kubernetes
`.status.containerStatuses[].imageID` for cluster images on 2026-09-14.

| Component | Version/chart | Immutable image reference or source |
| --- | --- | --- |
| Nakama | 3.40.0 | `heroiclabs/nakama@sha256:92fb184e3271be12fd4d239766afb285322a50aaf769a59433445d59624c78cd` |
| PostgreSQL | 16.10 | `postgres@sha256:21f6013073bc6b92830a2129570e2f5ec42a6c734b5a985a41e83aa58f54c3c1` |
| Agones | Helm chart 1.60.0 / components 1.60.0 | allocator `sha256:87ed41ad724e71b43cb6c4b4ec083f0da212e38fb7c8404c02fc7e7d15caa606`; controller `sha256:850b19e0a1fa95e23adf497585bae7c90b01d5141cfcd0ef204e2578697bfa3e`; extensions `sha256:63af080ad7c954fd5e8674f6b99e1a179dc2a4527f853f670fe8710663feaadf`; ping `sha256:9ab54adccaac028d331d78204fefd8e418adf715e808877ec6af7d08d25660b0` |
| External Secrets Operator | Helm chart/app 2.10.0 | `ghcr.io/external-secrets/external-secrets@sha256:814117b0fd6d121b03e8ba3b6db1cecbe7449a354fc0fc9c4faf73a37aa221b1` |
| ingress-nginx | Helm chart 4.15.1 / controller 1.15.1 | `registry.k8s.io/ingress-nginx/controller@sha256:594ceea76b01c592858f803f9ff4d2cb40542cae2060410b2c95f75907d659e1` |
| CloudNativePG | Helm chart 0.29.0 / operator 1.30.0 | `ghcr.io/cloudnative-pg/cloudnative-pg@sha256:a2701eb97cdd2a34b1fdb2cb51987f544b706e40bec72ae7146cd8580efefebb` |
| cert-manager | Helm chart 1.21.2 | controller `quay.io/jetstack/cert-manager-controller@sha256:70f532fd9cfde0b09d55687200942399d89838bc2d5d5b45152eb799a15912b8`; cainjector `sha256:c85268c64f2e0e76684bf5fe8906caff34b82523561c6affe0fae3546bd87562`; webhook `sha256:a60e2dac46dbb8a7f3df95c54ce941012f54c2fe022f0ee55aaa1ab40ed957ae` |
| Barman Cloud Plugin | manifest 0.15.0 | `ghcr.io/cloudnative-pg/plugin-barman-cloud@sha256:563c680fe7fda3466ca2b1f55a1397ed2ddc9e760360107dd7724f1959c1a536` |
| Kind node | Kubernetes v1.36.1 | `kindest/node@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5` |

Compose also pins the OpenTelemetry Collector, coturn, Prometheus,
Alertmanager, and Grafana by digest in
[`deploy/compose/compose.yaml`](../deploy/compose/compose.yaml). Application
images are built from the repository and are pinned by the release workflow
before a Helm production render.

The evidence proves that the candidate versions run together in the current
validation environment. It does not prove multi-node failure-domain behavior,
managed PostgreSQL PITR, production ingress/TLS, or signed application image
promotion; those remain separate release gates.
