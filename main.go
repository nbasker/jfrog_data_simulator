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

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/auth"
	jfauth "github.com/jfrog/jfrog-client-go/auth"
	//"github.com/jfrog/jfrog-client-go/artifactory/services"
	//serviceutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/config"
	jflog "github.com/jfrog/jfrog-client-go/utils/log"
)

// Config struct for insight data simulator
type Config struct {
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

// NewConfig returns a new decoded Config struct
func NewConfig(configPath string) (*Config, error) {
	// Create config structure
	config := &Config{}

	// Open config file
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Init new YAML decode
	d := yaml.NewDecoder(file)

	// Start YAML decoding from file
	if err := d.Decode(&config); err != nil {
		return nil, err
	}

	return config, nil
}

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

// ParseFlags will create and parse the CLI flags
// and return the path to be used elsewhere
func ParseFlags() (string, error) {
	// String that contains the configured configuration path
	var configPath string

	// Set up a CLI flag called "-config" to allow users
	// to supply the configuration file
	flag.StringVar(&configPath, "config", "./config.yml", "path to config file")

	// Actually parse the flags
	flag.Parse()

	// Validate the path first
	if err := ValidateConfigPath(configPath); err != nil {
		return "", err
	}

	// Return the configuration path
	return configPath, nil
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
		remoteRepos = append(remoteRepos, r.Key)
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

	// Generate our config based on the config supplied
	// by the user in the flags
	cfgPath, err := ParseFlags()
	if err != nil {
		jflog.Error("Cli parse failure")
	}
	cfg, err := NewConfig(cfgPath)
	if err != nil {
		jflog.Error("Config parse failure")
	}

	rtDetails := auth.NewArtifactoryDetails()
	rtDetails.SetUrl(cfg.RefArtiServer.ArtiURL)
	rtDetails.SetApiKey(cfg.RefArtiServer.ArtiApikey)
	rtDetails.SetUser(cfg.RefArtiServer.ArtiUsername)

	ctx := context.Background()

	serviceConfig, err := config.NewConfigBuilder().
		SetServiceDetails(rtDetails).
		SetThreads(1).
		SetDryRun(false).
		SetContext(ctx).
		Build()

	rtManager, err := artifactory.New(&rtDetails, serviceConfig)
	rtVersion, err := rtManager.GetVersion()
	jflog.Info("Artifactory Version = ", rtVersion)
	rtSvcID, err := rtManager.GetServiceId()
	jflog.Info("Artifactory ServiceId = ", rtSvcID)

	largeRemoteRepos, err := GetCachedRemoteRepos(&rtDetails)
	jflog.Info("Num of CACHE repos whose size is > 1GB : ", len(*largeRemoteRepos))
	jflog.Info(fmt.Sprintf("repo list : %+v", *largeRemoteRepos))
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
