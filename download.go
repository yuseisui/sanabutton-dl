package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fatih/semgroup"
	"github.com/flytam/filenamify"
	"golang.org/x/text/unicode/norm"
)

type Button struct {
	FileName string `json:"file-name"`
	Value    any    `json:"value"`
}

func flat[T any](s [][]T) []T {
	totalLen := 0
	for _, v := range s {
		totalLen += len(v)
	}
	result := make([]T, 0, totalLen)
	for _, v := range s {
		result = append(result, v...)
	}
	return result
}

func toValidFilename(str string) (string, error) {
	return filenamify.Filenamify(str, filenamify.Options{
		Replacement: "_",
	})
}

func createFile(path string, perm os.FileMode) (*os.File, error) {
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, perm)
	if err != nil {
		return nil, err
	}
	return os.Create(path)
}

func main() {
	const outDir = "sounds"
	const baseUrl = "https://www.natorisana.love"

	u, err := url.JoinPath(baseUrl, "/api/v1/buttons.json")
	if err != nil {
		log.Fatal(err)
	}

	resp, err := http.Get(u)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	var btns [][][]Button
	err = json.Unmarshal(body, &btns)
	if err != nil {
		log.Fatal(err)
	}

	buttons := flat(flat(btns))
	total := len(buttons)
	maxWorkers := runtime.GOMAXPROCS(0)
	sg := semgroup.NewGroup(context.Background(), int64(maxWorkers))

	for i, v := range buttons {
		sg.Go(func() error {
			i, v := i, v // https://go.dev/doc/faq#closures_and_goroutines

			urlPath := norm.NFC.String(v.FileName) + ".mp3"
			u, err := url.JoinPath(baseUrl, "sounds", urlPath)
			if err != nil {
				return err
			}

			resp, err := http.Get(u)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			decodedUrl, err := url.PathUnescape(resp.Request.URL.String())
			if err != nil {
				return err
			}
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				return fmt.Errorf("%s: %s", resp.Status, decodedUrl)
			}

			dirname, filename := filepath.Split(urlPath)
			validDirname, err := toValidFilename(filepath.Clean(dirname))
			if err != nil {
				return err
			}
			validFilename, err := toValidFilename(filepath.Clean(filename))
			if err != nil {
				return err
			}
			path := filepath.Join(outDir, validDirname, validFilename)
			file, err := createFile(path, 0o755)
			if err != nil {
				return err
			}
			defer file.Close()

			size, err := io.Copy(file, resp.Body)
			if err != nil {
				return err
			}
			kbSize := size / 1024

			log.Printf("[%d/%d] %s [%d KB]\n", i+1, total, urlPath, kbSize)

			return nil
		})
	}

	err = sg.Wait()
	if err != nil {
		log.Println(err)
	}

	log.Println("Done")
}
