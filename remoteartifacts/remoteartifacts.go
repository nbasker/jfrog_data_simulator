package remoteartifacts

import (
	"container/list"
	"encoding/json"
	"fmt"
	jfauth "github.com/jfrog/jfrog-client-go/auth"
	jflog "github.com/jfrog/jfrog-client-go/utils/log"
	"io/ioutil"
	"net/http"
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
				if rtfacts.Len()%100 == 0 {
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
