package workers

import (
	"net/http"
	"sync"
)

type Downloader interface {
	Download(client *http.Client, path string) error
}

func FileDownload(
	errorResolver ErrorResolver,
	wg *sync.WaitGroup,
	downloads <-chan Downloader,
	client *http.Client,
	pathResolver func(Downloader) string,
) {
	defer wg.Done()

	for download := range downloads {
		if err := download.Download(client, pathResolver(download)); err != nil {
			errorResolver(err)
		}
	}
}
