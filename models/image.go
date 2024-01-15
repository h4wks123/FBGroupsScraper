package models

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

var UnableToDownloadImage = fmt.Errorf("error: unable to download image")

type Image struct {
	Name string
	Url  string
}

func (img Image) Download(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("image %s: %s\n", img.Name, err.Error())
	}
	defer file.Close()

	resp, err := http.Get(img.Url)
	if err != nil {
		return fmt.Errorf("image %s: %s\n", img.Name, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("image %s: %s\n", img.Name, UnableToDownloadImage.Error())
	}

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("image %s: %s\n", img.Name, err.Error())
	}

	return nil
}
