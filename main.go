package main

import (
	"container/list"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/auth"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	jfauth "github.com/jfrog/jfrog-client-go/auth"
	//serviceutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/config"
	jflog "github.com/jfrog/jfrog-client-go/utils/log"
)

// ValidateConfigPath just makes sure, that the path provided is a file,
// that can be read
func ValidateConfigPath(path string) error {
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	if s.IsDir() {
		return fmt.Errorf("'%s' is a directory, not a normal file", path)
	}
	return nil
}

// RtConfig struct for insight data simulator
type RtConfig struct {
	ConfigPath    string
	RtCredentials RtUrlCreds
}
type RtUrlCreds struct {
	RefArtiServer struct {
		ArtiURL      string `yaml:"artiurl"`
		ArtiUsername string `yaml:"artiusername"`
		ArtiApikey   string `yaml:"artiapikey"`
	} `yaml:"refartiserver"`
	DutArtiServer struct {
		ArtiURL      string `yaml:"artiurl"`
		ArtiUsername string `yaml:"artiusername"`
		ArtiApikey   string `yaml:"artiapikey"`
	} `yaml:"dutartiserver"`
}

// NewRtConfig returns a new decoded RtConfig struct
func NewRtConfig() (*RtConfig, error) {
	// Create RT config structure
	config := &RtConfig{}

	flag.StringVar(&config.ConfigPath, "config", "./config.yaml", "path to config file")

	// Actually parse the flags
	flag.Parse()

	// Validate the path first
	if err := ValidateConfigPath(config.ConfigPath); err != nil {
		return config, err
	}

	return config, nil
}

