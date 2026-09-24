# Anuy

> A safer, more explicit language for the Go ecosystem.

## Status

The language core is designed and accepted: the semantic contracts for
values and initialization, nullability and nil safety, variables and
scope, interfaces and explicit `impl`, enums and exhaustive matching,
error handling, unsafe and foreign contracts, and the lowering and
generated-Go contract are accepted specifications.

The first stable artifact is the [`anuyabi`](anuyabi/) ABI support
package — the canonical tagged nullable representation and the foreign
boundary validation helpers consumed by generated Go. Its contract is
[RFC-009](specs/rfcs/); the API is stable within the
`v0.x` line and covered by repository tags.

The compiler itself lives in the `experimental/` and `internal/`
trees as a validation layer; it carries no stability promises.

## Specifications

Accepted RFCs are published under [specs/rfcs/](specs/rfcs/) as the canonical English texts of the language decisions. RFCs mature privately and are published here upon acceptance; section numbers inside published RFCs are stable anchors.

## What is available

- `anuyabi` — the stable ABI support package
  (`github.com/anuy-lang/anuy/anuyabi`): tagged nullable carriers and
  boundary validation with fail-stop semantics;
- accepted canonical specifications for the language core, lowering,
  and generated-Go contract.

## What is not available yet

- a full compiler or build tooling (`anuy build`/`anuy test`); the
  experimental validation layer is a design instrument, not a product;
- the remaining interop surface (type projection contracts, SourceMap
  serialization);
- compatibility commitments beyond the `anuyabi` API within `v0.x`.

## License

[Apache License 2.0](LICENSE)
