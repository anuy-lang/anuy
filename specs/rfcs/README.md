# Anuy RFC index

This directory holds the RFCs of the Anuy language. RFCs record language and
tooling decisions; they are not replaced by examples, spike reports, or
generated Go.

## Authority

If documents disagree, the document with the highest available authority
applies:

1. `Accepted` RFC;
2. `Proposed` RFC;
3. `Draft` RFC;
4. consolidated working draft;
5. historical spike report.

A lower-level document MUST NOT override a decision of a higher-level RFC. It
must reference the defining RFC or be updated, marked historical, or
superseded.

The status expresses the degree of adoption of a document; it does not replace
dependency review: an RFC becomes `Accepted` only after its semantically
significant open questions are closed and its dependencies are checked.

## Status lifecycle

- **Draft** — the document is in development; decisions may still change.
- **Proposed** — the proposal is formulated and ready for review; its
  decisions take precedence over working drafts unless superseded or
  rejected.
- **Accepted** — the decision is adopted as a normative part of the language.
- **Implemented** — an accepted RFC has implemented and verified support; the
  status complements rather than replaces `Accepted`.
- **Rejected** — the proposal is deliberately not adopted.
- **Superseded** — the decision is replaced by the referenced RFC.
- **Deferred** — the number or topic is reserved, but the semantic proposal is
  not yet formulated.

## How RFCs are published

RFCs are developed and reviewed in the project's private planning repository.
Upon acceptance, an RFC is published here as the canonical English text; that
text becomes the single home of the document. Section numbers are stable
anchors: cross-references in other RFCs, decisions, and tests rely on them, so
published texts preserve numbering and amend decisions by appending dated
entries and incrementing the document `Version`.

## Implementation status

RFC adoption is validated by implementation before acceptance. The dataflow
core of RFC-001–003 — the CFG kernel, definite-initialization and non-nil
narrowing facts, scope resolution, and the diagnostic registry — is
implemented and verified as an internal kernel package with conformance
tests anchored to RFC sections. Language coverage otherwise remains an
experimental narrow slice; no RFC is fully implemented yet.

## RFC index

| RFC | Title                                  | Status   | Version | Date       |
| --- | -------------------------------------- | -------- | ------- | ---------- |
| 000 | Goals, Philosophy, Brand and Non-goals | Accepted | 3       | 2026-09-19 |
| 001 | Values, Initialization and Trust       | Accepted | 4       | 2026-09-19 |
| 002 | Nullability and Nil Safety             | Accepted | 4       | 2026-09-19 |
| 003 | Variables, Assignment and Scope        | Accepted | 4       | 2026-09-19 |
| 004 | Interfaces and Explicit `impl`         | Accepted | 5       | 2026-09-24 |
| 006 | Enums and Exhaustive Matching          | Accepted | 6       | 2026-09-24 |
| 007 | Unsafe and Foreign Contracts           | Accepted | 4       | 2026-09-24 |
