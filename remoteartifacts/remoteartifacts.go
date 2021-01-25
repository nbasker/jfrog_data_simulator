package remoteartifacts

import (
	"container/list"
	"encoding/json"
	"fmt"
	jfauth "github.com/jfrog/jfrog-client-go/auth"
	jflog "github.com/jfrog/jfrog-client-go/utils/log"
	"io/ioutil"
	"net/http"
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

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		jflog.Error("ioutil.ReadAll call failed")
	}
	return body, err
}

// GetRemoteArtifactFiles gets file details from remote repos
func GetRemoteArtifactFiles(artDetails *jfauth.ServiceDetails, repo string) (*list.List, error) {
	folders := list.New()
	files := list.New()
	rmtBaseURL := "api/storage"

	folders.PushBack(repo)
	for folders.Len() > 0 {
		felem := folders.Front()
		f := felem.Value.(string)
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
			jflog.Error(fmt.Sprintf("GET HTTP failed for url : %s", rmtURL))
		}
		ArtiInfo := &ArtifactInfo{}
		if err := json.Unmarshal(resp, &ArtiInfo); err != nil {
			jflog.Error(fmt.Sprintf("Unable to fetch file and folders for url : %s", rmtURL))
			continue
		}
		for _, c := range ArtiInfo.Children {
			if c.Folder == true {
				folders.PushBack(rmtPath + c.Uri)
			} else {
				files.PushBack(rmtPath + c.Uri)
			}
		}
		folders.Remove(felem)

		if files.Len() >= 100 {
			break
		}
	}
	return files, nil
}
