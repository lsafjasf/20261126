// Package cea608 parses CEA-608/EIA-608 line-21 byte pairs.
//
// Caption bytes and Extended Data Services (XDS) packets are kept in
// separate state. Malformed control pairs are recorded for diagnostics,
// while truncated pair bytes can be continued or explicitly resynchronized.
// The package uses only the Go standard library.
package cea608
