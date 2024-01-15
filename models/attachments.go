package models

import (
	"encoding/csv"
	"fmt"
	"strings"
)

type Attachments struct {
	ID     string
	Images []Image
}

func (a Attachments) WriteToCSV(writer *csv.Writer) error {
	var errors []string

	for _, img := range a.Images {
		if err := writer.Write([]string{a.ID, img.Name}); err != nil {
			errors = append(errors, fmt.Sprintf("post %s image %s: %s\n", a.ID, img.Name, err.Error()))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf(strings.Join(errors, "\n"))
	}

	return nil
}
