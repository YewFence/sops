# TOML support

SOPS recognizes TOML files by their `.toml` extension. TOML can also be selected
explicitly with `--input-type toml` or `--output-type toml`, including when data
is read from standard input or converted to or from another structured format.

The TOML store supports TOML 1.1 values and document structures, including
tables, arrays of tables, dotted and quoted keys, multiline inline tables,
offset and local datetime values, and comments. The standard `encrypt`,
`decrypt`, `edit`, `rotate`, `updatekeys`, `set`, `unset`, `filestatus`, and
structured `publish` workflows use the same SOPS metadata and encryption rules
as the other structured stores.

## Preservation and normalization

The TOML store preserves decoded values and types, key and section order, table
and array-of-table order, leading comments, and inline comments. Output is
constructed deterministically and validated with
[`go-toml`](https://github.com/pelletier/go-toml). Loading and emitting the
canonical output again produces the same text.

The first load and emit cycle can normalize source representation without
changing TOML semantics. In particular, SOPS can change:

- whitespace, indentation, and blank-line placement;
- string quote style;
- dotted keys and inline tables into table sections where the SOPS tree does
  not retain the original representation;
- hexadecimal, octal, and binary integers into decimal integers;
- equivalent datetime spelling.

Exact byte-for-byte layout preservation is not currently supported. The SOPS
store interface passes an ordered semantic tree between separately created
input and output stores, so the emitter does not receive the original source
document or its representation hints.

TOML local datetime, local date, and local time values remain their native TOML
types during encryption and decryption. Destinations such as Vault that accept
generic maps receive these timezone-free values as their RFC 3339 text
representations.

## Upstream implementation status

Native TOML support follows [issue #369](https://github.com/getsops/sops/issues/369)
and uses the official `github.com/pelletier/go-toml/v2` module. It does not
depend on the historical `github.com/bckground/go-toml/v2` fork or use a module
replacement.

The earlier prototype in [PR #2031](https://github.com/getsops/sops/pull/2031)
was not reused directly because it targeted that fork and older parser and
encoder internals. This implementation instead uses the current upstream
comment-preserving parser and edit APIs, keeps those unstable APIs private to
`stores/toml`, and uses the existing `sops.Comment.Inline` model.

At the time this support was implemented, `unstable/edit` had been merged into
the official go-toml `v2` branch but was newer than the latest release,
`v2.4.3`. The dependency is therefore temporarily pinned to the official
upstream commit as
`v2.4.4-0.20260711173024-cbe6f88bcc08`. It should be replaced by the first
official go-toml release containing `unstable/edit` before an upstream SOPS
release, unless the maintainers explicitly accept the pseudo-version.
