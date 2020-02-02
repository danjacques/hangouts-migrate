package mattermost

import (
	"encoding/json"
	"fmt"
	"io"
)

func typedContainerFor(e BulkImportEntry) *typedContainer {
	var tc typedContainer
	e.addToTypedContainer(&tc)
	return &tc
}

type BulkImportWriter struct {
	lastIndex int
	w         io.Writer
	enc       *json.Encoder
}

func NewWriter(w io.Writer) *BulkImportWriter {
	// Encode each container on a single line (JSONL).
	enc := json.NewEncoder(w)
	enc.SetIndent("", "")

	return &BulkImportWriter{
		w:   w,
		enc: enc,
	}
}

func (biw *BulkImportWriter) Add(e BulkImportEntry) error {
	tc := typedContainerFor(e)
	if tc == nil {
		return fmt.Errorf("don't know how to format %T", e)
	}

	// Make sure we are not past our index-of-no-return.
	idx, _ := containerOrderMap[tc.Type]
	if idx < biw.lastIndex {
		return fmt.Errorf("container type %s must occur before %s",
			containerOrder[idx], containerOrder[biw.lastIndex])
	}
	biw.lastIndex = idx

	// Encode each container on a single line (JSONL).
	if err := biw.enc.Encode(tc); err != nil {
		return err
	}

	return nil
}
