# Publishing `terraform-provider-mojo-ddi` to the Terraform Registry (N1)

Prep-only runbook for the v0.1.0 release. No secrets live in this document —
the operator supplies them at each step. The GitHub release is produced by
GoReleaser via `.github/workflows/release.yml` (tag push `v*` → signed
checksums → GitHub Release + registry manifest upload).

## Required inputs (collect before starting)

| Input | Where it goes | Who provides |
|---|---|---|
| **GPG signing key pair** (RSA ≥4096 or Ed25519 recommended) used to sign release checksums | `GPG_PRIVATE_KEY` + `GPG_PASSPHRASE` repo secrets; the **public** key must be registered with the Terraform Registry namespace | Mike / release owner |
| **Terraform Registry namespace claim** for `metify` | registry.terraform.io → User Settings → Signing Keys / namespace verified against the GitHub org `Metify-io` | Mike (one-time, web UI) |
| **GitHub release permissions** | CI uses the built-in `GITHUB_TOKEN` with `contents: write` — already declared in the workflow; just confirm repo Actions settings permit tag-triggered releases and release creation | repo admin |
| Git tag `v0.1.0` on the commit to ship | `git tag` locally, then push | release owner |

## Preflight

```bash
# on main, clean tree, at the commit you want to ship
goreleaser check
goreleaser release --snapshot --clean --skip=sign,publish
go build ./... && go vet ./... && make lint && make test
```

`goreleaser release --snapshot` must produce `dist/terraform-provider-mojo-ddi_*_{linux,darwin,windows}_{amd64,arm64}.zip`,
`..._SHA256SUMS`, and `..._manifest.json` from `terraform-registry-manifest.json`
(`protocol_versions: ["6.0"]`).

## Release

```bash
git tag -s v0.1.0 -m "v0.1.0"    # or unsigned tag; GPG tag not required, but signed is nicer
git push origin v0.1.0
```

The tag push fires `.github/workflows/release.yml`:

1. checks out with full history,
2. imports `secrets.GPG_PRIVATE_KEY` (passphrase `secrets.GPG_PASSPHRASE`) via
   `crazy-max/ghaction-import-gpg` — the key's fingerprint is exported to
   `GPG_FINGERPRINT` and GoReleaser's `signs` block detach-signs
   `terraform-provider-mojo-ddi_v0.1.0_SHA256SUMS` → `...SHA256SUMS.sig`,
3. runs `goreleaser release --clean`, which creates a GitHub Release with all
   zips, `SHA256SUMS`, `SHA256SUMS.sig`, and `terraform-provider-mojo-ddi_0.1.0_manifest.json`.

Verify the release page contains **every** artifact — the Registry rejects
incomplete publishes.

## Terraform Registry submission (one-time per provider)

1. Claim/verify the `metify` namespace on registry.terraform.io (bound to the
   `Metify-io` GitHub org).
2. Upload the **public half** of the GPG key above under the namespace's
   signing keys — the Registry validates `SHA256SUMS.sig` against it.
3. "Publish provider" → select `Metify-io/terraform-provider-mojo-ddi` →
   version `v0.1.0`. The Registry pulls artifacts from the GitHub Release and
   renders docs from `docs/` (resource docs may also be generated via
   `tfplugindocs` if adopted later).

Subsequent releases are just a new `v*` tag — steps 1–3 above are only needed
for the first publish.

## Post-publish smoke test

```hcl
terraform {
  required_providers {
    mojo = { source = "metify-io/mojo-ddi", version = "~> 0.1" }
  }
}
```

`terraform init` must resolve the provider from the Registry.
