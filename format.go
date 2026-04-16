package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

type format string

const (
	formatTable format = "table"
	formatJSON  format = "json"
	formatCSV   format = "csv"
)

func parseFormat(s string) (format, error) {
	switch f := format(s); f {
	case formatTable, formatJSON, formatCSV:
		return f, nil
	}
	return "", usagef("--format: unknown format %q (want table, json or csv)", s)
}

// column describes one field of a report. Tables show the human-readable
// text under header; JSON and CSV use the raw value under key. A column
// with no header is left out of tables, one with no key out of JSON/CSV.
type column[T any] struct {
	header string
	text   func(T) string
	key    string
	raw    func(T) any
}

func render[T any](w io.Writer, f format, cols []column[T], rows []T) error {
	switch f {
	case formatJSON:
		objs := make([]object, len(rows))
		for i, r := range rows {
			objs[i] = toObject(cols, r)
		}
		return writeJSON(w, objs)
	case formatCSV:
		cw := csv.NewWriter(w)
		var header []string
		for _, c := range cols {
			if c.key != "" {
				header = append(header, c.key)
			}
		}
		cw.Write(header)
		for _, r := range rows {
			var rec []string
			for _, c := range cols {
				if c.key != "" {
					rec = append(rec, csvValue(c.raw(r)))
				}
			}
			cw.Write(rec)
		}
		cw.Flush()
		return cw.Error()
	default:
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		var cells []string
		for _, c := range cols {
			if c.header != "" {
				cells = append(cells, c.header)
			}
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
		for _, r := range rows {
			cells = cells[:0]
			for _, c := range cols {
				if c.header != "" {
					cells = append(cells, c.text(r))
				}
			}
			fmt.Fprintln(tw, strings.Join(cells, "\t"))
		}
		return tw.Flush()
	}
}

func csvValue(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// object is a JSON object that keeps its keys in order.
type object []field

type field struct {
	key string
	val any
}

func toObject[T any](cols []column[T], r T) object {
	var o object
	for _, c := range cols {
		if c.key != "" {
			o = append(o, field{c.key, c.raw(r)})
		}
	}
	return o
}

func (o object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(f.key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(f.val)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func writeJSON(w io.Writer, v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(out, '\n'))
	return err
}

// jsonTime formats t for JSON and CSV output: RFC 3339 in UTC, with
// milliseconds.
func jsonTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z07:00")
}
