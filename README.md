# go-xpt
XPT dataset read/write support in Golang according to [the official SAS documentation](https://support.sas.com/content/dam/SAS/support/en/technical-papers/record-layout-of-a-sas-version-5-or-6-data-set-in-sas-transport-xport-format.pdf).

Still a work in progress.

## Reading XPT Files

```go
datasets, err := goxpt.ReadXPTFile("example.xpt")
if err != nil {
	log.Fatal(err)
}
grid := datasets[0].AsSimpleGrid() // [][]string: header row + one row per observation
```

`ReadXPT(io.Reader)` is the same thing for an already-open stream. Both return
`[]*Dataset` because an XPT file may contain more than one member; most files
contain exactly one.

Each `Dataset` exposes the library/member metadata (`Library`, `Member`) and the
parsed columns in `Variables`. A `Variable` carries its name, label, length,
type, and format, plus `Data` (`[]DataCell`). Numeric SAS missing values —
including the special missings `.A`–`.Z` and `._` — are reported via
`DataCell.Missing` / `DataCell.MissingCode` rather than being silently turned
into `0`.

Known limitation: the XPT observation section has no row count and is
blank-padded to a multiple of 80 bytes, so a genuine trailing observation whose
every value is blank is indistinguishable from padding and will be dropped.

## Writing XPT Files

`WriteXPT(io.Writer, *Dataset)` exists but is not implemented yet; it returns an
error for now.

## License

Apache License 2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE). You may use
go-xpt in closed-source software; you must keep the copyright and license
notices and state any changes you make to the source files.
