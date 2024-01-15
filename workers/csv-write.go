package workers

import (
	"encoding/csv"
	"os"
	"sync"
)

type CSVWriter interface {
	WriteToCSV(*csv.Writer) error
}

func CSVWrite(
	errorResolver ErrorResolver,
	wg *sync.WaitGroup,
	rows <-chan CSVWriter,
	filepath string,
	headers []string,
) {
	defer wg.Done()

	file, err := os.Create(filepath)
	if err != nil {
		errorResolver(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write(headers); err != nil {
		errorResolver(err)
	}

	for row := range rows {
		if err := row.WriteToCSV(writer); err != nil {
			errorResolver(err)
		}
	}
}
