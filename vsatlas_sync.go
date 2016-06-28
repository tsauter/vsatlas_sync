package main

// TODO:
//  - very only on full sync
//  - create sha1 files
//  - sync only the last X versions
//  - sync only the specific projects
//  - cleanup directories, no longer existing boxes
//  - using https://github.com/cavaliercoder/grab

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/cavaliercoder/grab"
	"github.com/jcelliott/lumber"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	vsatlasMasterUri = flag.String("vsatlas.masteruri", "http://intra-tools.viastore.de/vsatlas/registry/api/v1/download/boxindex/", "")
	localBoxFilepath = flag.String("filepath", "d:\\localrepo", "")
	useDebug         = flag.Bool("debug", false, "")

	log         = lumber.NewConsoleLogger(lumber.INFO)
	concurrency = flag.Int("concurrency", 5, "xx")

	processedFiles []string
)

func downloadIndexFile() (AvailableBoxes, error) {
	var boxes AvailableBoxes

	log.Info("Downloading inventory list %s", *vsatlasMasterUri)

	dl := http.Client{}
	resp, err := dl.Get(*vsatlasMasterUri)
	if err != nil {
		return boxes, errors.New(fmt.Sprintf("Failed to download index file: %s", err))
	}
	if resp.StatusCode >= 400 {
		return boxes, errors.New(fmt.Sprintf("Inventory download failed: HTTP-Error: %s", resp.Status))
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return boxes, errors.New(fmt.Sprintf("Reading file content failed: %s", err))
	}

	err = json.Unmarshal(body, &boxes)
	if err != nil {
		return boxes, errors.New(fmt.Sprintf("Invalid JSON content: %s", err))
	}
	//fmt.Printf("%v\n", boxes)

	log.Debug("Found %d boxes in inventory.", len(boxes.Files))
	return boxes, nil
}

func validateBox(boxinfo BoxInfo, targetFilename string) (bool, error) {
	//hash, err := hash_file_sha1(targetFilename)
	hash, err := CalculateSha1(targetFilename)
	if err != nil {
		// when the file doesn't exist, the checksum calculation is simply invalid
		// all other errors should raise an error
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.New(fmt.Sprintf("Failed to compute hash value: %s\n", err))
	}

	// Checksum is equal
	if hash == boxinfo.Checksum {
		return true, nil
	}

	return false, nil
}

func downloadBox(boxinfo BoxInfo) error {
	filename := filepath.Join(*localBoxFilepath, strconv.FormatUint(boxinfo.Id, 10))
	log.Debug("[dl/%d] Processing box: %s -> %s", boxinfo.Id, boxinfo.Url, filename)

	log.Debug("[dl/%d] Adding file to to prcoessed list: %s", boxinfo.Id, filename)
	processedFiles = append(processedFiles, filename)

	log.Debug("[dl/%d] Local box file exists, validating checksum: %s", boxinfo.Id, filename)
	fileOk, err := validateBox(boxinfo, filename)
	if err != nil {
		return errors.New(fmt.Sprintf("Failed to validate checksum for file: %s", err))
	}
	if fileOk == true {
		// box already downloaded and checksum verified
		log.Info("[dl/%d] Local box is up to date: %s", boxinfo.Id, filename)
		return nil
	} else {
		log.Debug("[dl/%d] Checksum is invalid or file is missing: %s", boxinfo.Id, filename)
	}

	log.Info("[dl/%d] Downloading box: %s => %s", boxinfo.Id, boxinfo.Url, filename)

	respch, err := grab.GetAsync(filename, boxinfo.Url)
	if err != nil {
		return errors.New(fmt.Sprintf("Error downloading %s: %v\n", boxinfo.Url, err))
	}
	log.Debug("[dl/%d] Box download initialized: %s => %s", boxinfo.Id, boxinfo.Url, filename)
	resp := <-respch

	for !resp.IsComplete() {
		fmt.Printf("\033[1A[dl/%d] Progress %d / %d MBytes (%d%%)\033[K\n", boxinfo.Id, resp.BytesTransferred()/(1024*1024), resp.Size/(1024*1024), int(100*resp.Progress()))
		time.Sleep(200 * time.Millisecond)
	}

	if resp.Error != nil {
		return errors.New(fmt.Sprintf("Error downloading %s: %v\n", boxinfo.Url, resp.Error))
	}

	log.Info("[dl/%d] Validating checksum for downloaded file: %s", boxinfo.Id, filename)
	fileOk, err = validateBox(boxinfo, filename)
	if err != nil {
		defer os.Remove(filename)
		return errors.New(fmt.Sprintf("Failed to validate checksum for downloaded file: %s", err))
	}
	if fileOk != true {
		defer os.Remove(filename)
		return errors.New(fmt.Sprintf("Failed to validate checksum for downloaded file: Checksum is different: %s", err))
	}

	log.Debug("[dl/%d] Download of box finished.", boxinfo.Id)
	return nil
}

