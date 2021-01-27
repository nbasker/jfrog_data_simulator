package remoteartifacts

import (
	"container/list"
	"encoding/json"
	"fmt"
	jfauth "github.com/jfrog/jfrog-client-go/auth"
	jflog "github.com/jfrog/jfrog-client-go/utils/log"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PathInfo struct {
	Uri    string `json:"uri"`
	Folder bool   `json:"folder"`
}
type ArtifactInfo struct {
	Repo     string     `json:"repo"`
	Path     string     `json:"path"`
	Children []PathInfo `json:"children"`
}

// getHttpResp issues a GET request and returns response body
func getHttpResp(artDetails *jfauth.ServiceDetails, uri string) ([]byte, error) {
	rtURL := (*artDetails).GetUrl() + uri
	jflog.Debug("Getting '" + rtURL + "' details ...")
	//fmt.Printf("Fetching : %s\n", rtURL)
	req, err := http.NewRequest("GET", rtURL, nil)
	if err != nil {
		jflog.Error("http.NewRequest failed")
	}
	req.SetBasicAuth((*artDetails).GetUser(), (*artDetails).GetApiKey())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		jflog.Error("http.DefaultClient.Do failed")
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		jflog.Error("ioutil.ReadAll call failed")
	}
	//fmt.Printf("Fetching completed : %s\n", rtURL)
	return body, err
}

func getRemoteArtifactWorker(artDetails *jfauth.ServiceDetails, chFolder chan string, chFile chan<- string) {
	rmtBaseURL := "api/storage"
	for f := range chFolder {
		rmtPath := ""
		if f[0] != '/' {
			rmtPath = "/" + f
		} else {
			rmtPath = f
		}
		rmtURL := rmtBaseURL + rmtPath

		//fmt.Printf("accumulated files : %d, remaining folders : %d, checking : %s\n", files.Len(), folders.Len(), rmtURL)

		resp, err := getHttpResp(artDetails, rmtURL)
		if err != nil {
			fmt.Printf("GET HTTP failed for url : %s\n", rmtURL)
			jflog.Error(fmt.Sprintf("GET HTTP failed for url : %s", rmtURL))
		}
		//fmt.Printf("getHttpResp() done : %s\n", f)
		ArtiInfo := &ArtifactInfo{}
		if err := json.Unmarshal(resp, &ArtiInfo); err != nil {
			fmt.Printf("Unable to fetch file and folders for url : %s\n", rmtURL)
			jflog.Error(fmt.Sprintf("Unable to fetch file and folders for url : %s", rmtURL))
			continue
		}
		//fmt.Printf("json.Unmarshal done, count of items in folder : %d\n", len(ArtiInfo.Children))
		for _, c := range ArtiInfo.Children {
			if c.Folder == true {
				chFolder <- rmtPath + c.Uri
			} else {
				chFile <- rmtPath + c.Uri
			}
		}
		//fmt.Printf("completed folder : %s\n", f)
	}
}

// GetRemoteArtifactFiles gets file details from remote repos
func GetRemoteArtifactFiles(artDetails *jfauth.ServiceDetails, repos *[]string) (*list.List, error) {
	folders := make(chan string, 4096)
	files := make(chan string, 1024)
	rtfacts := list.New()

	var workerg sync.WaitGroup
	const numWorkers = 8
	workerg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			getRemoteArtifactWorker(artDetails, folders, files)
			workerg.Done()
		}()
	}
	fmt.Printf("Created %d getRemoteArtifactWorker() go routines\n", numWorkers)

	go func(rl *[]string, chFolder chan<- string) {
		for _, r := range *rl {
			chFolder <- r
		}
	}(repos, folders)
	fmt.Printf("Pushed initial remote repo's\n")

	var collectorg sync.WaitGroup
	collectorg.Add(1)
	go func() {
		defer collectorg.Done()
		for {
			timeout := time.After(60 * time.Second)
			select {
			case f := <-files:
				rtfacts.PushBack(f)
				if rtfacts.Len()%1000 == 0 {
					fmt.Printf("collector_goroutine() artifact : %s, rt-count = %d\n", f, rtfacts.Len())
				}
			case <-timeout:
				fmt.Println("Timeout after 60s")
				return
			}
		}
	}()

	collectorg.Wait()
	fmt.Println("All results collected, collector_goroutine() done, closing folders channel")
	close(folders)
	workerg.Wait()
	fmt.Println("All getRemoteArtifactWorker() completed, closing files channel")
	close(files)

	return rtfacts, nil
}

func downloadRemoteArtifactWorker(artDetails *jfauth.ServiceDetails, chFiles <-chan string, tgtDir string) {
	rtBase := (*artDetails).GetUrl()
	dlcount := 0
	for f := range chFiles {
		rtURL := rtBase + f
		jflog.Debug("Getting '" + rtURL + "' details ...")
		// fmt.Printf("Fetching : %s\n", rtURL)
		req, err := http.NewRequest("GET", rtURL, nil)
		if err != nil {
			jflog.Error("http.NewRequest failed")
		}
		req.SetBasicAuth((*artDetails).GetUser(), (*artDetails).GetApiKey())

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			jflog.Error("http.DefaultClient.Do failed")
		}
		defer resp.Body.Close()

		fpath := tgtDir + "/" + f
		fdir, _ := filepath.Split(fpath)
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			os.MkdirAll(fdir, 0700) // Create directory
		}

		// Create the file
		out, err := os.Create(fpath)
		if err != nil {
			jflog.Error("Failed to create file : %s", fpath)
			continue
		}
		defer out.Close()

		// Write the body to file
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			jflog.Error("Failed to copy download to file : %s", fpath)
		}
		//fmt.Printf("downloading to complete: %s\n", fpath)
		dlcount++
	}
	//fmt.Printf("downloadRemoteArtifactWorker() complete, downloaded %d files\n", dlcount)
	jflog.Info(fmt.Sprintf("downloadRemoteArtifactWorker() complete, downloaded %d files", dlcount))
}

// DownloadArtifacts and write to a target directory
func DownloadRemoteArtifacts(artDetails *jfauth.ServiceDetails, rtfacts *list.List, tgtDir string) error {
	files := make(chan string, 1024)

	var workerg sync.WaitGroup
	const numWorkers = 40
	workerg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			downloadRemoteArtifactWorker(artDetails, files, tgtDir)
			workerg.Done()
		}()
	}
	fmt.Printf("Created %d downloadRemoteArtifactWorker() go routines\n", numWorkers)

	count := 1
	for e := rtfacts.Front(); e != nil; e = e.Next() {
		f := e.Value.(string)
		if f[0] == '/' {
			f = strings.Replace(f, "/", "", 1)
		}

		files <- f
		if count%1000 == 0 {
			fmt.Printf("completed sending %d rtfacts for download\n", count)
			break
		}
		count++
	}
	fmt.Printf("Completed sending %d rtfacts for downloading, waiting for 60s\n", count)
	time.Sleep(60 * time.Second)
	close(files)
	fmt.Println("Closing files channel, waiting for all downloadRemoteArtifactWorker() to complete")
	workerg.Wait()
	fmt.Println("All downloadRemoteArtifactWorker() completed")
	return nil
}
