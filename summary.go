package tdrm

import (
	"encoding/json"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"io"
	"strconv"
)

type outputFormat int

const (
	formatTable outputFormat = iota + 1
	formatJSON
)

type SummaryTable []*Summary

type Summary struct {
	TaskDefinition  string `json:"task_definition"`
	ActiveRevisions int    `json:"active_revisions"`
}

func newOutputFormatFrom(s string) (outputFormat, error) {
	switch s {
	case "table":
		return formatTable, nil
	case "json":
		return formatJSON, nil
	default:
		return outputFormat(0), fmt.Errorf("invalid format name: %s", s)
	}
}

func (s *SummaryTable) print(w io.Writer, format outputFormat) error {
	switch format {
	case formatTable:
		return s.printTable(w)
	case formatJSON:
		return s.printJSON(w)
	default:
		return fmt.Errorf("unknown output format: %s", format)
	}
}

func (s *SummaryTable) printTable(w io.Writer) error {
	t := tablewriter.NewWriter(w)
	t.SetHeader(s.header())
	t.SetBorder(false)
	for _, s := range *s {
		row := s.row()
		t.Append(row)
	}
	t.Render()
	return nil
}

func (s *SummaryTable) printJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

func (s *SummaryTable) header() []string {
	return []string{
		"task definition",
		"active revisions",
	}
}

func (s *Summary) row() []string {
	return []string{
		s.TaskDefinition,
		strconv.Itoa(s.ActiveRevisions),
	}
}
