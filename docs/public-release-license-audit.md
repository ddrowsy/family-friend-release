# Public release license and provenance audit

This document records the repository-level checks required before exporting an approved Family Friend source snapshot to a public repository.

It is an engineering audit, not legal advice. A public repository license can only grant rights that the project actually holds; it does not create copyright in material that is not copyrightable and does not override third-party license terms.

## Current snapshot

Audited base commit: `315e9b4f191413236a709b2022fdf9c1d6c4ba5e`.

Verification status: full repository verification is required for the exact release commit.

The current repository does not contain a final product `LICENSE` file. Choosing the public/source-available license is intentionally deferred to a separate issue.

### Direct Go dependencies

| Module | Version | License | Notes |
| --- | --- | --- | --- |
| `fyne.io/fyne/v2` | `v2.8.1` | BSD-3-Clause | Permissive; retain required notices when redistributing dependency code/binaries. |
| `github.com/Microsoft/go-winio` | `v0.6.2` | MIT | Permissive; retain copyright/license notice where required. |
| `golang.org/x/sys` | `v0.36.0` | BSD-3-Clause | Permissive; retain required notices. |
| `modernc.org/sqlite` | `v1.39.1` | BSD-3-Clause | Permissive module license. SQLite upstream material is public-domain; dependency notices still need to be preserved for binary/source redistribution as applicable. |

All indirect Go modules resolved by `go list -m all` are checked by `scripts/audit-public-release.sh`. The script inspects the resolved module license files rather than treating `go.sum` checksums as license metadata. It fails on unknown, reciprocal, or restricted licenses so a new dependency cannot silently become release-approved.

### Notable transitive findings

The full module graph includes `github.com/golang/freetype` at commit `e2365dfdc4a0`. Its top-level license explicitly offers a choice between the FreeType License (FTL) and GPL-2.0-or-later. Family Friend can rely on the FTL option rather than GPL. The FTL permits commercial source and binary redistribution but requires acknowledgement and, for binary redistribution, a FreeType-based-work disclaimer in distribution documentation. The future release-notice issue must include this attribution if the dependency is present in shipped binaries.

`github.com/davecgh/go-spew v1.1.1` and `github.com/nfnt/resize` are ISC-licensed. Their license wording spans lines differently from some common ISC templates; the audit classifier handles this form explicitly.

### Browser-extension tooling

`browser-extension/chrome/package.json` has no production Node dependencies. It currently has one development dependency:

| Package | Version | License | Distribution |
| --- | --- | --- | --- |
| `playwright` | `1.63.0` | Apache-2.0 | Development/smoke-test tooling only; `node_modules` is not tracked or shipped in the source snapshot. |

The automated audit fails if production Node dependencies are added before an explicit Node dependency-license audit exists.

## Provenance checks

The repository-level review found no tracked `vendor`, `node_modules`, `third_party`, or `third-party` dependency tree at the audited base commit. No tracked executable/library/archive/font assets were identified that would require separate binary provenance approval.

Repository searches for existing copyright/SPDX headers and obvious `copied from`, `adapted from`, or `derived from` markers did not identify third-party source copied into the project at the audited base commit. The automated audit repeats these searches and prints any matches for human review.

The project source is AI-assisted: requirements, architecture, integration, review, testing, and changes are directed by the project owner, while AI tools contribute implementation. That fact alone is not treated as proof that every generated fragment is independently copyrightable. Before public release, the selected repository license should therefore be understood to apply only to rights the project actually owns or is authorized to license.

## Automated release gate

Run:

```bash
bash scripts/audit-public-release.sh
```

Optionally write the resolved Go dependency inventory to a CSV file:

```bash
bash scripts/audit-public-release.sh /tmp/family-friend-go-licenses.csv
```

The check currently blocks release when it detects:

- a Go module with no discoverable license file;
- a Go module whose license is unknown, reciprocal, or restricted without an explicitly recognized permissive alternative;
- tracked vendored/third-party dependency directories;
- production Node dependencies that do not yet have an explicit audit path;
- tracked executable/library/archive/font assets requiring explicit provenance review.

It also reports, without automatically failing, source copyright/SPDX/provenance markers and generated-Go markers so they can be reviewed in context.

Full repository verification runs this audit in the pinned Go container, using the same module cache as the Go tests.

## Current release assessment

No direct dependency or repository-provenance blocker has been identified in the manual review above. The automated full-verification run must still pass for the exact release commit before a snapshot is considered ready for publication.

Before the first public release, a separate issue must still decide the actual project license and the notice/attribution material distributed with the MSI/source snapshot, including required third-party acknowledgements such as the FreeType notice if applicable to the shipped binary.

## Limitations

This check is intentionally conservative but not a legal license classifier. It uses recognizable license text to catch common permissive, reciprocal, restricted, dual-license, and unknown cases. It does not prove copyright ownership, detect semantic similarity to external source, or replace review of third-party notices bundled inside dependencies. Any warning or unfamiliar dependency should be reviewed before publication.