func cleanupBoxDirectory() error {
	log.Info("Cleaning local box directory %s", *localBoxFilepath)

	files, err := filepath.Glob(*localBoxFilepath + "\\*")
	if err != nil {
		return errors.New(fmt.Sprintf("Listing directory content failed: %s: %s\n", *localBoxFilepath, err))
	}
	if files == nil {
		log.Debug("No files in local directory found. Nothing to cleanup.")
		return nil
	}
	log.Debug("Found %d existing files in %s", len(files), *localBoxFilepath)

	var obsoleteFiles []string

	for _, existingFile := range files {
		if stringInSlice(existingFile, processedFiles) {
			continue
		}
		log.Debug("Marke existing file for deletion: %s", existingFile)
		obsoleteFiles = append(obsoleteFiles, existingFile)
	}

	var errorStack []string

	for _, existingFile := range obsoleteFiles {
		log.Warn("Deleting local file: %s", existingFile)
		err := os.Remove(existingFile)
		if err != nil {
			errorStack = append(errorStack, fmt.Sprintf("%s: %s", existingFile, err))
		}
	}
	if len(errorStack) > 0 {
		return errors.New(strings.Join(errorStack, ", "))
	}

	return nil
}

func stringInSlice(str string, list []string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

func main() {
	flag.Parse()

	if *useDebug {
		log.Level(lumber.DEBUG)
	}

	log.Info("*** Downloading available boxes from %s", *vsatlasMasterUri)
	log.Info("*** Storing boxes in %s", *localBoxFilepath)

	log.Debug("Creating output directory %s", *localBoxFilepath)
	err := os.MkdirAll(*localBoxFilepath, 0777)
	if err != nil {
		log.Error(fmt.Sprintf("%s", err))
		os.Exit(1)
	}

	boxes, err := downloadIndexFile()
	if err != nil {
		log.Error("Downloading index file failed: %s", err)
		os.Exit(1)
	}

	// create a channel with a maximum of concurrency entries
	// as soon as the channel is full, the next go routine will blocked
	sem := make(chan bool, *concurrency)

	for _, f := range boxes.Files {
		// add an entry to the channel
		sem <- true

		go func(f BoxInfo) {
			// make sure, we free up the channel, regarless what happens
			defer func() { <-sem }()

			err = downloadBox(f)
			if err != nil {
				log.Error("[dl/%d] Downloading box failed: %s", f.Id, err)
			}
		}(f)
	}

	// Try to fill in the channel, as soon as we have filled up
	// everything no other routines are running any more
	log.Debug("Waiting for running go routines.")
	for i := 0; i < cap(sem); i++ {
		sem <- true
	}
	log.Debug("All downloader routines are finished now.")

	err = cleanupBoxDirectory()
	if err != nil {
		log.Error("Failed to clean box directory: %s", err)
		os.Exit(1)
	}

	os.Exit(0)
}
