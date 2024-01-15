package workers

import (
	"sync"
)

type Downloader interface {
	Download(path string) error
}

func FileDownload(
	errorResolver ErrorResolver,
	wg *sync.WaitGroup,
	downloads <-chan Downloader,
	pathResolver func(Downloader) string,
) {
	defer wg.Done()

	for download := range downloads {
		if err := download.Download(pathResolver(download)); err != nil {
			errorResolver(err)
		}
	}
}