// InitRtCreds initializes RT credentials from config file
func (rc *RtConfig) InitRtCreds() error {
	// Open config file
	file, err := os.Open(rc.ConfigPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Init new YAML decode
	d := yaml.NewDecoder(file)

	// Start YAML decoding from file
	if err := d.Decode(&rc.RtCredentials); err != nil {
		return err
	}

	return nil
}

// GetRefRtDetails gets the RT credential details
func (rc *RtConfig) GetRefRtDetails() jfauth.ServiceDetails {
	refRtDetails := auth.NewArtifactoryDetails()
	refRtDetails.SetUrl(rc.RtCredentials.RefArtiServer.ArtiURL)
	refRtDetails.SetApiKey(rc.RtCredentials.RefArtiServer.ArtiApikey)
	refRtDetails.SetUser(rc.RtCredentials.RefArtiServer.ArtiUsername)
	return refRtDetails
}

// GetDutRtDetails gets the RT credential details
func (rc *RtConfig) GetDutRtDetails() jfauth.ServiceDetails {
	dutRtDetails := auth.NewArtifactoryDetails()
	dutRtDetails.SetUrl(rc.RtCredentials.DutArtiServer.ArtiURL)
	dutRtDetails.SetApiKey(rc.RtCredentials.DutArtiServer.ArtiApikey)
	dutRtDetails.SetUser(rc.RtCredentials.DutArtiServer.ArtiUsername)
	return dutRtDetails
}

// GetRtMgr gets the reference RT manager
func (rc *RtConfig) GetRtMgr(refRtDetails jfauth.ServiceDetails) (artifactory.ArtifactoryServicesManager, error) {
	ctx := context.Background()
	svcConfig, err := config.NewConfigBuilder().
		SetServiceDetails(refRtDetails).
		SetThreads(1).
		SetDryRun(false).
		SetContext(ctx).
		Build()
	if err != nil {
		return nil, err
	}

	refRtMgr, err := artifactory.New(&refRtDetails, svcConfig)
	return refRtMgr, err
}

type FileStorageInfo struct {
	StorageType      string `json:"storageType"`
	StorageDirectory string `json:"storageDirectory"`
	TotalSpace       string `json:"totalSpace"`
	UsedSpace        string `json:"usedSpace"`
	FreeSpace        string `json:"freeSpace"`
}
type BinariesInfo struct {
	BinariesCount string `json:"binariesCount"`
	BinariesSize  string `json:"binariesSize"`
	ArtifactsSize string `json:"artifactsSize"`
	Optimization  string `json:"optimization"`
}
type RepoStorageInfo struct {
	Key          string `json:"repoKey"`
	RepoType     string `json:"repoType"`
	FoldersCount int    `json:"foldersCount"`
	FilesCount   int    `json:"filesCount"`
	UsedSpace    string `json:"usedSpace"`
	PackageType  string `json:"packageType"`
}
type RepoStorageUsedSpaceInfo struct {
	Key          string  `json:"repoKey"`
	RepoType     string  `json:"repoType"`
	FoldersCount int     `json:"foldersCount"`
	FilesCount   int     `json:"filesCount"`
	UsedSpaceGB  float64 `json:"usedSpaceGB"`
	PackageType  string  `json:"packageType"`
}
type StorageInfo struct {
	FileStorage     FileStorageInfo   `json:"fileStoreSummary"`
	BinariesStorage BinariesInfo      `json:"binariesSummary"`
	RepoStorage     []RepoStorageInfo `json:"repositoriesSummaryList"`
}

type PathInfo struct {
	Uri    string `json:"uri"`
	Folder bool   `json:"folder"`
}
type ArtifactInfo struct {
	Repo     string     `json:"repo"`
	Path     string     `json:"path"`
	Children []PathInfo `json:"children"`
}

// GetHttpResp issues a GET request and returns response body
func GetHttpResp(artDetails *jfauth.ServiceDetails, uri string) ([]byte, error) {
	rtURL := (*artDetails).GetUrl() + uri
	jflog.Debug("Getting '" + rtURL + "' details ...")
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

// GetCachedRemoteRepos fetches storage info of repositories
func GetCachedRemoteRepos(artDetails *jfauth.ServiceDetails) (*[]string, error) {
	remoteRepos := []string{}
	storageInfoGB := []RepoStorageUsedSpaceInfo{}
	resp, err := GetHttpResp(artDetails, "api/storageinfo")
	if err != nil {
		jflog.Error("Failed to get http resp for api/storageinfo")
	}
	StorageInfo := &StorageInfo{}
	if err := json.Unmarshal(resp, &StorageInfo); err != nil {
		return &remoteRepos, err
	}

	// Gather repoType CACHE that has storage space > 1 GB
	for _, r := range *&StorageInfo.RepoStorage {
		if r.RepoType == "CACHE" && strings.Contains(r.UsedSpace, "GB") {
			re := regexp.MustCompile(`[-]?\d[\d,]*[\.]?[\d{2}]*`)
			usedSpaceGB, err := strconv.ParseFloat(re.FindString(r.UsedSpace), 64)
			if err != nil {
				jflog.Error("Failed used space to float for repo %s", r.Key)
			}
			storageInfoGB = append(storageInfoGB, RepoStorageUsedSpaceInfo{r.Key, r.RepoType, r.FoldersCount, r.FilesCount, usedSpaceGB, r.PackageType})

		}
	}

	sort.Slice(storageInfoGB, func(i, j int) bool { return storageInfoGB[i].UsedSpaceGB > storageInfoGB[j].UsedSpaceGB })

	for _, r := range storageInfoGB {
		remoteRepos = append(remoteRepos, strings.ReplaceAll(r.Key, "-cache", ""))
	}
	return &remoteRepos, nil
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
		resp, err := GetHttpResp(artDetails, rmtURL)
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
	}
	return files, nil
}

func main() {

	f, err := os.OpenFile("./datasim.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		err = fmt.Errorf("Unable to open file for log writing")
		fmt.Println(err.Error())
		os.Exit(-1)
	}
	defer f.Close()

	jflog.SetLogger(jflog.NewLogger(jflog.INFO, f))
	jflog.Info("Started data simulator")

	cfg, err := NewRtConfig()
	if err != nil {
		jflog.Error("Cli parse failure")
		os.Exit(-1)
	}
	if err := cfg.InitRtCreds(); err != nil {
		jflog.Error("Config parse failure")
		os.Exit(-1)
	}

	refRtDetails := cfg.GetRefRtDetails()
	refRtMgr, err := cfg.GetRtMgr(refRtDetails)
	refRtVer, err := refRtMgr.GetVersion()
	jflog.Info("Ref RT Version = ", refRtVer)
	refRtSvcID, err := refRtMgr.GetServiceId()
	jflog.Info("Ref RT ServiceId = ", refRtSvcID)

	dutRtDetails := cfg.GetDutRtDetails()
	dutRtMgr, err := cfg.GetRtMgr(dutRtDetails)
	dutRtVer, err := dutRtMgr.GetVersion()
	jflog.Info("DUT RT Version = ", dutRtVer)
	dutRtSvcID, err := refRtMgr.GetServiceId()
	jflog.Info("DUT RT ServiceId = ", dutRtSvcID)

	largeRemoteRepos, err := GetCachedRemoteRepos(&refRtDetails)
	jflog.Info("Num of CACHE repos whose size is > 1GB : ", len(*largeRemoteRepos))
	jflog.Info(fmt.Sprintf("repo list : %+v", *largeRemoteRepos))

	refRtLargeRemoteRepo, err := refRtMgr.GetRepository((*largeRemoteRepos)[0])
	jflog.Info(fmt.Sprintf("Ref RT repo list : %+v", refRtLargeRemoteRepo))
	dutRtLargeRemoteRepo, err := dutRtMgr.GetRepository((*largeRemoteRepos)[0])
	jflog.Info(fmt.Sprintf("DUT RT repo list : %+v", dutRtLargeRemoteRepo))

	if dutRtLargeRemoteRepo != nil && dutRtLargeRemoteRepo.Key == (*largeRemoteRepos)[0] {
		jflog.Info(fmt.Sprintf("Large remote repo %s is present in DUT", dutRtLargeRemoteRepo.Key))
		if err := dutRtMgr.DeleteRepository(dutRtLargeRemoteRepo.Key); err != nil {
			jflog.Error(fmt.Sprintf("Failed to delete in the DUT large remote repo %s", dutRtLargeRemoteRepo.Key))
			os.Exit(-1)
		}
		jflog.Info(fmt.Sprintf("Pausing after deleting %s in DUT", dutRtLargeRemoteRepo.Key))
		time.Sleep(5 * time.Second)
	}

	params := services.NewDockerRemoteRepositoryParams()
	params.Key = (*largeRemoteRepos)[0]
	params.Url = "https://registry-1.docker.io/"
	params.RepoLayoutRef = "simple-default"
	params.Description = "A caching proxy repository for a registry-1.docker.io"
	params.XrayIndex = &[]bool{true}[0]
	params.AssumedOfflinePeriodSecs = 600
	if err = dutRtMgr.CreateRemoteRepository().Docker(params); err != nil {
		jflog.Error(fmt.Sprintf("Failed to remote repo %s in DUT", (*largeRemoteRepos)[0]))
		os.Exit(-1)
	}
	dutRtLargeRemoteRepo, err = dutRtMgr.GetRepository((*largeRemoteRepos)[0])
	jflog.Info(fmt.Sprintf("After recreation DUT RT repo list : %+v", dutRtLargeRemoteRepo))

	/*
		rr := []string{(*largeRemoteRepos)[0]}
		for _, r := range rr {
			fmt.Printf("Fetching files in repo : %s\n", r)
			files, err := GetRemoteArtifactFiles(&rtDetails, r)
			if err != nil {
				jflog.Error(fmt.Sprintf("Failed to get artifact files for repo %s", r))
			}
			jflog.Info(fmt.Sprintf("repo %s has #files : %d", r, files.Len()))
		}
	*/

	jflog.Info("Ending data simulator")
}
