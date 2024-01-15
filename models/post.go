package models

import (
	"encoding/csv"
	"fmt"
	"strings"
)

type Post struct {
	ID       string
	Location string
	Content  string
	Images   []Image
}

func (p Post) WriteToCSV(writer *csv.Writer) error {
	return writer.Write([]string{
		p.ID,
		fmt.Sprintf("%s", p.Location),
		fmt.Sprintf("%s", strings.ReplaceAll(p.Content, `"`, `'`)),
	})
}
